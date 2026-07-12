package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAvailableAuditRetentionTargetsSkipsMissingTables(t *testing.T) {
	original := auditRetentionHasTable
	t.Cleanup(func() { auditRetentionHasTable = original })
	auditRetentionHasTable = func(connection, table string) bool {
		return table != "tenant_permission_audit"
	}

	targets := availableAuditRetentionTargets("tenant_demo", []auditRetentionTarget{
		{Table: "user_login_log", Column: "login_time"},
		{Table: "tenant_permission_audit", Column: "created_at"},
	})

	require.Equal(t, []auditRetentionTarget{
		{Table: "user_login_log", Column: "login_time"},
	}, targets)
}

func TestAuditRetentionServiceRejectsDirectPhysicalPrune(t *testing.T) {
	_, err := NewAuditRetentionService("unused").Prune(30, false)

	require.ErrorIs(t, err, ErrAuditPruneExecutionRequired)
}
