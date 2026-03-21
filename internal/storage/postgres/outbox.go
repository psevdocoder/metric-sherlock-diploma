package postgres

import (
	"context"
	"time"
)

const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusSent       = "sent"
	OutboxStatusFailed     = "failed"
)

type OutboxMessage struct {
	ID         int64
	Topic      string
	EventKey   string
	EventBody  []byte
	RetryCount int
}

type outboxEvent struct {
	Topic     string
	Key       string
	Body      []byte
	CreatedAt time.Time
}

const saveOutboxMessageSQL = `
INSERT INTO outbox (topic, event_key, event_body, status, retry_count, last_error, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
`

func (s *Storage) enqueueOutboxEvents(ctx context.Context, tx dbtx, events []outboxEvent) error {
	for _, event := range events {
		_, err := tx.Exec(
			ctx,
			saveOutboxMessageSQL,
			event.Topic,
			event.Key,
			event.Body,
			OutboxStatusPending,
			0,
			"",
			event.CreatedAt,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

const fetchPendingOutboxMessagesSQL = `
WITH picked AS (
	SELECT id
	FROM outbox
	WHERE status = $1
	   OR (status = $2 AND updated_at < NOW() - INTERVAL '1 minute')
	ORDER BY created_at
	LIMIT $3
	FOR UPDATE SKIP LOCKED
)
UPDATE outbox o
SET status = $2,
	updated_at = NOW()
FROM picked p
WHERE o.id = p.id
RETURNING o.id, o.topic, o.event_key, o.event_body, o.retry_count
`

func (s *Storage) FetchPendingOutboxMessages(ctx context.Context, limit int) ([]*OutboxMessage, error) {
	if limit <= 0 {
		return nil, nil
	}

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	rows, err := conn.Query(
		ctx,
		fetchPendingOutboxMessagesSQL,
		OutboxStatusPending,
		OutboxStatusProcessing,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]*OutboxMessage, 0, limit)
	for rows.Next() {
		message := &OutboxMessage{}
		if err = rows.Scan(&message.ID, &message.Topic, &message.EventKey, &message.EventBody, &message.RetryCount); err != nil {
			return nil, err
		}

		messages = append(messages, message)
	}

	return messages, rows.Err()
}

const markOutboxMessageSentSQL = `
UPDATE outbox
SET status = $2,
	last_error = '',
	updated_at = NOW()
WHERE id = $1
`

func (s *Storage) MarkOutboxMessageSent(ctx context.Context, id int64) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, markOutboxMessageSentSQL, id, OutboxStatusSent)
	return err
}

const markOutboxMessageFailedSQL = `
UPDATE outbox
SET status = CASE
		WHEN retry_count + 1 >= $3 THEN $4
		ELSE $5
	END,
	retry_count = retry_count + 1,
	last_error = $2,
	updated_at = NOW()
WHERE id = $1
`

func (s *Storage) MarkOutboxMessageFailed(ctx context.Context, id int64, lastError string, maxRetries int) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	_, err = conn.Exec(
		ctx,
		markOutboxMessageFailedSQL,
		id,
		lastError,
		maxRetries,
		OutboxStatusFailed,
		OutboxStatusPending,
	)

	return err
}
