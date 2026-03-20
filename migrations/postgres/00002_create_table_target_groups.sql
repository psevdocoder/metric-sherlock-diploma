-- +goose Up
create table target_groups
(
    id          serial primary key,
    target_group text,
    first_check timestamp,
    last_check  timestamp,
    job         text,
    env         text,
    cluster     text,
    team_name   text
);

create unique index target_groups_target_env_cluster_uidx
    on target_groups (target_group, env, cluster);

-- +goose Down
drop table target_groups;
