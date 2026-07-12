package config

import "goravel/app/facades"

func init() {
	config := facades.Config()
	config.Add("tenant", map[string]any{
		"default":                             config.Env("TENANT_DEFAULT", "default"),
		"platform_connection":                 config.Env("TENANT_PLATFORM_CONNECTION", config.Env("DB_CONNECTION", "postgres")),
		"database_connection_budget":          config.Env("TENANT_DATABASE_CONNECTION_BUDGET", 500),
		"database_connection_budget_override": config.Env("TENANT_DATABASE_CONNECTION_BUDGET_OVERRIDE", false),
		"pod_count":                           config.Env("APP_POD_COUNT", 3),
		// Only trusted proxy sources can supply this header for tenant domain routing.
		"trusted_forwarded_host_header": config.Env("TENANT_TRUSTED_FORWARDED_HOST_HEADER", "X-Forwarded-Host"),
		"trusted_forwarded_host_proxies": config.Env(
			"TENANT_TRUSTED_FORWARDED_HOST_PROXIES",
			"127.0.0.1,::1",
		),
	})
}
