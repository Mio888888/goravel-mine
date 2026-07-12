package admin

import (
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type ReferenceCaseController struct {
	service *services.ReferenceCaseService
}

func NewReferenceCaseController() *ReferenceCaseController {
	return &ReferenceCaseController{service: services.NewReferenceCaseService()}
}

func (r *ReferenceCaseController) List(ctx contractshttp.Context) contractshttp.Response {
	items, err := r.service.WithContext(ctx.Context()).List(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, items, err)
}

func (r *ReferenceCaseController) Create(ctx contractshttp.Context) contractshttp.Response {
	var req services.ReferenceCasePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	item, err := r.service.WithContext(ctx.Context()).Create(req)
	return jsonResult(ctx, item, err)
}

func (r *ReferenceCaseController) Update(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.ReferenceCasePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	item, err := r.service.WithContext(ctx.Context()).Update(id, req)
	return jsonResult(ctx, item, err)
}

func (r *ReferenceCaseController) Delete(ctx contractshttp.Context) contractshttp.Response {
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).Delete(ids))
}
