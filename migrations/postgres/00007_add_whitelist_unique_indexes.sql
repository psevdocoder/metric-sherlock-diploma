-- +goose Up
DELETE
FROM metrics_whitelist a
USING metrics_whitelist b
WHERE a.ctid < b.ctid
  AND a.target_group IS NOT DISTINCT FROM b.target_group
  AND a.env IS NOT DISTINCT FROM b.env
  AND a.metric_name IS NOT DISTINCT FROM b.metric_name;

DELETE
FROM targets_whitelist a
USING targets_whitelist b
WHERE a.ctid < b.ctid
  AND a.target_group IS NOT DISTINCT FROM b.target_group
  AND a.env IS NOT DISTINCT FROM b.env;

CREATE UNIQUE INDEX IF NOT EXISTS metrics_whitelist_target_env_metric_uidx
	ON metrics_whitelist (target_group, env, metric_name);

CREATE UNIQUE INDEX IF NOT EXISTS targets_whitelist_target_env_uidx
	ON targets_whitelist (target_group, env);

-- +goose Down
DROP INDEX IF EXISTS metrics_whitelist_target_env_metric_uidx;
DROP INDEX IF EXISTS targets_whitelist_target_env_uidx;
