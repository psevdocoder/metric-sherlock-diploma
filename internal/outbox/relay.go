package outbox

import (
	"context"
	"time"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/storage/postgres"
	"git.server.lan/pkg/zaplogger/logger"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type storage interface {
	FetchPendingOutboxMessages(ctx context.Context, limit int) ([]*postgres.OutboxMessage, error)
	MarkOutboxMessageSent(ctx context.Context, id int64) error
	MarkOutboxMessageFailed(ctx context.Context, id int64, lastError string, maxRetries int) error
}

type writer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type Relay struct {
	storage      storage
	writer       writer
	batchSize    int
	pollInterval time.Duration
	maxRetries   int
}

func NewRelay(
	storage storage,
	writer writer,
	batchSize int,
	pollInterval time.Duration,
	maxRetries int,
) *Relay {
	if batchSize <= 0 {
		batchSize = 100
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	if maxRetries <= 0 {
		maxRetries = 5
	}

	return &Relay{
		storage:      storage,
		writer:       writer,
		batchSize:    batchSize,
		pollInterval: pollInterval,
		maxRetries:   maxRetries,
	}
}

func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.flush(ctx)
		}
	}
}

func (r *Relay) flush(ctx context.Context) {
	messages, err := r.storage.FetchPendingOutboxMessages(ctx, r.batchSize)
	if err != nil {
		logger.Error("Failed to fetch pending outbox messages", zap.Error(err))
		return
	}

	for _, message := range messages {
		err = r.writer.WriteMessages(ctx, kafka.Message{
			Topic: message.Topic,
			Key:   []byte(message.EventKey),
			Value: message.EventBody,
		})
		if err != nil {
			logger.Error(
				"Failed to write outbox message to kafka",
				zap.Int64("outbox_id", message.ID),
				zap.String("topic", message.Topic),
				zap.Error(err),
			)

			markErr := r.storage.MarkOutboxMessageFailed(ctx, message.ID, err.Error(), r.maxRetries)
			if markErr != nil {
				logger.Error(
					"Failed to mark outbox message as failed",
					zap.Int64("outbox_id", message.ID),
					zap.Error(markErr),
				)
			}

			continue
		}

		if err = r.storage.MarkOutboxMessageSent(ctx, message.ID); err != nil {
			logger.Error(
				"Failed to mark outbox message as sent",
				zap.Int64("outbox_id", message.ID),
				zap.Error(err),
			)
		}
	}
}
