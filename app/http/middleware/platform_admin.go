package middleware

import (
	"net/http"
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

func PlatformAdmin() contractshttp.Middleware {
	return func(ctx contractshttp.Context) {
		passport := services.NewPlatformPassportService().WithContext(ctx.Context())
		casbinService := services.NewPlatformCasbinService().WithContext(ctx.Context())
		user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
		if err != nil {
			_ = ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err)).Abort()
			return
		}
		allowed, err := casbinService.Authorize(user, ctx.Request().Method(), ctx.Request().OriginPath())
		if err != nil {
			_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{})).Abort()
			return
		}
		if !allowed {
			_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeForbidden, "禁止访问", []any{})).Abort()
			return
		}

		method := strings.ToUpper(ctx.Request().Method())
		route := ctx.Request().OriginPath()
		path := ctx.Request().Path()
		defer func() {
			if recovered := recover(); recovered != nil {
				markAuditPanic(ctx)
				recordPlatformAuditEvent(ctx, user, method, route, path)
				panic(recovered)
			}
			recordPlatformAuditEvent(ctx, user, method, route, path)
		}()
		ctx.Request().Next()
	}
}
