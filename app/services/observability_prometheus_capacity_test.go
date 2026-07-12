package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrometheusMetricsTextIncludesCapacityAndMigrationMetrics(t *testing.T) {
	text := PrometheusMetricsText(MetricsSnapshot{
		CasbinCache:       CasbinEnforcerCacheMetrics{Entries: 2, Hits: 3, Misses: 1, Reloads: 1},
		TenantConnections: TenantConnectionCapacityMetrics{Pools: 4, RequiredConnections: 480, PostgreSQLBudget: 500, Safe: true},
		MigrationLocks:    MigrationLockMetrics{AcquiredTotal: 2, TimeoutTotal: 1, WaitDurationSecondsTotal: 0.5},
	})
	require.Contains(t, text, "goravel_casbin_enforcer_cache_entries 2")
	require.Contains(t, text, `goravel_casbin_enforcer_cache_events_total{event="hit"} 3`)
	require.Contains(t, text, "goravel_tenant_connection_pools 4")
	require.Contains(t, text, `goravel_tenant_connection_budget{type="required"} 480`)
	require.Contains(t, text, `goravel_migration_lock_events_total{event="timeout"} 1`)
	require.Contains(t, text, "goravel_migration_lock_wait_seconds_total 0.500000")
}
