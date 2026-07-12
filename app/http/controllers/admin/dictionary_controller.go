package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type PlatformDictionaryController struct {
	passport *services.PlatformPassportService
	service  *services.PlatformDictionaryService
	tenant   *services.TenantService
}

func NewPlatformDictionaryController() *PlatformDictionaryController {
	return &PlatformDictionaryController{
		passport: services.NewPlatformPassportService(),
		service:  services.NewPlatformDictionaryService(),
		tenant:   services.NewTenantService(),
	}
}

func (r *PlatformDictionaryController) List(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).List(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *PlatformDictionaryController) Detail(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).Detail(id)
	return jsonResult(ctx, result, err)
}

func (r *PlatformDictionaryController) Options(ctx contractshttp.Context) contractshttp.Response {
	code := ctx.Request().Query("code")
	if code == "" {
		result, err := r.service.WithContext(ctx.Context()).AllOptions()
		return jsonResult(ctx, result, err)
	}
	result, err := r.service.WithContext(ctx.Context()).Options(code)
	return jsonResult(ctx, result, err)
}

func (r *PlatformDictionaryController) Create(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	var req services.PlatformDictTypePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).Create(req, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *PlatformDictionaryController) Update(ctx contractshttp.Context) contractshttp.Response {
	user, err := r.passport.WithContext(ctx.Context()).UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.PlatformDictTypePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).Update(id, req, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *PlatformDictionaryController) Delete(ctx contractshttp.Context) contractshttp.Response {
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).Delete(ids))
}

func (r *PlatformDictionaryController) DispatchAll(ctx contractshttp.Context) contractshttp.Response {
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).DispatchAllTenants())
}

func (r *PlatformDictionaryController) DispatchTenant(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	tenant, err := r.tenant.WithContext(ctx.Context()).FindByID(id)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).DispatchToTenant(tenant))
}

type TenantDictionaryController struct{}

func NewTenantDictionaryController() *TenantDictionaryController {
	return &TenantDictionaryController{}
}

func (r *TenantDictionaryController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantDictionaryService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.List(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *TenantDictionaryController) Items(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantDictionaryService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	items, err := service.Items(id)
	return jsonResult(ctx, items, err)
}

func (r *TenantDictionaryController) Options(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantDictionaryService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	code := ctx.Request().Query("code")
	if code == "" {
		result, err := service.AllOptions()
		return jsonResult(ctx, result, err)
	}
	result, err := service.Options(code)
	return jsonResult(ctx, result, err)
}

func (r *TenantDictionaryController) UpdateType(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantDictionaryService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	user, err := tenantCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.TenantDictTypePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := service.UpdateType(id, req, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *TenantDictionaryController) UpdateItem(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantDictionaryService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	user, err := tenantCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	var req services.TenantDictItemPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := service.UpdateItem(id, req, user.ID)
	return jsonResult(ctx, result, err)
}
