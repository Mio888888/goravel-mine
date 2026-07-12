package middleware

import (
	"net/http"
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/services"
)

func PlatformAuth() contractshttp.Middleware {
	return func(ctx contractshttp.Context) {
		passport := services.NewPlatformPassportService().WithContext(ctx.Context())
		if _, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", "")); err != nil {
			_ = ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err)).Abort()
			return
		}
		ctx.Request().Next()
	}
}

func PlatformAuthAudit() contractshttp.Middleware {
	return func(ctx contractshttp.Context) {
		passport := services.NewPlatformPassportService().WithContext(ctx.Context())
		user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
		if err != nil {
			_ = ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err)).Abort()
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
