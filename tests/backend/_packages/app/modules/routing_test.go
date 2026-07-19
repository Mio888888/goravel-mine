package modules

import (
	"testing"

	contractshttp "github.com/goravel/framework/contracts/http"
)

func TestBindRouteHandlersAssignsInstallersForSupportedPolicies(t *testing.T) {
	policies := []string{
		"public",
		"platform-self-audit",
		"platform-auth-audit",
		"platform-auth",
		"platform-admin",
		"tenant-audit-only",
		"tenant-rbac-audit",
		"tenant-rbac",
		"tenant",
	}

	for _, policy := range policies {
		t.Run(policy, func(t *testing.T) {
			routes := BindRouteHandlers("test", []Route{{
				Name:        "test.route",
				Method:      "GET",
				Path:        "/test",
				Middlewares: []string{policy},
			}}, RouteHandlers{
				"test.route": func(contractshttp.Context) contractshttp.Response { return nil },
			})

			if routes[0].Install == nil {
				t.Fatal("route installer is nil")
			}
		})
	}
}

func TestBindRouteHandlersPanicsWhenHandlerMissing(t *testing.T) {
	assertRoutingPanic(t, "test route handler missing: test.route", func() {
		BindRouteHandlers("test", []Route{{Name: "test.route"}}, nil)
	})
}

func TestBindRouteHandlersPanicsWhenMiddlewarePolicyUnsupported(t *testing.T) {
	assertRoutingPanic(t, "module route middleware unsupported: test.route", func() {
		BindRouteHandlers("test", []Route{{
			Name:        "test.route",
			Method:      "GET",
			Path:        "/test",
			Middlewares: []string{"custom"},
		}}, RouteHandlers{
			"test.route": func(contractshttp.Context) contractshttp.Response { return nil },
		})
	})
}

func TestRouteHasMiddleware(t *testing.T) {
	route := Route{Middlewares: []string{"tenant", "tenant-rbac"}}

	if !route.HasMiddleware("tenant-rbac") {
		t.Fatal("expected route middleware")
	}
	if route.HasMiddleware("platform-admin") {
		t.Fatal("unexpected route middleware")
	}
}

func assertRoutingPanic(t *testing.T, expected string, action func()) {
	t.Helper()

	defer func() {
		if recovered := recover(); recovered != expected {
			t.Fatalf("panic = %#v, want %q", recovered, expected)
		}
	}()

	action()
}
