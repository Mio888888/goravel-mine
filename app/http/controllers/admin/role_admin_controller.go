package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type RoleAdminController struct {
	passport *services.PassportService
	service  *services.PermissionAdminService
}

type permissionsRequest struct {
	Permissions []string `json:"permissions"`
	ReAuthToken string   `json:"reauth_token"`
	ApprovalID  string   `json:"approval_id"`
}

func NewRoleAdminController() *RoleAdminController {
	return &RoleAdminController{passport: services.NewPassportService(), service: services.NewPermissionAdminService()}
}

func (r *RoleAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	result, err := service.ListRoles(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *RoleAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req services.RolePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.CreateRole(req, user.ID))
}

func (r *RoleAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
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
	var req services.RolePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.UpdateRole(id, req, user.ID))
}

func (r *RoleAdminController) Delete(ctx contractshttp.Context) contractshttp.Response {
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.DeleteRoles(ids))
}

func (r *RoleAdminController) Permissions(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	permissions, err := service.RolePermissions(id)
	return jsonResult(ctx, permissions, err)
}

func (r *RoleAdminController) SetPermissions(ctx contractshttp.Context) contractshttp.Response {
	actor, err := tenantCurrentUser(ctx)
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
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.SyncRolePermissionsSensitive(actor.ID, id, req.Permissions, services.SensitiveOperationEvidence{
		ReAuthToken: req.ReAuthToken, ApprovalID: req.ApprovalID,
	}))
}
