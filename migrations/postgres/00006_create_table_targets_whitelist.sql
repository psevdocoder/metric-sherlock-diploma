-- +goose Up
create table targets_whitelist
(
    target_group text,
    env          text,
    -- поле для проверок и кастомных лимитов по ним
    checks jsonb
);

-- +goose Down
drop table targets_whitelist;
