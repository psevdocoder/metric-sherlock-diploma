-- +goose Up
create table outbox
(
    topic       text,
    event_key   text,
    event_body  jsonb,
    status      text,
    retry_count int,
    last_error  text,
    created_at  timestamp,
    updated_at  timestamp
);

-- +goose Down
drop table outbox;
