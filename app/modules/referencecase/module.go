package referencecase

import (
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"

	"goravel/app/http/controllers/admin"
	"goravel/app/modules"
	"goravel/database/migrations"
	"goravel/database/seeders"
)

type Module struct{}

func New() Module {
	return Module{}
}

func (m Module) ID() string {
	return "reference-case"
}

func (m Module) Metadata() modules.Metadata {
	metadata := modules.BuiltinMetadata("Golden Reference Case", modules.RequiredDependency("platform-rbac"))
	metadata.Lifecycle.Install = "go run . artisan migrate"
	metadata.Lifecycle.Upgrade = "go run . artisan reference-case:upgrade"
	metadata.Lifecycle.Rollback = "go run . artisan reference-case:rollback"
	metadata.Lifecycle.DestructiveCheck = "go run . artisan module:manifest:check"
	metadata.Lifecycle.BreakingChangePolicy = "approval required with CRUD/API/E2E evidence"
	metadata.SeedStrategy.Notes = "idempotently seeds golden-case baseline"
	return metadata
}

func (m Module) Package() modules.Package {
	return modules.BuiltinPackage(m.ID(), "platform-team")
}

func (m Module) Routes() []modules.Route {
	controller := admin.NewReferenceCaseController()
	return modules.BindRouteHandlers(m.ID(), referenceCaseRoutes(), modules.RouteHandlers{
		"platform.reference-case.list":   controller.List,
		"platform.reference-case.create": controller.Create,
		"platform.reference-case.update": controller.Update,
		"platform.reference-case.delete": controller.Delete,
	})
}

func referenceCaseRoutes() []modules.Route {
	return []modules.Route{
		{Name: "platform.reference-case.list", Method: "GET", Path: "/admin/platform/reference-case/list", Permission: "platform:referenceCase:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.reference-case.create", Method: "POST", Path: "/admin/platform/reference-case", Permission: "platform:referenceCase:save", Middlewares: []string{"platform-admin"}},
		{Name: "platform.reference-case.update", Method: "PUT", Path: "/admin/platform/reference-case/{id}", Permission: "platform:referenceCase:update", Middlewares: []string{"platform-admin"}},
		{Name: "platform.reference-case.delete", Method: "DELETE", Path: "/admin/platform/reference-case", Permission: "platform:referenceCase:delete", Middlewares: []string{"platform-admin"}},
	}
}

func (m Module) Menus() []modules.Menu {
	return []modules.Menu{
		{Key: "platform:referenceCase", ParentKey: "platform:system", Title: "参考模块", Path: "/platform-system/reference-case", Component: "base/views/platform/referenceCase/index", Permission: "platform:referenceCase:list", Type: "M", I18n: "baseMenu.platform.referenceCase", Sort: 50},
	}
}

func (m Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "platform:referenceCase:list", Description: "参考模块列表"},
		{Key: "platform:referenceCase:save", Description: "参考模块保存"},
		{Key: "platform:referenceCase:update", Description: "参考模块更新"},
		{Key: "platform:referenceCase:delete", Description: "参考模块删除"},
	}
}

func (m Module) Migrations() []schema.Migration {
	return []schema.Migration{
		&migrations.M202607090003CreateReferenceCaseTable{},
	}
}

func (m Module) Seeders() []seeder.Seeder {
	return []seeder.Seeder{&seeders.ReferenceCaseSeeder{}}
}

func (m Module) OpenAPIFiles() []string {
	return []string{"docs/api-contract/openapi/reference-case.openapi.json"}
}

func (m Module) TestTemplates() []string {
	return []string{
		"tests/feature/admin/reference_case_test.go",
		"MineAdmin-web/tests/e2e/reference-case.spec.ts",
	}
}
