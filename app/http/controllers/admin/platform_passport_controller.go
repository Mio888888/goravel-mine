package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type PlatformPassportController struct {
	passport *services.PlatformPassportService
}

func NewPlatformPassportController() *PlatformPassportController {
	return &PlatformPassportController{
		passport: services.NewPlatformPassportService(),
	}
}

func (r *PlatformPassportController) Login(ctx contractshttp.Context) contractshttp.Response {
	var req loginRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}

	if !services.NewCaptchaService().Verify(captchaKey(req), req.Code) {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "验证码错误", []any{}))
	}

	passport := r.passport.WithContext(ctx.Context())
	result, err := passport.Login(req.Username, req.Password, services.LoginSignal{
		IP:        requestIP(ctx),
		UserAgent: requestUserAgent(ctx),
	})
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func (r *PlatformPassportController) MFALogin(ctx contractshttp.Context) contractshttp.Response {
	var req loginRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	result, err := r.passport.WithContext(ctx.Context()).CompleteMFALogin(req.MFAToken, req.MFACode, services.LoginSignal{
		IP:        requestIP(ctx),
		UserAgent: requestUserAgent(ctx),
	})
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func (r *PlatformPassportController) PasswordChange(ctx contractshttp.Context) contractshttp.Response {
	var req passwordChangeRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	result, err := r.passport.WithContext(ctx.Context()).CompletePasswordChange(req.PasswordChangeToken, services.ProfileUpdate{
		OldPassword:             req.OldPassword,
		NewPassword:             req.NewPassword,
		NewPasswordConfirmation: req.NewPasswordConfirmation,
	}, services.LoginSignal{
		IP:        requestIP(ctx),
		UserAgent: requestUserAgent(ctx),
	})
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func (r *PlatformPassportController) GetInfo(ctx contractshttp.Context) contractshttp.Response {
	passport := r.passport.WithContext(ctx.Context())
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

func (r *PlatformPassportController) Logout(ctx contractshttp.Context) contractshttp.Response {
	if err := r.passport.WithContext(ctx.Context()).Logout(ctx.Request().Header("Authorization", "")); err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	return ctx.Response().Json(http.StatusOK, response.SuccessEmpty())
}

func (r *PlatformPassportController) Refresh(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.passport.WithContext(ctx.Context()).Refresh(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	return ctx.Response().Json(http.StatusOK, response.Success(result))
}
