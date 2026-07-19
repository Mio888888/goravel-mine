package unit

import (
	"testing"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"github.com/stretchr/testify/require"

	"goravel/app/services"
)

func TestCasbinSyncPermissionForRouteMapsAdminRoutesToCodes(t *testing.T) {
	require.Equal(t, "permission:user:index", services.PermissionForRoute("GET", "/admin/user/list"))
	require.Equal(t, "permission:role:index", services.PermissionForRoute("get", "/admin/role/list"))
	require.Empty(t, services.PermissionForRoute("GET", "/admin/unknown"))
}

func TestCasbinSyncPermissionCodeMatcherIsExact(t *testing.T) {
	m, err := model.NewModelFromString(`
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && (p.obj == "*" || r.obj == p.obj) && (p.act == "*" || regexMatch(r.act, p.act))
`)
	require.NoError(t, err)

	enforcer, err := casbin.NewEnforcer(m)
	require.NoError(t, err)
	ok, err := enforcer.AddGroupingPolicy("user:2", "role:Auditor")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = enforcer.AddPolicy("role:Auditor", "permission:user:index", "*")
	require.NoError(t, err)
	require.True(t, ok)

	allowed, err := enforcer.Enforce("user:2", "permission:user:index", "GET")
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, err = enforcer.Enforce("user:2", "permission:role:index", "GET")
	require.NoError(t, err)
	require.False(t, allowed)
}
