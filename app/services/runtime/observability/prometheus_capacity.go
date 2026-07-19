package observability

import (
	"fmt"
	"strings"

	queueservice "goravel/app/services/runtime/queue"
)

func writeQueuePrometheusMetrics(b *strings.Builder, queue queueservice.QueueBacklogMetric) {
	b.WriteString("# HELP goravel_queue_failed_jobs Current failed queue jobs retained in the DLQ table.\n")
	b.WriteString("# TYPE goravel_queue_failed_jobs gauge\n")
	b.WriteString(fmt.Sprintf("goravel_queue_failed_jobs %d\n", queue.FailedJobs))
	b.WriteString("# HELP goravel_queue_outbox_events Current outbox events by backlog status.\n")
	b.WriteString("# TYPE goravel_queue_outbox_events gauge\n")
	b.WriteString(fmt.Sprintf("goravel_queue_outbox_events{status=%q} %d\n", queueservice.QueueOutboxStatusPending, queue.OutboxPending))
	b.WriteString(fmt.Sprintf("goravel_queue_outbox_events{status=%q} %d\n", queueservice.QueueOutboxStatusProcessing, queue.OutboxProcessing))
	b.WriteString(fmt.Sprintf("goravel_queue_outbox_events{status=%q} %d\n", queueservice.QueueOutboxStatusFailed, queue.OutboxFailed))
	b.WriteString(fmt.Sprintf("goravel_queue_outbox_events{status=%q} %d\n", queueservice.QueueOutboxStatusSent, queue.OutboxSent))
	b.WriteString("# HELP goravel_queue_pending_jobs Current pending queue jobs by workload class.\n")
	b.WriteString("# TYPE goravel_queue_pending_jobs gauge\n")
	b.WriteString("# HELP goravel_queue_oldest_backlog_age_seconds Age of the oldest pending queue job by workload class.\n")
	b.WriteString("# TYPE goravel_queue_oldest_backlog_age_seconds gauge\n")
	b.WriteString("# HELP goravel_queue_arrival_rate Queue arrivals per second by workload class.\n")
	b.WriteString("# TYPE goravel_queue_arrival_rate gauge\n")
	b.WriteString("# HELP goravel_queue_completion_rate Queue completions per second by workload class.\n")
	b.WriteString("# TYPE goravel_queue_completion_rate gauge\n")
	for _, metric := range queue.Classes {
		b.WriteString(fmt.Sprintf("goravel_queue_pending_jobs{queue_class=%q} %d\n", metric.Class, metric.Pending))
		b.WriteString(fmt.Sprintf("goravel_queue_oldest_backlog_age_seconds{queue_class=%q} %.0f\n", metric.Class, metric.OldestAge.Seconds()))
		b.WriteString(fmt.Sprintf("goravel_queue_arrival_rate{queue_class=%q} %.6f\n", metric.Class, metric.ArrivalRate))
		b.WriteString(fmt.Sprintf("goravel_queue_completion_rate{queue_class=%q} %.6f\n", metric.Class, metric.CompletionRate))
	}
}

func writeCapacityPrometheusMetrics(b *strings.Builder, snapshot MetricsSnapshot) {
	b.WriteString("# TYPE goravel_casbin_enforcer_cache_entries gauge\n")
	b.WriteString(fmt.Sprintf("goravel_casbin_enforcer_cache_entries %d\n", snapshot.CasbinCache.Entries))
	b.WriteString("# TYPE goravel_casbin_enforcer_cache_events_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_casbin_enforcer_cache_events_total{event=%q} %d\n", "hit", snapshot.CasbinCache.Hits))
	b.WriteString(fmt.Sprintf("goravel_casbin_enforcer_cache_events_total{event=%q} %d\n", "miss", snapshot.CasbinCache.Misses))
	b.WriteString(fmt.Sprintf("goravel_casbin_enforcer_cache_events_total{event=%q} %d\n", "reload", snapshot.CasbinCache.Reloads))
	b.WriteString("# TYPE goravel_tenant_connection_pools gauge\n")
	b.WriteString(fmt.Sprintf("goravel_tenant_connection_pools %d\n", snapshot.TenantConnections.Pools))
	b.WriteString("# TYPE goravel_tenant_connection_budget gauge\n")
	b.WriteString(fmt.Sprintf("goravel_tenant_connection_budget{type=%q} %d\n", "required", snapshot.TenantConnections.RequiredConnections))
	b.WriteString(fmt.Sprintf("goravel_tenant_connection_budget{type=%q} %d\n", "postgresql", snapshot.TenantConnections.PostgreSQLBudget))
	b.WriteString("# TYPE goravel_migration_lock_events_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_migration_lock_events_total{event=%q} %d\n", "acquired", snapshot.MigrationLocks.AcquiredTotal))
	b.WriteString(fmt.Sprintf("goravel_migration_lock_events_total{event=%q} %d\n", "timeout", snapshot.MigrationLocks.TimeoutTotal))
	b.WriteString(fmt.Sprintf("goravel_migration_lock_events_total{event=%q} %d\n", "failure", snapshot.MigrationLocks.FailureTotal))
	b.WriteString("# TYPE goravel_migration_lock_wait_seconds_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_migration_lock_wait_seconds_total %.6f\n", snapshot.MigrationLocks.WaitDurationSecondsTotal))
	b.WriteString("# TYPE goravel_migration_lock_hold_seconds_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_migration_lock_hold_seconds_total %.6f\n", snapshot.MigrationLocks.HoldDurationSecondsTotal))
}
