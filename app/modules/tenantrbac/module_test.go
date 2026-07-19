package tenantrbac

import "testing"

func TestModuleDeclaresTenantRBACContract(t *testing.T) {
	module := New()

	if module.ID() != "tenant-rbac" {
		t.Fatalf("ID() = %q", module.ID())
	}
	if got := len(module.Routes()); got != 34 {
		t.Fatalf("len(Routes()) = %d, want 34", got)
	}

	paths := make(map[string]bool)
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
		paths[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"GET /admin/permission/menus",
		"PUT /admin/user/info",
		"GET /admin/user/list",
		"PUT /admin/role/{id}/permissions",
		"PUT /admin/position/{id}/data_permission",
		"DELETE /admin/leader",
	} {
		if !paths[expected] {
			t.Fatalf("missing route %s", expected)
		}
	}
	if got := module.Menus()[0].Key; got != "permission:user" {
		t.Fatalf("first menu key = %q", got)
	}
	if got := module.Permissions()[0].Key; got != "permission:user:index" {
		t.Fatalf("first permission = %q", got)
	}
}
