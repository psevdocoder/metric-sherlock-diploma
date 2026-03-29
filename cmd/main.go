package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/config"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/httpapi"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/leaderelection"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/outbox"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/runtimeconfig"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/scrapetask"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/storage/etcdstorage"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/storage/postgres"
	"git.server.lan/maksim/metric-sherlock-diploma/pkg/cron"
	"git.server.lan/maksim/metric-sherlock-diploma/pkg/jwtclaims"
	"git.server.lan/pkg/closer/v2"
	"git.server.lan/pkg/config/realtimeconfig"
	"git.server.lan/pkg/zaplogger/logger"
	"git.server.lan/pkg/zaplogger/zaploggercore"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/segmentio/kafka-go"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

const (
	gracefulShutdownTimeout = 10 * time.Second
	scheduleSyncInterval    = 5 * time.Second
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

	pgDsnRaw, _ := config.GetSecret(config.PgDsn)
	pgDsn, _ := pgDsnRaw.String()
	if err := runMigrations(ctx, pgDsn); err != nil {
		logger.Fatal("Failed to apply database migrations", zap.Error(err))
	}

	kafkaBrokersRaw, _ := config.GetValue(config.KafkaBrokers)
	kafkaBrokersStr, _ := kafkaBrokersRaw.String()
	kafkaBrokers := strings.Split(kafkaBrokersStr, ",")
	if len(kafkaBrokers) < 1 {
		logger.Fatal("Kafka brokers list is empty")
	}

	kafkaTopicRaw, _ := config.GetValue(config.KafkaViolationsTopic)
	kafkaTopic, _ := kafkaTopicRaw.String()

	if kafkaTopic == "" {
		logger.Fatal("Kafka violations topic is empty")
	}

	pgStorage, err := postgres.New(ctx, pgDsn, kafkaTopic)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}

	logger.Debug("Connected to database")

	outboxPollIntervalRaw, _ := config.GetValue(config.OutboxPollInterval)
	outboxPollInterval, _ := outboxPollIntervalRaw.Duration()

	outboxBatchSizeRaw, _ := config.GetValue(config.OutboxBatchSize)
	outboxBatchSize, _ := outboxBatchSizeRaw.Int()

	outboxMaxRetriesRaw, _ := config.GetValue(config.OutboxMaxRetries)
	outboxMaxRetries, _ := outboxMaxRetriesRaw.Int()

	kafkaWriter := &kafka.Writer{
		Addr:         kafka.TCP(kafkaBrokers...),
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}

	closer.Add(func() error {
		return kafkaWriter.Close()
	})

	outboxRelay := outbox.NewRelay(
		pgStorage,
		kafkaWriter,
		outboxBatchSize,
		outboxPollInterval,
		outboxMaxRetries,
	)
	go outboxRelay.Run(ctx)

	portRaw, _ := config.GetValue(config.Port)
	port, _ := portRaw.Int()

	jwtIssuerRaw, _ := config.GetValue(config.JWTIssuer)
	jwtIssuer, _ := jwtIssuerRaw.String()
	if jwtIssuer == "" {
		logger.Fatal("JWT issuer is empty")
	}

	jwtJWKSEndpointRaw, _ := config.GetValue(config.JWTJWKSEndpoint)
	jwtJWKSEndpoint, _ := jwtJWKSEndpointRaw.String()

	jwtExpectedAZPRaw, _ := config.GetValue(config.JWTExpectedAZP)
	jwtExpectedAZP, _ := jwtExpectedAZPRaw.String()

	jwtVerifier, err := jwtclaims.NewJWKSVerifier(jwtclaims.Config{
		Issuer:       jwtIssuer,
		JWKSEndpoint: jwtJWKSEndpoint,
		ExpectedAZP:  jwtExpectedAZP,
	})
	if err != nil {
		logger.Fatal("Failed to initialize JWT verifier", zap.Error(err))
	}

	cronManager := cron.NewCronManager()
	closer.Add(func() error {
		return cronManager.Stop(ctx)
	})

	limitsConfigRaw, _ := config.GetValue(config.LimitsConfig)
	limitsConfigStr, _ := limitsConfigRaw.String()

	var defaultLimitsConfig scraper.LimitsConfig
	if err = json.Unmarshal([]byte(limitsConfigStr), &defaultLimitsConfig); err != nil {
		logger.Fatal("Failed to parse default limits config", zap.Error(err))
	}

	produceTasksCronRaw, _ := config.GetValue(config.ProduceTasksCronExpr)
	defaultProduceTasksCron, _ := produceTasksCronRaw.String()

	sdConfigPathRaw, _ := config.GetValue(config.SdConfigPath)
	sdConfigPath, _ := sdConfigPathRaw.String()

	etcdEndpointsRaw, _ := config.GetValue(config.EtcdEndpoints)
	etcdEndpointsStr, _ := etcdEndpointsRaw.String()
	rawEtcdEndpoints := strings.Split(etcdEndpointsStr, ",")
	etcdEndpoints := make([]string, 0, len(rawEtcdEndpoints))
	for _, endpoint := range rawEtcdEndpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		etcdEndpoints = append(etcdEndpoints, endpoint)
	}

	if len(etcdEndpoints) == 0 {
		logger.Fatal("Etcd endpoints list is empty")
	}

	settingsEtcdClient, err := clientv3.New(clientv3.Config{
		Endpoints: etcdEndpoints,
	})
	if err != nil {
		logger.Fatal("Failed to create etcd client for runtime settings", zap.Error(err))
	}

	statusCtx, statusCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer statusCancel()
	if _, err = settingsEtcdClient.Status(statusCtx, etcdEndpoints[0]); err != nil {
		logger.Fatal(
			"Failed to connect to etcd for runtime settings",
			zap.Any("endpoints", etcdEndpoints),
			zap.Error(err),
		)
	}

	closer.Add(func() error {
		return settingsEtcdClient.Close()
	})

	etcdSettingsStorage := etcdstorage.New(settingsEtcdClient, 3*time.Second)
	runtimeSettingsService := runtimeconfig.New(etcdSettingsStorage, defaultLimitsConfig, defaultProduceTasksCron)
	if err = runtimeSettingsService.Bootstrap(ctx); err != nil {
		logger.Fatal("Failed to bootstrap runtime settings in etcd", zap.Error(err))
	}

	produceScrapeTasksJob := scrapetask.NewJob(pgStorage, sdConfigPath)
	produceScrapeTasksScheduler := scrapetask.NewScheduler(cronManager, produceScrapeTasksJob, runtimeSettingsService)
	runtimeSettingsService.SetCronScheduleApplier(produceScrapeTasksScheduler)

	if err = produceScrapeTasksScheduler.Init(ctx); err != nil {
		logger.Fatal("Failed to initialize scrape tasks scheduler", zap.Error(err))
	}

	scheduleSyncCtx, scheduleSyncCancel := context.WithCancel(context.Background())
	closer.Add(func() error {
		scheduleSyncCancel()
		return nil
	})
	go produceScrapeTasksScheduler.RunSyncLoop(scheduleSyncCtx, scheduleSyncInterval)

	taskProcessor := scraper.NewProcessor(scraper.NewMetricsClient(), defaultLimitsConfig, runtimeSettingsService, pgStorage)
	taskConsumerPool := scraper.NewWorkerPool(ctx, taskProcessor, 5)
	taskConsumer := scraper.NewTaskConsumer(pgStorage, pgStorage, pgStorage, taskConsumerPool, false)
	go taskConsumer.Run(ctx)

	closer.Add(func() error {
		taskConsumer.Stop()
		return nil
	})

	apiHandler, err := httpapi.NewHandler(pgStorage, runtimeSettingsService, jwtVerifier, jwtIssuer, jwtExpectedAZP)
	if err != nil {
		logger.Fatal("Failed to create HTTP API handler", zap.Error(err))
	}

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           apiHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("HTTP API server starting", zap.String("addr", httpServer.Addr))
		if serverErr := httpServer.ListenAndServe(); serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
			logger.Fatal("HTTP API server failed", zap.Error(serverErr))
		}
	}()

	closer.Add(func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	})

	logger.Debug("Connected to etcd for runtime settings", zap.Any("endpoints", etcdEndpoints))

	var elector leaderelection.LeaderElector
	if env == localEnv {
		elector = leaderelection.NewLocalElector()
		logger.Debug("Leader election started provided by local elector")
	} else {
		etcdEndpointsRaw, _ := config.GetValue(config.EtcdEndpoints)
		etcdEndpointsStr, _ := etcdEndpointsRaw.String()
		etcdEndpoints := strings.Split(etcdEndpointsStr, ",")

		etcdClient, err := clientv3.New(clientv3.Config{
			Endpoints: etcdEndpoints,
		})

		if err != nil {
			logger.Fatal("Failed to create etcd client", zap.Error(err))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_, err = etcdClient.Status(ctx, etcdEndpoints[0])
		if err != nil {
			logger.Fatal("Failed to connect to etcd",
				zap.Any("endpoints", etcdEndpoints),
				zap.Error(err),
			)
		}

		logger.Debug("Connected to etcd", zap.Any("endpoints", etcdEndpoints))

		closer.Add(func() error {
			return etcdClient.Close()
		})

		elector, err = leaderelection.NewEtcdElector(ctx, etcdClient)
		if err != nil {
			logger.Fatal("Failed to create etcd leader elector", zap.Error(err))
		}

		logger.Debug("Leader election started provided by etcd elector")
	}

	go func() {
		if err := elector.Run(); err != nil {
			logger.Fatal("Failed to start leader election", zap.Error(err))
		}
		logger.Debug("Leader election started")
	}()

	elector.AddOnStart(func() {
		logger.Debug("Handling start leadership")

		// Если запущен локально, то хоть и лидер, но собираем метрики как ведомый
		if env != localEnv {
			taskConsumer.SetLeader(true)
		}
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

	logger.Warn("Application started")

	closer.Wait()
	logger.Warn("Application stopped")
}

func runMigrations(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database for migrations: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Error("Failed to close migrations DB connection", zap.Error(closeErr))
		}
	}()

	if err = db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database for migrations: %w", err)
	}

	if err = goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	migrationsDir := filepath.Join("migrations", "postgres")
	if err = goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("apply goose up migrations from %q: %w", migrationsDir, err)
	}

	return nil
}
