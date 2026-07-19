package migrations

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTenantGovernanceTaskSeedsEnableIsolationVerification(t *testing.T) {
	tasks := tenantGovernanceTaskSeeds()
	require.Len(t, tasks, 2)
	require.Equal(t, "tenant_retention_governance", tasks[0].Code)
	require.Equal(t, int8(2), tasks[0].Status)
	require.Equal(t, "0 0 2 * * *", tasks[0].Cron)
	require.Equal(t, "tenant_isolation_verification", tasks[1].Code)
	require.Equal(t, int8(1), tasks[1].Status)
	require.Equal(t, "0 0 3 * * *", tasks[1].Cron)
	require.Contains(t, tasks[1].Payload, "scheduler.tenant_isolation_verify")
}

func TestNextTenantGovernanceTaskRunUsesDeclaredUTCSlot(t *testing.T) {
	after := time.Date(2026, time.July, 12, 10, 30, 0, 0, time.UTC)
	next, err := nextTenantGovernanceTaskRun("0 0 3 * * *", after)

	require.NoError(t, err)
	require.Equal(t, time.Date(2026, time.July, 13, 3, 0, 0, 0, time.UTC), next)
}

func TestNextTenantGovernanceTaskRunRejectsFiveFields(t *testing.T) {
	_, err := nextTenantGovernanceTaskRun("0 3 * * *", time.Now())
	require.Error(t, err)
}

func TestTenantGovernanceTaskConflictUpdatePreservesRuntimeConfiguration(t *testing.T) {
	for _, column := range []string{
		"cron_expression", "timezone", "timeout_seconds", "allow_overlap", "max_log_output",
		"target_ips", "tenant_ids", "run_on_one_server", "status", "next_run_at", "remark",
	} {
		require.NotContains(t, strings.ToLower(tenantGovernanceTaskConflictUpdate), column+" =")
	}
}

func TestScheduledTaskMigrationsSharePlatformConnection(t *testing.T) {
	create := M202607040001CreateScheduledTaskTables{}
	upsert := M202607110010UpsertTenantGovernanceTasks{}
	require.Equal(t, create.Connection(), upsert.Connection())
}
