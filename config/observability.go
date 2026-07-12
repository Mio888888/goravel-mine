package config

import "goravel/app/facades"

func init() {
	config := facades.Config()
	config.Add("observability", map[string]any{
		"service": map[string]any{
			"name":        config.Env("APP_NAME", "goravel"),
			"version":     config.Env("APP_VERSION", ""),
			"environment": config.Env("APP_ENV", ""),
		},
		"request_id": map[string]any{
			"header": config.Env("OBS_REQUEST_ID_HEADER", "X-Request-Id"),
		},
		"trace_id": map[string]any{
			"header": config.Env("OBS_TRACE_ID_HEADER", "X-Trace-Id"),
		},
		"metrics": map[string]any{
			"enabled": config.Env("OBS_METRICS_ENABLED", false),
			"path":    config.Env("OBS_METRICS_PATH", "/metrics"),
			"token":   config.Env("OBS_METRICS_TOKEN", ""),
		},
		"slow_request": map[string]any{
			"threshold_ms": config.Env("OBS_SLOW_REQUEST_THRESHOLD_MS", 1000),
			"max_entries":  config.Env("OBS_SLOW_REQUEST_MAX_ENTRIES", 100),
		},
		"slow_sql": map[string]any{
			"max_entries": config.Env("OBS_SLOW_SQL_MAX_ENTRIES", 100),
		},
		"audit": map[string]any{
			"enabled": config.Env("OBS_AUDIT_ENABLED", true),
		},
	})
}
