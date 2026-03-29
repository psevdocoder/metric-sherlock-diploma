package config

import "git.server.lan/pkg/config/realtimeconfig"

type (
	configKey         realtimeconfig.Key
	realtimeConfigKey realtimeconfig.Key
)

const (
	// Port HTTP port to bind to
	Port configKey = "values.port"
	// Env Application environment enum of [local, deploy]
	Env configKey = "values.env"
	// EtcdEndpoints Etcd cluster endpoints, separated via commas
	EtcdEndpoints configKey = "values.etcd_endpoints"
	// SdConfigPath Specifies location of service discovery config
	SdConfigPath configKey = "values.sd_config_path"
	// KafkaBrokers Comma separated kafka brokers
	KafkaBrokers configKey = "values.kafka_brokers"
	// KafkaViolationsTopic Kafka topic for service violation facts
	KafkaViolationsTopic configKey = "values.kafka_violations_topic"
	// OutboxPollInterval Interval for polling outbox rows
	OutboxPollInterval configKey = "values.outbox_poll_interval"
	// OutboxBatchSize Max outbox messages per poll
	OutboxBatchSize configKey = "values.outbox_batch_size"
	// OutboxMaxRetries Max retries before marking outbox row as failed
	OutboxMaxRetries configKey = "values.outbox_max_retries"
	// JwtIssuerInternal Expected JWT issuer (iss) for requests from app, e.g. http://keycloak:8080/realms/local
	JwtIssuerInternal configKey = "values.jwt_issuer_internal"
	// JwtIssuerPublic Expected JWT issuer (iss) for e.g. swagger, e.g. http://localhost:8080/realms/local
	JwtIssuerPublic configKey = "values.jwt_issuer_public"
	// JwtJwksEndpoint Optional JWKS endpoint override; empty means issuer + /protocol/openid-connect/certs
	JwtJwksEndpoint configKey = "values.jwt_jwks_endpoint"
	// JwtExpectedAzp Optional expected azp value (client id)
	JwtExpectedAzp configKey = "values.jwt_expected_azp"
	// ProduceTasksCronExpr Task producer cron schedule
	ProduceTasksCronExpr realtimeConfigKey = "realtime_config.produce_tasks_cron_expr"
	// LimitsConfig Setups limits configs
	LimitsConfig realtimeConfigKey = "realtime_config.limits_config"
)

func GetValue[T configKey | realtimeConfigKey](key T) (realtimeconfig.Value, error) {
	return realtimeconfig.Get(realtimeconfig.Key(key))
}

func Watch(key realtimeConfigKey, callback realtimeconfig.WatchCallback) {
	realtimeconfig.Watch(realtimeconfig.Key(key), callback)
}
