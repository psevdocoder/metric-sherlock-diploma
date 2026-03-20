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
