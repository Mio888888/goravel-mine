package middleware

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

func TenantContext() contractshttp.Middleware {
	return func(ctx contractshttp.Context) {
		resolver := services.NewTenantService().WithContext(ctx.Context())
		runtime := services.NewTenantRuntimeService().WithContext(ctx.Context())
		tenant, err := resolver.ResolveByCodeOrHost(tenantCode(ctx), tenantHost(ctx))
		if err != nil {
			_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnauthorized, "租户不存在或已停用", []any{})).Abort()
			return
		}
		if err := runtime.EnsureSubscription(tenant); err != nil {
			_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeDisabled, "租户订阅不可用", []any{})).Abort()
			return
		}
		if err := runtime.AllowRequest(tenant); err != nil {
			_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeTooManyRequests, "租户 API 配额已用尽", []any{})).Abort()
			return
		}

		ctx.WithValue(services.TenantContextKey(), tenant)
		ctx.Request().Next()
	}
}

func tenantCode(ctx contractshttp.Context) string {
	if code := ctx.Request().Header("X-Tenant-Code", ""); code != "" {
		return code
	}
	return ctx.Request().Header("X-Tenant", "")
}

func tenantHost(ctx contractshttp.Context) string {
	if host := services.TrustedForwardedHost(ctx.Request().Header, ctx.Request().Origin().RemoteAddr); host != "" {
		return host
	}
	if host := ctx.Request().Header("Host", ""); host != "" {
		return host
	}
	return ctx.Request().Host()
}
