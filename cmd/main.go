package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"syscall"
	"time"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/config"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/leaderelection"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/scrapetask"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/storage/postgres"
	"git.server.lan/maksim/metric-sherlock-diploma/pkg/cron"
	"git.server.lan/pkg/closer/v2"
	"git.server.lan/pkg/config/realtimeconfig"
	"git.server.lan/pkg/zaplogger/logger"
	"git.server.lan/pkg/zaplogger/zaploggercore"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

const (
	gracefulShutdownTimeout = 10 * time.Second
	localEnv                = "local"
)

func main() {
	logger.Init(zaploggercore.LogPretty)
	logger.SetLogLevel(zaploggercore.TraceLevel)
	closer.Init(
		closer.WithSignals(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP),
		closer.WithTimeout(gracefulShutdownTimeout),
	)

	ctx := context.Background()

	if err := realtimeconfig.StartWatching(); err != nil {
		logger.Fatal("Failed to start watching config", zap.Error(err))
	}

	envRaw, _ := config.GetValue(config.Env)
	env, _ := envRaw.String()

	var elector leaderelection.LeaderElector
	if env == localEnv {
		elector = leaderelection.NewLocalElector()

	} else {
		etcdEndpointsRaw, _ := config.GetValue(config.EtcdEndpoints)
		etcdEndpointsStr, _ := etcdEndpointsRaw.String()
		etcdEndpoints := strings.Split(etcdEndpointsStr, ",")

		etcdClient, err := clientv3.New(clientv3.Config{
			Endpoints: etcdEndpoints,
		})
		if err != nil {
			logger.Fatal("Failed to connect to etcd", zap.Any("endpoints", etcdEndpoints), zap.Error(err))
		}
		logger.Debug("Connected to etcd", zap.Any("endpoints", etcdEndpoints))

		closer.Add(func() error {
			return etcdClient.Close()
		})

		elector, err = leaderelection.NewEtcdElector(ctx, etcdClient)
		if err != nil {
			logger.Fatal("Failed to create etcd leader elector", zap.Error(err))
		}

		logger.Debug("Leader election started provided by etcd")
	}

	go func() {
		if err := elector.Run(); err != nil {
			logger.Fatal("Failed to start leader election", zap.Error(err))
		}
		logger.Debug("Leader election started")
	}()

	pgDsnRaw, _ := config.GetSecret(config.PgDsn)
	pgDsn, _ := pgDsnRaw.String()

	pgStorage, err := postgres.New(ctx, pgDsn)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}

	logger.Debug("Connected to database")

	cronManager := cron.NewCronManager()
	closer.Add(func() error {
		return cronManager.Stop(ctx)
	})

	limitsConfigRaw, _ := config.GetValue(config.LimitsConfig)
	limitsConfigStr, _ := limitsConfigRaw.String()

	var limitsConfig scraper.LimitsConfig
	if err := json.Unmarshal([]byte(limitsConfigStr), &limitsConfig); err != nil {
		logger.Fatal("Failed to parse limits config", zap.Error(err))
	}

	taskProcessor := scraper.NewProcessor(scraper.NewMetricsClient(), limitsConfig)
	taskConsumerPool := scraper.NewWorkerPool(ctx, taskProcessor, 5)
	// инициируем консьюмера с флагом isLeader = true чтобы он не начал сразу брать задачи в обработку,
	// если под действительно будет лидером
	taskConsumer := scraper.NewTaskConsumer(pgStorage, pgStorage, pgStorage, taskConsumerPool, false)
	go taskConsumer.Run(ctx)

	closer.Add(func() error {
		taskConsumer.Stop()
		return nil
	})

	elector.AddOnStart(func() {
		logger.Debug("Handling start leadership")
		taskConsumer.SetLeader(true)
		cronManager.Start()
		logger.Info("Started leadership")
	})

	elector.AddOnStop(func() {
		logger.Debug("Handling stop leadership")
		taskConsumer.SetLeader(false)
		if err = cronManager.Stop(ctx); err != nil {
			logger.Error("Failed to stop cron manager on leadership stop", zap.Error(err))
			return
		}

		logger.Info("Stopped leadership")
	})

	sdConfigPathRaw, _ := config.GetValue(config.SdConfigPath)
	sdConfigPath, _ := sdConfigPathRaw.String()

	produceScrapeTasksJob := scrapetask.NewJob(pgStorage, sdConfigPath)
	produceTasksCronRaw, _ := config.GetValue(config.ProduceTasksCronExpr)
	produceTasksCron, _ := produceTasksCronRaw.String()

	err = errors.Join(
		cronManager.AddTask(ctx, produceTasksCron, produceScrapeTasksJob),
	)
	if err != nil {
		logger.Fatal("Failed to start cron manager", zap.Error(err))
	}

	logger.Warn("Application started")

	closer.Wait()
	logger.Warn("Application stopped")
}
