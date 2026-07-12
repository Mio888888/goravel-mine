package admin

import (
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
)

type SSOUserBindingController struct{}

func NewSSOUserBindingController() *SSOUserBindingController {
	return &SSOUserBindingController{}
}

func (r *SSOUserBindingController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantSSOAuditService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.ListBindings(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *SSOUserBindingController) Detail(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantSSOAuditService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.Binding(id)
	return jsonResult(ctx, result, err)
}

func (r *SSOUserBindingController) UserBindings(ctx contractshttp.Context) contractshttp.Response {
	userID, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantSSOAuditService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.UserBindings(userID)
	return jsonResult(ctx, result, err)
}

func (r *SSOUserBindingController) Unbind(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantSSOAuditService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	return jsonResult(ctx, nil, service.DeleteBinding(id))
}

type SSOLoginLogController struct{}

func NewSSOLoginLogController() *SSOLoginLogController {
	return &SSOLoginLogController{}
}

func (r *SSOLoginLogController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantSSOAuditService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.ListLoginLogs(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *SSOLoginLogController) Stats(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantSSOAuditService(ctx)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	result, err := service.LoginStats(queryFilters(ctx))
	return jsonResult(ctx, result, err)
}
