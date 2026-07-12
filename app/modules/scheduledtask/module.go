package scheduledtask

import (
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/controllers/admin"
	"goravel/app/modules"
	"goravel/database/migrations"
	"goravel/database/seeders"
)

type handlerFunc = contractshttp.HandlerFunc

type Module struct{}

func New() Module {
	return Module{}
}

func (m Module) ID() string {
	return "scheduled-task"
}

func (m Module) Metadata() modules.Metadata {
	return modules.BuiltinMetadata("Scheduled Task", modules.RequiredDependency("platform-tenant"))
}

func (m Module) Package() modules.Package {
	return modules.BuiltinPackage(m.ID(), "platform-team")
}

func (m Module) Routes() []modules.Route {
	controller := admin.NewScheduledTaskController()
	return buildRoutesWithHandlers(map[string]handlerFunc{
		"platform.scheduled-task.list":           controller.List,
		"platform.scheduled-task.tenant-options": controller.TenantOptions,
		"platform.scheduled-task.detail":         controller.Detail,
		"platform.scheduled-task.create":         controller.Create,
		"platform.scheduled-task.update":         controller.Update,
		"platform.scheduled-task.delete":         controller.Delete,
		"platform.scheduled-task.enable":         controller.Enable,
		"platform.scheduled-task.disable":        controller.Disable,
		"platform.scheduled-task.run":            controller.Run,
		"platform.scheduled-task.logs":           controller.Logs,
	})
}

func buildRoutesWithHandlers(handlers map[string]handlerFunc) []modules.Route {
	routes := scheduledTaskRoutes()
	for index, route := range routes {
		handler, ok := handlers[route.Name]
		if !ok {
			panic("scheduled-task route handler missing: " + route.Name)
		}
		routes[index].Install = modules.InstallPlatformRoute(route.Method, route.Path, handler)
	}

	return routes
}

func scheduledTaskRoutes() []modules.Route {
	return []modules.Route{
		{Name: "platform.scheduled-task.list", Method: "GET", Path: "/admin/platform/scheduled-task/list", Permission: "platform:scheduledTask:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.scheduled-task.tenant-options", Method: "GET", Path: "/admin/platform/scheduled-task/tenant-options", Permission: "platform:scheduledTask:list", Permissions: []string{"platform:scheduledTask:list", "platform:scheduledTask:save", "platform:scheduledTask:update"}, Middlewares: []string{"platform-admin"}},
		{Name: "platform.scheduled-task.detail", Method: "GET", Path: "/admin/platform/scheduled-task/{id}", Permission: "platform:scheduledTask:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.scheduled-task.create", Method: "POST", Path: "/admin/platform/scheduled-task", Permission: "platform:scheduledTask:save", Middlewares: []string{"platform-admin"}},
		{Name: "platform.scheduled-task.update", Method: "PUT", Path: "/admin/platform/scheduled-task/{id}", Permission: "platform:scheduledTask:update", Middlewares: []string{"platform-admin"}},
		{Name: "platform.scheduled-task.delete", Method: "DELETE", Path: "/admin/platform/scheduled-task", Permission: "platform:scheduledTask:delete", Middlewares: []string{"platform-admin"}},
		{Name: "platform.scheduled-task.enable", Method: "PUT", Path: "/admin/platform/scheduled-task/{id}/enable", Permission: "platform:scheduledTask:update", Middlewares: []string{"platform-admin"}},
		{Name: "platform.scheduled-task.disable", Method: "PUT", Path: "/admin/platform/scheduled-task/{id}/disable", Permission: "platform:scheduledTask:update", Middlewares: []string{"platform-admin"}},
		{Name: "platform.scheduled-task.run", Method: "POST", Path: "/admin/platform/scheduled-task/{id}/run", Permission: "platform:scheduledTask:run", Middlewares: []string{"platform-admin"}},
		{Name: "platform.scheduled-task.logs", Method: "GET", Path: "/admin/platform/scheduled-task-log/list", Permission: "platform:scheduledTask:log", Middlewares: []string{"platform-admin"}},
	}
}

func (m Module) Menus() []modules.Menu {
	return []modules.Menu{
		{
			Key:        "platform:scheduledTask",
			ParentKey:  "platform:system",
			Title:      "计划任务",
			Path:       "/platform-system/scheduled-task",
			Component:  "base/views/platform/scheduledTask/index",
			Permission: "platform:scheduledTask:list",
			Type:       "M",
			I18n:       "baseMenu.platform.scheduledTask",
			Sort:       30,
		},
	}
}

func (m Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "platform:scheduledTask:list", Description: "计划任务列表"},
		{Key: "platform:scheduledTask:save", Description: "计划任务保存"},
		{Key: "platform:scheduledTask:update", Description: "计划任务更新"},
		{Key: "platform:scheduledTask:delete", Description: "计划任务删除"},
		{Key: "platform:scheduledTask:run", Description: "计划任务执行"},
		{Key: "platform:scheduledTask:log", Description: "计划任务日志"},
	}
}

func (m Module) Migrations() []schema.Migration {
	return []schema.Migration{
		&migrations.M202607040001CreateScheduledTaskTables{},
		&migrations.M202607110010UpsertTenantGovernanceTasks{},
	}
}

func (m Module) Seeders() []seeder.Seeder {
	return []seeder.Seeder{&seeders.ScheduledTaskSeeder{}}
}

func (m Module) OpenAPIFiles() []string {
	return []string{"docs/api-contract/openapi/admin-base-apis.openapi.json"}
}

func (m Module) TestTemplates() []string {
	return []string{
		"tests/feature/admin/scheduled_task_test.go",
		"tests/unit/scheduled_task_cron_test.go",
		"tests/unit/scheduled_task_runner_test.go",
	}
}
