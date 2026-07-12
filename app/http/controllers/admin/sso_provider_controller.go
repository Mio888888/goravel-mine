package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type SSOProviderController struct{}

type ssoProviderSensitiveRequest struct {
	services.SSOProviderPayload
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

type sensitiveDeleteRequest struct {
	IDs         []uint64 `json:"ids"`
	ReAuthToken string   `json:"reauth_token"`
	ApprovalID  string   `json:"approval_id"`
}

func NewSSOProviderController() *SSOProviderController {
	return &SSOProviderController{}
}

func (r *SSOProviderController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantSSOProviderService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	result, err := service.List(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *SSOProviderController) Create(ctx contractshttp.Context) contractshttp.Response {
	user, err := tenantCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req ssoProviderSensitiveRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantSSOProviderService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	tenant, _ := currentTenant(ctx)
	provider, err := service.CreateSensitive(req.SSOProviderPayload, user.ID, tenant.ID, sensitiveEvidence(req.ReAuthToken, req.ApprovalID))
	return jsonResult(ctx, provider, err)
}

func (r *SSOProviderController) Update(ctx contractshttp.Context) contractshttp.Response {
	user, err := tenantCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req ssoProviderSensitiveRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantSSOProviderService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	tenant, _ := currentTenant(ctx)
	provider, err := service.UpdateSensitive(id, req.SSOProviderPayload, user.ID, tenant.ID, sensitiveEvidence(req.ReAuthToken, req.ApprovalID))
	return jsonResult(ctx, provider, err)
}

func (r *SSOProviderController) Delete(ctx contractshttp.Context) contractshttp.Response {
	user, err := tenantCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req sensitiveDeleteRequest
	if err := bindJSONBody(ctx, &req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantSSOProviderService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	tenant, _ := currentTenant(ctx)
	return jsonResult(ctx, nil, service.DeleteSensitive(req.IDs, user.ID, tenant.ID, sensitiveEvidence(req.ReAuthToken, req.ApprovalID)))
}
