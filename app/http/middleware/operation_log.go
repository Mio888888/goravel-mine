package middleware

import (
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/services"
)

func OperationLog() contractshttp.Middleware {
	return func(ctx contractshttp.Context) {
		tenant, ok := services.CurrentTenant(ctx.Context())
		if !ok {
			ctx.Request().Next()
			return
		}
		passport := services.NewPassportServiceForTenant(tenant).WithContext(ctx.Context())
		method := strings.ToUpper(ctx.Request().Method())
		path := ctx.Request().Path()
		route := ctx.Request().OriginPath()
		defer func() {
			if recovered := recover(); recovered != nil {
				markAuditPanic(ctx)
				recordTenantOperationAudit(ctx, tenant, passport, method, route, path)
				panic(recovered)
			}
			recordTenantOperationAudit(ctx, tenant, passport, method, route, path)
		}()
		ctx.Request().Next()
	}
}

func recordTenantOperationAudit(ctx contractshttp.Context, tenant services.Tenant, passport *services.PassportService, method, route, path string) {
	if shouldSkipOperationLog(method, route) {
		return
	}
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return
	}
	payload := services.OperationLogPayload{
		Username:    user.Username,
		Method:      method,
		Router:      path,
		ServiceName: operationServiceName(method, route),
		IP:          ctx.Request().Ip(),
		Connection:  services.TenantConnectionName(tenant),
	}
	services.DispatchOperationLog(payload)
	recordTenantAuditEvent(ctx, tenant, payload.Username, payload.ServiceName, method, route, path, payload.IP)
}

func shouldSkipOperationLog(method, route string) bool {
	return !shouldRecordAudit(method, route) ||
		strings.HasPrefix(route, "/admin/user-login-log") ||
		strings.HasPrefix(route, "/admin/user-operation-log") ||
		strings.HasPrefix(route, "/admin/passport/")
}

func operationServiceName(method, route string) string {
	if name := permissionOperationNames()[method+" "+route]; name != "" {
		return name
	}
	if name := orgOperationNames()[method+" "+route]; name != "" {
		return name
	}
	return "后台操作"
}

func permissionOperationNames() map[string]string {
	return map[string]string{
		"POST /admin/security/mfa/setup":   "设置 MFA",
		"POST /admin/security/mfa/confirm": "确认 MFA",
		"POST /admin/security/mfa/disable": "关闭 MFA",
		"POST /admin/permission/update":    "更新个人资料",
		"PUT /admin/user/info":             "更新个人资料",
		"POST /admin/user":                 "创建用户",
		"PUT /admin/user/{id}":             "更新用户",
		"DELETE /admin/user":               "删除用户",
		"PUT /admin/user/password":         "初始化用户密码",
		"PUT /admin/user/{id}/roles":       "设置用户角色",
		"POST /admin/role":                 "创建角色",
		"PUT /admin/role/{id}":             "更新角色",
		"DELETE /admin/role":               "删除角色",
		"PUT /admin/role/{id}/permissions": "设置角色权限",
		"POST /admin/menu":                 "创建菜单",
		"PUT /admin/menu/{id}":             "更新菜单",
		"DELETE /admin/menu":               "删除菜单",
		"POST /admin/attachment/upload":    "上传附件",
		"DELETE /admin/attachment/{id}":    "删除附件",
	}
}

func orgOperationNames() map[string]string {
	return map[string]string{
		"POST /admin/department":                   "创建部门",
		"PUT /admin/department/{id}":               "更新部门",
		"DELETE /admin/department":                 "删除部门",
		"POST /admin/position":                     "创建岗位",
		"PUT /admin/position/{id}":                 "更新岗位",
		"PUT /admin/position/{id}/data_permission": "设置数据权限",
		"DELETE /admin/position":                   "删除岗位",
		"POST /admin/leader":                       "创建领导",
		"PUT /admin/leader/{id}":                   "更新领导",
		"DELETE /admin/leader":                     "删除领导",
	}
}
