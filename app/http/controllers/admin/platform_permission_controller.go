package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/models"
	"goravel/app/services"
)

type PlatformPermissionController struct {
	passport *services.PlatformPassportService
}

func NewPlatformPermissionController() *PlatformPermissionController {
	return &PlatformPermissionController{
		passport: services.NewPlatformPassportService(),
	}
}

func (r *PlatformPermissionController) Menus(ctx contractshttp.Context) contractshttp.Response {
	passport := r.passport.WithContext(ctx.Context())
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	menus, err := r.currentMenus(passport, user)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}

	return ctx.Response().Json(http.StatusOK, response.Success(buildMenuTree(menus, 0)))
}

func (r *PlatformPermissionController) Roles(ctx contractshttp.Context) contractshttp.Response {
	passport := r.passport.WithContext(ctx.Context())
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	isSuperAdmin, err := passport.IsSuperAdmin(user)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}

	roles := make([]services.RoleInfo, 0)
	query := passport.Orm().Query().
		Table("platform_role").
		Select("platform_role.id", "platform_role.code", "platform_role.name").
		Where("platform_role.status", 1).
		OrderBy("platform_role.sort").
		OrderBy("platform_role.id")
	if !isSuperAdmin {
		query = query.
			Join("JOIN platform_user_belongs_role ubr ON ubr.role_id = platform_role.id").
			Where("ubr.user_id", user.ID)
	}

	if err := query.Scan(&roles); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}

	return ctx.Response().Json(http.StatusOK, response.Success(roles))
}

func (r *PlatformPermissionController) Update(ctx contractshttp.Context) contractshttp.Response {
	passport := r.passport.WithContext(ctx.Context())
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

func (r *PlatformPermissionController) currentMenus(passport *services.PlatformPassportService, user models.User) ([]MenuItem, error) {
	menus := make([]MenuItem, 0)
	query := passport.Orm().Query().
		Table("platform_menu").
		Select(
			"platform_menu.id", "platform_menu.parent_id", "platform_menu.name", "platform_menu.meta",
			"platform_menu.path", "platform_menu.component", "platform_menu.redirect",
			"platform_menu.status", "platform_menu.sort", "platform_menu.remark",
		).
		Where("platform_menu.status", 1).
		OrderBy("platform_menu.sort").
		OrderBy("platform_menu.id")

	isSuperAdmin, err := passport.IsSuperAdmin(user)
	if err != nil {
		return nil, err
	}
	if !isSuperAdmin {
		query = query.
			Distinct("platform_menu.id", "platform_menu.parent_id", "platform_menu.name", "platform_menu.meta",
				"platform_menu.path", "platform_menu.component", "platform_menu.redirect",
				"platform_menu.status", "platform_menu.sort", "platform_menu.remark").
			Join("JOIN platform_role_belongs_menu rbm ON rbm.menu_id = platform_menu.id").
			Join("JOIN platform_user_belongs_role ubr ON ubr.role_id = rbm.role_id").
			Join("JOIN platform_role ON platform_role.id = ubr.role_id").
			Where("ubr.user_id", user.ID).
			Where("platform_role.status", 1)
	}

	if err := query.Scan(&menus); err != nil {
		return nil, err
	}

	return menus, nil
}
