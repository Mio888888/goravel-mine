package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type ObservabilityController struct{}

func NewObservabilityController() *ObservabilityController {
	return &ObservabilityController{}
}

func (r *ObservabilityController) SlowRequests(ctx contractshttp.Context) contractshttp.Response {
	snapshot := services.ObservabilityMetrics()
	limit := ctx.Request().QueryInt("limit", 20)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	items := snapshot.SlowRequests
	if len(items) > limit {
		items = items[:limit]
	}
	return ctx.Response().Json(http.StatusOK, response.Success(map[string]any{
		"summary":       services.MetricsSummary(snapshot),
		"slow_requests": items,
		"slow_sql":      services.SlowSQLMetrics(),
	}))
}
