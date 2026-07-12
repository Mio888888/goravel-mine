package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type UserProfileController struct {
	passport *services.PassportService
}

func NewUserProfileController() *UserProfileController {
	return &UserProfileController{
		passport: services.NewPassportService(),
	}
}

func (r *UserProfileController) UpdateInfo(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	var req profileUpdateRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}

	if err := passport.UpdateProfile(user.ID, services.ProfileUpdate{
		Nickname:                req.Nickname,
		Avatar:                  req.Avatar,
		Signed:                  req.Signed,
		BackendSetting:          req.BackendSetting,
		OldPassword:             req.OldPassword,
		NewPassword:             req.NewPassword,
		NewPasswordConfirmation: req.NewPasswordConfirmation,
	}); err != nil {
		if services.IsProfileValidationError(err) {
			return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, services.ProfileValidationMessage(err), []any{}))
		}

		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}

	return ctx.Response().Json(http.StatusOK, response.SuccessEmpty())
}
