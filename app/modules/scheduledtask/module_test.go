package scheduledtask

import (
	"reflect"
	"testing"

	"goravel/app/modules"
)

func TestModuleDeclaresScheduledTaskContract(t *testing.T) {
	module := New()

	if module.ID() != "scheduled-task" {
		t.Fatalf("ID() = %q", module.ID())
	}
	if got := len(module.Routes()); got != 10 {
		t.Fatalf("len(Routes()) = %d, want 10", got)
	}

	paths := make(map[string]bool)
	for _, route := range module.Routes() {
		if route.Name == "" {
			t.Fatal("route name is empty")
		}
		paths[route.Method+" "+route.Path] = true
		if route.Install == nil {
			t.Fatalf("route %s %s has nil installer", route.Method, route.Path)
		}
	}
	for _, expected := range []string{
		"GET /admin/platform/scheduled-task/list",
		"GET /admin/platform/scheduled-task/tenant-options",
		"GET /admin/platform/scheduled-task/{id}",
		"POST /admin/platform/scheduled-task",
		"PUT /admin/platform/scheduled-task/{id}",
		"DELETE /admin/platform/scheduled-task",
		"PUT /admin/platform/scheduled-task/{id}/enable",
		"PUT /admin/platform/scheduled-task/{id}/disable",
		"POST /admin/platform/scheduled-task/{id}/run",
		"GET /admin/platform/scheduled-task-log/list",
	} {
		if !paths[expected] {
			t.Fatalf("missing route %s", expected)
		}
	}
	if got := module.Permissions()[0].Key; got != "platform:scheduledTask:list" {
		t.Fatalf("first permission = %q", got)
	}
	if got := module.Menus()[0].Path; got != "/platform-system/scheduled-task" {
		t.Fatalf("menu path = %q", got)
	}
	if got := module.Menus()[0].Key; got != "platform:scheduledTask" {
		t.Fatalf("menu key = %q", got)
	}
	if got := module.Menus()[0].Component; got != "base/views/platform/scheduledTask/index" {
		t.Fatalf("menu component = %q", got)
	}
	migrations := module.Migrations()
	if got := len(migrations); got != 2 {
		t.Fatalf("len(Migrations()) = %d, want 2", got)
	}
	if got := migrations[1].Signature(); got != "202607110010_upsert_tenant_governance_tasks" {
		t.Fatalf("Migrations()[1] = %q", got)
	}
	if got := len(module.Seeders()); got != 1 {
		t.Fatalf("len(Seeders()) = %d, want 1", got)
	}
	if got := module.OpenAPIFiles()[0]; got != "docs/api-contract/openapi/admin-base-apis.openapi.json" {
		t.Fatalf("OpenAPIFiles()[0] = %q", got)
	}
}

func TestTenantOptionsDeclaresAlternatePermissions(t *testing.T) {
	route := findRoute(t, scheduledTaskRoutes(), "platform.scheduled-task.tenant-options")

	want := []string{"platform:scheduledTask:list", "platform:scheduledTask:save", "platform:scheduledTask:update"}
	if got := route.PermissionKeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("tenant options permissions = %#v, want %#v", got, want)
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

func TestRouteInstallerPanicsWhenMethodUnknown(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected unknown method panic")
		}
	}()

	modules.InstallPlatformRoute("TRACE", "/admin/platform/scheduled-task/list", nil)
}
