package admin

import (
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type QueueFailedJobController struct {
	service *services.QueueFailedJobService
}

func NewQueueFailedJobController() *QueueFailedJobController {
	return &QueueFailedJobController{service: services.NewQueueFailedJobService()}
}

func (r *QueueFailedJobController) List(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).List(queueFailedJobFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *QueueFailedJobController) Retry(ctx contractshttp.Context) contractshttp.Response {
	var req services.QueueFailedJobFilters
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).Retry(req)
	return jsonResult(ctx, result, err)
}

func (r *QueueFailedJobController) Delete(ctx contractshttp.Context) contractshttp.Response {
	var req services.QueueFailedJobFilters
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).Delete(req)
	return jsonResult(ctx, result, err)
}

func queueFailedJobFilters(ctx contractshttp.Context) services.QueueFailedJobFilters {
	return services.QueueFailedJobFilters{
		Connection: ctx.Request().Query("connection"),
		Queue:      ctx.Request().Query("queue"),
		UUIDs:      ctx.Request().QueryArray("uuid"),
	}
}
