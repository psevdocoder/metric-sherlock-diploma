-- +goose Up
create table scrape_tasks
(
    id           bigserial PRIMARY KEY,
    status       text,
    created_at   timestamp,
    job          text,
    addresses    text[],
    cluster      text,
    env          text,
    target_group text,
    team_name    text
);

-- +goose Down
drop table scrape_tasks;
