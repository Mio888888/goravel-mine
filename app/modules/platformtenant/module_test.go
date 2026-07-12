package platformtenant

import (
	"reflect"
	"testing"

	"goravel/app/modules"
)

func TestModuleDeclaresPlatformTenantContract(t *testing.T) {
	module := New()

	if module.ID() != "platform-tenant" {
		t.Fatalf("ID() = %q", module.ID())
	}
	if got := len(module.Routes()); got != 23 {
		t.Fatalf("len(Routes()) = %d, want 23", got)
	}

	paths := make(map[string]bool)
	permissionsByRoute := make(map[string]string)
	for _, route := range module.Routes() {
		if route.Name == "" {
			t.Fatal("route name is empty")
		}
		if route.Permission == "" {
			t.Fatalf("route %s has empty permission", route.Name)
		}
		if route.Install == nil {
			t.Fatalf("route %s %s has nil installer", route.Method, route.Path)
		}
		routeKey := route.Method + " " + route.Path
		paths[routeKey] = true
		permissionsByRoute[routeKey] = route.Permission
	}
	for _, expected := range []string{
		"GET /admin/platform/tenant/list",
		"GET /admin/platform/tenant/permission-catalog",
		"GET /admin/platform/tenant/{id}/governance",
		"PUT /admin/platform/tenant/{id}/governance",
		"PUT /admin/platform/tenant/{id}/permissions",
		"POST /admin/platform/tenant/{id}/exports",
		"GET /admin/platform/tenant/{id}/exports/{run_id}",
		"GET /admin/platform/tenant/{id}/exports/{run_id}/download",
		"GET /admin/platform/tenant-plan/list",
		"DELETE /admin/platform/tenant-plan",
	} {
		if !paths[expected] {
			t.Fatalf("missing route %s", expected)
		}
	}
	if got := permissionsByRoute["POST /admin/platform/tenant/{id}/permissions/plan-diff"]; got != "platform:tenant:updatePlan" {
		t.Fatalf("plan-diff permission = %q, want platform:tenant:updatePlan", got)
	}
	if got := module.Menus()[0].Key; got != "platform:tenant" {
		t.Fatalf("first menu key = %q", got)
	}
	if got := module.Menus()[1].Key; got != "platform:tenantPlan" {
		t.Fatalf("second menu key = %q", got)
	}
	if got := module.Permissions()[0].Key; got != "platform:tenant:list" {
		t.Fatalf("first permission = %q", got)
	}
	if got := module.OpenAPIFiles()[0]; got != "docs/api-contract/openapi/admin-base-apis.openapi.json" {
		t.Fatalf("OpenAPIFiles()[0] = %q", got)
	}
}

func TestPermissionCatalogDeclaresAlternatePermissions(t *testing.T) {
	route := findRoute(t, platformTenantRoutes(), "platform.tenant.permission-list")

	want := []string{"platform:tenant:permissions", "platform:tenantPlan:save", "platform:tenantPlan:update"}
	if got := route.PermissionKeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("permission catalog permissions = %#v, want %#v", got, want)
	}
}

func TestTenantGovernanceReadAllowsDestroyPermission(t *testing.T) {
	route := findRoute(t, platformTenantRoutes(), "platform.tenant.governance")

	want := []string{"platform:tenant:governance", "platform:tenant:destroy"}
	if got := route.PermissionKeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("tenant governance read permissions = %#v, want %#v", got, want)
	}
}

func findRoute(t *testing.T, routes []modules.Route, name string) modules.Route {
	t.Helper()

	for _, route := range routes {
		if route.Name == name {
			return route
		}
	}

	t.Fatalf("route %s not found", name)
	return modules.Route{}
}

func TestRouteInstallerPanicsWhenHandlerMissing(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected missing handler panic")
		}
	}()

	buildRoutesWithHandlers(map[string]handlerFunc{})
}
