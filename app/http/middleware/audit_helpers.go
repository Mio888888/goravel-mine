package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/models"
	"goravel/app/services"
)

type auditStatusContextKey struct{}

func shouldRecordAudit(method, route string) bool {
	if method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
		return false
	}
	if route == "" {
		return true
	}
	return !strings.Contains(route, "/passport/")
}

func auditOutcome(ctx contractshttp.Context, status int) string {
	outcome := services.AuditOutcome(ctx.Context())
	if status == 0 {
		status = http.StatusOK
	}
	code, hasBusinessCode := auditBusinessCode(ctx)
	return auditOutcomeFromSignals(status, outcome, code, hasBusinessCode)
}

func auditOutcomeFromSignals(status int, contextOutcome string, businessCode int, hasBusinessCode bool) string {
	if contextOutcome != "" {
		return contextOutcome
	}
	if status >= http.StatusBadRequest {
		return services.AuditOutcomeFailure
	}
	if hasBusinessCode && businessCode != response.CodeSuccess {
		return services.AuditOutcomeFailure
	}
	return services.AuditOutcomeSuccess
}

func auditBusinessCode(ctx contractshttp.Context) (int, bool) {
	response := ctx.Response()
	if response == nil {
		return 0, false
	}
	origin := response.Origin()
	if origin == nil {
		return 0, false
	}
	body := origin.Body()
	if body == nil || body.Len() == 0 {
		return 0, false
	}
	return auditBusinessCodeFromBody(body.Bytes())
}

func auditBusinessCodeFromBody(body []byte) (int, bool) {
	var result struct {
		Code *int `json:"code"`
	}
	if len(body) == 0 {
		return 0, false
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Code == nil {
		return 0, false
	}
	return *result.Code, true
}

func auditResponseStatus(ctx contractshttp.Context) int {
	if status, ok := ctx.Context().Value(auditStatusContextKey{}).(int); ok && status > 0 {
		return status
	}
	if ctx.Response() == nil || ctx.Response().Origin() == nil {
		return http.StatusOK
	}
	status := ctx.Response().Origin().Status()
	if status == 0 {
		return http.StatusOK
	}
	return status
}

func markAuditPanic(ctx contractshttp.Context) {
	next := services.WithAuditOutcome(ctx.Context(), services.AuditOutcomeFailure)
	next = context.WithValue(next, auditStatusContextKey{}, http.StatusInternalServerError)
	ctx.WithContext(next)
}

func recordPlatformAuditEvent(ctx contractshttp.Context, user models.User, method, route, path string) {
	if !shouldRecordAudit(method, route) {
		return
	}
	status := auditResponseStatus(ctx)
	services.RecordAuditEvent(ctx.Context(), services.AuditEvent{
		Action:  platformOperationName(method, route),
		Outcome: auditOutcome(ctx, status),
		Actor:   user.Username,
		Method:  method,
		Route:   route,
		Path:    path,
		IP:      ctx.Request().Ip(),
		Fields: map[string]any{
			"platform_user_id": user.ID,
			"status":           status,
		},
	})
}

func recordTenantAuditEvent(ctx contractshttp.Context, tenant services.Tenant, username, action, method, route, path, ip string) {
	status := auditResponseStatus(ctx)
	services.RecordAuditEvent(ctx.Context(), services.AuditEvent{
		Action:  action,
		Outcome: auditOutcome(ctx, status),
		Actor:   username,
		Method:  method,
		Route:   route,
		Path:    path,
		IP:      ip,
		Fields: map[string]any{
			"tenant_id":   tenant.ID,
			"tenant_code": tenant.Code,
			"status":      status,
		},
	})
}

func platformOperationName(method, route string) string {
	if name := platformOperationNames()[method+" "+route]; name != "" {
		return name
	}
	if permission := services.PlatformPermissionForRoute(method, route); permission != "" {
		return permission
	}
	return "平台操作"
}

func platformOperationNames() map[string]string {
	return map[string]string{
		"POST /admin/platform/security/mfa/setup":                "平台 MFA 设置",
		"POST /admin/platform/security/mfa/confirm":              "平台 MFA 确认",
		"POST /admin/platform/security/mfa/disable":              "平台 MFA 关闭",
		"POST /admin/platform/tenant-plan":                       "创建套餐",
		"PUT /admin/platform/tenant-plan/{id}":                   "更新套餐",
		"DELETE /admin/platform/tenant-plan":                     "删除套餐",
		"POST /admin/platform/tenant":                            "创建租户",
		"PUT /admin/platform/tenant/{id}":                        "更新租户",
		"PUT /admin/platform/tenant/{id}/permissions":            "更新租户权限",
		"POST /admin/platform/tenant/{id}/permissions/plan-diff": "查看套餐权限差异",
		"PUT /admin/platform/tenant/{id}/plan":                   "更新租户套餐",
		"PUT /admin/platform/tenant/{id}/suspend":                "暂停租户",
		"PUT /admin/platform/tenant/{id}/resume":                 "恢复租户",
		"PUT /admin/platform/tenant/{id}/archive":                "归档租户",
		"DELETE /admin/platform/tenant":                          "销毁租户",
		"POST /admin/platform/user":                              "创建平台用户",
		"PUT /admin/platform/user/{id}":                          "更新平台用户",
		"DELETE /admin/platform/user":                            "删除平台用户",
		"PUT /admin/platform/user/password":                      "重置平台用户密码",
		"PUT /admin/platform/user/{id}/roles":                    "设置平台用户角色",
		"POST /admin/platform/attachment/upload":                 "上传平台附件",
		"POST /admin/platform/role":                              "创建平台角色",
		"PUT /admin/platform/role/{id}":                          "更新平台角色",
		"DELETE /admin/platform/role":                            "删除平台角色",
		"PUT /admin/platform/role/{id}/permissions":              "设置平台角色权限",
		"POST /admin/platform/menu":                              "创建平台菜单",
		"PUT /admin/platform/menu/{id}":                          "更新平台菜单",
		"DELETE /admin/platform/menu":                            "删除平台菜单",
		"POST /admin/platform/dictionary":                        "创建默认字典",
		"PUT /admin/platform/dictionary/{id}":                    "更新默认字典",
		"DELETE /admin/platform/dictionary":                      "删除默认字典",
		"POST /admin/platform/dictionary/dispatch":               "分发默认字典",
		"POST /admin/platform/dictionary/dispatch/{id}":          "分发租户字典",
		"POST /admin/platform/storage-config":                    "创建储存配置",
		"PUT /admin/platform/storage-config/{id}":                "更新储存配置",
		"DELETE /admin/platform/storage-config":                  "删除储存配置",
		"POST /admin/platform/scheduled-task":                    "创建计划任务",
		"PUT /admin/platform/scheduled-task/{id}":                "更新计划任务",
		"DELETE /admin/platform/scheduled-task":                  "删除计划任务",
		"PUT /admin/platform/scheduled-task/{id}/enable":         "启用计划任务",
		"PUT /admin/platform/scheduled-task/{id}/disable":        "禁用计划任务",
		"POST /admin/platform/scheduled-task/{id}/run":           "执行计划任务",
		"POST /admin/platform/queue/failed-jobs/retry":           "重试失败队列任务",
		"DELETE /admin/platform/queue/failed-jobs":               "丢弃失败队列任务",
	}
}
