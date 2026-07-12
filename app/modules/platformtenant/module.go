package platformtenant

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
	return "platform-tenant"
}

func (m Module) Metadata() modules.Metadata {
	return modules.BuiltinMetadata("Platform Tenant", modules.RequiredDependency("platform-rbac"))
}

func (m Module) Package() modules.Package {
	return modules.BuiltinPackage(m.ID(), "platform-team")
}

func (m Module) Routes() []modules.Route {
	tenantController := admin.NewTenantAdminController()
	planController := admin.NewTenantPlanAdminController()

	return buildRoutesWithHandlers(map[string]handlerFunc{
		"platform.tenant-plan.list":       planController.List,
		"platform.tenant-plan.options":    planController.Options,
		"platform.tenant-plan.create":     planController.Create,
		"platform.tenant-plan.update":     planController.Update,
		"platform.tenant-plan.delete":     planController.Delete,
		"platform.tenant.list":            tenantController.List,
		"platform.tenant.permission-list": tenantController.PermissionCatalog,
		"platform.tenant.create":          tenantController.Create,
		"platform.tenant.update":          tenantController.Update,
		"platform.tenant.usage":           tenantController.Usage,
		"platform.tenant.governance":      tenantController.Governance,
		"platform.tenant.set-governance":  tenantController.UpdateGovernance,
		"platform.tenant.permissions":     tenantController.Permissions,
		"platform.tenant.set-permissions": tenantController.UpdatePermissions,
		"platform.tenant.plan-diff":       tenantController.PermissionPlanDiff,
		"platform.tenant.update-plan":     tenantController.UpdatePlan,
		"platform.tenant.suspend":         tenantController.Suspend,
		"platform.tenant.resume":          tenantController.Resume,
		"platform.tenant.archive":         tenantController.Archive,
		"platform.tenant.destroy":         tenantController.Destroy,
		"platform.tenant.export":          tenantController.RequestExport,
		"platform.tenant.export-status":   tenantController.ExportStatus,
		"platform.tenant.export-download": tenantController.DownloadExport,
	})
}

func buildRoutesWithHandlers(handlers map[string]handlerFunc) []modules.Route {
	routes := platformTenantRoutes()
	for index, route := range routes {
		handler, ok := handlers[route.Name]
		if !ok {
			panic("platform-tenant route handler missing: " + route.Name)
		}
		routes[index].Install = modules.InstallPlatformRoute(route.Method, route.Path, handler)
	}

	return routes
}

func platformTenantRoutes() []modules.Route {
	return []modules.Route{
		{Name: "platform.tenant-plan.list", Method: "GET", Path: "/admin/platform/tenant-plan/list", Permission: "platform:tenantPlan:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant-plan.options", Method: "GET", Path: "/admin/platform/tenant-plan/options", Permission: "platform:tenantPlan:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant-plan.create", Method: "POST", Path: "/admin/platform/tenant-plan", Permission: "platform:tenantPlan:save", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant-plan.update", Method: "PUT", Path: "/admin/platform/tenant-plan/{id}", Permission: "platform:tenantPlan:update", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant-plan.delete", Method: "DELETE", Path: "/admin/platform/tenant-plan", Permission: "platform:tenantPlan:delete", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.list", Method: "GET", Path: "/admin/platform/tenant/list", Permission: "platform:tenant:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.permission-list", Method: "GET", Path: "/admin/platform/tenant/permission-catalog", Permission: "platform:tenant:permissions", Permissions: []string{"platform:tenant:permissions", "platform:tenantPlan:save", "platform:tenantPlan:update"}, Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.create", Method: "POST", Path: "/admin/platform/tenant", Permission: "platform:tenant:save", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.update", Method: "PUT", Path: "/admin/platform/tenant/{id}", Permission: "platform:tenant:update", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.usage", Method: "GET", Path: "/admin/platform/tenant/{id}/usage", Permission: "platform:tenant:usage", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.governance", Method: "GET", Path: "/admin/platform/tenant/{id}/governance", Permission: "platform:tenant:governance", Permissions: []string{"platform:tenant:governance", "platform:tenant:destroy"}, Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.set-governance", Method: "PUT", Path: "/admin/platform/tenant/{id}/governance", Permission: "platform:tenant:governance", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.permissions", Method: "GET", Path: "/admin/platform/tenant/{id}/permissions", Permission: "platform:tenant:permissions", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.set-permissions", Method: "PUT", Path: "/admin/platform/tenant/{id}/permissions", Permission: "platform:tenant:permissions", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.plan-diff", Method: "POST", Path: "/admin/platform/tenant/{id}/permissions/plan-diff", Permission: "platform:tenant:updatePlan", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.update-plan", Method: "PUT", Path: "/admin/platform/tenant/{id}/plan", Permission: "platform:tenant:updatePlan", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.suspend", Method: "PUT", Path: "/admin/platform/tenant/{id}/suspend", Permission: "platform:tenant:suspend", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.resume", Method: "PUT", Path: "/admin/platform/tenant/{id}/resume", Permission: "platform:tenant:resume", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.archive", Method: "PUT", Path: "/admin/platform/tenant/{id}/archive", Permission: "platform:tenant:archive", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.destroy", Method: "DELETE", Path: "/admin/platform/tenant", Permission: "platform:tenant:destroy", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.export", Method: "POST", Path: "/admin/platform/tenant/{id}/exports", Permission: "platform:tenant:export", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.export-status", Method: "GET", Path: "/admin/platform/tenant/{id}/exports/{run_id}", Permission: "platform:tenant:export", Middlewares: []string{"platform-admin"}},
		{Name: "platform.tenant.export-download", Method: "GET", Path: "/admin/platform/tenant/{id}/exports/{run_id}/download", Permission: "platform:tenant:export", Middlewares: []string{"platform-admin"}},
	}
}

func (m Module) Menus() []modules.Menu {
	return []modules.Menu{
		{Key: "platform:tenant", ParentKey: "platform:tenantManage", Title: "租户管理", Path: "/tenant-manage/tenant", Component: "base/views/platform/tenant/index", Permission: "platform:tenant:list", Type: "M", I18n: "baseMenu.platform.tenant", Sort: 10},
		{Key: "platform:tenantPlan", ParentKey: "platform:tenantManage", Title: "套餐管理", Path: "/tenant-manage/tenant-plan", Component: "base/views/platform/tenantPlan/index", Permission: "platform:tenantPlan:list", Type: "M", I18n: "baseMenu.platform.tenantPlan", Sort: 20},
	}
}

func (m Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "platform:tenant:list", Description: "租户列表"},
		{Key: "platform:tenant:save", Description: "租户保存"},
		{Key: "platform:tenant:update", Description: "租户更新"},
		{Key: "platform:tenant:suspend", Description: "租户挂起"},
		{Key: "platform:tenant:resume", Description: "租户恢复"},
		{Key: "platform:tenant:archive", Description: "租户归档"},
		{Key: "platform:tenant:destroy", Description: "租户销毁"},
		{Key: "platform:tenant:usage", Description: "租户用量"},
		{Key: "platform:tenant:governance", Description: "租户治理"},
		{Key: "platform:tenant:export", Description: "租户数据导出"},
		{Key: "platform:tenant:permissions", Description: "租户权限分配"},
		{Key: "platform:tenant:updatePlan", Description: "租户套餐变更"},
		{Key: "platform:tenantPlan:list", Description: "套餐列表"},
		{Key: "platform:tenantPlan:save", Description: "套餐保存"},
		{Key: "platform:tenantPlan:update", Description: "套餐更新"},
		{Key: "platform:tenantPlan:delete", Description: "套餐删除"},
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
		"tests/feature/admin/tenant_platform_test.go",
		"tests/unit/tenant_service_test.go",
	}
}
