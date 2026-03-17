-- +goose Up
create table reports
(
    target_group text,
    env          text,
    cluster      text,
    details      jsonb,
    created_at   timestamp
);

-- +goose Down
drop table reports;
