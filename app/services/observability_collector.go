package services

import (
	"context"
	"time"
)

func ObservabilityMetrics() MetricsSnapshot {
	snapshot := defaultObservabilityRecorder.Snapshot()
	slowSQL := SlowSQLMetricsSnapshot()
	snapshot.SlowSQLRetained = len(slowSQL.Items)
	snapshot.SlowSQLObserved = slowSQL.Total
	snapshot.GoRuntime = GoRuntimeMetrics()
	snapshot.DBPool = DBPoolMetrics()
	snapshot.SchedulerNodes = SchedulerHeartbeatSnapshot(time.Now()).SchedulerNodes
	snapshot.Queue = QueueBacklogMetrics(context.Background())
	snapshot.CasbinCache = CasbinEnforcerCacheSnapshot()
	snapshot.TenantConnections = TenantConnectionCapacitySnapshot()
	snapshot.MigrationLocks = MigrationLockMetricsSnapshot()
	if governance, err := TenantGovernanceObservabilitySnapshot(context.Background(), time.Now()); err == nil {
		snapshot.TenantGovernance = governance
	}
	return snapshot
}
