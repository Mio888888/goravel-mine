package config

import (
	"strings"

	"goravel/app/facades"
)

func init() {
	config := facades.Config()
	defaultEnabled := schedulerEnabledByDefault(config.Env("APP_ENV", "production"))
	config.Add("scheduler", map[string]any{
		"enabled": config.Env("SCHEDULER_ENABLED", defaultEnabled),
		"node_ip": config.Env("SCHEDULER_NODE_IP", ""),
	})
}

func schedulerEnabledByDefault(environment any) bool {
	value, _ := environment.(string)
	return !strings.EqualFold(strings.TrimSpace(value), "testing")
}
