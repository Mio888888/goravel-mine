package platformobservability

import (
	"slices"
	"testing"

	"goravel/app/modules"
)

func TestModuleDeclaresPlatformObservabilityContract(t *testing.T) {
	module := New()

	if module.ID() != "platform-observability" {
		t.Fatalf("ID() = %q", module.ID())
	}
	routes := module.Routes()
	if got := len(routes); got != 8 {
		t.Fatalf("len(Routes()) = %d, want 8", got)
	}

	route := routes[0]
	if route.Name != "platform.observability.slow-requests" {
		t.Fatalf("route name = %q", route.Name)
	}
	if route.Method != "GET" || route.Path != "/admin/platform/observability/slow-requests" {
		t.Fatalf("route = %s %s", route.Method, route.Path)
	}
	if route.Permission != "platform:observability:list" {
		t.Fatalf("route permission = %q", route.Permission)
	}
	if route.Install == nil {
		t.Fatal("route installer is nil")
	}
	if route := findRoute(routes, "platform.module-lifecycle.execute"); route == nil {
		t.Fatal("module lifecycle execute route missing")
	} else if route.Permission != "platform:moduleLifecycle:execute" || route.Install == nil {
		t.Fatalf("module lifecycle execute route = %#v", route)
	}
	if route := findRoute(routes, "platform.module-lifecycle.steps"); route == nil {
		t.Fatal("module lifecycle steps route missing")
	} else if route.Permission != "platform:moduleLifecycle:log" || route.Install == nil {
		t.Fatalf("module lifecycle steps route = %#v", route)
	}
	if route := findRoute(routes, "platform.module-lifecycle.diff"); route == nil {
		t.Fatal("module lifecycle diff route missing")
	} else if route.Permission != "platform:moduleLifecycle:list" || route.Install == nil {
		t.Fatalf("module lifecycle diff route = %#v", route)
	}
	if route := findRoute(routes, "platform.module-lifecycle.release"); route == nil {
		t.Fatal("module lifecycle lock release route missing")
	} else if route.Permission != "platform:moduleLifecycle:execute" || route.Install == nil {
		t.Fatalf("module lifecycle lock release route = %#v", route)
	}

	if got := module.Menus()[0].Key; got != "platform:observability" {
		t.Fatalf("menu key = %q", got)
	}
	if got := module.Menus()[0].ParentKey; got != "dashboard" {
		t.Fatalf("menu parent key = %q", got)
	}
	if got := module.Permissions()[0].Key; got != "platform:observability:list" {
		t.Fatalf("permission key = %q", got)
	}
	if !hasPermission(module.Permissions(), "platform:moduleLifecycle:execute") {
		t.Fatal("module lifecycle execute permission missing")
	}
	if !hasPermission(module.Permissions(), "platform:moduleLifecycle:log") {
		t.Fatal("module lifecycle log permission missing")
	}
	for _, file := range []string{
		"docs/api-contract/openapi/admin-base-apis.openapi.json",
		"docs/api-contract/openapi/module-governance.openapi.json",
	} {
		if !slices.Contains(module.OpenAPIFiles(), file) {
			t.Fatalf("OpenAPIFiles() = %q, missing %q", module.OpenAPIFiles(), file)
		}
	}
}

func findRoute(routes []modules.Route, name string) *modules.Route {
	for index := range routes {
		if routes[index].Name == name {
			return &routes[index]
		}
	}
	return nil
}

func hasPermission(permissions []modules.Permission, key string) bool {
	for _, permission := range permissions {
		if permission.Key == key {
			return true
		}
	}
	return false
}

func TestRouteInstallerPanicsWhenHandlerMissing(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected missing handler panic")
		}
	}()

	buildRoutesWithHandlers(map[string]handlerFunc{})
}
