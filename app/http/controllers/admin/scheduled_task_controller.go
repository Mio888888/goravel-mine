package admin

import (
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/models"
	"goravel/app/services"
)

type ScheduledTaskController struct {
	service *services.ScheduledTaskService
}

func NewScheduledTaskController() *ScheduledTaskController {
	return &ScheduledTaskController{service: services.NewScheduledTaskService()}
}

func (r *ScheduledTaskController) List(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).List(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *ScheduledTaskController) Detail(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	task, err := r.service.WithContext(ctx.Context()).Detail(id)
	return jsonResult(ctx, task, err)
}

func (r *ScheduledTaskController) TenantOptions(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).TenantOptions()
	return jsonResult(ctx, result, err)
}

func (r *ScheduledTaskController) Create(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUserModel(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req services.ScheduledTaskPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	if res := r.authorizeConfiguration(ctx, user, req); res != nil {
		return res
	}
	task, err := r.service.WithContext(ctx.Context()).Create(req, user.ID)
	return jsonResult(ctx, task, err)
}

func (r *ScheduledTaskController) Update(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	user, err := platformCurrentUserModel(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req services.ScheduledTaskPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	if res := r.authorizeConfiguration(ctx, user, req); res != nil {
		return res
	}
	task, err := r.service.WithContext(ctx.Context()).Update(id, req, user.ID)
	return jsonResult(ctx, task, err)
}

func (r *ScheduledTaskController) Delete(ctx contractshttp.Context) contractshttp.Response {
	ids, err := bindIDList(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).Delete(ids))
}

func (r *ScheduledTaskController) Enable(ctx contractshttp.Context) contractshttp.Response {
	return r.setStatus(ctx, true)
}

func (r *ScheduledTaskController) Disable(ctx contractshttp.Context) contractshttp.Response {
	return r.setStatus(ctx, false)
}

func (r *ScheduledTaskController) Run(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	log, err := r.service.WithContext(ctx.Context()).ManualRun(ctx.Context(), id)
	return jsonResult(ctx, log, err)
}

func (r *ScheduledTaskController) Logs(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).Logs(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *ScheduledTaskController) setStatus(ctx contractshttp.Context, enabled bool) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	if enabled {
		task, err := r.service.WithContext(ctx.Context()).Enable(id, user.ID)
		return jsonResult(ctx, task, err)
	}
	task, err := r.service.WithContext(ctx.Context()).Disable(id, user.ID)
	return jsonResult(ctx, task, err)
}

func (r *ScheduledTaskController) authorizeConfiguration(ctx contractshttp.Context, user models.User, req services.ScheduledTaskPayload) contractshttp.Response {
	task, err := req.ScheduledTask()
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	if task.TaskType != services.ScheduledTaskTypeScript &&
		task.TaskType != services.ScheduledTaskTypeBackup &&
		!services.ScheduledTaskUsesPrivilegedHandler(task.TaskType, task.Payload) {
		return nil
	}
	ok, err := services.NewPlatformPassportService().WithContext(ctx.Context()).IsSuperAdmin(user)
	if err != nil {
		return jsonError(ctx, response.CodeFail, "服务器错误")
	}
	if !ok {
		return jsonError(ctx, response.CodeForbidden, "仅平台超级管理员可配置脚本或备份任务")
	}
	return nil
}
