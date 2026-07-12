package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRBACSensitiveSelectorsBindCanonicalDesiredRoles(t *testing.T) {
	selector, err := rbacUserRolesSelector(42, []string{" Auditor ", "Ops", "Auditor"})
	require.NoError(t, err)

	resource, userID, roles, err := parseRBACUserRolesSelector(selector)
	require.NoError(t, err)
	require.Equal(t, uint64(42), userID)
	require.Equal(t, []string{"Auditor", "Ops"}, roles)
	require.NotEqual(t, "rbac:user:42:roles", resource)
	require.Contains(t, resource, "rbac:user:42:roles:")
}

func TestRBACSensitiveSelectorsBindCanonicalDesiredPermissions(t *testing.T) {
	selector, err := rbacRolePermissionsSelector(7, []string{" permission:user:index ", "permission:role:index", "permission:user:index"})
	require.NoError(t, err)

	resource, roleID, permissions, err := parseRBACRolePermissionsSelector(selector)
	require.NoError(t, err)
	require.Equal(t, uint64(7), roleID)
	require.Equal(t, []string{"permission:role:index", "permission:user:index"}, permissions)
	require.NotEqual(t, "rbac:role:7:permissions", resource)
	require.Contains(t, resource, "rbac:role:7:permissions:")
}

func TestRBACPasswordSnapshotNeverContainsPasswordHash(t *testing.T) {
	before, after, err := rbacPasswordSnapshots(42, true)
	require.NoError(t, err)
	require.NotContains(t, before, "password_hash")
	require.NotContains(t, before, "$2")
	require.NotContains(t, after, "password_hash")
	require.NotContains(t, after, "$2")
}

func TestPermissionAdminUpdateUserRejectsPasswordMutation(t *testing.T) {
	err := NewPermissionAdminService().UpdateUser(42, UserPayload{Password: "new-password"}, 7)

	require.ErrorIs(t, err, ErrSensitiveOperationPolicy)
}

func TestPlatformPermissionAdminUpdateUserRejectsPasswordMutation(t *testing.T) {
	err := NewPlatformPermissionAdminService().UpdateUser(42, UserPayload{Password: "new-password"}, 7)

	require.ErrorIs(t, err, ErrSensitiveOperationPolicy)
}
