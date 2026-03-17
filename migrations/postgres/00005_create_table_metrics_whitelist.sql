-- +goose Up
create table metrics_whitelist
(
    target_group text,
    env          text,
    metric_name  text,
    -- поле для кастомных лимитов в разрезе проверок самих метрик
    checks       jsonb
);


-- +goose Down
drop table metrics_whitelist;
