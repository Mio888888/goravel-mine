package datacenter

import (
	"reflect"
	"testing"

	"goravel/app/modules"
)

func TestModuleDeclaresDataCenterContract(t *testing.T) {
	module := New()

	if module.ID() != "data-center" {
		t.Fatalf("ID() = %q", module.ID())
	}
	if got := len(module.Routes()); got != 26 {
		t.Fatalf("len(Routes()) = %d, want 26", got)
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
		"POST /admin/platform/attachment/upload",
		"GET /admin/platform/dictionary/list",
		"DELETE /admin/platform/storage-config",
		"GET /admin/dictionary/list",
		"POST /admin/attachment/upload",
		"GET /admin/user-operation-log/list",
	} {
		if !paths[expected] {
			t.Fatalf("missing route %s", expected)
		}
	}
	for _, forbidden := range []string{
		"DELETE /admin/user-login-log",
		"DELETE /admin/user-operation-log",
	} {
		if paths[forbidden] {
			t.Fatalf("forbidden audit log delete route %s", forbidden)
		}
	}
	for _, permission := range module.Permissions() {
		if permission.Key == "log:userLogin:delete" || permission.Key == "log:userOperation:delete" {
			t.Fatalf("forbidden audit log delete permission %s", permission.Key)
		}
	}
	if got := module.Menus()[0].Key; got != "platform:dictionary" {
		t.Fatalf("first menu key = %q", got)
	}
	if got := module.Permissions()[0].Key; got != "platform:attachment:upload" {
		t.Fatalf("first permission = %q", got)
	}
}

func TestPlatformAttachmentUploadDeclaresAlternatePermissions(t *testing.T) {
	route := findRoute(t, dataCenterRoutes(), "platform.attachment.upload")

	want := []string{"platform:attachment:upload", "platform:user:save", "platform:user:update"}
	if got := route.PermissionKeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("upload permissions = %#v, want %#v", got, want)
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
