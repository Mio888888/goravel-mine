package security

import (
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/controllers/admin"
	systemcontrollers "goravel/app/http/controllers/admin/system"
	"goravel/app/http/middleware"
	"goravel/app/modules"
)

type handlerFunc = contractshttp.HandlerFunc

type Module struct{}

func New() Module {
	return Module{}
}

func (m Module) ID() string {
	return "security"
}

func (m Module) Metadata() modules.Metadata {
	return modules.BuiltinMetadata(
		"Security",
		modules.RequiredDependency("platform-rbac"),
		modules.RequiredDependency("tenant-rbac"),
	)
}

func (m Module) Package() modules.Package {
	return modules.BuiltinPackage(m.ID(), "security-team")
}

func (m Module) Routes() []modules.Route {
	passportController := admin.NewPassportController()
	platformPassportController := admin.NewPlatformPassportController()
	securityController := admin.NewSecurityController()
	captchaController := systemcontrollers.NewCaptchaController()
	ssoProviderController := admin.NewSSOProviderController()
	ssoUserBindingController := admin.NewSSOUserBindingController()
	ssoLoginLogController := admin.NewSSOLoginLogController()

	return buildRoutesWithHandlers(map[string]handlerFunc{
		"tenant.passport.entry":              passportController.Entry,
		"tenant.passport.csrf-token":         securityController.CSRFToken,
		"tenant.passport.login":              passportController.Login,
		"tenant.passport.mfa-login":          passportController.MFALogin,
		"tenant.passport.password-change":    passportController.PasswordChange,
		"tenant.passport.sso-login":          passportController.SSOLogin,
		"tenant.passport.sso-authorize":      passportController.SSOAuthorize,
		"tenant.passport.sso-callback":       passportController.SSOCallback,
		"tenant.passport.branding":           passportController.Branding,
		"tenant.passport.info":               passportController.GetInfo,
		"tenant.passport.logout":             passportController.Logout,
		"tenant.passport.refresh":            passportController.Refresh,
		"tenant.passport.captcha":            captchaController.Show,
		"system.captcha":                     captchaController.Show,
		"platform.passport.csrf-token":       securityController.CSRFToken,
		"platform.passport.login":            platformPassportController.Login,
		"platform.passport.mfa-login":        platformPassportController.MFALogin,
		"platform.passport.password-change":  platformPassportController.PasswordChange,
		"platform.passport.info":             platformPassportController.GetInfo,
		"platform.passport.logout":           platformPassportController.Logout,
		"platform.passport.refresh":          platformPassportController.Refresh,
		"platform.security.mfa.setup":        securityController.PlatformMFASetup,
		"platform.security.mfa.confirm":      securityController.PlatformMFAConfirm,
		"platform.security.mfa.disable":      securityController.PlatformMFADisable,
		"platform.security.reauth-token":     securityController.PlatformReAuthToken,
		"platform.security.approval.create":  securityController.PlatformApprovalCreate,
		"platform.security.approval.detail":  securityController.PlatformApprovalDetail,
		"platform.security.approval.approve": securityController.PlatformApprovalApprove,
		"tenant.security.reauth-token":       securityController.TenantReAuthToken,
		"tenant.security.approval.create":    securityController.TenantApprovalCreate,
		"tenant.security.approval.detail":    securityController.TenantApprovalDetail,
		"tenant.security.approval.approve":   securityController.TenantApprovalApprove,
		"tenant.security.mfa.setup":          securityController.TenantMFASetup,
		"tenant.security.mfa.confirm":        securityController.TenantMFAConfirm,
		"tenant.security.mfa.disable":        securityController.TenantMFADisable,
		"tenant.sso-provider.list":           ssoProviderController.List,
		"tenant.sso-provider.create":         ssoProviderController.Create,
		"tenant.sso-provider.update":         ssoProviderController.Update,
		"tenant.sso-provider.delete":         ssoProviderController.Delete,
		"tenant.sso-user-binding.list":       ssoUserBindingController.List,
		"tenant.sso-user-binding.detail":     ssoUserBindingController.Detail,
		"tenant.sso-user-binding.user":       ssoUserBindingController.UserBindings,
		"tenant.sso-user-binding.unbind":     ssoUserBindingController.Unbind,
		"tenant.sso-login-log.list":          ssoLoginLogController.List,
		"tenant.sso-login-log.stats":         ssoLoginLogController.Stats,
	})
}

func buildRoutesWithHandlers(handlers map[string]handlerFunc) []modules.Route {
	routes := securityRoutes()
	for index, route := range routes {
		handler, ok := handlers[route.Name]
		if !ok {
			panic("security route handler missing: " + route.Name)
		}
		switch {
		case hasMiddleware(route, "public"):
			routes[index].Install = modules.InstallRoute(route.Method, route.Path, handler)
		case hasMiddleware(route, "tenant"):
			routes[index].Install = modules.InstallTenantOnlyRoute(route.Method, route.Path, handler)
		case hasMiddleware(route, "platform-self-audit"):
			routes[index].Install = modules.InstallRoute(route.Method, route.Path, handler, middleware.PlatformSelfAudit())
		case hasMiddleware(route, "platform-admin"):
			routes[index].Install = modules.InstallPlatformRoute(route.Method, route.Path, handler)
		case hasMiddleware(route, "tenant-audit-only"):
			routes[index].Install = modules.InstallTenantAuditOnlyRoute(route.Method, route.Path, handler)
		case hasMiddleware(route, "tenant-rbac-audit"):
			routes[index].Install = modules.InstallTenantAuditRoute(route.Method, route.Path, handler)
		case hasMiddleware(route, "tenant-rbac"):
			routes[index].Install = modules.InstallTenantRoute(route.Method, route.Path, handler)
		default:
			routes[index].Install = modules.InstallTenantOnlyRoute(route.Method, route.Path, handler)
		}
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

func securityRoutes() []modules.Route {
	platformSensitivePermissions := []string{
		"platform:security:control", "platform:security:mfa", "platform:moduleLifecycle:execute",
		"platform:tenant:destroy", "platform:tenant:export", "platform:tenant:permissions", "platform:tenant:updatePlan", "platform:tenant:governance",
		"platform:tenant:suspend", "platform:tenant:resume", "platform:tenant:archive",
		"platform:user:password", "platform:user:setRole", "platform:role:setMenu",
		"platform:storageConfig:save", "platform:storageConfig:update", "platform:storageConfig:delete",
	}
	return []modules.Route{
		{Name: "tenant.passport.entry", Method: "GET", Path: "/admin/passport/entry", Middlewares: []string{"public"}},
		{Name: "tenant.passport.csrf-token", Method: "GET", Path: "/admin/passport/csrf-token", Middlewares: []string{"public"}},
		{Name: "tenant.passport.login", Method: "POST", Path: "/admin/passport/login", Middlewares: []string{"tenant"}},
		{Name: "tenant.passport.mfa-login", Method: "POST", Path: "/admin/passport/mfa/login", Middlewares: []string{"tenant"}},
		{Name: "tenant.passport.password-change", Method: "POST", Path: "/admin/passport/password/change", Middlewares: []string{"tenant"}},
		{Name: "tenant.passport.sso-login", Method: "POST", Path: "/admin/passport/sso/login", Middlewares: []string{"tenant"}},
		{Name: "tenant.passport.sso-authorize", Method: "POST", Path: "/admin/passport/sso/authorize", Middlewares: []string{"tenant"}},
		{Name: "tenant.passport.sso-callback", Method: "POST", Path: "/admin/passport/sso/callback", Middlewares: []string{"tenant"}},
		{Name: "tenant.passport.branding", Method: "GET", Path: "/admin/passport/branding", Middlewares: []string{"tenant"}},
		{Name: "tenant.passport.info", Method: "GET", Path: "/admin/passport/getInfo", Middlewares: []string{"tenant"}},
		{Name: "tenant.passport.logout", Method: "POST", Path: "/admin/passport/logout", Middlewares: []string{"tenant"}},
		{Name: "tenant.passport.refresh", Method: "POST", Path: "/admin/passport/refresh", Middlewares: []string{"tenant"}},
		{Name: "tenant.passport.captcha", Method: "GET", Path: "/admin/passport/captcha", Middlewares: []string{"public"}},
		{Name: "system.captcha", Method: "GET", Path: "/api/system/captcha", Middlewares: []string{"public"}},
		{Name: "platform.passport.csrf-token", Method: "GET", Path: "/admin/platform/passport/csrf-token", Middlewares: []string{"public"}},
		{Name: "platform.passport.login", Method: "POST", Path: "/admin/platform/passport/login", Middlewares: []string{"public"}},
		{Name: "platform.passport.mfa-login", Method: "POST", Path: "/admin/platform/passport/mfa/login", Middlewares: []string{"public"}},
		{Name: "platform.passport.password-change", Method: "POST", Path: "/admin/platform/passport/password/change", Middlewares: []string{"public"}},
		{Name: "platform.passport.info", Method: "GET", Path: "/admin/platform/passport/getInfo", Middlewares: []string{"public"}},
		{Name: "platform.passport.logout", Method: "POST", Path: "/admin/platform/passport/logout", Middlewares: []string{"public"}},
		{Name: "platform.passport.refresh", Method: "POST", Path: "/admin/platform/passport/refresh", Middlewares: []string{"public"}},
		{Name: "platform.security.mfa.setup", Method: "POST", Path: "/admin/platform/security/mfa/setup", Permission: "platform:security:mfa", Middlewares: []string{"platform-self-audit"}},
		{Name: "platform.security.mfa.confirm", Method: "POST", Path: "/admin/platform/security/mfa/confirm", Permission: "platform:security:mfa", Middlewares: []string{"platform-self-audit"}},
		{Name: "platform.security.mfa.disable", Method: "POST", Path: "/admin/platform/security/mfa/disable", Permission: "platform:security:mfa", Middlewares: []string{"platform-self-audit"}},
		{Name: "platform.security.reauth-token", Method: "POST", Path: "/admin/platform/security/reauth-token", Permission: "platform:security:control", Permissions: platformSensitivePermissions, Middlewares: []string{"platform-admin"}},
		{Name: "platform.security.approval.create", Method: "POST", Path: "/admin/platform/security/approvals", Permission: "platform:security:control", Permissions: platformSensitivePermissions, Middlewares: []string{"platform-admin"}},
		{Name: "platform.security.approval.detail", Method: "GET", Path: "/admin/platform/security/approvals/{approval_id}", Permission: "platform:security:control", Permissions: platformSensitivePermissions, Middlewares: []string{"platform-admin"}},
		{Name: "platform.security.approval.approve", Method: "PUT", Path: "/admin/platform/security/approvals/{approval_id}/approve", Permission: "platform:security:control", Permissions: platformSensitivePermissions, Middlewares: []string{"platform-admin"}},
		{Name: "tenant.security.reauth-token", Method: "POST", Path: "/admin/security/reauth-token", Permission: "security:mfa", Permissions: []string{"security:mfa", "permission:user:password", "permission:user:setRole", "permission:role:setMenu", "security:ssoProvider:save", "security:ssoProvider:update", "security:ssoProvider:delete"}, Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.security.approval.create", Method: "POST", Path: "/admin/security/approvals", Permission: "security:mfa", Permissions: []string{"security:mfa", "permission:user:password", "permission:user:setRole", "permission:role:setMenu", "security:ssoProvider:save", "security:ssoProvider:update", "security:ssoProvider:delete"}, Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.security.approval.detail", Method: "GET", Path: "/admin/security/approvals/{approval_id}", Permission: "security:mfa", Permissions: []string{"security:mfa", "permission:user:password", "permission:user:setRole", "permission:role:setMenu", "security:ssoProvider:save", "security:ssoProvider:update", "security:ssoProvider:delete"}, Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.security.approval.approve", Method: "PUT", Path: "/admin/security/approvals/{approval_id}/approve", Permission: "security:mfa", Permissions: []string{"security:mfa", "permission:user:password", "permission:user:setRole", "permission:role:setMenu", "security:ssoProvider:save", "security:ssoProvider:update", "security:ssoProvider:delete"}, Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.security.mfa.setup", Method: "POST", Path: "/admin/security/mfa/setup", Permission: "security:mfa", Middlewares: []string{"tenant-audit-only"}},
		{Name: "tenant.security.mfa.confirm", Method: "POST", Path: "/admin/security/mfa/confirm", Permission: "security:mfa", Middlewares: []string{"tenant-audit-only"}},
		{Name: "tenant.security.mfa.disable", Method: "POST", Path: "/admin/security/mfa/disable", Permission: "security:mfa", Middlewares: []string{"tenant-audit-only"}},
		{Name: "tenant.sso-provider.list", Method: "GET", Path: "/admin/sso-provider/list", Permission: "security:ssoProvider:list", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.sso-provider.create", Method: "POST", Path: "/admin/sso-provider", Permission: "security:ssoProvider:save", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.sso-provider.update", Method: "PUT", Path: "/admin/sso-provider/{id}", Permission: "security:ssoProvider:update", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.sso-provider.delete", Method: "DELETE", Path: "/admin/sso-provider", Permission: "security:ssoProvider:delete", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.sso-user-binding.list", Method: "GET", Path: "/admin/sso-user-binding/list", Permission: "security:ssoUserBinding:list", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.sso-user-binding.detail", Method: "GET", Path: "/admin/sso-user-binding/{id}", Permission: "security:ssoUserBinding:detail", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.sso-user-binding.user", Method: "GET", Path: "/admin/sso-user-binding/user/{id}", Permission: "security:ssoUserBinding:user", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.sso-user-binding.unbind", Method: "DELETE", Path: "/admin/sso-user-binding/{id}", Permission: "security:ssoUserBinding:unbind", Middlewares: []string{"tenant-rbac-audit"}},
		{Name: "tenant.sso-login-log.list", Method: "GET", Path: "/admin/sso-login-log/list", Permission: "log:ssoLogin:list", Middlewares: []string{"tenant-rbac"}},
		{Name: "tenant.sso-login-log.stats", Method: "GET", Path: "/admin/sso-login-log/stats", Permission: "log:ssoLogin:stats", Middlewares: []string{"tenant-rbac"}},
	}
}

func (m Module) Menus() []modules.Menu {
	return []modules.Menu{
		{Key: "security:ssoProvider", ParentKey: "security:sso", Title: "身份源配置", Path: "/security/sso/provider", Component: "base/views/security/ssoProvider/index", Permission: "security:ssoProvider:list", Type: "M", I18n: "baseMenu.security.ssoProvider", Sort: 10},
		{Key: "security:ssoUserBinding", ParentKey: "security:sso", Title: "用户绑定", Path: "/security/sso/binding", Component: "base/views/security/ssoUserBinding/index", Permission: "security:ssoUserBinding:list", Type: "M", I18n: "baseMenu.security.ssoUserBinding", Sort: 20},
		{Key: "security:ssoLoginAudit", ParentKey: "security:sso", Title: "登录审计", Path: "/security/sso/login-audit", Component: "base/views/log/ssoLogin", Permission: "log:ssoLogin:list", Type: "M", I18n: "baseMenu.security.ssoLoginAudit", Sort: 30},
	}
}

func (m Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "platform:security:mfa", Description: "平台 MFA 管理"},
		{Key: "platform:security:control", Description: "平台敏感操作控制"},
		{Key: "security:mfa", Description: "租户 MFA 管理"},
		{Key: "security:ssoProvider:list", Description: "单点登录配置列表"},
		{Key: "security:ssoProvider:save", Description: "单点登录配置保存"},
		{Key: "security:ssoProvider:update", Description: "单点登录配置更新"},
		{Key: "security:ssoProvider:delete", Description: "单点登录配置删除"},
		{Key: "security:ssoUserBinding:list", Description: "单点登录用户绑定列表"},
		{Key: "security:ssoUserBinding:detail", Description: "单点登录用户绑定详情"},
		{Key: "security:ssoUserBinding:user", Description: "用户单点登录绑定"},
		{Key: "security:ssoUserBinding:unbind", Description: "单点登录解绑"},
		{Key: "log:ssoLogin:list", Description: "SSO 登录日志列表"},
		{Key: "log:ssoLogin:stats", Description: "SSO 登录统计"},
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
		"tests/feature/admin/sso_provider_test.go",
		"tests/feature/admin/passport_test.go",
	}
}
