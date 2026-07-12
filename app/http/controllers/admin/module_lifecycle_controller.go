package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/modulecatalog"
	"goravel/app/services"
)

type ModuleLifecycleController struct {
	service *modulecatalog.AdminService
}

func NewModuleLifecycleController() *ModuleLifecycleController {
	return &ModuleLifecycleController{service: modulecatalog.NewDefaultAdminService()}
}

func (r *ModuleLifecycleController) State(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).State()
	return jsonResult(ctx, result, err)
}

func (r *ModuleLifecycleController) Runs(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).Runs(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *ModuleLifecycleController) Steps(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).Steps(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *ModuleLifecycleController) Locks(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).Locks()
	return jsonResult(ctx, result, err)
}

func (r *ModuleLifecycleController) StateDiff(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).StateDiff()
	return jsonResult(ctx, result, err)
}

func (r *ModuleLifecycleController) ReleaseStaleLocks(ctx contractshttp.Context) contractshttp.Response {
	var req modulecatalog.AdminLockReleasePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	if !req.DryRun {
		user, err := platformCurrentUserModel(ctx)
		if err != nil {
			return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
		}
		req.OperatorID = user.ID
	}
	result, err := r.service.WithContext(ctx.Context()).ReleaseStaleLocks(req)
	return jsonResult(ctx, result, err)
}

func (r *ModuleLifecycleController) Execute(ctx contractshttp.Context) contractshttp.Response {
	var req modulecatalog.AdminExecutePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	if req.Execute {
		user, err := platformCurrentUserModel(ctx)
		if err != nil {
			return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
		}
		req.OperatorID = user.ID
	}
	result, err := r.service.WithContext(ctx.Context()).Execute(req)
	return jsonResult(ctx, result, err)
}
