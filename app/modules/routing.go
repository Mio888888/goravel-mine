package modules

import (
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/facades"
	"goravel/app/http/middleware"
)

type RouteHandlers map[string]contractshttp.HandlerFunc

func BindRouteHandlers(moduleID string, routes []Route, handlers RouteHandlers) []Route {
	for index, route := range routes {
		handler, ok := handlers[route.Name]
		if !ok {
			panic(moduleID + " route handler missing: " + route.Name)
		}
		routes[index].Install = routeInstaller(route, handler)
	}

	return routes
}

func routeInstaller(route Route, handler contractshttp.HandlerFunc) InstallRouteFunc {
	switch {
	case route.HasMiddleware("public"):
		return InstallRoute(route.Method, route.Path, handler)
	case route.HasMiddleware("platform-self-audit"):
		return InstallRoute(route.Method, route.Path, handler, middleware.PlatformSelfAudit())
	case route.HasMiddleware("platform-auth-audit"):
		return InstallPlatformAuthAuditRoute(route.Method, route.Path, handler)
	case route.HasMiddleware("platform-auth"):
		return InstallPlatformAuthRoute(route.Method, route.Path, handler)
	case route.HasMiddleware("platform-admin"):
		return InstallPlatformRoute(route.Method, route.Path, handler)
	case route.HasMiddleware("tenant-audit-only"):
		return InstallTenantAuditOnlyRoute(route.Method, route.Path, handler)
	case route.HasMiddleware("tenant-rbac-audit"):
		return InstallTenantAuditRoute(route.Method, route.Path, handler)
	case route.HasMiddleware("tenant-rbac"):
		return InstallTenantRoute(route.Method, route.Path, handler)
	case route.HasMiddleware("tenant"):
		return InstallTenantOnlyRoute(route.Method, route.Path, handler)
	default:
		panic("module route middleware unsupported: " + route.Name)
	}
}

func (r Route) HasMiddleware(name string) bool {
	for _, routeMiddleware := range r.Middlewares {
		if routeMiddleware == name {
			return true
		}
	}

	return false
}

func InstallPlatformRoute(method string, path string, handler contractshttp.HandlerFunc) InstallRouteFunc {
	return installRoute(method, path, handler, middleware.PlatformAdmin())
}

func InstallPlatformAuthRoute(method string, path string, handler contractshttp.HandlerFunc) InstallRouteFunc {
	return installRoute(method, path, handler, middleware.PlatformAuth())
}

func InstallPlatformAuthAuditRoute(method string, path string, handler contractshttp.HandlerFunc) InstallRouteFunc {
	return installRoute(method, path, handler, middleware.PlatformAuthAudit())
}

func InstallTenantRoute(method string, path string, handler contractshttp.HandlerFunc) InstallRouteFunc {
	return installTenantGovernedRoute(method, path, handler, middleware.CasbinAuthz())
}

func InstallTenantAuditRoute(method string, path string, handler contractshttp.HandlerFunc) InstallRouteFunc {
	return installTenantGovernedRoute(method, path, handler, middleware.CasbinAuthz(), middleware.OperationLog())
}

func InstallTenantOnlyRoute(method string, path string, handler contractshttp.HandlerFunc) InstallRouteFunc {
	return installTenantGovernedRoute(method, path, handler)
}

func InstallTenantAuditOnlyRoute(method string, path string, handler contractshttp.HandlerFunc) InstallRouteFunc {
	return installTenantGovernedRoute(method, path, handler, middleware.OperationLog())
}

func installTenantGovernedRoute(method string, path string, handler contractshttp.HandlerFunc, middlewares ...contractshttp.Middleware) InstallRouteFunc {
	chain := []contractshttp.Middleware{middleware.TenantContext()}
	if module := middleware.TenantGovernanceModuleFromPath(path); module != "" {
		chain = append(chain, middleware.TenantGovernanceModule(module))
	}
	chain = append(chain, middlewares...)
	return installRoute(method, path, handler, chain...)
}

func InstallRoute(method string, path string, handler contractshttp.HandlerFunc, middlewares ...contractshttp.Middleware) InstallRouteFunc {
	return installRoute(method, path, handler, middlewares...)
}

func installRoute(method string, path string, handler contractshttp.HandlerFunc, middlewares ...contractshttp.Middleware) InstallRouteFunc {
	normalized := strings.ToUpper(strings.TrimSpace(method))
	if !isSupportedRouteMethod(normalized) {
		panic("module route method unsupported: " + method)
	}

	return func() {
		router := facades.Route().Middleware(middlewares...)
		switch normalized {
		case "GET":
			router.Get(path, handler)
		case "POST":
			router.Post(path, handler)
		case "PUT":
			router.Put(path, handler)
		case "DELETE":
			router.Delete(path, handler)
		}
	}
}

func isSupportedRouteMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "DELETE":
		return true
	default:
		return false
	}
}
