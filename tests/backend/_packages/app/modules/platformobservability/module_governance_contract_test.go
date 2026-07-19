package platformobservability

import (
	"strings"
	"testing"

	"goravel/app/modules"
	"goravel/tests/backend/testsupport"
)

type governanceModuleContract struct {
	Routes      []governanceRouteContract      `json:"routes"`
	Menus       []governanceMenuContract       `json:"menus"`
	Permissions []governancePermissionContract `json:"permissions"`
}

type governanceRouteContract struct {
	Name        string   `json:"name"`
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Permission  string   `json:"permission,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Middlewares []string `json:"middlewares"`
}

type governanceMenuContract struct {
	Key        string `json:"key"`
	ParentKey  string `json:"parent_key,omitempty"`
	Title      string `json:"title"`
	Path       string `json:"path"`
	Component  string `json:"component,omitempty"`
	Permission string `json:"permission,omitempty"`
	Type       string `json:"type,omitempty"`
	I18n       string `json:"i18n,omitempty"`
	Sort       int    `json:"sort"`
}

type governancePermissionContract struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

func TestModuleGovernanceContract(t *testing.T) {
	module := New()

	contract := governanceModuleContract{
		Routes:      governanceRoutes(module.Routes()),
		Menus:       governanceMenus(module.Menus()),
		Permissions: governancePermissions(module.Permissions()),
	}

	requireGovernanceGoldenJSON(t, "module-governance-contract", contract)
}

func governanceRoutes(routes []modules.Route) []governanceRouteContract {
	items := make([]governanceRouteContract, 0, len(routes))
	for _, route := range routes {
		if !strings.HasPrefix(route.Name, "platform.module-lifecycle.") {
			continue
		}
		items = append(items, governanceRouteContract{
			Name:        route.Name,
			Method:      route.Method,
			Path:        route.Path,
			Permission:  route.Permission,
			Permissions: route.Permissions,
			Middlewares: route.Middlewares,
		})
	}
	return items
}

func governanceMenus(menus []modules.Menu) []governanceMenuContract {
	items := make([]governanceMenuContract, 0, len(menus))
	for _, menu := range menus {
		if !strings.HasPrefix(menu.Key, "platform:moduleLifecycle") {
			continue
		}
		items = append(items, governanceMenuContract(menu))
	}
	return items
}

func governancePermissions(permissions []modules.Permission) []governancePermissionContract {
	items := make([]governancePermissionContract, 0, len(permissions))
	for _, permission := range permissions {
		if !strings.HasPrefix(permission.Key, "platform:moduleLifecycle") {
			continue
		}
		items = append(items, governancePermissionContract(permission))
	}
	return items
}

func requireGovernanceGoldenJSON(t *testing.T, name string, value any) {
	t.Helper()
	path := testsupport.RepositoryPath(t,
		"tests", "backend", "_packages", "app", "modules", "platformobservability", "testdata",
		name+".golden.json",
	)
	testsupport.RequireGoldenJSON(t, path, value)
}
