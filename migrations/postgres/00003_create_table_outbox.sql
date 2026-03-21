-- +goose Up
CREATE TABLE outbox
(
    id          BIGSERIAL PRIMARY KEY,
    topic       TEXT      NOT NULL,
    event_key   TEXT      NOT NULL,
    event_body  BYTEA     NOT NULL,
    status      TEXT      NOT NULL DEFAULT 'pending',
    retry_count INT       NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX outbox_status_created_idx
    ON outbox (status, created_at, id);


-- +goose Down
DROP TABLE IF EXISTS outbox;