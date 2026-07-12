package routes

import (
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/support"

	"goravel/app/facades"
	"goravel/app/http/controllers"
	"goravel/app/modules"
)

func Web(moduleRegistry modules.Registry) {
	facades.Route().Get("/", func(ctx http.Context) http.Response {
		return ctx.Response().View().Make("welcome.tmpl", map[string]any{
			"version": support.Version,
		})
	})

	facades.Route().Static("public", "./public")
	facades.Route().Static("storage", facades.App().BasePath("storage/app/public"))

	healthController := controllers.NewHealthController()
	facades.Route().Get("/health/live", healthController.Live)
	facades.Route().Get("/health/ready", healthController.Ready)

	observabilityController := controllers.NewObservabilityController()
	if facades.Config().GetBool("observability.metrics.enabled", true) {
		metricsPath := facades.Config().GetString("observability.metrics.path", "/metrics")
		facades.Route().Get(metricsPath, observabilityController.Metrics)
	}

	moduleRegistry.RegisterRoutes()
}
