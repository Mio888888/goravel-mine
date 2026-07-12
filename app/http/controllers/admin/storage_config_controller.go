package admin

import (
	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type StorageConfigController struct {
	service *services.StorageConfigService
}

type storageConfigSensitiveRequest struct {
	services.StorageConfigPayload
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

func NewStorageConfigController() *StorageConfigController {
	return &StorageConfigController{service: services.NewStorageConfigService()}
}

func (r *StorageConfigController) List(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).List(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *StorageConfigController) Create(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req storageConfigSensitiveRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	config, err := r.service.WithContext(ctx.Context()).CreateSensitive(req.StorageConfigPayload, user.ID, sensitiveEvidence(req.ReAuthToken, req.ApprovalID))
	return jsonResult(ctx, config, err)
}

func (r *StorageConfigController) Update(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req storageConfigSensitiveRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	config, err := r.service.WithContext(ctx.Context()).UpdateSensitive(id, req.StorageConfigPayload, user.ID, sensitiveEvidence(req.ReAuthToken, req.ApprovalID))
	return jsonResult(ctx, config, err)
}

func (r *StorageConfigController) Delete(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req sensitiveDeleteRequest
	if err := bindJSONBody(ctx, &req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return jsonResult(ctx, nil, r.service.WithContext(ctx.Context()).DeleteSensitive(req.IDs, user.ID, sensitiveEvidence(req.ReAuthToken, req.ApprovalID)))
}
