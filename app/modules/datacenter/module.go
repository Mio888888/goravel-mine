package datacenter

import (
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/controllers/admin"
	"goravel/app/modules"
)

type Module struct{}

func New() Module {
	return Module{}
}

func (m Module) ID() string {
	return "data-center"
}

func (m Module) Metadata() modules.Metadata {
	return modules.BuiltinMetadata(
		"Data Center",
		modules.RequiredDependency("platform-rbac"),
		modules.RequiredDependency("tenant-rbac"),
	)
}

func (m Module) Package() modules.Package {
	return modules.BuiltinPackage(m.ID(), "platform-team")
}

func (m Module) Routes() []modules.Route {
	attachmentController := admin.NewAttachmentController()
	platformDictionaryController := admin.NewPlatformDictionaryController()
	storageConfigController := admin.NewStorageConfigController()
	dictionaryController := admin.NewTenantDictionaryController()
	userLoginLogController := admin.NewUserLoginLogController()
	userOperationLogController := admin.NewUserOperationLogController()

	return modules.BindRouteHandlers(m.ID(), dataCenterRoutes(), modules.RouteHandlers{
		"platform.attachment.upload":       attachmentController.PlatformUpload,
		"platform.dictionary.options":      platformDictionaryController.Options,
		"platform.dictionary.list":         platformDictionaryController.List,
		"platform.dictionary.detail":       platformDictionaryController.Detail,
		"platform.dictionary.create":       platformDictionaryController.Create,
		"platform.dictionary.update":       platformDictionaryController.Update,
		"platform.dictionary.delete":       platformDictionaryController.Delete,
		"platform.dictionary.dispatch-all": platformDictionaryController.DispatchAll,
		"platform.dictionary.dispatch":     platformDictionaryController.DispatchTenant,
		"platform.storage-config.list":     storageConfigController.List,
		"platform.storage-config.create":   storageConfigController.Create,
		"platform.storage-config.update":   storageConfigController.Update,
		"platform.storage-config.delete":   storageConfigController.Delete,
		"platform.queue-failed.list":       lazyQueueFailedJobList,
		"platform.queue-failed.retry":      lazyQueueFailedJobRetry,
		"platform.queue-failed.delete":     lazyQueueFailedJobDelete,
		"tenant.dictionary.options":        dictionaryController.Options,
		"tenant.dictionary.list":           dictionaryController.List,
		"tenant.dictionary.items":          dictionaryController.Items,
		"tenant.dictionary.update-type":    dictionaryController.UpdateType,
		"tenant.dictionary.update-item":    dictionaryController.UpdateItem,
		"tenant.attachment.list":           attachmentController.List,
		"tenant.attachment.upload":         attachmentController.Upload,
		"tenant.attachment.delete":         attachmentController.Delete,
		"tenant.user-login-log.list":       userLoginLogController.List,
		"tenant.user-operation-log.list":   userOperationLogController.List,
	})
}

func lazyQueueFailedJobList(ctx contractshttp.Context) contractshttp.Response {
	return admin.NewQueueFailedJobController().List(ctx)
}

func lazyQueueFailedJobRetry(ctx contractshttp.Context) contractshttp.Response {
	return admin.NewQueueFailedJobController().Retry(ctx)
}

func lazyQueueFailedJobDelete(ctx contractshttp.Context) contractshttp.Response {
	return admin.NewQueueFailedJobController().Delete(ctx)
}

func dataCenterRoutes() []modules.Route {
	return []modules.Route{
		{Name: "platform.attachment.upload", Method: "POST", Path: "/admin/platform/attachment/upload", Permission: "platform:attachment:upload", Permissions: []string{"platform:attachment:upload", "platform:user:save", "platform:user:update"}, Middlewares: []string{"platform-admin"}},
		{Name: "platform.dictionary.options", Method: "GET", Path: "/admin/platform/dictionary/options", Permission: "platform:dictionary:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.dictionary.list", Method: "GET", Path: "/admin/platform/dictionary/list", Permission: "platform:dictionary:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.dictionary.detail", Method: "GET", Path: "/admin/platform/dictionary/{id}", Permission: "platform:dictionary:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.dictionary.create", Method: "POST", Path: "/admin/platform/dictionary", Permission: "platform:dictionary:save", Middlewares: []string{"platform-admin"}},
		{Name: "platform.dictionary.update", Method: "PUT", Path: "/admin/platform/dictionary/{id}", Permission: "platform:dictionary:update", Middlewares: []string{"platform-admin"}},
		{Name: "platform.dictionary.delete", Method: "DELETE", Path: "/admin/platform/dictionary", Permission: "platform:dictionary:delete", Middlewares: []string{"platform-admin"}},
		{Name: "platform.dictionary.dispatch-all", Method: "POST", Path: "/admin/platform/dictionary/dispatch", Permission: "platform:dictionary:dispatch", Middlewares: []string{"platform-admin"}},
		{Name: "platform.dictionary.dispatch", Method: "POST", Path: "/admin/platform/dictionary/dispatch/{id}", Permission: "platform:dictionary:dispatch", Middlewares: []string{"platform-admin"}},
		{Name: "platform.storage-config.list", Method: "GET", Path: "/admin/platform/storage-config/list", Permission: "platform:storageConfig:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.storage-config.create", Method: "POST", Path: "/admin/platform/storage-config", Permission: "platform:storageConfig:save", Middlewares: []string{"platform-admin"}},
		{Name: "platform.storage-config.update", Method: "PUT", Path: "/admin/platform/storage-config/{id}", Permission: "platform:storageConfig:update", Middlewares: []string{"platform-admin"}},
		{Name: "platform.storage-config.delete", Method: "DELETE", Path: "/admin/platform/storage-config", Permission: "platform:storageConfig:delete", Middlewares: []string{"platform-admin"}},
		{Name: "platform.queue-failed.list", Method: "GET", Path: "/admin/platform/queue/failed-jobs", Permission: "platform:queueFailedJob:list", Middlewares: []string{"platform-admin"}},
		{Name: "platform.queue-failed.retry", Method: "POST", Path: "/admin/platform/queue/failed-jobs/retry", Permission: "platform:queueFailedJob:retry", Middlewares: []string{"platform-admin"}},
		{Name: "platform.queue-failed.delete", Method: "DELETE", Path: "/admin/platform/queue/failed-jobs", Permission: "platform:queueFailedJob:delete", Middlewares: []string{"platform-admin"}},
		{Name: "tenant.dictionary.options", Method: "GET", Path: "/admin/dictionary/options", Permission: "dataCenter:dictionary:list", Middlewares: []string{"tenant"}},
		{Name: "tenant.dictionary.list", Method: "GET", Path: "/admin/dictionary/list", Permission: "dataCenter:dictionary:list", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.dictionary.items", Method: "GET", Path: "/admin/dictionary/{id}/items", Permission: "dataCenter:dictionary:list", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.dictionary.update-type", Method: "PUT", Path: "/admin/dictionary/{id}", Permission: "dataCenter:dictionary:update", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.dictionary.update-item", Method: "PUT", Path: "/admin/dictionary-item/{id}", Permission: "dataCenter:dictionary:update", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.attachment.list", Method: "GET", Path: "/admin/attachment/list", Permission: "dataCenter:attachment:list", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.attachment.upload", Method: "POST", Path: "/admin/attachment/upload", Permission: "dataCenter:attachment:upload", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.attachment.delete", Method: "DELETE", Path: "/admin/attachment/{id}", Permission: "dataCenter:attachment:delete", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.user-login-log.list", Method: "GET", Path: "/admin/user-login-log/list", Permission: "log:userLogin:list", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.user-operation-log.list", Method: "GET", Path: "/admin/user-operation-log/list", Permission: "log:userOperation:list", Middlewares: []string{"tenant-rbac"}},
	}
}

func (m Module) Menus() []modules.Menu {
	return []modules.Menu{
		{Key: "platform:dictionary", ParentKey: "platform:system", Title: "字典管理", Path: "/platform-system/dictionary", Component: "base/views/platform/dictionary/index", Permission: "platform:dictionary:list", Type: "M", I18n: "baseMenu.platform.dictionary", Sort: 10},
		{Key: "platform:storageConfig", ParentKey: "platform:system", Title: "储存配置", Path: "/platform-system/storage-config", Component: "base/views/platform/storageConfig/index", Permission: "platform:storageConfig:list", Type: "M", I18n: "baseMenu.platform.storageConfig", Sort: 20},
		{Key: "dataCenter:dictionary", ParentKey: "dataCenter", Title: "数据字典", Path: "/data-center/dictionary", Component: "base/views/dataCenter/dictionary/index", Permission: "dataCenter:dictionary:list", Type: "M", I18n: "baseMenu.dataCenter.dictionary", Sort: 20},
		{Key: "dataCenter:attachment", ParentKey: "dataCenter", Title: "附件管理", Path: "/data-center/attachment", Component: "base/views/dataCenter/attachment/index", Permission: "dataCenter:attachment:list", Type: "M", I18n: "baseMenu.dataCenter.attachment", Sort: 10},
		{Key: "log:userLogin", ParentKey: "log", Title: "登录日志", Path: "/log/user-login", Component: "base/views/log/userLogin", Permission: "log:userLogin:list", Type: "M", I18n: "baseMenu.log.userLoginLog", Sort: 10},
		{Key: "log:userOperation", ParentKey: "log", Title: "操作日志", Path: "/log/user-operation", Component: "base/views/log/userOperation", Permission: "log:userOperation:list", Type: "M", I18n: "baseMenu.log.operationLog", Sort: 20},
	}
}

func (m Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "platform:attachment:upload", Description: "平台附件上传"},
		{Key: "platform:dictionary:list", Description: "字典列表"},
		{Key: "platform:dictionary:save", Description: "字典保存"},
		{Key: "platform:dictionary:update", Description: "字典更新"},
		{Key: "platform:dictionary:delete", Description: "字典删除"},
		{Key: "platform:dictionary:dispatch", Description: "字典分发"},
		{Key: "platform:storageConfig:list", Description: "储存配置列表"},
		{Key: "platform:storageConfig:save", Description: "储存配置保存"},
		{Key: "platform:storageConfig:update", Description: "储存配置更新"},
		{Key: "platform:storageConfig:delete", Description: "储存配置删除"},
		{Key: "platform:queueFailedJob:list", Description: "失败队列列表"},
		{Key: "platform:queueFailedJob:retry", Description: "失败队列重试"},
		{Key: "platform:queueFailedJob:delete", Description: "失败队列丢弃"},
		{Key: "dataCenter:dictionary:list", Description: "数据字典列表"},
		{Key: "dataCenter:dictionary:update", Description: "数据字典更新"},
		{Key: "dataCenter:attachment:list", Description: "附件列表"},
		{Key: "dataCenter:attachment:upload", Description: "附件上传"},
		{Key: "dataCenter:attachment:delete", Description: "附件删除"},
		{Key: "log:userLogin:list", Description: "登录日志列表"},
		{Key: "log:userOperation:list", Description: "操作日志列表"},
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
		"tests/feature/admin/attachment_test.go",
		"tests/feature/admin/storage_config_test.go",
		"tests/feature/admin/log_test.go",
	}
}
