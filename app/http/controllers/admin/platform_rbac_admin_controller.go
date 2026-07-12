package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type PlatformUserAdminController struct {
	passport *services.PlatformPassportService
	service  *services.PlatformPermissionAdminService
}

func NewPlatformUserAdminController() *PlatformUserAdminController {
	return &PlatformUserAdminController{
		passport: services.NewPlatformPassportService(),
		service:  services.NewPlatformPermissionAdminService(),
	}
}

func (r *PlatformUserAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	service := r.service.WithContext(ctx.Context())
	result, err := service.ListUsers(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *PlatformUserAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req services.UserPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).CreateUser(req, user.ID))
}

func (r *PlatformUserAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.UserPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).UpdateUser(id, req, user.ID))
}

func (r *PlatformUserAdminController) Delete(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).DeleteUsers(ids, user.ID))
}

func (r *PlatformUserAdminController) ResetPassword(ctx contractshttp.Context) contractshttp.Response {
	actor, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req resetPasswordRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).ResetPasswordSensitive(actor.ID, req.ID, services.SensitiveOperationEvidence{
		ReAuthToken: req.ReAuthToken, ApprovalID: req.ApprovalID,
	}))
}

func (r *PlatformUserAdminController) Roles(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	roles, err := r.service.WithContext(ctx.Context()).UserRoles(id)
	return jsonResult(ctx, roles, err)
}

func (r *PlatformUserAdminController) SetRoles(ctx contractshttp.Context) contractshttp.Response {
	actor, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req roleCodesRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).SyncUserRolesSensitive(actor.ID, id, req.RoleCodes, services.SensitiveOperationEvidence{
		ReAuthToken: req.ReAuthToken, ApprovalID: req.ApprovalID,
	}))
}

type PlatformRoleAdminController struct {
	passport *services.PlatformPassportService
	service  *services.PlatformPermissionAdminService
}

func NewPlatformRoleAdminController() *PlatformRoleAdminController {
	return &PlatformRoleAdminController{
		passport: services.NewPlatformPassportService(),
		service:  services.NewPlatformPermissionAdminService(),
	}
}

func (r *PlatformRoleAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).ListRoles(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *PlatformRoleAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req services.RolePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).CreateRole(req, user.ID))
}

func (r *PlatformRoleAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.RolePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).UpdateRole(id, req, user.ID))
}

func (r *PlatformRoleAdminController) Delete(ctx contractshttp.Context) contractshttp.Response {
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).DeleteRoles(ids))
}

func (r *PlatformRoleAdminController) Permissions(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	permissions, err := r.service.WithContext(ctx.Context()).RolePermissions(id)
	return jsonResult(ctx, permissions, err)
}

func (r *PlatformRoleAdminController) SetPermissions(ctx contractshttp.Context) contractshttp.Response {
	actor, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req permissionsRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).SyncRolePermissionsSensitive(actor.ID, id, req.Permissions, services.SensitiveOperationEvidence{
		ReAuthToken: req.ReAuthToken, ApprovalID: req.ApprovalID,
	}))
}

type PlatformMenuAdminController struct {
	passport *services.PlatformPassportService
	service  *services.PlatformPermissionAdminService
}

func NewPlatformMenuAdminController() *PlatformMenuAdminController {
	return &PlatformMenuAdminController{
		passport: services.NewPlatformPassportService(),
		service:  services.NewPlatformPermissionAdminService(),
	}
}

func (r *PlatformMenuAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	menus, err := r.service.WithContext(ctx.Context()).ListMenus()
	return jsonResult(ctx, menus, err)
}

func (r *PlatformMenuAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req services.MenuPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).CreateMenu(req, user.ID))
}

func (r *PlatformMenuAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
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
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).UpdateMenu(id, req, user.ID))
}

func (r *PlatformMenuAdminController) Delete(ctx contractshttp.Context) contractshttp.Response {
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).DeleteMenus(ids))
}
