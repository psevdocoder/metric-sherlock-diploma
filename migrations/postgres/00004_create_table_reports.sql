-- +goose Up
create table reports
(
    target_group text,
    env          text,
    cluster      text,
    team_name    text,
    details      jsonb,
    created_at   timestamp
);

create unique index reports_target_env_cluster_uidx
    on reports (target_group, env, cluster);

-- +goose Down
drop table reports;