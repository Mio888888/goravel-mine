package config

import (
	"strconv"
	"strings"

	"github.com/goravel/framework/contracts/queue"
	redisfacades "github.com/goravel/redis/facades"

	"goravel/app/facades"
)

func init() {
	config := facades.Config()
	defaultConcurrent := queueConcurrentFromEnv(config.Env("QUEUE_CONCURRENT", 1))
	config.Add("queue", map[string]any{
		// Default Queue Connection Name
		"default": config.Env("QUEUE_CONNECTION", "sync"),
		"worker": map[string]any{
			"enabled": config.Env("QUEUE_WORKER_ENABLED", true),
		},
		"outbox": map[string]any{
			"enabled":          config.Env("QUEUE_OUTBOX_ENABLED", true),
			"interval_seconds": config.Env("QUEUE_OUTBOX_INTERVAL_SECONDS", 5),
			"batch":            config.Env("QUEUE_OUTBOX_BATCH", 20),
			"owner":            config.Env("QUEUE_OUTBOX_OWNER", "queue-outbox-runner"),
		},

		// Queue Connections
		//
		// Here you may configure the connection information for each server that is used by your application.
		// Drivers: "sync", "database", "custom"
		"connections": map[string]any{
			"sync": map[string]any{
				"driver": "sync",
			},
			"database": map[string]any{
				"driver":     "database",
				"connection": "postgres",
				"queue":      "default",
				"concurrent": defaultConcurrent,
			},
			"redis": map[string]any{
				"driver":     "custom",
				"connection": "default",
				"queue":      "default",
				"concurrent": defaultConcurrent,
				"via": func() (queue.Driver, error) {
					return redisfacades.Queue("redis")
				},
			},
		},

		// Failed Queue Jobs
		//
		// These options configure the behavior of failed queue job logging so you
		// can control how and where failed jobs are stored.
		"failed": map[string]any{
			"database": config.Env("DB_CONNECTION", "postgres"),
			"table":    "failed_jobs",
		},
	})
}

func queueConcurrentFromEnv(value any) int {
	concurrent := 0
	switch typed := value.(type) {
	case int:
		concurrent = typed
	case int64:
		concurrent = int(typed)
	case float64:
		concurrent = int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			concurrent = parsed
		}
	}
	if concurrent < 1 {
		return 1
	}
	return concurrent
}
