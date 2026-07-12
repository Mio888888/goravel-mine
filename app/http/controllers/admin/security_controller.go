package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/models"
	"goravel/app/services"
)

type SecurityController struct{}

type mfaConfirmRequest struct {
	Code        string `json:"code"`
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

type platformReAuthRequest struct {
	Password  string `json:"password"`
	MFACode   string `json:"mfa_code"`
	Operation string `json:"operation"`
	Resource  string `json:"resource"`
}

type platformApprovalCreateRequest struct {
	PolicyKey string `json:"policy_key"`
	Scope     string `json:"scope"`
	Resource  string `json:"resource"`
	Reason    string `json:"reason"`
}

func NewSecurityController() *SecurityController {
	return &SecurityController{}
}

func (r *SecurityController) CSRFToken(ctx contractshttp.Context) contractshttp.Response {
	token, err := services.NewCSRFToken()
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	ctx.Response().Cookie(contractshttp.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: csrfSameSite(),
		Secure:   services.CSRFCookieSecure(),
	})
	return ctx.Response().Json(http.StatusOK, response.Success(map[string]any{
		"csrf_token": token,
	}))
}

func (r *SecurityController) TenantMFASetup(ctx contractshttp.Context) contractshttp.Response {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := tenantCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	result, err := services.NewMFAServiceForTenant(tenant).WithContext(ctx.Context()).Setup(user.ID, user.Username)
	return jsonResult(ctx, result, err)
}

func (r *SecurityController) TenantMFAConfirm(ctx contractshttp.Context) contractshttp.Response {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := tenantCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req mfaConfirmRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	codes, err := services.NewMFAServiceForTenant(tenant).WithContext(ctx.Context()).Confirm(user.ID, req.Code)
	return jsonResult(ctx, map[string]any{"recovery_codes": codes}, err)
}

func (r *SecurityController) TenantMFADisable(ctx contractshttp.Context) contractshttp.Response {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := tenantCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req mfaConfirmRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	return jsonResult(ctx, nil, services.NewMFAServiceForTenant(tenant).WithContext(ctx.Context()).DisableSensitive(user.ID, tenant.ID, req.Code, services.SensitiveOperationEvidence{
		ReAuthToken: req.ReAuthToken, ApprovalID: req.ApprovalID,
	}))
}

func (r *SecurityController) PlatformMFASetup(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	result, err := services.NewPlatformMFAService().WithContext(ctx.Context()).Setup(user.ID, user.Username)
	return jsonResult(ctx, result, err)
}

func (r *SecurityController) PlatformMFAConfirm(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req mfaConfirmRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	codes, err := services.NewPlatformMFAService().WithContext(ctx.Context()).Confirm(user.ID, req.Code)
	return jsonResult(ctx, map[string]any{"recovery_codes": codes}, err)
}

func (r *SecurityController) PlatformMFADisable(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req mfaConfirmRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	return jsonResult(ctx, nil, services.NewPlatformMFAService().WithContext(ctx.Context()).DisableSensitive(user.ID, 0, req.Code, services.SensitiveOperationEvidence{
		ReAuthToken: req.ReAuthToken, ApprovalID: req.ApprovalID,
	}))
}

func (r *SecurityController) TenantReAuthToken(ctx contractshttp.Context) contractshttp.Response {
	tenant, user, req, failed := tenantSensitiveEvidenceRequest(ctx)
	if failed != nil {
		return failed
	}
	if denied := denyUnauthorizedTenantSensitiveEvidence(ctx, tenant, user, req.Operation, req.Resource); denied != nil {
		return denied
	}
	result, err := services.NewEnterpriseSecurityControlService().IssueTenantReAuthToken(ctx.Context(), services.TenantReAuthRequest{
		Tenant: tenant, UserID: user.ID, Password: req.Password, MFACode: req.MFACode,
		Operation: req.Operation, Resource: req.Resource,
	})
	return jsonResult(ctx, result, err)
}

func (r *SecurityController) PlatformReAuthToken(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUserModel(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req platformReAuthRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	if denied := denyUnauthorizedPlatformSensitiveEvidence(ctx, user, req.Operation, req.Resource); denied != nil {
		return denied
	}
	result, err := services.NewEnterpriseSecurityControlService().IssuePlatformReAuthToken(ctx.Context(), services.PlatformReAuthRequest{
		UserID:    user.ID,
		Password:  req.Password,
		MFACode:   req.MFACode,
		Operation: req.Operation,
		Resource:  req.Resource,
	})
	return jsonResult(ctx, result, err)
}

func (r *SecurityController) TenantApprovalCreate(ctx contractshttp.Context) contractshttp.Response {
	tenant, user, req, failed := tenantApprovalRequest(ctx)
	if failed != nil {
		return failed
	}
	policyKey := approvalRequestPolicyKey(req)
	if denied := denyUnauthorizedTenantSensitiveEvidence(ctx, tenant, user, policyKey, req.Resource); denied != nil {
		return denied
	}
	result, err := services.NewEnterpriseSecurityControlService().CreatePlatformApproval(ctx.Context(), services.PlatformApprovalCreateRequest{
		RequesterID: user.ID, TenantID: tenant.ID, PolicyKey: policyKey, Scope: req.Scope,
		Resource: req.Resource, Reason: req.Reason,
	})
	return jsonResult(ctx, result, err)
}

func (r *SecurityController) PlatformApprovalCreate(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUserModel(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req platformApprovalCreateRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	policyKey := approvalRequestPolicyKey(req)
	if denied := denyUnauthorizedPlatformSensitiveEvidence(ctx, user, policyKey, req.Resource); denied != nil {
		return denied
	}
	result, err := services.NewEnterpriseSecurityControlService().CreatePlatformApproval(ctx.Context(), services.PlatformApprovalCreateRequest{
		RequesterID: user.ID,
		PolicyKey:   policyKey,
		Scope:       req.Scope,
		Resource:    req.Resource,
		Reason:      req.Reason,
	})
	return jsonResult(ctx, result, err)
}

func denyUnauthorizedPlatformSensitiveEvidence(ctx contractshttp.Context, user models.User, policyKey, resource string) contractshttp.Response {
	allowed, err := services.NewPlatformCasbinService().WithContext(ctx.Context()).AuthorizeSensitiveEvidence(user, policyKey, resource)
	if err != nil {
		return jsonError(ctx, response.CodeFail, "服务器错误")
	}
	if !allowed {
		return jsonError(ctx, response.CodeForbidden, "禁止申请该敏感操作凭证")
	}
	return nil
}

func denyUnauthorizedTenantSensitiveEvidence(ctx contractshttp.Context, tenant services.Tenant, user models.User, policyKey, resource string) contractshttp.Response {
	allowed, err := services.NewCasbinServiceForTenant(tenant).WithContext(ctx.Context()).AuthorizeSensitiveEvidence(user, policyKey, resource)
	if err != nil {
		return jsonError(ctx, response.CodeFail, "服务器错误")
	}
	if !allowed {
		return jsonError(ctx, response.CodeForbidden, "禁止申请该敏感操作凭证")
	}
	return nil
}

func (r *SecurityController) PlatformApprovalApprove(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUserModel(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	security := services.NewEnterpriseSecurityControlService()
	approvalID := ctx.Request().Route("approval_id")
	approval, err := security.PlatformApproval(ctx.Context(), approvalID)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	if denied := denyUnauthorizedPlatformSensitiveEvidence(ctx, user, approvalEvidencePolicyKey(approval), approval.Resource); denied != nil {
		return denied
	}
	result, err := security.ApprovePlatformApproval(ctx.Context(), services.PlatformApprovalApproveRequest{
		ApprovalID: approvalID,
		ApproverID: user.ID,
		TenantID:   0,
	})
	return jsonResult(ctx, result, err)
}

func (r *SecurityController) PlatformApprovalDetail(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUserModel(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	approval, err := services.NewEnterpriseSecurityControlService().PlatformApproval(ctx.Context(), ctx.Request().Route("approval_id"))
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	if denied := denyUnauthorizedPlatformSensitiveEvidence(ctx, user, approvalEvidencePolicyKey(approval), approval.Resource); denied != nil {
		return denied
	}
	return jsonResult(ctx, approval, nil)
}

func (r *SecurityController) TenantApprovalApprove(ctx contractshttp.Context) contractshttp.Response {
	tenant, user, approval, failed := tenantApprovalContext(ctx)
	if failed != nil {
		return failed
	}
	if denied := denyUnauthorizedTenantSensitiveEvidence(ctx, tenant, user, approvalEvidencePolicyKey(approval), approval.Resource); denied != nil {
		return denied
	}
	result, err := services.NewEnterpriseSecurityControlService().ApprovePlatformApproval(ctx.Context(), services.PlatformApprovalApproveRequest{
		ApprovalID: approval.ApprovalID, ApproverID: user.ID, TenantID: tenant.ID,
	})
	return jsonResult(ctx, result, err)
}

func (r *SecurityController) TenantApprovalDetail(ctx contractshttp.Context) contractshttp.Response {
	tenant, user, approval, failed := tenantApprovalContext(ctx)
	if failed != nil {
		return failed
	}
	if denied := denyUnauthorizedTenantSensitiveEvidence(ctx, tenant, user, approvalEvidencePolicyKey(approval), approval.Resource); denied != nil {
		return denied
	}
	return jsonResult(ctx, approval, nil)
}

func tenantSensitiveEvidenceRequest(ctx contractshttp.Context) (services.Tenant, models.User, platformReAuthRequest, contractshttp.Response) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return services.Tenant{}, models.User{}, platformReAuthRequest{}, ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := tenantCurrentUserModel(ctx)
	if err != nil {
		return services.Tenant{}, models.User{}, platformReAuthRequest{}, ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req platformReAuthRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return services.Tenant{}, models.User{}, platformReAuthRequest{}, jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return tenant, user, req, nil
}

func tenantApprovalRequest(ctx contractshttp.Context) (services.Tenant, models.User, platformApprovalCreateRequest, contractshttp.Response) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return services.Tenant{}, models.User{}, platformApprovalCreateRequest{}, ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := tenantCurrentUserModel(ctx)
	if err != nil {
		return services.Tenant{}, models.User{}, platformApprovalCreateRequest{}, ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req platformApprovalCreateRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return services.Tenant{}, models.User{}, platformApprovalCreateRequest{}, jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return tenant, user, req, nil
}

func tenantApprovalContext(ctx contractshttp.Context) (services.Tenant, models.User, services.PermissionApprovalRecord, contractshttp.Response) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return services.Tenant{}, models.User{}, services.PermissionApprovalRecord{}, ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := tenantCurrentUserModel(ctx)
	if err != nil {
		return services.Tenant{}, models.User{}, services.PermissionApprovalRecord{}, ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	approval, err := services.NewEnterpriseSecurityControlService().Approval(ctx.Context(), ctx.Request().Route("approval_id"), tenant.ID)
	if err != nil {
		return services.Tenant{}, models.User{}, services.PermissionApprovalRecord{}, jsonResult(ctx, nil, err)
	}
	return tenant, user, approval, nil
}

func approvalRequestPolicyKey(req platformApprovalCreateRequest) string {
	if req.PolicyKey != "" {
		return req.PolicyKey
	}
	return req.Scope
}

func csrfSameSite() string {
	return services.SecuritySameSite()
}

func approvalEvidencePolicyKey(approval services.PermissionApprovalRecord) string {
	if approval.PolicyKey != "" {
		return approval.PolicyKey
	}
	return approval.Scope
}
