package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/models"
	"goravel/app/services"
)

type PermissionController struct {
	passport *services.PassportService
}

type profileUpdateRequest struct {
	Nickname                string         `json:"nickname"`
	Avatar                  string         `json:"avatar"`
	Signed                  string         `json:"signed"`
	BackendSetting          models.JSONMap `json:"backend_setting"`
	OldPassword             string         `json:"old_password"`
	NewPassword             string         `json:"new_password"`
	NewPasswordConfirmation string         `json:"new_password_confirmation"`
}

type MenuItem struct {
	ID        uint64         `gorm:"column:id" json:"id"`
	ParentID  uint64         `gorm:"column:parent_id" json:"parent_id"`
	Name      string         `gorm:"column:name" json:"name"`
	Meta      models.JSONMap `gorm:"column:meta;type:jsonb" json:"meta"`
	Path      string         `gorm:"column:path" json:"path"`
	Component string         `gorm:"column:component" json:"component"`
	Redirect  string         `gorm:"column:redirect" json:"redirect"`
	Status    int8           `gorm:"column:status" json:"status"`
	Sort      int16          `gorm:"column:sort" json:"sort"`
	Remark    string         `gorm:"column:remark" json:"remark"`
	Children  []MenuItem     `gorm:"-" json:"children"`
}

func NewPermissionController() *PermissionController {
	return &PermissionController{
		passport: services.NewPassportService(),
	}
}

func (r *PermissionController) Menus(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}

	tenant, err := currentTenant(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	menus, err := r.currentMenus(tenant, passport, user)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}

	return ctx.Response().Json(http.StatusOK, response.Success(buildMenuTree(menus, 0)))
}

func (r *PermissionController) Roles(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
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
		Table("role").
		Select("role.id", "role.code", "role.name").
		Where("role.status", 1).
		OrderBy("role.sort").
		OrderBy("role.id")
	if !isSuperAdmin {
		query = query.
			Join("JOIN user_belongs_role ubr ON ubr.role_id = role.id").
			Where("ubr.user_id", user.ID)
	}

	if err := query.Scan(&roles); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}

	return ctx.Response().Json(http.StatusOK, response.Success(roles))
}

func (r *PermissionController) Update(ctx contractshttp.Context) contractshttp.Response {
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

func (r *PermissionController) currentMenus(tenant services.Tenant, passport *services.PassportService, user models.User) ([]MenuItem, error) {
	menus := make([]MenuItem, 0)
	query := passport.Orm().Query().
		Table("menu").
		Select(
			"menu.id", "menu.parent_id", "menu.name", "menu.meta", "menu.path",
			"menu.component", "menu.redirect", "menu.status", "menu.sort", "menu.remark",
		).
		Where("menu.status", 1).
		OrderBy("menu.sort").
		OrderBy("menu.id")

	isSuperAdmin, err := passport.IsSuperAdmin(user)
	if err != nil {
		return nil, err
	}
	if !isSuperAdmin {
		query = query.
			Distinct("menu.id", "menu.parent_id", "menu.name", "menu.meta", "menu.path",
				"menu.component", "menu.redirect", "menu.status", "menu.sort", "menu.remark").
			Join("JOIN role_belongs_menu rbm ON rbm.menu_id = menu.id").
			Join("JOIN user_belongs_role ubr ON ubr.role_id = rbm.role_id").
			Join("JOIN role ON role.id = ubr.role_id").
			Where("ubr.user_id", user.ID).
			Where("role.status", 1)
	}

	if err := query.Scan(&menus); err != nil {
		return nil, err
	}

	return filterMenuItemsByTenantPermissions(tenant, menus), nil
}

func filterMenuItemsByTenantPermissions(tenant services.Tenant, menus []MenuItem) []MenuItem {
	if services.TenantPermissionSnapshotFromFeatures(tenant.Features).LegacyFullAccess {
		return menus
	}
	byID := make(map[uint64]MenuItem, len(menus))
	allowed := make(map[uint64]struct{})
	for _, menu := range menus {
		byID[menu.ID] = menu
		if services.TenantAllowsPermission(tenant, menu.Name) {
			allowed[menu.ID] = struct{}{}
		}
	}
	for id := range allowed {
		parentID := byID[id].ParentID
		for parentID != 0 {
			parent, ok := byID[parentID]
			if !ok {
				break
			}
			allowed[parent.ID] = struct{}{}
			parentID = parent.ParentID
		}
	}
	filtered := make([]MenuItem, 0, len(allowed))
	for _, menu := range menus {
		if _, ok := allowed[menu.ID]; ok {
			filtered = append(filtered, menu)
		}
	}
	return filtered
}

func buildMenuTree(menus []MenuItem, parentID uint64) []MenuItem {
	tree := make([]MenuItem, 0)
	for _, menu := range menus {
		if menu.ParentID != parentID {
			continue
		}

		if menu.Meta == nil {
			menu.Meta = models.JSONMap{}
		}
		normalizeMenuComponent(&menu)
		menu.Children = buildMenuTree(menus, menu.ID)
		tree = append(tree, menu)
	}

	return tree
}

func normalizeMenuComponent(menu *MenuItem) {
	switch menu.Name {
	case "permission:role:getMenu":
		menu.Meta["i18n"] = "baseMenu.permission.getRolePermission"
	case "permission:role:setMenu":
		menu.Meta["i18n"] = "baseMenu.permission.setRolePermission"
	case "permission:department:update":
		menu.Meta["i18n"] = "baseMenu.permission.departmentSave"
	case "permission:position":
		menu.Component = "base/views/permission/department/index"
		menu.Meta["i18n"] = "baseMenu.permission.positionList"
		menu.Meta["cache"] = 0
	case "permission:position:update":
		menu.Meta["i18n"] = "baseMenu.permission.positionSave"
	case "permission:position:data_permission":
		menu.Meta["i18n"] = "baseMenu.permission.positionDataScope"
	case "permission:leader":
		menu.Component = "base/views/permission/department/index"
		menu.Meta["i18n"] = "baseMenu.permission.leaderList"
		menu.Meta["cache"] = 0
	case "log:userLogin":
		menu.Component = "base/views/log/userLogin"
		menu.Meta["i18n"] = "baseMenu.log.userLoginLog"
	case "log:userOperation":
		menu.Component = "base/views/log/userOperation"
		menu.Meta["i18n"] = "baseMenu.log.operationLog"
	}
}
