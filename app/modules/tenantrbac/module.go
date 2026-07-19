package tenantrbac

import (
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"

	"goravel/app/http/controllers/admin"
	"goravel/app/modules"
)

type Module struct{}

func New() Module {
	return Module{}
}

func (m Module) ID() string {
	return "tenant-rbac"
}

func (m Module) Metadata() modules.Metadata {
	return modules.BuiltinMetadata("Tenant RBAC", modules.RequiredDependency("platform-tenant"))
}

func (m Module) Package() modules.Package {
	return modules.BuiltinPackage(m.ID(), "tenant-team")
}

func (m Module) Routes() []modules.Route {
	permissionController := admin.NewPermissionController()
	profileController := admin.NewUserProfileController()
	userController := admin.NewUserAdminController()
	roleController := admin.NewRoleAdminController()
	menuController := admin.NewMenuAdminController()
	departmentController := admin.NewDepartmentAdminController()
	positionController := admin.NewPositionAdminController()
	leaderController := admin.NewLeaderAdminController()

	return modules.BindRouteHandlers(m.ID(), tenantRBACRoutes(), modules.RouteHandlers{
		"tenant.permission.menus":     permissionController.Menus,
		"tenant.permission.roles":     permissionController.Roles,
		"tenant.permission.update":    permissionController.Update,
		"tenant.user-profile.update":  profileController.UpdateInfo,
		"tenant.user.list":            userController.List,
		"tenant.user.create":          userController.Create,
		"tenant.user.update":          userController.Update,
		"tenant.user.delete":          userController.Delete,
		"tenant.user.password":        userController.ResetPassword,
		"tenant.user.roles":           userController.Roles,
		"tenant.user.set-roles":       userController.SetRoles,
		"tenant.role.list":            roleController.List,
		"tenant.role.create":          roleController.Create,
		"tenant.role.update":          roleController.Update,
		"tenant.role.delete":          roleController.Delete,
		"tenant.role.permissions":     roleController.Permissions,
		"tenant.role.set-permissions": roleController.SetPermissions,
		"tenant.menu.list":            menuController.List,
		"tenant.menu.create":          menuController.Create,
		"tenant.menu.update":          menuController.Update,
		"tenant.menu.delete":          menuController.Delete,
		"tenant.department.list":      departmentController.List,
		"tenant.department.create":    departmentController.Create,
		"tenant.department.update":    departmentController.Update,
		"tenant.department.delete":    departmentController.Delete,
		"tenant.position.list":        positionController.List,
		"tenant.position.create":      positionController.Create,
		"tenant.position.update":      positionController.Update,
		"tenant.position.data-scope":  positionController.SetDataPermission,
		"tenant.position.delete":      positionController.Delete,
		"tenant.leader.list":          leaderController.List,
		"tenant.leader.create":        leaderController.Create,
		"tenant.leader.update":        leaderController.Update,
		"tenant.leader.delete":        leaderController.Delete,
	})
}

func tenantRBACRoutes() []modules.Route {
	return []modules.Route{
		{Name: "tenant.permission.menus", Method: "GET", Path: "/admin/permission/menus", Permission: "permission:menu:index", Middlewares: []string{"tenant"}},
		{Name: "tenant.permission.roles", Method: "GET", Path: "/admin/permission/roles", Permission: "permission:role:index", Middlewares: []string{"tenant"}},
		{Name: "tenant.permission.update", Method: "POST", Path: "/admin/permission/update", Permission: "permission:user:update", Middlewares: []string{"tenant-audit-only"}},
		{Name: "tenant.user-profile.update", Method: "PUT", Path: "/admin/user/info", Permission: "permission:user:update", Middlewares: []string{"tenant-audit-only"}},
		{Name: "tenant.user.list", Method: "GET", Path: "/admin/user/list", Permission: "permission:user:index", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.user.create", Method: "POST", Path: "/admin/user", Permission: "permission:user:save", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.user.update", Method: "PUT", Path: "/admin/user/{id}", Permission: "permission:user:update", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.user.delete", Method: "DELETE", Path: "/admin/user", Permission: "permission:user:delete", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.user.password", Method: "PUT", Path: "/admin/user/password", Permission: "permission:user:password", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.user.roles", Method: "GET", Path: "/admin/user/{id}/roles", Permission: "permission:user:getRole", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.user.set-roles", Method: "PUT", Path: "/admin/user/{id}/roles", Permission: "permission:user:setRole", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.role.list", Method: "GET", Path: "/admin/role/list", Permission: "permission:role:index", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.role.create", Method: "POST", Path: "/admin/role", Permission: "permission:role:save", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.role.update", Method: "PUT", Path: "/admin/role/{id}", Permission: "permission:role:update", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.role.delete", Method: "DELETE", Path: "/admin/role", Permission: "permission:role:delete", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.role.permissions", Method: "GET", Path: "/admin/role/{id}/permissions", Permission: "permission:role:getMenu", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.role.set-permissions", Method: "PUT", Path: "/admin/role/{id}/permissions", Permission: "permission:role:setMenu", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.menu.list", Method: "GET", Path: "/admin/menu/list", Permission: "permission:menu:index", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.menu.create", Method: "POST", Path: "/admin/menu", Permission: "permission:menu:create", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.menu.update", Method: "PUT", Path: "/admin/menu/{id}", Permission: "permission:menu:save", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.menu.delete", Method: "DELETE", Path: "/admin/menu", Permission: "permission:menu:delete", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.department.list", Method: "GET", Path: "/admin/department/list", Permission: "permission:department:index", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.department.create", Method: "POST", Path: "/admin/department", Permission: "permission:department:save", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.department.update", Method: "PUT", Path: "/admin/department/{id}", Permission: "permission:department:update", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.department.delete", Method: "DELETE", Path: "/admin/department", Permission: "permission:department:delete", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.position.list", Method: "GET", Path: "/admin/position/list", Permission: "permission:position:index", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.position.create", Method: "POST", Path: "/admin/position", Permission: "permission:position:save", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.position.update", Method: "PUT", Path: "/admin/position/{id}", Permission: "permission:position:update", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.position.data-scope", Method: "PUT", Path: "/admin/position/{id}/data_permission", Permission: "permission:position:data_permission", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.position.delete", Method: "DELETE", Path: "/admin/position", Permission: "permission:position:delete", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.leader.list", Method: "GET", Path: "/admin/leader/list", Permission: "permission:leader:index", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.leader.create", Method: "POST", Path: "/admin/leader", Permission: "permission:leader:save", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.leader.update", Method: "PUT", Path: "/admin/leader/{id}", Permission: "permission:leader:save", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.leader.delete", Method: "DELETE", Path: "/admin/leader", Permission: "permission:leader:delete", Middlewares: []string{"tenant-rbac-audit"}},
	}
}

func (m Module) Menus() []modules.Menu {
	return []modules.Menu{
		{Key: "permission:user", ParentKey: "permission", Title: "用户管理", Path: "/permission/user", Component: "base/views/permission/user/index", Permission: "permission:user:index", Type: "M", I18n: "baseMenu.permission.user", Sort: 10},
		{Key: "permission:menu", ParentKey: "permission", Title: "菜单管理", Path: "/permission/menu", Component: "base/views/permission/menu/index", Permission: "permission:menu:index", Type: "M", I18n: "baseMenu.permission.menu", Sort: 20},
		{Key: "permission:role", ParentKey: "permission", Title: "角色管理", Path: "/permission/role", Component: "base/views/permission/role/index", Permission: "permission:role:index", Type: "M", I18n: "baseMenu.permission.role", Sort: 30},
		{Key: "permission:department", ParentKey: "permission", Title: "部门管理", Path: "/permission/department", Component: "base/views/permission/department/index", Permission: "permission:department:index", Type: "M", I18n: "baseMenu.permission.department", Sort: 40},
		{Key: "permission:position", ParentKey: "permission", Title: "岗位管理", Path: "/permission/position", Component: "base/views/permission/department/index", Permission: "permission:position:index", Type: "M", I18n: "baseMenu.permission.positionList", Sort: 50},
		{Key: "permission:leader", ParentKey: "permission", Title: "领导管理", Path: "/permission/leader", Component: "base/views/permission/department/index", Permission: "permission:leader:index", Type: "M", I18n: "baseMenu.permission.leaderList", Sort: 60},
	}
}

func (m Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "permission:user:index", Description: "用户列表"},
		{Key: "permission:user:save", Description: "用户保存"},
		{Key: "permission:user:update", Description: "用户更新"},
		{Key: "permission:user:delete", Description: "用户删除"},
		{Key: "permission:user:password", Description: "用户初始化密码"},
		{Key: "permission:user:getRole", Description: "获取用户角色"},
		{Key: "permission:user:setRole", Description: "用户角色赋予"},
		{Key: "permission:menu:index", Description: "菜单列表"},
		{Key: "permission:menu:create", Description: "菜单保存"},
		{Key: "permission:menu:save", Description: "菜单更新"},
		{Key: "permission:menu:delete", Description: "菜单删除"},
		{Key: "permission:role:index", Description: "角色列表"},
		{Key: "permission:role:save", Description: "角色保存"},
		{Key: "permission:role:update", Description: "角色更新"},
		{Key: "permission:role:delete", Description: "角色删除"},
		{Key: "permission:role:getMenu", Description: "获取角色菜单"},
		{Key: "permission:role:setMenu", Description: "角色菜单赋予"},
		{Key: "permission:department:index", Description: "部门列表"},
		{Key: "permission:department:save", Description: "部门保存"},
		{Key: "permission:department:update", Description: "部门更新"},
		{Key: "permission:department:delete", Description: "部门删除"},
		{Key: "permission:position:index", Description: "岗位列表"},
		{Key: "permission:position:save", Description: "岗位保存"},
		{Key: "permission:position:update", Description: "岗位更新"},
		{Key: "permission:position:delete", Description: "岗位删除"},
		{Key: "permission:position:data_permission", Description: "岗位数据权限"},
		{Key: "permission:leader:index", Description: "领导列表"},
		{Key: "permission:leader:save", Description: "领导保存"},
		{Key: "permission:leader:delete", Description: "领导删除"},
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
		"tests/feature/admin/user_role_menu_test.go",
		"tests/feature/admin/org_test.go",
		"tests/unit/data_permission_test.go",
	}
}
