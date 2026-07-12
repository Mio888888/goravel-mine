package config

import "goravel/app/facades"

func init() {
	config := facades.Config()
	config.Add("scheduler", map[string]any{
		"enabled": config.Env("SCHEDULER_ENABLED", true),
		"node_ip": config.Env("SCHEDULER_NODE_IP", ""),
	})
}
