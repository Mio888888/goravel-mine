package admin

import (
	"errors"
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type PassportController struct {
	passport *services.PassportService
}

type loginEntry struct {
	Mode      string                       `json:"mode"`
	Available bool                         `json:"available"`
	Message   string                       `json:"message"`
	Tenant    *loginEntryTenant            `json:"tenant"`
	Config    *services.TenantPublicConfig `json:"config"`
}

type loginEntryTenant struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Status int8   `json:"status"`
}

type loginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	Code       string `json:"code"`
	CaptchaKey string `json:"captcha_key"`
	Key        string `json:"key"`
	MFAToken   string `json:"mfa_token"`
	MFACode    string `json:"mfa_code"`
}

type passwordChangeRequest struct {
	PasswordChangeToken     string `json:"password_change_token"`
	OldPassword             string `json:"old_password"`
	NewPassword             string `json:"new_password"`
	NewPasswordConfirmation string `json:"new_password_confirmation"`
}

type ssoAuthorizationRequest struct {
	Provider string `json:"provider"`
	Scene    string `json:"scene"`
}

type ssoCallbackRequest struct {
	TransactionID string `json:"transaction_id"`
	Code          string `json:"code"`
	State         string `json:"state"`
}

func NewPassportController() *PassportController {
	return &PassportController{
		passport: services.NewPassportService(),
	}
}

func (r *PassportController) Entry(ctx contractshttp.Context) contractshttp.Response {
	tenantService := services.NewTenantService().WithContext(ctx.Context())
	runtime := services.NewTenantRuntimeService().WithContext(ctx.Context())
	tenant, err := tenantService.FindByCustomDomain(requestHost(ctx))
	if err != nil {
		if !errors.Is(err, services.ErrTenantNotFound) {
			return ctx.Response().Json(http.StatusOK, response.Success(loginEntry{
				Mode:      "tenant",
				Available: false,
				Message:   "登录入口加载失败，请稍后重试",
				Tenant:    nil,
				Config:    nil,
			}))
		}
		return ctx.Response().Json(http.StatusOK, response.Success(loginEntry{
			Mode:      "platform",
			Available: true,
			Message:   "",
			Tenant:    nil,
			Config:    nil,
		}))
	}

	entry := loginEntry{
		Mode:      "tenant",
		Available: tenant.Status == services.TenantStatusActive,
		Message:   "",
		Tenant: &loginEntryTenant{
			Code:   tenant.Code,
			Name:   tenant.Name,
			Status: tenant.Status,
		},
	}
	if !entry.Available {
		entry.Message = "租户不存在或已停用"
		return ctx.Response().Json(http.StatusOK, response.Success(entry))
	}
	if err := runtime.EnsureSubscription(tenant); err != nil {
		entry.Available = false
		entry.Message = "租户订阅不可用"
		return ctx.Response().Json(http.StatusOK, response.Success(entry))
	}

	config := runtime.PublicConfig(tenant, ctx.Request().Query("scene", services.DefaultSSOScene))
	entry.Config = &config
	return ctx.Response().Json(http.StatusOK, response.Success(entry))
}

func (r *PassportController) Login(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req loginRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}

	if !services.NewCaptchaService().Verify(captchaKey(req), req.Code) {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "验证码错误", []any{}))
	}

	result, err := passport.Login(req.Username, req.Password, requestIP(ctx), requestUserAgent(ctx), "unknown")
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func (r *PassportController) MFALogin(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req loginRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	result, err := passport.CompleteMFALogin(req.MFAToken, req.MFACode, requestIP(ctx), requestUserAgent(ctx), "unknown")
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func (r *PassportController) PasswordChange(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req passwordChangeRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	result, err := passport.CompletePasswordChange(req.PasswordChangeToken, services.ProfileUpdate{
		OldPassword:             req.OldPassword,
		NewPassword:             req.NewPassword,
		NewPasswordConfirmation: req.NewPasswordConfirmation,
	}, requestIP(ctx), requestUserAgent(ctx), "unknown")
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func ssoAuthorizationError(ctx contractshttp.Context, err error) contractshttp.Response {
	switch {
	case errors.Is(err, services.ErrSSOAuthorizationTransactionExpired):
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "SSO 授权事务已过期", []any{}))
	case errors.Is(err, services.ErrSSOAuthorizationTransactionReused):
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "SSO 授权事务已使用", []any{}))
	case errors.Is(err, services.ErrSSOAuthorizationTransactionInvalid):
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "SSO 授权事务无效", []any{}))
	default:
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
}

func (r *PassportController) SSOLogin(ctx contractshttp.Context) contractshttp.Response {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req services.SSOLoginPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	result, err := services.NewTenantRuntimeService().WithContext(ctx.Context()).SSOLogin(
		tenant,
		req,
		ctx.Request().Ip(),
		ctx.Request().Header("User-Agent", "unknown"),
		"unknown",
	)
	if err != nil {
		return ssoAuthorizationError(ctx, err)
	}
	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func (r *PassportController) SSOAuthorize(ctx contractshttp.Context) contractshttp.Response {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req ssoAuthorizationRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	result, err := services.NewTenantRuntimeService().WithContext(ctx.Context()).StartSSOAuthorization(
		tenant,
		req.Provider,
		req.Scene,
	)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func (r *PassportController) SSOCallback(ctx contractshttp.Context) contractshttp.Response {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req ssoCallbackRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	result, err := services.NewTenantRuntimeService().WithContext(ctx.Context()).CompleteSSOAuthorization(
		tenant,
		req.TransactionID,
		req.Code,
		req.State,
		requestIP(ctx),
		requestUserAgent(ctx),
		"unknown",
	)
	if err != nil {
		return ssoAuthorizationError(ctx, err)
	}
	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func (r *PassportController) Branding(ctx contractshttp.Context) contractshttp.Response {
	tenant, err := currentTenant(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	config := services.NewTenantRuntimeService().WithContext(ctx.Context()).PublicConfig(tenant, ctx.Request().Query("scene", services.DefaultSSOScene))
	return ctx.Response().Json(http.StatusOK, response.Success(config))
}

func requestHost(ctx contractshttp.Context) string {
	if host := services.TrustedForwardedHost(ctx.Request().Header, ctx.Request().Origin().RemoteAddr); host != "" {
		return host
	}
	if host := ctx.Request().Header("Host", ""); host != "" {
		return host
	}
	return ctx.Request().Host()
}

func captchaKey(req loginRequest) string {
	if req.CaptchaKey != "" {
		return req.CaptchaKey
	}
	return req.Key
}

func requestIP(ctx contractshttp.Context) string {
	return ctx.Request().Ip()
}

func requestUserAgent(ctx contractshttp.Context) string {
	return ctx.Request().Header("User-Agent", "unknown")
}

func (r *PassportController) GetInfo(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	info, err := passport.FormatUserInfo(user)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}

	return ctx.Response().Json(http.StatusOK, response.Success(info))
}

func (r *PassportController) Logout(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	if err := passport.Logout(ctx.Request().Header("Authorization", "")); err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	return ctx.Response().Json(http.StatusOK, response.SuccessEmpty())
}

func (r *PassportController) Refresh(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	result, err := passport.Refresh(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	return ctx.Response().Json(http.StatusOK, response.Success(result))
}
