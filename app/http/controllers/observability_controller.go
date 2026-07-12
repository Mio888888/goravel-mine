package controllers

import (
	"crypto/subtle"
	"net/http"
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/facades"
	"goravel/app/services"
)

type ObservabilityController struct{}

func NewObservabilityController() *ObservabilityController {
	return &ObservabilityController{}
}

func (r *ObservabilityController) Metrics(ctx contractshttp.Context) contractshttp.Response {
	if token := facades.Config().GetString("observability.metrics.token", ""); token != "" {
		authHeader := ctx.Request().Header("Authorization", "")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return ctx.Response().String(http.StatusUnauthorized, "unauthorized")
		}
		authToken := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(authToken), []byte(token)) != 1 {
			return ctx.Response().String(http.StatusUnauthorized, "unauthorized")
		}
	}
	text := services.PrometheusMetricsText(services.ObservabilityMetrics())
	return ctx.Response().Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(text))
}
