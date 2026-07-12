package platformrbac

import (
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/controllers/admin"
	"goravel/app/modules"
)

type handlerFunc = contractshttp.HandlerFunc

type Module struct{}

func New() Module {
	return Module{}
}

func (m Module) ID() string {
	return "platform-rbac"
}

func (m Module) Metadata() modules.Metadata {
	return modules.BuiltinMetadata("Platform RBAC")
}

func (m Module) Package() modules.Package {
	return modules.BuiltinPackage(m.ID(), "platform-team")
}

func (m Module) Routes() []modules.Route {
	permissionController := admin.NewPlatformPermissionController()
	userController := admin.NewPlatformUserAdminController()
	roleController := admin.NewPlatformRoleAdminController()
	menuController := admin.NewPlatformMenuAdminController()

	return buildRoutesWithHandlers(map[string]handlerFunc{
		"platform.permission.menus":  permissionController.Menus,
		"platform.permission.roles":  permissionController.Roles,
		"platform.permission.update": permissionController.Update,
		"platform.user.list":         userController.List,
		"platform.user.create":       userController.Create,
		"platform.user.update":       userController.Update,
		"platform.user.delete":       userController.Delete,
		"platform.user.password":     userController.ResetPassword,
		"platform.user.roles":        userController.Roles,
		"platform.user.set-roles":    userController.SetRoles,
		"platform.role.list":         roleController.List,
		"platform.role.create":       roleController.Create,
		"platform.role.update":       roleController.Update,
		"platform.role.delete":       roleController.Delete,
		"platform.role.permissions":  roleController.Permissions,
		"platform.role.set-menu":     roleController.SetPermissions,
		"platform.menu.list":         menuController.List,
		"platform.menu.create":       menuController.Create,
		"platform.menu.update":       menuController.Update,
		"platform.menu.delete":       menuController.Delete,
	})
}

func buildRoutesWithHandlers(handlers map[string]handlerFunc) []modules.Route {
	routes := platformRBACRoutes()
	for index, route := range routes {
		handler, ok := handlers[route.Name]
		if !ok {
			panic("platform-rbac route handler missing: " + route.Name)
		}
		if hasMiddleware(route, "platform-auth-audit") {
			routes[index].Install = modules.InstallPlatformAuthAuditRoute(route.Method, route.Path, handler)
			continue
		}
		if hasMiddleware(route, "platform-auth") {
			routes[index].Install = modules.InstallPlatformAuthRoute(route.Method, route.Path, handler)
			continue
		}
		routes[index].Install = modules.InstallPlatformRoute(route.Method, route.Path, handler)
	}

	return routes
}

func hasMiddleware(route modules.Route, name string) bool {
	for _, middleware := range route.Middlewares {
		if middleware == name {
			return true
		}
	}

	return false
}

func platformRBACRoutes() []modules.Route {
	return []modules.Route{
		{Name: "platform.permission.menus", Method: "GET", Path: "/admin/platform/permission/menus", Middlewares: []string{"platform-auth"}},
		{Name: "platform.permission.roles", Method: "GET", Path: "/admin/platform/permission/roles", Middlewares: []string{"platform-auth"}},
		{Name: "platform.permission.update", Method: "POST", Path: "/admin/platform/permission/update", Middlewares: []string{"platform-auth-audit"}},
		{Name: "platform.user.list", Method: "GET", Path: "/admin/platform/user/list", Permission: "platform:user:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.user.create", Method: "POST", Path: "/admin/platform/user", Permission: "platform:user:save", Middlewares: []string{"platform-admin"}},
		{Name: "platform.user.update", Method: "PUT", Path: "/admin/platform/user/{id}", Permission: "platform:user:update", Middlewares: []string{"platform-admin"}},
		{Name: "platform.user.delete", Method: "DELETE", Path: "/admin/platform/user", Permission: "platform:user:delete", Middlewares: []string{"platform-admin"}},
		{Name: "platform.user.password", Method: "PUT", Path: "/admin/platform/user/password", Permission: "platform:user:password", Middlewares: []string{"platform-admin"}},
		{Name: "platform.user.roles", Method: "GET", Path: "/admin/platform/user/{id}/roles", Permission: "platform:user:getRole", Middlewares: []string{"platform-admin"}},
		{Name: "platform.user.set-roles", Method: "PUT", Path: "/admin/platform/user/{id}/roles", Permission: "platform:user:setRole", Middlewares: []string{"platform-admin"}},
		{Name: "platform.role.list", Method: "GET", Path: "/admin/platform/role/list", Permission: "platform:role:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.role.create", Method: "POST", Path: "/admin/platform/role", Permission: "platform:role:save", Middlewares: []string{"platform-admin"}},
		{Name: "platform.role.update", Method: "PUT", Path: "/admin/platform/role/{id}", Permission: "platform:role:update", Middlewares: []string{"platform-admin"}},
		{Name: "platform.role.delete", Method: "DELETE", Path: "/admin/platform/role", Permission: "platform:role:delete", Middlewares: []string{"platform-admin"}},
		{Name: "platform.role.permissions", Method: "GET", Path: "/admin/platform/role/{id}/permissions", Permission: "platform:role:getMenu", Middlewares: []string{"platform-admin"}},
		{Name: "platform.role.set-menu", Method: "PUT", Path: "/admin/platform/role/{id}/permissions", Permission: "platform:role:setMenu", Middlewares: []string{"platform-admin"}},
		{Name: "platform.menu.list", Method: "GET", Path: "/admin/platform/menu/list", Permission: "platform:menu:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.menu.create", Method: "POST", Path: "/admin/platform/menu", Permission: "platform:menu:create", Middlewares: []string{"platform-admin"}},
		{Name: "platform.menu.update", Method: "PUT", Path: "/admin/platform/menu/{id}", Permission: "platform:menu:save", Middlewares: []string{"platform-admin"}},
		{Name: "platform.menu.delete", Method: "DELETE", Path: "/admin/platform/menu", Permission: "platform:menu:delete", Middlewares: []string{"platform-admin"}},
	}
}

func (m Module) Menus() []modules.Menu {
	return []modules.Menu{
		{Key: "platform:user", ParentKey: "platform", Title: "平台用户", Path: "/platform/user", Component: "base/views/platform/user/index", Permission: "platform:user:list", Type: "M", I18n: "baseMenu.platform.user", Sort: 30},
		{Key: "platform:role", ParentKey: "platform", Title: "平台角色", Path: "/platform/role", Component: "base/views/platform/role/index", Permission: "platform:role:list", Type: "M", I18n: "baseMenu.platform.role", Sort: 40},
		{Key: "platform:menu", ParentKey: "platform", Title: "平台菜单", Path: "/platform/menu", Component: "base/views/platform/menu/index", Permission: "platform:menu:list", Type: "M", I18n: "baseMenu.platform.menu", Sort: 50},
	}
}

func (m Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "platform:user:list", Description: "平台用户列表"},
		{Key: "platform:user:save", Description: "平台用户保存"},
		{Key: "platform:user:update", Description: "平台用户更新"},
		{Key: "platform:user:delete", Description: "平台用户删除"},
		{Key: "platform:user:password", Description: "平台用户初始化密码"},
		{Key: "platform:user:getRole", Description: "获取平台用户角色"},
		{Key: "platform:user:setRole", Description: "平台用户角色赋予"},
		{Key: "platform:role:list", Description: "平台角色列表"},
		{Key: "platform:role:save", Description: "平台角色保存"},
		{Key: "platform:role:update", Description: "平台角色更新"},
		{Key: "platform:role:delete", Description: "平台角色删除"},
		{Key: "platform:role:getMenu", Description: "获取平台角色菜单"},
		{Key: "platform:role:setMenu", Description: "平台角色菜单赋予"},
		{Key: "platform:menu:list", Description: "平台菜单列表"},
		{Key: "platform:menu:create", Description: "平台菜单保存"},
		{Key: "platform:menu:save", Description: "平台菜单更新"},
		{Key: "platform:menu:delete", Description: "平台菜单删除"},
	}
}

func (m Module) Migrations() []schema.Migration {
	return nil
}

func (m Module) Seeders() []seeder.Seeder {
	return nil
}

func (m Module) OpenAPIFiles() []string {
	return []string{"docs/api-contract/openapi/admin-base-apis.openapi.json"}
}

func (m Module) TestTemplates() []string {
	return []string{
		"tests/feature/admin/platform_rbac_test.go",
		"tests/feature/admin/platform_bootstrap_test.go",
	}
}
