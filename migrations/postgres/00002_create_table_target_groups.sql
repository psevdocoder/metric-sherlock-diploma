-- +goose Up
create table target_groups
(
    id          serial primary key,
    first_check timestamp,
    last_check  timestamp,
    job         text,
    env         text,
    cluster     text,
    team_name   text
);

-- +goose Down
drop table target_groups;
