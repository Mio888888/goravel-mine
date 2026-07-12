package platformrbac

import "testing"

func TestModuleDeclaresPlatformRBACContract(t *testing.T) {
	module := New()

	if module.ID() != "platform-rbac" {
		t.Fatalf("ID() = %q", module.ID())
	}
	if got := len(module.Routes()); got != 20 {
		t.Fatalf("len(Routes()) = %d, want 20", got)
	}

	paths := make(map[string]bool)
	for _, route := range module.Routes() {
		if route.Name == "" {
			t.Fatal("route name is empty")
		}
		if route.Permission == "" && !hasMiddleware(route, "platform-auth") && !hasMiddleware(route, "platform-auth-audit") {
			t.Fatalf("route %s has empty permission", route.Name)
		}
		if route.Install == nil {
			t.Fatalf("route %s %s has nil installer", route.Method, route.Path)
		}
		paths[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"GET /admin/platform/permission/menus",
		"GET /admin/platform/user/list",
		"PUT /admin/platform/user/{id}/roles",
		"GET /admin/platform/role/list",
		"DELETE /admin/platform/menu",
	} {
		if !paths[expected] {
			t.Fatalf("missing route %s", expected)
		}
	}
	if got := module.Menus()[0].Key; got != "platform:user" {
		t.Fatalf("first menu key = %q", got)
	}
	if got := module.Permissions()[0].Key; got != "platform:user:list" {
		t.Fatalf("first permission = %q", got)
	}
}

func TestRouteInstallerPanicsWhenHandlerMissing(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected missing handler panic")
		}
	}()

	buildRoutesWithHandlers(map[string]handlerFunc{})
}
