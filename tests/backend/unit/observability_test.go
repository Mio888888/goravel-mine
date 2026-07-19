package unit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
	"goravel/database/seeders"
)

func TestPrometheusMetricsTextIncludesHTTPCounters(t *testing.T) {
	recorder := services.NewObservabilityRecorder(time.Second, 10)
	recorder.Record(services.HTTPObservation{
		Method:   "GET",
		Route:    "/users",
		Status:   200,
		Duration: 25 * time.Millisecond,
	})

	text := services.PrometheusMetricsText(recorder.Snapshot())

	require.Contains(t, text, "# TYPE goravel_http_requests_total counter")
	require.Contains(t, text, `goravel_http_requests_total{method="GET",route="/users",status="200"} 1`)
	require.Contains(t, text, `goravel_http_request_duration_milliseconds_by_status_sum{method="GET",route="/users",status="200"} 25.000`)
	require.Contains(t, text, `goravel_http_request_duration_milliseconds_bucket{method="GET",route="/users",le="50"} 1`)
	require.Contains(t, text, `goravel_http_request_duration_milliseconds_bucket{method="GET",route="/users",le="+Inf"} 1`)
	require.Contains(t, text, `goravel_http_request_duration_milliseconds_count{method="GET",route="/users"} 1`)
	require.Contains(t, text, `goravel_http_slow_requests_observed_total 0`)
	require.Contains(t, text, `goravel_sql_slow_queries_retained 0`)
	require.Contains(t, text, `goravel_sql_slow_queries_observed_total 0`)
	require.Contains(t, text, `goravel_go_goroutines`)
	require.Contains(t, text, `goravel_db_pool_open_connections`)
}

func TestPrometheusMetricsTextBindsReleaseGitSHA(t *testing.T) {
	t.Setenv("RELEASE_GIT_SHA", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	text := services.PrometheusMetricsText(services.MetricsSnapshot{
		Queue: services.QueueBacklogMetric{FailedJobs: 2},
	})

	require.Contains(t, text, `goravel_queue_failed_jobs{release_git_sha="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} 2`)
	require.NotContains(t, text, "goravel_queue_failed_jobs 2")
}

func TestPrometheusMetricsTextIncludesQueueBacklog(t *testing.T) {
	text := services.PrometheusMetricsText(services.MetricsSnapshot{
		Queue: services.QueueBacklogMetric{
			FailedJobs:       2,
			OutboxPending:    3,
			OutboxProcessing: 1,
			OutboxFailed:     4,
		},
	})

	require.Contains(t, text, "# TYPE goravel_queue_failed_jobs gauge")
	require.Contains(t, text, "goravel_queue_failed_jobs 2")
	require.Contains(t, text, "# TYPE goravel_queue_outbox_events gauge")
	require.Contains(t, text, `goravel_queue_outbox_events{status="pending"} 3`)
	require.Contains(t, text, `goravel_queue_outbox_events{status="processing"} 1`)
	require.Contains(t, text, `goravel_queue_outbox_events{status="failed"} 4`)
}

func TestPrometheusMetricsTextIncludesTenantGovernance(t *testing.T) {
	text := services.PrometheusMetricsText(services.MetricsSnapshot{
		TenantGovernance: services.TenantGovernanceObservability{
			EvidenceExpired:    2,
			VerificationFailed: 3,
			OldestRunAge:       4 * time.Minute,
		},
	})

	require.Contains(t, text, "# TYPE goravel_tenant_governance_evidence_expired gauge")
	require.Contains(t, text, "goravel_tenant_governance_evidence_expired 2")
	require.Contains(t, text, "# TYPE goravel_tenant_governance_verification_failed gauge")
	require.Contains(t, text, "goravel_tenant_governance_verification_failed 3")
	require.Contains(t, text, "# TYPE goravel_tenant_governance_run_age_seconds gauge")
	require.Contains(t, text, "goravel_tenant_governance_run_age_seconds 240")
}

func TestSlowSQLRecorderParsesFrameworkSlowQueryLog(t *testing.T) {
	services.ResetSlowSQLMetricsForTest()

	services.RecordSlowSQLFromMessage(`[205.500ms] [rows:3] [SLOW] select * from "user"`, "req-1", "trace-1")

	items := services.SlowSQLMetrics()
	require.Len(t, items, 1)
	require.Equal(t, `select * from "user"`, items[0].SQL)
	require.Equal(t, "3", items[0].Rows)
	require.Equal(t, 205.5, items[0].DurationMS)
	require.Equal(t, "req-1", items[0].RequestID)
	require.Equal(t, "trace-1", items[0].TraceID)
	require.Equal(t, uint64(1), services.SlowSQLMetricsSnapshot().Total)
}

func TestSlowSQLRecorderFillsIDsFromContext(t *testing.T) {
	services.ResetSlowSQLMetricsForTest()
	ctx := services.WithRequestID(context.Background(), "req-context")
	ctx = services.WithTraceID(ctx, "trace-context")

	services.RecordSlowSQLFromContext(ctx, `[205.500ms] [rows:3] [SLOW] select 1`, "", "")

	items := services.SlowSQLMetrics()
	require.Len(t, items, 1)
	require.Equal(t, "req-context", items[0].RequestID)
	require.Equal(t, "trace-context", items[0].TraceID)
}

func TestEmptyObservabilityCollectionsEncodeAsArrays(t *testing.T) {
	services.ResetObservabilityMetricsForTest()
	services.ResetSlowSQLMetricsForTest()

	snapshot := services.ObservabilityMetrics()

	require.NotNil(t, snapshot.ByRoute)
	require.NotNil(t, snapshot.SlowRequests)
	require.NotNil(t, services.SlowSQLMetrics())
}

func TestNormalizeObservabilityIDCapsLength(t *testing.T) {
	value := services.NormalizeObservabilityID(strings.Repeat("a", 200))

	require.Len(t, value, 128)
}

func TestPlatformObservabilityRouteHasPermission(t *testing.T) {
	permissions := services.PlatformPermissionsForRoute("GET", "/admin/platform/observability/slow-requests")

	require.Equal(t, []string{"platform:observability:list"}, permissions)
}

func TestPlatformTenantGovernanceRoutesHavePermissions(t *testing.T) {
	require.Equal(t, []string{"platform:tenant:governance", "platform:tenant:destroy"}, services.PlatformPermissionsForRoute("GET", "/admin/platform/tenant/{id}/governance"))
	require.Equal(t, []string{"platform:tenant:governance"}, services.PlatformPermissionsForRoute("PUT", "/admin/platform/tenant/{id}/governance"))
}

func TestPlatformModuleLifecycleRoutesHavePermissions(t *testing.T) {
	require.Equal(t, []string{"platform:moduleLifecycle:list"}, services.PlatformPermissionsForRoute("GET", "/admin/platform/module-lifecycle/state"))
	require.Equal(t, []string{"platform:moduleLifecycle:list"}, services.PlatformPermissionsForRoute("GET", "/admin/platform/module-lifecycle/runs"))
	require.Equal(t, []string{"platform:moduleLifecycle:log"}, services.PlatformPermissionsForRoute("GET", "/admin/platform/module-lifecycle/steps"))
	require.Equal(t, []string{"platform:moduleLifecycle:list"}, services.PlatformPermissionsForRoute("GET", "/admin/platform/module-lifecycle/locks"))
	require.Equal(t, []string{"platform:moduleLifecycle:list"}, services.PlatformPermissionsForRoute("GET", "/admin/platform/module-lifecycle/diff"))
	require.Equal(t, []string{"platform:moduleLifecycle:execute"}, services.PlatformPermissionsForRoute("POST", "/admin/platform/module-lifecycle/locks/release-stale"))
	require.Equal(t, []string{"platform:moduleLifecycle:execute"}, services.PlatformPermissionsForRoute("POST", "/admin/platform/module-lifecycle/execute"))
}

func TestPlatformSecurityControlRoutesHavePermissions(t *testing.T) {
	expected := []string{
		"platform:security:control", "platform:security:mfa", "platform:moduleLifecycle:execute",
		"platform:tenant:destroy", "platform:tenant:export", "platform:tenant:permissions", "platform:tenant:updatePlan", "platform:tenant:governance",
		"platform:tenant:suspend", "platform:tenant:resume", "platform:tenant:archive",
		"platform:user:password", "platform:user:setRole", "platform:role:setMenu",
		"platform:storageConfig:save", "platform:storageConfig:update", "platform:storageConfig:delete",
	}
	require.Equal(t, expected, services.PlatformPermissionsForRoute("POST", "/admin/platform/security/reauth-token"))
	require.Equal(t, expected, services.PlatformPermissionsForRoute("POST", "/admin/platform/security/approvals"))
	require.Equal(t, expected, services.PlatformPermissionsForRoute("GET", "/admin/platform/security/approvals/{approval_id}"))
	require.Equal(t, expected, services.PlatformPermissionsForRoute("PUT", "/admin/platform/security/approvals/{approval_id}/approve"))
}

func TestPlatformReferenceCaseRoutesHavePermissions(t *testing.T) {
	require.Equal(t, []string{"platform:referenceCase:list"}, services.PlatformPermissionsForRoute("GET", "/admin/platform/reference-case/list"))
	require.Equal(t, []string{"platform:referenceCase:save"}, services.PlatformPermissionsForRoute("POST", "/admin/platform/reference-case"))
	require.Equal(t, []string{"platform:referenceCase:update"}, services.PlatformPermissionsForRoute("PUT", "/admin/platform/reference-case/{id}"))
	require.Equal(t, []string{"platform:referenceCase:delete"}, services.PlatformPermissionsForRoute("DELETE", "/admin/platform/reference-case"))
}

func TestPlatformQueueFailedJobRoutesHaveDedicatedPermissions(t *testing.T) {
	require.Equal(t, []string{"platform:queueFailedJob:list"}, services.PlatformPermissionsForRoute("GET", "/admin/platform/queue/failed-jobs"))
	require.Equal(t, []string{"platform:queueFailedJob:retry"}, services.PlatformPermissionsForRoute("POST", "/admin/platform/queue/failed-jobs/retry"))
	require.Equal(t, []string{"platform:queueFailedJob:delete"}, services.PlatformPermissionsForRoute("DELETE", "/admin/platform/queue/failed-jobs"))
	require.Equal(t, []string{"platform:attachment:upload", "platform:user:save", "platform:user:update"}, services.PlatformPermissionsForRoute("POST", "/admin/platform/attachment/upload"))
}

func TestPlatformObservabilityMenuBelongsToDashboard(t *testing.T) {
	var dashboardID uint64
	var observabilityFound bool
	for _, item := range seeders.PlatformMenuCatalogSeeds() {
		switch item.Name {
		case "dashboard":
			dashboardID = item.ID
			require.Equal(t, uint64(0), item.ParentID)
			require.Equal(t, "/dashboard", item.Path)
		case "platform:observability":
			observabilityFound = true
			require.Equal(t, dashboardID, item.ParentID)
			require.Equal(t, "/dashboard/observability", item.Path)
		}
	}

	require.NotZero(t, dashboardID)
	require.True(t, observabilityFound)
}

func TestPlatformQueueFailedJobMenuBelongsToScheduledTask(t *testing.T) {
	var scheduledTaskID uint64
	found := map[string]bool{}
	for _, item := range seeders.PlatformMenuCatalogSeeds() {
		if item.Name == "platform:scheduledTask" {
			scheduledTaskID = item.ID
		}
		if strings.HasPrefix(item.Name, "platform:queueFailedJob:") {
			found[item.Name] = true
			require.Equal(t, scheduledTaskID, item.ParentID)
		}
	}

	require.NotZero(t, scheduledTaskID)
	require.True(t, found["platform:queueFailedJob:list"])
	require.True(t, found["platform:queueFailedJob:retry"])
	require.True(t, found["platform:queueFailedJob:delete"])
}
