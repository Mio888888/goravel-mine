package middleware

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/facades"
	"goravel/app/http/response"
	"goravel/app/services"
)

func CasbinAuthz() contractshttp.Middleware {
	return func(ctx contractshttp.Context) {
		tenant, ok := services.CurrentTenant(ctx.Context())
		if !ok {
			_ = ctx.Response().Json(http.StatusOK, services.LoginErrorResult(services.ErrUnauthorized)).Abort()
			return
		}
		passport := services.NewPassportServiceForTenant(tenant).WithContext(ctx.Context())
		casbinService := services.NewCasbinServiceForTenant(tenant).WithContext(ctx.Context())
		user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
		if err != nil {
			_ = ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err)).Abort()
			return
		}
		if !services.TenantAllowsRoute(tenant, ctx.Request().Method(), ctx.Request().OriginPath()) {
			_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeForbidden, "禁止访问", []any{})).Abort()
			return
		}

		allowed, err := casbinService.Authorize(user, ctx.Request().Method(), ctx.Request().OriginPath())
		if err != nil {
			facades.Log().Error("tenant authorization failed", map[string]any{
				"error": err.Error(), "tenant": tenant.Code, "user_id": user.ID,
				"method": ctx.Request().Method(), "route": ctx.Request().OriginPath(),
			})
			_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{})).Abort()
			return
		}
		if !allowed {
			_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeForbidden, "禁止访问", []any{})).Abort()
			return
		}

		ctx.Request().Next()
	}
}
