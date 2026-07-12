package middleware

import (
	"net/http"
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

func TenantGovernanceModule(module string) contractshttp.Middleware {
	return func(ctx contractshttp.Context) {
		tenant, ok := services.CurrentTenant(ctx.Context())
		if !ok {
			_ = ctx.Response().Json(http.StatusOK, services.LoginErrorResult(services.ErrUnauthorized)).Abort()
			return
		}
		policy, err := services.NewTenantGovernanceService().WithContext(ctx.Context()).Policy(tenant)
		if err != nil {
			_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{})).Abort()
			return
		}
		if err := policy.RequireModule(module); err != nil {
			_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeForbidden, "租户模块已禁用", []any{})).Abort()
			return
		}
		ctx.Request().Next()
	}
}

func TenantGovernanceModuleFromPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/admin/attachment"),
		strings.HasPrefix(path, "/admin/dictionary"),
		strings.HasPrefix(path, "/admin/dictionary-item"),
		strings.HasPrefix(path, "/admin/user-login-log"),
		strings.HasPrefix(path, "/admin/user-operation-log"):
		return "data-center"
	case strings.HasPrefix(path, "/admin/sso-provider"),
		strings.HasPrefix(path, "/admin/sso-user-binding"),
		strings.HasPrefix(path, "/admin/sso-login-log"),
		strings.HasPrefix(path, "/admin/security/mfa"):
		return "security"
	case strings.HasPrefix(path, "/admin/user"),
		strings.HasPrefix(path, "/admin/permission"),
		strings.HasPrefix(path, "/admin/role"),
		strings.HasPrefix(path, "/admin/menu"),
		strings.HasPrefix(path, "/admin/department"),
		strings.HasPrefix(path, "/admin/position"),
		strings.HasPrefix(path, "/admin/leader"):
		return "tenant-rbac"
	default:
		return ""
	}
}
