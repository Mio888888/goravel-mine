package platformobservability

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
	return "platform-observability"
}

func (m Module) Metadata() modules.Metadata {
	return modules.BuiltinMetadata("Platform Observability", modules.RequiredDependency("platform-rbac"))
}

func (m Module) Package() modules.Package {
	return modules.BuiltinPackage(m.ID(), "platform-team")
}

func (m Module) Routes() []modules.Route {
	controller := admin.NewObservabilityController()
	lifecycleController := admin.NewModuleLifecycleController()

	return modules.BindRouteHandlers(m.ID(), platformObservabilityRoutes(), modules.RouteHandlers{
		"platform.observability.slow-requests": controller.SlowRequests,
		"platform.module-lifecycle.state":      lifecycleController.State,
		"platform.module-lifecycle.runs":       lifecycleController.Runs,
		"platform.module-lifecycle.steps":      lifecycleController.Steps,
		"platform.module-lifecycle.locks":      lifecycleController.Locks,
		"platform.module-lifecycle.diff":       lifecycleController.StateDiff,
		"platform.module-lifecycle.release":    lifecycleController.ReleaseStaleLocks,
		"platform.module-lifecycle.execute":    lifecycleController.Execute,
	})
}

func platformObservabilityRoutes() []modules.Route {
	return []modules.Route{
		{Name: "platform.observability.slow-requests", Method: "GET", Path: "/admin/platform/observability/slow-requests", Permission: "platform:observability:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.module-lifecycle.state", Method: "GET", Path: "/admin/platform/module-lifecycle/state", Permission: "platform:moduleLifecycle:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.module-lifecycle.runs", Method: "GET", Path: "/admin/platform/module-lifecycle/runs", Permission: "platform:moduleLifecycle:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.module-lifecycle.steps", Method: "GET", Path: "/admin/platform/module-lifecycle/steps", Permission: "platform:moduleLifecycle:log", Middlewares: []string{"platform-admin"}},
		{Name: "platform.module-lifecycle.locks", Method: "GET", Path: "/admin/platform/module-lifecycle/locks", Permission: "platform:moduleLifecycle:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.module-lifecycle.diff", Method: "GET", Path: "/admin/platform/module-lifecycle/diff", Permission: "platform:moduleLifecycle:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.module-lifecycle.release", Method: "POST", Path: "/admin/platform/module-lifecycle/locks/release-stale", Permission: "platform:moduleLifecycle:execute", Middlewares: []string{"platform-admin"}},
		{Name: "platform.module-lifecycle.execute", Method: "POST", Path: "/admin/platform/module-lifecycle/execute", Permission: "platform:moduleLifecycle:execute", Middlewares: []string{"platform-admin"}},
	}
}

func (m Module) Menus() []modules.Menu {
	return []modules.Menu{
		{Key: "platform:observability", ParentKey: "dashboard", Title: "系统监控", Path: "/dashboard/observability", Component: "base/views/platform/observability/index", Permission: "platform:observability:list", Type: "M", I18n: "baseMenu.platform.observability", Sort: 40},
		{Key: "platform:moduleLifecycle", ParentKey: "platform:system", Title: "模块治理", Path: "/platform-system/module-lifecycle", Component: "base/views/platform/moduleLifecycle/index", Permission: "platform:moduleLifecycle:list", Type: "M", I18n: "baseMenu.platform.moduleLifecycle", Sort: 40},
	}
}

func (m Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "platform:observability:list", Description: "监控面板"},
		{Key: "platform:moduleLifecycle:list", Description: "模块治理面板"},
		{Key: "platform:moduleLifecycle:log", Description: "模块生命周期日志"},
		{Key: "platform:moduleLifecycle:execute", Description: "模块生命周期执行"},
	}
}

func (m Module) Migrations() []schema.Migration {
	return nil
}

func (m Module) Seeders() []seeder.Seeder {
	return nil
}

func (m Module) OpenAPIFiles() []string {
	return []string{
		"docs/api-contract/openapi/admin-base-apis.openapi.json",
		"docs/api-contract/openapi/module-governance.openapi.json",
	}
}

func (m Module) TestTemplates() []string {
	return []string{
		"tests/unit/observability_test.go",
		"tests/feature/observability_test.go",
	}
}
