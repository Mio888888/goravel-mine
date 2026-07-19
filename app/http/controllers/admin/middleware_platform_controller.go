package admin

import (
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type MiddlewarePlatformController struct {
	service           *services.MiddlewarePlatformService
	protectionService *services.ProtectionRuleSetService
}

type middlewareRouteVersionRequest struct {
	Version int `json:"version"`
}

type middlewareAdapterVersionRequest struct {
	Version int  `json:"version"`
	Confirm bool `json:"confirm"`
}

type protectionRollbackRequest struct {
	Version       int `json:"version"`
	TargetVersion int `json:"target_version"`
}

func NewMiddlewarePlatformController() *MiddlewarePlatformController {
	return &MiddlewarePlatformController{
		service:           services.NewMiddlewarePlatformService(),
		protectionService: services.NewProtectionRuleSetService(),
	}
}

func (r *MiddlewarePlatformController) Registry(ctx contractshttp.Context) contractshttp.Response {
	return jsonResult(ctx, r.service.Registry(), nil)
}

func (r *MiddlewarePlatformController) Adapters(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).Adapters()
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) AdapterDetail(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).Adapter(id)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) CreateAdapter(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req services.AdapterPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).RegisterConfiguredAdapter(req, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) UpdateAdapter(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req services.AdapterPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).UpdateAdapter(id, req, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) CheckAdapterHealth(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).CheckAdapterHealth(id)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) TestAdapterConnection(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).TestAdapterConnection(id)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) EnableAdapter(ctx contractshttp.Context) contractshttp.Response {
	return r.setAdapterEnabled(ctx, true)
}

func (r *MiddlewarePlatformController) DisableAdapter(ctx contractshttp.Context) contractshttp.Response {
	return r.setAdapterEnabled(ctx, false)
}

func (r *MiddlewarePlatformController) ReplaceAdapterConfig(ctx contractshttp.Context) contractshttp.Response {
	id, version, _, _, res := middlewareAdapterVersionedRequest(ctx)
	if res != nil {
		return res
	}
	err := r.service.WithContext(ctx.Context()).ReplaceAdapterConfig(id, version)
	return jsonResult(ctx, nil, err)
}

func (r *MiddlewarePlatformController) Routes(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).Routes(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) RouteDetail(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).Route(id)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) CreateRoute(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req services.RoutePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).CreateRoute(req, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) UpdateRoute(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req services.RoutePayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).UpdateRoute(id, req, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) ValidateRoute(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).ValidateRoute(id)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) PublishRoute(ctx contractshttp.Context) contractshttp.Response {
	id, version, userID, res := middlewareVersionedRequest(ctx)
	if res != nil {
		return res
	}
	key, res := middlewareIdempotencyKey(ctx)
	if res != nil {
		return res
	}
	result, err := r.service.WithContext(ctx.Context()).PublishRouteIdempotent(id, version, userID, key)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) EnableRoute(ctx contractshttp.Context) contractshttp.Response {
	return r.setRouteEnabled(ctx, true)
}

func (r *MiddlewarePlatformController) DisableRoute(ctx contractshttp.Context) contractshttp.Response {
	return r.setRouteEnabled(ctx, false)
}

func (r *MiddlewarePlatformController) Deliveries(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).Deliveries(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) DeadLetters(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.WithContext(ctx.Context()).DeadLetters(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) DeadLetterDetail(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.service.WithContext(ctx.Context()).DeadLetter(id)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) ReplayDeadLetter(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	key := strings.TrimSpace(ctx.Request().Header("Idempotency-Key", ""))
	result, err := r.service.WithContext(ctx.Context()).ReplayDeadLetter(id, key)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) ResolveDeadLetter(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	result, err := r.service.WithContext(ctx.Context()).ResolveDeadLetter(id, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) ProtectionRules(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.protectionService.WithContext(ctx.Context()).
		List(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) ProtectionRuleDetail(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.protectionService.WithContext(ctx.Context()).Find(id)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) CreateProtectionRule(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req services.ProtectionRuleSetPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.protectionService.WithContext(ctx.Context()).Create(req, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) UpdateProtectionRule(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req services.ProtectionRuleSetPayload
	if err := ctx.Request().Bind(&req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.protectionService.WithContext(ctx.Context()).Update(id, req, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) DeleteProtectionRule(ctx contractshttp.Context) contractshttp.Response {
	id, version, _, res := middlewareVersionedRequest(ctx)
	if res != nil {
		return res
	}
	err := r.protectionService.WithContext(ctx.Context()).Delete(id, version)
	return jsonResult(ctx, nil, err)
}

func (r *MiddlewarePlatformController) ValidateProtectionRule(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.protectionService.WithContext(ctx.Context()).Validate(id)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) PublishProtectionRule(ctx contractshttp.Context) contractshttp.Response {
	id, version, userID, res := middlewareVersionedRequest(ctx)
	if res != nil {
		return res
	}
	key, res := middlewareIdempotencyKey(ctx)
	if res != nil {
		return res
	}
	result, err := r.protectionService.WithContext(ctx.Context()).PublishIdempotent(id, version, userID, key)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) EnableProtectionRule(ctx contractshttp.Context) contractshttp.Response {
	return r.setProtectionRuleEnabled(ctx, true)
}

func (r *MiddlewarePlatformController) DisableProtectionRule(ctx contractshttp.Context) contractshttp.Response {
	return r.setProtectionRuleEnabled(ctx, false)
}

func (r *MiddlewarePlatformController) ProtectionRuleVersions(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.protectionService.WithContext(ctx.Context()).Versions(id)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) RollbackProtectionRule(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req protectionRollbackRequest
	if err := ctx.Request().Bind(&req); err != nil || req.Version < 1 || req.TargetVersion < 1 {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.protectionService.WithContext(ctx.Context()).
		Rollback(id, req.TargetVersion, req.Version, user.ID)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) ProtectionRuleState(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	result, err := r.protectionService.WithContext(ctx.Context()).State(id)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) ProtectionMetrics(ctx contractshttp.Context) contractshttp.Response {
	return jsonResult(ctx, services.ProtectionRuntimeMetrics(), nil)
}

func (r *MiddlewarePlatformController) Metrics(ctx contractshttp.Context) contractshttp.Response {
	return jsonResult(ctx, map[string]any{
		"message":    services.MiddlewareRuntimeMetrics(ctx.Context()),
		"outbox":     services.QueueBacklogMetrics(ctx.Context()),
		"protection": services.ProtectionRuntimeMetrics(),
	}, nil)
}

func (r *MiddlewarePlatformController) setAdapterEnabled(ctx contractshttp.Context, enabled bool) contractshttp.Response {
	id, version, confirm, userID, res := middlewareAdapterVersionedRequest(ctx)
	if res != nil {
		return res
	}
	result, err := r.service.WithContext(ctx.Context()).
		SetAdapterEnabled(id, enabled, version, confirm, userID)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) setRouteEnabled(ctx contractshttp.Context, enabled bool) contractshttp.Response {
	id, version, userID, res := middlewareVersionedRequest(ctx)
	if res != nil {
		return res
	}
	result, err := r.service.WithContext(ctx.Context()).SetRouteEnabled(id, enabled, version, userID)
	return jsonResult(ctx, result, err)
}

func (r *MiddlewarePlatformController) setProtectionRuleEnabled(ctx contractshttp.Context, enabled bool) contractshttp.Response {
	id, version, userID, res := middlewareVersionedRequest(ctx)
	if res != nil {
		return res
	}
	result, err := r.protectionService.WithContext(ctx.Context()).
		SetEnabled(id, enabled, version, userID)
	return jsonResult(ctx, result, err)
}

func middlewareVersionedRequest(ctx contractshttp.Context) (uint64, int, uint64, contractshttp.Response) {
	id, err := routeID(ctx)
	if err != nil {
		return 0, 0, 0, jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return 0, 0, 0, jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req middlewareRouteVersionRequest
	if err := ctx.Request().Bind(&req); err != nil || req.Version < 1 {
		return 0, 0, 0, jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return id, req.Version, user.ID, nil
}

func middlewareIdempotencyKey(ctx contractshttp.Context) (string, contractshttp.Response) {
	key := strings.TrimSpace(ctx.Request().Header("Idempotency-Key", ""))
	if key == "" {
		return "", jsonError(ctx, response.CodeUnprocessableEntity, "Idempotency-Key 不能为空")
	}
	return key, nil
}

func middlewareAdapterVersionedRequest(ctx contractshttp.Context) (uint64, int, bool, uint64, contractshttp.Response) {
	id, err := routeID(ctx)
	if err != nil {
		return 0, 0, false, 0, jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return 0, 0, false, 0, jsonError(ctx, response.CodeUnauthorized, "未登录")
	}
	var req middlewareAdapterVersionRequest
	if err := ctx.Request().Bind(&req); err != nil || req.Version < 1 {
		return 0, 0, false, 0, jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	return id, req.Version, req.Confirm, user.ID, nil
}
