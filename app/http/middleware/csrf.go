package middleware

import (
	"net/http"
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

const csrfTokenHeader = "X-CSRF-Token"

func CSRF() contractshttp.Middleware {
	return func(ctx contractshttp.Context) {
		if !services.CSRFEnabled() || csrfSafeMethod(ctx.Request().Method()) {
			ctx.Request().Next()
			return
		}
		if !services.CSRFOriginAllowed(requestOrigin(ctx)) {
			abortCSRF(ctx, "CSRF Origin 不可信")
			return
		}
		if !services.CSRFTokenValid(ctx.Request().Header(csrfTokenHeader, ""), ctx.Request().Cookie("csrf_token", "")) {
			abortCSRF(ctx, "CSRF Token 无效")
			return
		}
		ctx.Request().Next()
	}
}

func csrfSafeMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func requestOrigin(ctx contractshttp.Context) string {
	if origin := ctx.Request().Header("Origin", ""); origin != "" {
		return origin
	}
	return ctx.Request().Header("Referer", "")
}

func abortCSRF(ctx contractshttp.Context, message string) {
	_ = ctx.Response().Json(http.StatusOK, response.Error(response.CodeForbidden, message, []any{})).Abort()
}
