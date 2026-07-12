package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type MenuAdminController struct {
	passport *services.PassportService
	service  *services.PermissionAdminService
}

func NewMenuAdminController() *MenuAdminController {
	return &MenuAdminController{passport: services.NewPassportService(), service: services.NewPermissionAdminService()}
}

func (r *MenuAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	menus, err := service.ListMenus()
	return jsonResult(ctx, menus, err)
}

func (r *MenuAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req services.MenuPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.CreateMenu(req, user.ID))
}

func (r *MenuAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.MenuPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.UpdateMenu(id, req, user.ID))
}

func (r *MenuAdminController) Delete(ctx contractshttp.Context) contractshttp.Response {
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.DeleteMenus(ids))
}
