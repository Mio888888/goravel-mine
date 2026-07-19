package security

import "testing"

func TestModuleDeclaresSecurityContract(t *testing.T) {
	module := New()

	if module.ID() != "security" {
		t.Fatalf("ID() = %q", module.ID())
	}
	if got := len(module.Routes()); got != 45 {
		t.Fatalf("len(Routes()) = %d, want 45", got)
	}

	paths := make(map[string]bool)
	for _, route := range module.Routes() {
		if route.Name == "" {
			t.Fatal("route name is empty")
		}
		if route.Install == nil {
			t.Fatalf("route %s %s has nil installer", route.Method, route.Path)
		}
		paths[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"POST /admin/passport/login",
		"POST /admin/passport/sso/authorize",
		"POST /admin/passport/sso/callback",
		"GET /admin/passport/captcha",
		"GET /api/system/captcha",
		"POST /admin/platform/passport/login",
		"POST /admin/platform/security/mfa/setup",
		"POST /admin/platform/security/reauth-token",
		"POST /admin/platform/security/approvals",
		"GET /admin/platform/security/approvals/{approval_id}",
		"PUT /admin/platform/security/approvals/{approval_id}/approve",
		"POST /admin/security/reauth-token",
		"POST /admin/security/approvals",
		"GET /admin/security/approvals/{approval_id}",
		"PUT /admin/security/approvals/{approval_id}/approve",
		"POST /admin/security/mfa/setup",
		"GET /admin/sso-provider/list",
		"DELETE /admin/sso-user-binding/{id}",
		"GET /admin/sso-login-log/stats",
	} {
		if !paths[expected] {
			t.Fatalf("missing route %s", expected)
		}
	}
	if got := module.Menus()[0].Key; got != "security:ssoProvider" {
		t.Fatalf("first menu key = %q", got)
	}
	if got := module.Permissions()[0].Key; got != "platform:security:mfa" {
		t.Fatalf("first permission = %q", got)
	}
	if got := module.Permissions()[1].Key; got != "platform:security:control" {
		t.Fatalf("second permission = %q", got)
	}
}
