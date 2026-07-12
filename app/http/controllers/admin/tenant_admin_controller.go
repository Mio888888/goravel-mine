package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type TenantAdminController struct {
	passport *services.PlatformPassportService
	service  *services.TenantService
}

type tenantPermissionSensitiveRequest struct {
	services.TenantPermissionPayload
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

type tenantPlanSensitiveRequest struct {
	services.TenantPlanUpdatePayload
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

type tenantGovernanceSensitiveRequest struct {
	services.TenantGovernancePatch
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

type tenantStatusSensitiveRequest struct {
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

func NewTenantAdminController() *TenantAdminController {
	return &TenantAdminController{
		passport: services.NewPlatformPassportService(),
		service:  services.NewTenantService(),
	}
}

func (r *TenantAdminController) List(ctx contractshttp.Context) contractshttp.Response {
	tenants, err := r.service.WithContext(ctx.Context()).List(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, tenants, err)
}

func (r *TenantAdminController) Create(ctx contractshttp.Context) contractshttp.Response {
	var req services.TenantPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	tenant, err := r.service.WithContext(ctx.Context()).Create(req)
	return jsonResult(ctx, tenant, err)
}

func (r *TenantAdminController) Update(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.TenantPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	tenant, err := r.service.WithContext(ctx.Context()).Update(id, req)
	return jsonResult(ctx, tenant, err)
}

func (r *TenantAdminController) Usage(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	usage, err := r.service.WithContext(ctx.Context()).Usage(id)
	return jsonResult(ctx, usage, err)
}

func (r *TenantAdminController) Governance(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	tenant, err := r.service.WithContext(ctx.Context()).FindByID(id)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	policy, err := services.NewTenantGovernanceService().WithContext(ctx.Context()).Policy(tenant)
	return jsonResult(ctx, policy, err)
}

func (r *TenantAdminController) UpdateGovernance(ctx contractshttp.Context) contractshttp.Response {
	operator, err := r.currentOperator(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	tenant, err := r.service.WithContext(ctx.Context()).FindByID(id)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	var req tenantGovernanceSensitiveRequest
	if err := bindJSONBody(ctx, &req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	governance := services.NewTenantGovernanceService().WithContext(ctx.Context())
	policy, err := governance.PatchPolicySensitive(operator.ID, tenant, req.TenantGovernancePatch, sensitiveEvidence(req.ReAuthToken, req.ApprovalID))
	return jsonResult(ctx, policy, err)
}

func (r *TenantAdminController) PermissionCatalog(ctx contractshttp.Context) contractshttp.Response {
	return jsonResult(ctx, services.TenantPermissionCatalogMenus(), nil)
}

func (r *TenantAdminController) Permissions(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	permissions, err := r.service.WithContext(ctx.Context()).Permissions(id)
	return jsonResult(ctx, permissions, err)
}

func (r *TenantAdminController) UpdatePermissions(ctx contractshttp.Context) contractshttp.Response {
	operator, err := r.currentOperator(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req tenantPermissionSensitiveRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	permissions, err := r.service.WithContext(ctx.Context()).UpdatePermissionsSensitive(id, req.TenantPermissionPayload, operator, sensitiveEvidence(req.ReAuthToken, req.ApprovalID))
	return jsonResult(ctx, permissions, err)
}

func (r *TenantAdminController) PermissionPlanDiff(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.TenantPlanUpdatePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	diff, err := r.service.WithContext(ctx.Context()).PermissionPlanDiff(id, req.Plan)
	return jsonResult(ctx, diff, err)
}

func (r *TenantAdminController) UpdatePlan(ctx contractshttp.Context) contractshttp.Response {
	operator, err := r.currentOperator(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req tenantPlanSensitiveRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	tenant, err := r.service.WithContext(ctx.Context()).UpdatePlanSensitive(id, req.TenantPlanUpdatePayload, operator, sensitiveEvidence(req.ReAuthToken, req.ApprovalID))
	return jsonResult(ctx, tenant, err)
}

func (r *TenantAdminController) currentOperator(ctx contractshttp.Context) (services.TenantPermissionOperator, error) {
	user, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return services.TenantPermissionOperator{}, err
	}
	return services.TenantPermissionOperator{ID: user.ID, Name: user.Username}, nil
}

func (r *TenantAdminController) Suspend(ctx contractshttp.Context) contractshttp.Response {
	return r.updateStatus(ctx, services.TenantStatusSuspended)
}

func (r *TenantAdminController) Resume(ctx contractshttp.Context) contractshttp.Response {
	return r.updateStatus(ctx, services.TenantStatusActive)
}

func (r *TenantAdminController) Archive(ctx contractshttp.Context) contractshttp.Response {
	return r.updateStatus(ctx, services.TenantStatusArchived)
}

func (r *TenantAdminController) updateStatus(ctx contractshttp.Context, status int8) contractshttp.Response {
	operator, err := r.currentOperator(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req tenantStatusSensitiveRequest
	if err := bindJSONBody(ctx, &req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).UpdateStatusSensitive(operator.ID, id, status, sensitiveEvidence(req.ReAuthToken, req.ApprovalID)))
}

func sensitiveEvidence(reauthToken, approvalID string) services.SensitiveOperationEvidence {
	return services.SensitiveOperationEvidence{ReAuthToken: reauthToken, ApprovalID: approvalID}
}

func (r *TenantAdminController) Destroy(ctx contractshttp.Context) contractshttp.Response {
	operator, err := r.currentOperator(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req services.TenantDestroyPayload
	if err := bindJSONBody(ctx, &req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	req.OperatorID = operator.ID
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).Destroy(req))
}
