package admin

import (
	"net/http"
	"strconv"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type UserAdminController struct {
	passport *services.PassportService
	service  *services.PermissionAdminService
}

type roleCodesRequest struct {
	RoleCodes   []string `json:"role_codes"`
	ReAuthToken string   `json:"reauth_token"`
	ApprovalID  string   `json:"approval_id"`
}

type resetPasswordRequest struct {
	ID          uint64 `json:"id"`
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

func NewUserAdminController() *UserAdminController {
	return &UserAdminController{passport: services.NewPassportService(), service: services.NewPermissionAdminService()}
}

func (r *UserAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	result, err := service.ListUsers(queryFilters(ctx), page(ctx), pageSize(ctx), user.ID)
	return jsonResult(ctx, result, err)
}

func (r *UserAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req services.UserPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.CreateUser(req, user.ID))
}

func (r *UserAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.currentUser(ctx)
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
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.UpdateUser(id, req, user.ID))
}

func (r *UserAdminController) Delete(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.DeleteUsers(ids, user.ID))
}

func (r *UserAdminController) ResetPassword(ctx contractshttp.Context) contractshttp.Response {
	actor, err := r.currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req resetPasswordRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.ResetPasswordSensitive(actor.ID, req.ID, services.SensitiveOperationEvidence{
		ReAuthToken: req.ReAuthToken, ApprovalID: req.ApprovalID,
	}))
}

func (r *UserAdminController) Roles(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	roles, err := service.UserRoles(id)
	return jsonResult(ctx, roles, err)
}

func (r *UserAdminController) SetRoles(ctx contractshttp.Context) contractshttp.Response {
	actor, err := r.currentUser(ctx)
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
	service, err := tenantPermissionService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.SyncUserRolesSensitive(actor.ID, id, req.RoleCodes, services.SensitiveOperationEvidence{
		ReAuthToken: req.ReAuthToken, ApprovalID: req.ApprovalID,
	}))
}

func (r *UserAdminController) currentUser(ctx contractshttp.Context) (services.UserInfo, error) {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return services.UserInfo{}, err
	}
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return services.UserInfo{}, err
	}
	return services.UserInfo{ID: user.ID}, nil
}

func routeID(ctx contractshttp.Context) (uint64, error) {
	return strconv.ParseUint(ctx.Request().Route("id"), 10, 64)
}
