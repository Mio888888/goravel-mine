package middleware

import (
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/services"
)

func PlatformSelfAudit() contractshttp.Middleware {
	return func(ctx contractshttp.Context) {
		passport := services.NewPlatformPassportService().WithContext(ctx.Context())
		method := strings.ToUpper(ctx.Request().Method())
		path := ctx.Request().Path()
		route := ctx.Request().OriginPath()
		defer func() {
			if recovered := recover(); recovered != nil {
				markAuditPanic(ctx)
				recordPlatformSelfAudit(ctx, passport, method, route, path)
				panic(recovered)
			}
			recordPlatformSelfAudit(ctx, passport, method, route, path)
		}()
		ctx.Request().Next()
	}
}

func recordPlatformSelfAudit(ctx contractshttp.Context, passport *services.PlatformPassportService, method, route, path string) {
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err == nil {
		recordPlatformAuditEvent(ctx, user, method, route, path)
	}
}
