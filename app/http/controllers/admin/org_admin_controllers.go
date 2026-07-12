package admin

import (
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type DepartmentAdminController struct {
	service *services.OrgAdminService
}

func NewDepartmentAdminController() *DepartmentAdminController {
	return &DepartmentAdminController{service: services.NewOrgAdminService()}
}

func (r *DepartmentAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.ListDepartments(queryFilters(ctx))
	return jsonResult(ctx, result, err)
}

func (r *DepartmentAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	var req services.DepartmentPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.CreateDepartment(req))
}

func (r *DepartmentAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.DepartmentPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.UpdateDepartment(id, req))
}

func (r *DepartmentAdminController) Delete(ctx contractshttp.Context) contractshttp.Response {
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.DeleteDepartments(ids))
}

type PositionAdminController struct {
	service *services.OrgAdminService
}

func NewPositionAdminController() *PositionAdminController {
	return &PositionAdminController{service: services.NewOrgAdminService()}
}

func (r *PositionAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.ListPositions(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *PositionAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	var req services.PositionPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.CreatePosition(req))
}

func (r *PositionAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.PositionPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.UpdatePosition(id, req))
}

func (r *PositionAdminController) Delete(ctx contractshttp.Context) contractshttp.Response {
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.DeletePositions(ids))
}

func (r *PositionAdminController) SetDataPermission(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.PositionPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.SetPositionPolicy(id, req))
}

type LeaderAdminController struct {
	service *services.OrgAdminService
}

func NewLeaderAdminController() *LeaderAdminController {
	return &LeaderAdminController{service: services.NewOrgAdminService()}
}

func (r *LeaderAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.ListLeaders(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *LeaderAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	var req services.LeaderPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.SaveLeaders(req))
}

func (r *LeaderAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
	var req services.LeaderPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.SaveLeaders(req))
}

func (r *LeaderAdminController) Delete(ctx contractshttp.Context) contractshttp.Response {
	var req services.LeaderPayload
	if err := bindJSONBody(ctx, &req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantOrgService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.DeleteLeaders(req))
}
