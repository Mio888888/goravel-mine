package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/models"
)

func TestTenantPermissionSnapshotAllowsLegacyTenantWithoutSnapshot(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{}}

	require.True(t, TenantAllowsPermission(tenant, "permission:user:index"))
}

func TestTenantPermissionSnapshotAllowsListedPermission(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index", "permission:role", "permission:role:index"},
		},
	}}

	require.True(t, TenantAllowsPermission(tenant, "permission:user:index"))
	require.False(t, TenantAllowsPermission(tenant, "permission:user:delete"))
}

func TestTenantPermissionRequiresAuthorizedAncestors(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission:user:index"},
		},
	}}

	require.False(t, TenantAllowsPermission(tenant, "permission:user:index"))

	tenant.Features = models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
	}

	require.True(t, TenantAllowsPermission(tenant, "permission:user:index"))
}

func TestTenantAllowsRoutePermission(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
	}}

	require.True(t, TenantAllowsRoute(tenant, "GET", "/admin/user/list"))
	require.False(t, TenantAllowsRoute(tenant, "DELETE", "/admin/user"))
	require.True(t, TenantAllowsRoute(tenant, "GET", "/admin/passport/getInfo"))
}

func TestSnapshotFeaturesForPlanCopiesPlanPermissions(t *testing.T) {
	planFeatures := models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
		"sso": map[string]any{"enabled": true},
	}
	input := models.JSONMap{"theme": "dark"}

	features := SnapshotFeaturesForPlan(planFeatures, input)

	require.Equal(t, "dark", features["theme"])
	require.NotContains(t, features, "sso")
	require.Equal(t, models.JSONMap{
		"allowed": []string{"permission", "permission:user", "permission:user:index"},
	}, features["permissions"])
}

func TestSnapshotFeaturesForPlanUsesExplicitPermissionOverride(t *testing.T) {
	planFeatures := models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
	}
	input := models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:role", "permission:role:index"},
		},
	}

	features := SnapshotFeaturesForPlan(planFeatures, input)

	require.Equal(t, models.JSONMap{
		"allowed": []string{"permission", "permission:role", "permission:role:index"},
	}, features["permissions"])
}

func TestFilterMenusByTenantPermissionsKeepsAuthorizedAncestors(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
	}}
	menus := []AdminMenuItem{
		{ID: 1, ParentID: 0, Name: "permission"},
		{ID: 2, ParentID: 1, Name: "permission:user"},
		{ID: 3, ParentID: 2, Name: "permission:user:index"},
		{ID: 4, ParentID: 2, Name: "permission:user:delete"},
		{ID: 5, ParentID: 1, Name: "permission:role"},
	}

	filtered := FilterAdminMenusByTenantPermissions(tenant, menus)

	require.ElementsMatch(t, []uint64{1, 2, 3}, adminMenuIDs(filtered))
}

func TestValidateTenantRolePermissionsRejectsOutOfScope(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
	}}

	require.NoError(t, ValidateTenantRolePermissions(tenant, []string{"permission:user:index"}))
	require.Error(t, ValidateTenantRolePermissions(tenant, []string{"permission:user:delete"}))
}
