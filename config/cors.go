package config

import (
	"goravel/app/facades"
)

func init() {
	config := facades.Config()
	config.Add("cors", map[string]any{
		// Cross-Origin Resource Sharing (CORS) Configuration
		//
		// Here you may configure your settings for cross-origin resource sharing
		// or "CORS". This determines what cross-origin operations may execute
		// in web browsers. You are free to adjust these settings as needed.
		//
		// To learn more: https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS
		"paths": envStringList("CORS_PATHS", "*"),
		"allowed_methods": envStringList(
			"CORS_ALLOWED_METHODS",
			"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD",
		),
		"allowed_origins": corsAllowedOrigins(config.EnvBool("CORS_SUPPORTS_CREDENTIALS", false)),
		"allowed_headers": envStringList(
			"CORS_ALLOWED_HEADERS",
			"Authorization",
			"Content-Type",
			"X-Requested-With",
			"X-Request-Id",
			"X-Trace-Id",
			"X-CSRF-Token",
			"X-Tenant",
			"X-Tenant-Code",
		),
		"exposed_headers": envStringList(
			"CORS_EXPOSED_HEADERS",
			"Content-Disposition",
			"X-Request-Id",
			"X-Trace-Id",
		),
		"max_age":              config.Env("CORS_MAX_AGE", 600),
		"supports_credentials": config.Env("CORS_SUPPORTS_CREDENTIALS", false),
	})
}

func corsAllowedOrigins(supportsCredentials bool) []string {
	origins := envStringList(
		"CORS_ALLOWED_ORIGINS",
		"http://localhost:2888",
		"http://127.0.0.1:2888",
	)
	if !supportsCredentials {
		return origins
	}
	filtered := make([]string, 0, len(origins))
	for _, origin := range origins {
		if origin != "*" {
			filtered = append(filtered, origin)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}
	return []string{"http://localhost:2888", "http://127.0.0.1:2888"}
}
