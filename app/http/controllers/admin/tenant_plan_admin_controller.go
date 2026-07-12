package admin

import (
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type TenantPlanAdminController struct {
	service *services.TenantPlanService
}

func NewTenantPlanAdminController() *TenantPlanAdminController {
	return &TenantPlanAdminController{service: services.NewTenantPlanService()}
}

func (r *TenantPlanAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	plans, err := r.service.WithContext(ctx.Context()).List(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, plans, err)
}

func (r *TenantPlanAdminController) Options(ctx contractshttp.Context) contractshttp.Response {
	plans, err := r.service.WithContext(ctx.Context()).Options()
	return jsonResult(ctx, plans, err)
}

func (r *TenantPlanAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	var req services.TenantPlanPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	plan, err := r.service.WithContext(ctx.Context()).Create(req)
	return jsonResult(ctx, plan, err)
}

func (r *TenantPlanAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.TenantPlanPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	plan, err := r.service.WithContext(ctx.Context()).Update(id, req)
	return jsonResult(ctx, plan, err)
}

func (r *TenantPlanAdminController) Delete(ctx contractshttp.Context) contractshttp.Response {
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).Delete(ids))
}
