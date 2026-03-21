package postgres

import (
	"context"
	"errors"
	"time"

	"git.server.lan/pkg/closer/v2"
	"git.server.lan/pkg/zaplogger/logger"
	"github.com/cenkalti/backoff/v4"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const (
	errCodeDeadlock = "40P01"
	backoffInterval = 1 * time.Second

	rowsPerBatch       = 1000
	defaultOutboxTopic = "metric-sherlock"
)

// Storage представляет собой хранилище Postgres
type Storage struct {
	pool        *pgxpool.Pool
	outboxTopic string
}

// New конструктор для Storage, принимает контекст и адрес подключения для соединения с Postgres
func New(ctx context.Context, dsn string, outboxTopic string) (*Storage, error) {
	conn, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	closer.Add(func() error {
		conn.Close()
		return nil
	})

	if outboxTopic == "" {
		outboxTopic = defaultOutboxTopic
	}

	return &Storage{
		pool:        conn,
		outboxTopic: outboxTopic,
	}, nil
}

type batchSender interface {
	SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults
}

type dbtx interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func (s *Storage) sendBatch(ctx context.Context, batch *pgx.Batch, sender batchSender) error {
	return backoff.RetryNotify(func() error {
		cmd := sender.SendBatch(ctx, batch)

		var err error

		defer func() {
			//
			if closeErr := cmd.Close(); closeErr != nil && err == nil {
				logger.Error("failed to close batch", zap.Error(closeErr))
			}
		}()

		countOpts := batch.Len()

		for countOpts > 0 {
			_, err = cmd.Exec()
			if err == nil {
				countOpts--
				continue
			}

			if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == errCodeDeadlock {
				return err
			}

			return backoff.Permanent(err)
		}

		return nil
	}, backoff.NewConstantBackOff(backoffInterval), func(err error, duration time.Duration) {
		logger.Error("failed to send batch, retry", zap.Error(err))
	})
}
