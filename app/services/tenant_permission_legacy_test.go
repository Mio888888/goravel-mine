package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/models"
)

func TestTenantFullPermissionNamesIncludesMenuButtonsAndRoutes(t *testing.T) {
	names := TenantFullPermissionNames()

	require.Contains(t, names, "permission:user")
	require.Contains(t, names, "permission:user:index")
	require.Contains(t, names, "permission:user:delete")
	require.Contains(t, names, "log:ssoLogin:stats")
	require.NotContains(t, names, "platform:tenant:list")
}

func TestBuildTenantPermissionAuditComputesDiff(t *testing.T) {
	record := BuildTenantPermissionAudit(TenantPermissionAuditInput{
		TenantID:   9,
		TenantCode: "acme",
		Operation:  TenantPermissionAuditOperationUpdate,
		Source:     TenantPermissionAuditSourcePlatform,
		Before: TenantPermissionPayload{
			Allowed: []string{"permission:user:index", "permission:role:index"},
		},
		After: TenantPermissionPayload{
			Allowed: []string{"permission:user:index", "permission:menu:index"},
		},
		OperatorID: 7,
		Remark:     "manual adjustment",
	})

	require.Equal(t, uint64(9), record.TenantID)
	require.Equal(t, "acme", record.TenantCode)
	require.Equal(t, TenantPermissionAuditOperationUpdate, record.Operation)
	require.Equal(t, []string{"permission:menu:index"}, record.Added)
	require.Equal(t, []string{"permission:role:index"}, record.Removed)
	require.Equal(t, []string{"permission:user:index"}, record.Unchanged)
	require.Equal(t, uint64(7), record.OperatorID)
	require.Equal(t, "manual adjustment", record.Remark)
}

func TestBuildLegacyTenantPermissionSnapshotSkipsExplicitSnapshot(t *testing.T) {
	tenant := Tenant{
		ID:   1,
		Code: "legacy",
	}

	snapshot, ok := BuildLegacyTenantPermissionSnapshot(tenant)

	require.True(t, ok)
	require.Contains(t, snapshot.Allowed, "permission:user:index")

	tenant.Features = models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission:user:index"},
		},
	}

	_, ok = BuildLegacyTenantPermissionSnapshot(tenant)
	require.False(t, ok)
}
