package config

import "goravel/app/facades"

func init() {
	config := facades.Config()
	config.Add("telemetry", map[string]any{
		"service": map[string]any{
			"name":        config.Env("APP_NAME", "goravel"),
			"version":     config.Env("APP_VERSION", ""),
			"environment": config.Env("APP_ENV", ""),
		},
		"resource":    map[string]any{},
		"propagators": config.Env("OTEL_PROPAGATORS", "tracecontext,baggage"),
		"traces": map[string]any{
			"exporter": config.Env("OTEL_TRACES_EXPORTER", ""),
			"sampler": map[string]any{
				"parent": config.Env("OTEL_TRACES_SAMPLER_PARENT", true),
				"type":   config.Env("OTEL_TRACES_SAMPLER_TYPE", "traceidratio"),
				"ratio":  config.Env("OTEL_TRACES_SAMPLER_RATIO", 0.05),
			},
		},
		"metrics": map[string]any{
			"exporter": config.Env("OTEL_METRICS_EXPORTER", ""),
			"reader": map[string]any{
				"interval": config.Env("OTEL_METRIC_EXPORT_INTERVAL", "60s"),
				"timeout":  config.Env("OTEL_METRIC_EXPORT_TIMEOUT", "30s"),
			},
		},
		"logs": map[string]any{
			"exporter": config.Env("OTEL_LOGS_EXPORTER", ""),
			"processor": map[string]any{
				"interval": config.Env("OTEL_LOG_EXPORT_INTERVAL", "1s"),
				"timeout":  config.Env("OTEL_LOG_EXPORT_TIMEOUT", "30s"),
			},
		},
		"exporters": map[string]any{
			"otlptrace": map[string]any{
				"driver":   "otlp",
				"endpoint": config.Env("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://localhost:4318"),
				"protocol": config.Env("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf"),
				"insecure": config.Env("OTEL_EXPORTER_OTLP_TRACES_INSECURE", true),
				"timeout":  config.Env("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "10s"),
			},
			"otlpmetric": map[string]any{
				"driver":             "otlp",
				"endpoint":           config.Env("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "http://localhost:4318"),
				"protocol":           config.Env("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL", "http/protobuf"),
				"insecure":           config.Env("OTEL_EXPORTER_OTLP_METRICS_INSECURE", true),
				"timeout":            config.Env("OTEL_EXPORTER_OTLP_METRICS_TIMEOUT", "10s"),
				"metric_temporality": config.Env("OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY", "cumulative"),
			},
			"otlplog": map[string]any{
				"driver":   "otlp",
				"endpoint": config.Env("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "http://localhost:4318"),
				"protocol": config.Env("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL", "http/protobuf"),
				"insecure": config.Env("OTEL_EXPORTER_OTLP_LOGS_INSECURE", true),
				"timeout":  config.Env("OTEL_EXPORTER_OTLP_LOGS_TIMEOUT", "10s"),
			},
			"zipkin": map[string]any{
				"driver":   "zipkin",
				"endpoint": config.Env("OTEL_EXPORTER_ZIPKIN_ENDPOINT", "http://localhost:9411/api/v2/spans"),
			},
			"console": map[string]any{
				"driver":       "console",
				"pretty_print": config.Env("OTEL_CONSOLE_PRETTY_PRINT", false),
			},
		},
	})
}
