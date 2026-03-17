package config

import "git.server.lan/pkg/config/realtimeconfig"

type secretKey realtimeconfig.Key

const (
	// PgDsn Database connection dsn
	PgDsn secretKey = "secrets.pg_dsn"
)

func GetSecret(key secretKey) (realtimeconfig.Value, error) {
	return realtimeconfig.Get(realtimeconfig.Key(key))
}
