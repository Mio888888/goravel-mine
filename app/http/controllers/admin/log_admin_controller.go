package admin

import (
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/services"
)

type UserLoginLogController struct {
	service *services.LogAdminService
}

func NewUserLoginLogController() *UserLoginLogController {
	return &UserLoginLogController{service: services.NewLogAdminService()}
}

func (r *UserLoginLogController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantLogService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.ListLoginLogs(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

type UserOperationLogController struct {
	service *services.LogAdminService
}

func NewUserOperationLogController() *UserOperationLogController {
	return &UserOperationLogController{service: services.NewLogAdminService()}
}

func (r *UserOperationLogController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantLogService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.ListOperationLogs(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}
