package messagebus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	redisfacades "github.com/goravel/redis/facades"

	"goravel/app/facades"
	"goravel/app/models"
)

const (
	AdapterHealthUnknown  = "UNKNOWN"
	AdapterHealthUp       = "UP"
	AdapterHealthDegraded = "DEGRADED"
	AdapterHealthDown     = "DOWN"
)

type AdapterCapabilities struct {
	Persistent      bool `json:"persistent"`
	Cluster         bool `json:"cluster"`
	Broadcast       bool `json:"broadcast"`
	OfflineRecovery bool `json:"offline_recovery"`
	Retry           bool `json:"retry"`
	DeadLetter      bool `json:"dead_letter"`
	Ordering        bool `json:"ordering"`
}

func (c AdapterCapabilities) JSONMap() models.JSONMap {
	return models.JSONMap{
		"persistent": c.Persistent, "cluster": c.Cluster, "broadcast": c.Broadcast,
		"offline_recovery": c.OfflineRecovery, "retry": c.Retry,
		"dead_letter": c.DeadLetter, "ordering": c.Ordering,
	}
}

func EnsureConfiguredAdapters(ctx context.Context) error {
	connections, _ := facades.Config().Get("queue.connections").(map[string]any)
	if len(connections) == 0 {
		connections = map[string]any{
			"sync":     nil,
			"database": nil,
			"redis":    nil,
		}
	}
	for name := range connections {
		definition := adapterForQueueConnection(name)
		if definition.AdapterKey == "" {
			continue
		}
		if err := upsertAdapter(ctx, definition); err != nil {
			return err
		}
	}
	return nil
}

func ConfiguredAdapter(connection string) (models.MiddlewareAdapter, error) {
	adapter := adapterForQueueConnection(connection)
	if adapter.AdapterKey == "" {
		return models.MiddlewareAdapter{}, fmt.Errorf("queue connection is not configured: %s", strings.TrimSpace(connection))
	}
	return adapter, nil
}

func adapterForQueueConnection(connection string) models.MiddlewareAdapter {
	connection = strings.TrimSpace(connection)
	driver := strings.TrimSpace(facades.Config().GetString("queue.connections." + connection + ".driver"))
	if connection == "" || driver == "" {
		return models.MiddlewareAdapter{}
	}
	adapterType := "goravel_queue"
	capabilities := AdapterCapabilities{
		Persistent: driver != "sync", Cluster: driver != "sync",
		OfflineRecovery: driver != "sync", Retry: true, DeadLetter: true,
	}
	if connection == "redis" {
		adapterType = "redis_queue"
	}
	if driver == "sync" {
		adapterType = "memory"
		capabilities.Broadcast = true
	}
	return models.MiddlewareAdapter{
		AdapterKey:   "queue:" + connection,
		Name:         "Goravel Queue " + connection,
		AdapterType:  adapterType,
		Connection:   connection,
		Capabilities: capabilities.JSONMap(),
		Enabled:      true,
		HealthStatus: AdapterHealthUnknown,
		Version:      1,
	}
}

func upsertAdapter(ctx context.Context, adapter models.MiddlewareAdapter) error {
	now := time.Now()
	query := OrmForConnectionWithContext(ctx, PlatformConnection()).Query()
	capabilities, err := json.Marshal(adapter.Capabilities)
	if err != nil {
		return err
	}
	var existing models.MiddlewareAdapter
	err = query.Table("middleware_adapter").Where("adapter_key", adapter.AdapterKey).First(&existing)
	if err == nil && existing.ID > 0 {
		_, err = query.Exec(`
			UPDATE middleware_adapter
			SET adapter_type = ?, connection = ?, capabilities = ?::jsonb, updated_at = ?
			WHERE id = ?
		`, adapter.AdapterType, adapter.Connection, string(capabilities), now, existing.ID)
		return err
	}
	_, err = query.Exec(`
		INSERT INTO middleware_adapter (
			adapter_key, name, adapter_type, connection, capabilities, enabled,
			health_status, version, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?::jsonb, true, ?, 1, ?, ?)
	`, adapter.AdapterKey, adapter.Name, adapter.AdapterType, adapter.Connection,
		string(capabilities), AdapterHealthUnknown, now, now)
	return err
}

func ProbeAdapterHealth(ctx context.Context, adapter models.MiddlewareAdapter) (string, error) {
	ctx = contextOrBackground(ctx)
	driver := strings.TrimSpace(facades.Config().GetString("queue.connections." + adapter.Connection + ".driver"))
	status := AdapterHealthDown
	var healthErr error
	switch {
	case driver == "sync":
		status = AdapterHealthUp
	case adapter.Connection == "redis":
		client, err := redisfacades.Instance(facades.Config().GetString("queue.connections.redis.connection", "default"))
		if err != nil {
			healthErr = err
			break
		}
		healthErr = client.Ping(ctx).Err()
		if healthErr == nil {
			status = AdapterHealthUp
		}
	case driver == "database":
		db, err := OrmForConnectionWithContext(ctx, facades.Config().GetString("queue.connections."+adapter.Connection+".connection")).DB()
		if err != nil {
			healthErr = err
			break
		}
		healthErr = db.PingContext(ctx)
		if healthErr == nil {
			status = AdapterHealthUp
		}
	default:
		healthErr = fmt.Errorf("unsupported adapter connection: %s", adapter.Connection)
	}
	return status, healthErr
}

func CheckAdapterHealth(ctx context.Context, adapter models.MiddlewareAdapter) (string, error) {
	status, healthErr := ProbeAdapterHealth(ctx, adapter)
	now := time.Now()
	_, updateErr := OrmForConnectionWithContext(ctx, PlatformConnection()).Query().
		Table("middleware_adapter").
		Where("id", adapter.ID).
		Update(map[string]any{"health_status": status, "last_checked_at": now, "updated_at": now})
	if updateErr != nil {
		return status, updateErr
	}
	return status, healthErr
}

func adapterCapabilities(adapter models.MiddlewareAdapter) AdapterCapabilities {
	return AdapterCapabilities{
		Persistent:      jsonBool(adapter.Capabilities, "persistent"),
		Cluster:         jsonBool(adapter.Capabilities, "cluster"),
		Broadcast:       jsonBool(adapter.Capabilities, "broadcast"),
		OfflineRecovery: jsonBool(adapter.Capabilities, "offline_recovery"),
		Retry:           jsonBool(adapter.Capabilities, "retry"),
		DeadLetter:      jsonBool(adapter.Capabilities, "dead_letter"),
		Ordering:        jsonBool(adapter.Capabilities, "ordering"),
	}
}

func jsonBool(value models.JSONMap, key string) bool {
	typed, _ := value[key].(bool)
	return typed
}
