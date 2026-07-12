package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTenantRetentionHandlerIsPrivileged(t *testing.T) {
	require.True(t, isPrivilegedScheduledTaskHandler("scheduler.tenant_retention"))
	require.True(t, ScheduledTaskUsesPrivilegedHandler(ScheduledTaskTypeMethod, map[string]any{"handler": "scheduler.tenant_retention"}))
}

func TestGovernanceTaskOutcome(t *testing.T) {
	require.Equal(t, "success", governanceTaskOutcome(ScheduledTaskLogStatusSuccess))
	require.Equal(t, "failure", governanceTaskOutcome(ScheduledTaskLogStatusFailed))
}

func TestTenantRetentionPolicyVersionChangesWithRetention(t *testing.T) {
	first := tenantGovernancePolicyVersion(TenantGovernancePolicy{Retention: TenantRetentionPolicy{AuditDays: 30, DataDays: 90}})
	second := tenantGovernancePolicyVersion(TenantGovernancePolicy{Retention: TenantRetentionPolicy{AuditDays: 60, DataDays: 90}})
	require.NotEqual(t, first, second)
}

func TestTenantRetentionGovernanceTaskCannotBeCreatedByUsers(t *testing.T) {
	err := validateScheduledTaskPayload(ScheduledTask{
		TaskType: ScheduledTaskTypeGovernance,
		Payload:  map[string]any{"handler": "scheduler.tenant_retention"},
	})
	require.Error(t, err)
}
