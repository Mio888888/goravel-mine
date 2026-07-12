package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/models"
	"goravel/app/services"
)

func jsonResult(ctx contractshttp.Context, data any, err error) contractshttp.Response {
	if err != nil {
		var businessErr services.BusinessError
		if errors.As(err, &businessErr) {
			return jsonError(ctx, response.CodeUnprocessableEntity, businessErr.Message)
		}
		if errors.Is(err, services.ErrQuotaExceeded) {
			return jsonError(ctx, response.CodeTooManyRequests, "租户配额已用尽")
		}
		if errors.Is(err, services.ErrSubscriptionInactive) {
			return jsonError(ctx, response.CodeDisabled, "租户订阅不可用")
		}
		if errors.Is(err, services.ErrReAuthRequired) ||
			errors.Is(err, services.ErrApprovalRequired) ||
			errors.Is(err, services.ErrApprovalSelfApproved) ||
			errors.Is(err, services.ErrWORMProofRequired) ||
			errors.Is(err, services.ErrCSPUnsafeInline) ||
			errors.Is(err, services.ErrCSPNonceHashRequired) {
			return jsonError(ctx, response.CodeUnprocessableEntity, err.Error())
		}
		if errors.Is(err, services.ErrInvalidCredentials) {
			return jsonError(ctx, response.CodeUnprocessableEntity, "用户名或密码错误")
		}
		if errors.Is(err, services.ErrUserDisabled) {
			return jsonError(ctx, response.CodeDisabled, "用户已停用")
		}
		return jsonError(ctx, response.CodeFail, "服务器错误")
	}
	if data == nil {
		return ctx.Response().Json(http.StatusOK, response.SuccessEmpty())
	}
	return ctx.Response().Json(http.StatusOK, response.Success(data))
}

func jsonError(ctx contractshttp.Context, code int, message string) contractshttp.Response {
	ctx.WithContext(services.WithAuditOutcome(ctx.Context(), services.AuditOutcomeFailure))
	return ctx.Response().Json(http.StatusOK, response.Error(code, message, []any{}))
}

func bindIDList(ctx contractshttp.Context) ([]uint64, error) {
	var ids []uint64
	if err := bindJSONBody(ctx, &ids); err != nil {
		return nil, err
	}
	if ids == nil {
		return make([]uint64, 0), nil
	}
	return ids, nil
}

func bindIDsObject(ctx contractshttp.Context) ([]uint64, error) {
	var req struct {
		IDs []uint64 `json:"ids"`
	}
	if err := bindJSONBody(ctx, &req); err != nil {
		return nil, err
	}
	if req.IDs == nil {
		return make([]uint64, 0), nil
	}
	return req.IDs, nil
}

func bindJSONBody(ctx contractshttp.Context, dest any) error {
	body, err := io.ReadAll(ctx.Request().Origin().Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, dest)
}

func queryFilters(ctx contractshttp.Context) map[string]string {
	return map[string]string{
		"username":      ctx.Request().Query("username"),
		"nickname":      ctx.Request().Query("nickname"),
		"phone":         ctx.Request().Query("phone"),
		"email":         ctx.Request().Query("email"),
		"status":        ctx.Request().Query("status"),
		"action":        ctx.Request().Query("action"),
		"owner":         ctx.Request().Query("owner"),
		"run_key":       ctx.Request().Query("run_key"),
		"module_id":     ctx.Request().Query("module_id"),
		"method":        ctx.Request().Query("method"),
		"router":        ctx.Request().Query("router"),
		"message":       ctx.Request().Query("message"),
		"ip":            ctx.Request().Query("ip"),
		"os":            ctx.Request().Query("os"),
		"browser":       ctx.Request().Query("browser"),
		"remark":        ctx.Request().Query("remark"),
		"suffix":        ctx.Request().Query("suffix"),
		"mime_type":     ctx.Request().Query("mime_type"),
		"storage_mode":  ctx.Request().Query("storage_mode"),
		"origin_name":   ctx.Request().Query("origin_name"),
		"object_name":   ctx.Request().Query("object_name"),
		"hash":          ctx.Request().Query("hash"),
		"storage_path":  ctx.Request().Query("storage_path"),
		"name":          ctx.Request().Query("name"),
		"title":         ctx.Request().Query("title"),
		"label":         ctx.Request().Query("label"),
		"value":         ctx.Request().Query("value"),
		"display_name":  ctx.Request().Query("display_name"),
		"code":          ctx.Request().Query("code"),
		"plan":          ctx.Request().Query("plan"),
		"level":         ctx.Request().Query("level"),
		"scene":         ctx.Request().Query("scene"),
		"type":          ctx.Request().Query("type"),
		"enabled":       ctx.Request().Query("enabled"),
		"dept_id":       ctx.Request().Query("dept_id"),
		"user_id":       ctx.Request().Query("user_id"),
		"service_name":  ctx.Request().Query("service_name"),
		"provider_id":   ctx.Request().Query("provider_id"),
		"provider_name": ctx.Request().Query("provider_name"),
		"provider":      ctx.Request().Query("provider"),
		"version":       ctx.Request().Query("version"),
		"driver":        ctx.Request().Query("driver"),
		"task_id":       ctx.Request().Query("task_id"),
		"task_code":     ctx.Request().Query("task_code"),
		"task_type":     ctx.Request().Query("task_type"),
		"trigger_mode":  ctx.Request().Query("trigger_mode"),
		"sso_user_id":   ctx.Request().Query("sso_user_id"),
		"sso_email":     ctx.Request().Query("sso_email"),
		"sso_username":  ctx.Request().Query("sso_username"),
		"start_date":    ctx.Request().Query("start_date"),
		"end_date":      ctx.Request().Query("end_date"),
	}
}

func page(ctx contractshttp.Context) int {
	value := ctx.Request().QueryInt("page", 1)
	if value < 1 {
		return 1
	}
	return value
}

func pageSize(ctx contractshttp.Context) int {
	value := ctx.Request().QueryInt("per_page", 15)
	if ctx.Request().Query("per_page") == "" {
		value = ctx.Request().QueryInt("page_size", value)
	}
	if value < 1 {
		return 15
	}
	return value
}

func currentTenant(ctx contractshttp.Context) (services.Tenant, error) {
	tenant, ok := services.CurrentTenant(ctx.Context())
	if !ok {
		return services.Tenant{}, services.ErrUnauthorized
	}
	return tenant, nil
}

func tenantPassport(ctx contractshttp.Context) (*services.PassportService, error) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return nil, err
	}
	return services.NewPassportServiceForTenant(tenant).WithContext(ctx.Context()), nil
}

func tenantPermissionService(ctx contractshttp.Context) (*services.PermissionAdminService, error) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return nil, err
	}
	return services.NewPermissionAdminServiceForTenant(tenant).WithContext(ctx.Context()), nil
}

func tenantOrgService(ctx contractshttp.Context) (*services.OrgAdminService, error) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return nil, err
	}
	return services.NewOrgAdminServiceForTenant(tenant).WithContext(ctx.Context()), nil
}

func tenantAttachmentService(ctx contractshttp.Context) (*services.AttachmentService, error) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return nil, err
	}
	return services.NewAttachmentServiceForTenant(tenant).WithContext(ctx.Context()), nil
}

func tenantLogService(ctx contractshttp.Context) (*services.LogAdminService, error) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return nil, err
	}
	return services.NewLogAdminServiceForTenant(tenant).WithContext(ctx.Context()), nil
}

func tenantSSOProviderService(ctx contractshttp.Context) (*services.SSOProviderService, error) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return nil, err
	}
	return services.NewSSOProviderServiceForTenant(tenant).WithContext(ctx.Context()), nil
}

func tenantSSOAuditService(ctx contractshttp.Context) (*services.SSOAuditService, error) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return nil, err
	}
	return services.NewSSOAuditServiceForTenant(tenant).WithContext(ctx.Context()), nil
}

func tenantDictionaryService(ctx contractshttp.Context) (*services.TenantDictionaryService, error) {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return nil, err
	}
	return services.NewTenantDictionaryServiceForTenant(tenant).WithContext(ctx.Context()), nil
}

func tenantCurrentUser(ctx contractshttp.Context) (services.UserInfo, error) {
	user, err := tenantCurrentUserModel(ctx)
	if err != nil {
		return services.UserInfo{}, err
	}
	return services.UserInfo{ID: user.ID, Username: user.Username}, nil
}

func tenantCurrentUserModel(ctx contractshttp.Context) (models.User, error) {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return models.User{}, err
	}
	return passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
}

func platformCurrentUser(ctx contractshttp.Context) (services.UserInfo, error) {
	user, err := platformCurrentUserModel(ctx)
	if err != nil {
		return services.UserInfo{}, err
	}
	return services.UserInfo{ID: user.ID, Username: user.Username}, nil
}

func platformCurrentUserModel(ctx contractshttp.Context) (models.User, error) {
	return services.NewPlatformPassportService().WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
}
