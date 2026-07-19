package middlewareplatform

import (
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"

	"goravel/app/http/controllers/admin"
	"goravel/app/modules"
	"goravel/database/migrations"
)

type Module struct{}

func New() Module {
	return Module{}
}

func (m Module) ID() string {
	return "middleware-platform"
}

func (m Module) Metadata() modules.Metadata {
	return modules.BuiltinMetadata(
		"Middleware Platform",
		modules.RequiredDependency("platform-rbac"),
	)
}

func (m Module) Package() modules.Package {
	return modules.BuiltinPackage(m.ID(), "platform-team")
}

func (m Module) Routes() []modules.Route {
	controller := admin.NewMiddlewarePlatformController()
	return modules.BindRouteHandlers(m.ID(), middlewarePlatformRoutes(), modules.RouteHandlers{
		"platform.middleware.registry":                 controller.Registry,
		"platform.middleware.adapters":                 controller.Adapters,
		"platform.middleware.adapter-detail":           controller.AdapterDetail,
		"platform.middleware.adapter-create":           controller.CreateAdapter,
		"platform.middleware.adapter-update":           controller.UpdateAdapter,
		"platform.middleware.adapter-health":           controller.CheckAdapterHealth,
		"platform.middleware.adapter-test":             controller.TestAdapterConnection,
		"platform.middleware.adapter-enable":           controller.EnableAdapter,
		"platform.middleware.adapter-disable":          controller.DisableAdapter,
		"platform.middleware.adapter-config":           controller.ReplaceAdapterConfig,
		"platform.middleware.routes":                   controller.Routes,
		"platform.middleware.route-detail":             controller.RouteDetail,
		"platform.middleware.route-create":             controller.CreateRoute,
		"platform.middleware.route-update":             controller.UpdateRoute,
		"platform.middleware.route-validate":           controller.ValidateRoute,
		"platform.middleware.route-publish":            controller.PublishRoute,
		"platform.middleware.route-enable":             controller.EnableRoute,
		"platform.middleware.route-disable":            controller.DisableRoute,
		"platform.middleware.deliveries":               controller.Deliveries,
		"platform.middleware.dead-letters":             controller.DeadLetters,
		"platform.middleware.dead-letter-detail":       controller.DeadLetterDetail,
		"platform.middleware.dead-letter-replay":       controller.ReplayDeadLetter,
		"platform.middleware.dead-letter-resolve":      controller.ResolveDeadLetter,
		"platform.middleware.protection-rules":         controller.ProtectionRules,
		"platform.middleware.protection-rule-detail":   controller.ProtectionRuleDetail,
		"platform.middleware.protection-rule-create":   controller.CreateProtectionRule,
		"platform.middleware.protection-rule-update":   controller.UpdateProtectionRule,
		"platform.middleware.protection-rule-delete":   controller.DeleteProtectionRule,
		"platform.middleware.protection-rule-validate": controller.ValidateProtectionRule,
		"platform.middleware.protection-rule-publish":  controller.PublishProtectionRule,
		"platform.middleware.protection-rule-enable":   controller.EnableProtectionRule,
		"platform.middleware.protection-rule-disable":  controller.DisableProtectionRule,
		"platform.middleware.protection-rule-versions": controller.ProtectionRuleVersions,
		"platform.middleware.protection-rule-rollback": controller.RollbackProtectionRule,
		"platform.middleware.protection-rule-state":    controller.ProtectionRuleState,
		"platform.middleware.protection-metrics":       controller.ProtectionMetrics,
		"platform.middleware.metrics":                  controller.Metrics,
	})
}

func middlewarePlatformRoutes() []modules.Route {
	return []modules.Route{
		{Name: "platform.middleware.registry", Method: "GET", Path: "/admin/platform/middleware/registry", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.adapters", Method: "GET", Path: "/admin/platform/middleware/adapters", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.adapter-detail", Method: "GET", Path: "/admin/platform/middleware/adapters/{id}", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.adapter-create", Method: "POST", Path: "/admin/platform/middleware/adapters", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.adapter-update", Method: "PUT", Path: "/admin/platform/middleware/adapters/{id}", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.adapter-health", Method: "POST", Path: "/admin/platform/middleware/adapters/{id}/health", Permission: "platform:middleware:execute", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.adapter-test", Method: "POST", Path: "/admin/platform/middleware/adapters/{id}/test", Permission: "platform:middleware:execute", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.adapter-enable", Method: "PUT", Path: "/admin/platform/middleware/adapters/{id}/enable", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.adapter-disable", Method: "PUT", Path: "/admin/platform/middleware/adapters/{id}/disable", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.adapter-config", Method: "PUT", Path: "/admin/platform/middleware/adapters/{id}/config", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.routes", Method: "GET", Path: "/admin/platform/middleware/routes", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.route-detail", Method: "GET", Path: "/admin/platform/middleware/routes/{id}", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.route-create", Method: "POST", Path: "/admin/platform/middleware/routes", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.route-update", Method: "PUT", Path: "/admin/platform/middleware/routes/{id}", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.route-validate", Method: "POST", Path: "/admin/platform/middleware/routes/{id}/validate", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.route-publish", Method: "POST", Path: "/admin/platform/middleware/routes/{id}/publish", Permission: "platform:middleware:publish", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.route-enable", Method: "PUT", Path: "/admin/platform/middleware/routes/{id}/enable", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.route-disable", Method: "PUT", Path: "/admin/platform/middleware/routes/{id}/disable", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.deliveries", Method: "GET", Path: "/admin/platform/middleware/deliveries", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.dead-letters", Method: "GET", Path: "/admin/platform/middleware/dead-letters", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.dead-letter-detail", Method: "GET", Path: "/admin/platform/middleware/dead-letters/{id}", Permission: "platform:middleware:payload", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.dead-letter-replay", Method: "POST", Path: "/admin/platform/middleware/dead-letters/{id}/replay", Permission: "platform:middleware:replay", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.dead-letter-resolve", Method: "PUT", Path: "/admin/platform/middleware/dead-letters/{id}/resolve", Permission: "platform:middleware:replay", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rules", Method: "GET", Path: "/admin/platform/middleware/protection-rules", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-detail", Method: "GET", Path: "/admin/platform/middleware/protection-rules/{id}", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-create", Method: "POST", Path: "/admin/platform/middleware/protection-rules", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-update", Method: "PUT", Path: "/admin/platform/middleware/protection-rules/{id}", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-delete", Method: "DELETE", Path: "/admin/platform/middleware/protection-rules/{id}", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-validate", Method: "POST", Path: "/admin/platform/middleware/protection-rules/{id}/validate", Permission: "platform:middleware:configure", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-publish", Method: "POST", Path: "/admin/platform/middleware/protection-rules/{id}/publish", Permission: "platform:middleware:publish", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-enable", Method: "PUT", Path: "/admin/platform/middleware/protection-rules/{id}/enable", Permission: "platform:middleware:publish", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-disable", Method: "PUT", Path: "/admin/platform/middleware/protection-rules/{id}/disable", Permission: "platform:middleware:publish", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-versions", Method: "GET", Path: "/admin/platform/middleware/protection-rules/{id}/versions", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-rollback", Method: "POST", Path: "/admin/platform/middleware/protection-rules/{id}/rollback", Permission: "platform:middleware:publish", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-rule-state", Method: "GET", Path: "/admin/platform/middleware/protection-rules/{id}/state", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.protection-metrics", Method: "GET", Path: "/admin/platform/middleware/protection-metrics", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.middleware.metrics", Method: "GET", Path: "/admin/platform/middleware/metrics", Permission: "platform:middleware:list", Middlewares: []string{"platform-admin"}},
	}
}

func (m Module) Menus() []modules.Menu {
	return []modules.Menu{
		{
			Key:        "platform:middleware",
			ParentKey:  "platform:system",
			Title:      "中间件平台",
			Path:       "/platform-system/middleware",
			Component:  "base/views/platform/middleware/index",
			Permission: "platform:middleware:list",
			Type:       "M",
			I18n:       "baseMenu.platform.middleware",
			Sort:       35,
		},
	}
}

func (m Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "platform:middleware:list", Description: "中间件平台查看"},
		{Key: "platform:middleware:configure", Description: "中间件平台配置"},
		{Key: "platform:middleware:execute", Description: "中间件平台执行"},
		{Key: "platform:middleware:publish", Description: "消息路由发布"},
		{Key: "platform:middleware:replay", Description: "消息死信重放"},
		{Key: "platform:middleware:payload", Description: "消息载荷查看"},
	}
}

func (m Module) Migrations() []schema.Migration {
	return []schema.Migration{
		&migrations.M202607190002CreateMiddlewarePlatformTables{},
	}
}

func (m Module) Seeders() []seeder.Seeder {
	return nil
}

func (m Module) OpenAPIFiles() []string {
	return []string{"docs/api-contract/openapi/middleware-platform.openapi.json"}
}

func (m Module) TestTemplates() []string {
	return []string{
		"tests/backend/feature/admin/middleware_platform_test.go",
		"tests/backend/unit/message_registry_test.go",
	}
}
