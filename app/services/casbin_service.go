package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/casbin/casbin/v3/model"
	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/models"
)

type CasbinService struct {
	ctx        context.Context
	passport   *PassportService
	connection string
}

func NewCasbinService() *CasbinService {
	return &CasbinService{
		passport: NewPassportService(),
	}
}

func NewCasbinServiceForTenant(tenant Tenant) *CasbinService {
	return &CasbinService{
		passport:   NewPassportServiceForTenant(tenant),
		connection: TenantConnectionName(tenant),
	}
}

func (s *CasbinService) WithContext(ctx context.Context) *CasbinService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	clone.passport = clone.passport.WithContext(clone.ctx)
	return &clone
}

func (s *CasbinService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func (s *CasbinService) Authorize(user models.User, method, path string) (bool, error) {
	if ok, err := s.passport.IsSuperAdmin(user); err != nil || ok {
		return ok, err
	}

	permission := PermissionForRoute(method, path)
	if permission == "" {
		return true, nil
	}

	enforcer, err := s.enforcer()
	if err != nil {
		return false, err
	}

	return enforcer.Enforce(fmt.Sprintf("user:%d", user.ID), permission, strings.ToUpper(method))
}

func (s *CasbinService) AuthorizeSensitiveEvidence(user models.User, policyKey, resource string) (bool, error) {
	if ok, err := s.passport.IsSuperAdmin(user); err != nil || ok {
		return ok, err
	}
	permission, action, err := NewSensitiveOperationPolicyRegistry().PermissionFor(policyKey, 1, resource)
	if err != nil {
		return false, nil
	}
	enforcer, err := s.enforcer()
	if err != nil {
		return false, err
	}
	return enforcer.Enforce(fmt.Sprintf("user:%d", user.ID), permission, action)
}

func (s *CasbinService) enforcer() (casbinAuthorizer, error) {
	return defaultCasbinEnforcerCache.Get("tenant:"+s.connection, s.loadEnforcer)
}

func (s *CasbinService) loadEnforcer() (casbinAuthorizer, error) {
	return loadCasbinEnforcer(s.orm().Query(), "casbin_rule")
}

func casbinModel() (model.Model, error) {
	path := facades.Config().GetString("casbin.model")
	if !filepath.IsAbs(path) {
		path = facades.App().BasePath(path)
	}

	return model.NewModelFromFile(path)
}

func PermissionForRoute(method, path string) string {
	return routePermissionMap()[strings.ToUpper(method)+" "+path]
}

func routePermissionMap() map[string]string {
	return map[string]string{
		"GET /admin/platform/tenant/list":                     "platform:tenant:list",
		"POST /admin/platform/tenant":                         "platform:tenant:save",
		"PUT /admin/platform/tenant/{id}":                     "platform:tenant:update",
		"PUT /admin/platform/tenant/{id}/suspend":             "platform:tenant:suspend",
		"PUT /admin/platform/tenant/{id}/resume":              "platform:tenant:resume",
		"PUT /admin/platform/tenant/{id}/archive":             "platform:tenant:archive",
		"DELETE /admin/platform/tenant":                       "platform:tenant:destroy",
		"GET /admin/user/list":                                "permission:user:index",
		"POST /admin/user":                                    "permission:user:save",
		"PUT /admin/user":                                     "permission:user:update",
		"PUT /admin/user/{id}":                                "permission:user:update",
		"PUT /admin/user/info":                                "permission:user:update",
		"DELETE /admin/user":                                  "permission:user:delete",
		"PUT /admin/user/password":                            "permission:user:password",
		"GET /admin/user/{id}/roles":                          "permission:user:getRole",
		"PUT /admin/user/{id}/roles":                          "permission:user:setRole",
		"GET /admin/role/list":                                "permission:role:index",
		"POST /admin/role":                                    "permission:role:save",
		"PUT /admin/role/{id}":                                "permission:role:update",
		"DELETE /admin/role":                                  "permission:role:delete",
		"GET /admin/role/{id}/permissions":                    "permission:role:getMenu",
		"PUT /admin/role/{id}/permissions":                    "permission:role:setMenu",
		"POST /admin/security/reauth-token":                   "security:mfa",
		"POST /admin/security/approvals":                      "security:mfa",
		"GET /admin/security/approvals/{approval_id}":         "security:mfa",
		"PUT /admin/security/approvals/{approval_id}/approve": "security:mfa",
		"GET /admin/menu/list":                                "permission:menu:index",
		"POST /admin/menu":                                    "permission:menu:create",
		"PUT /admin/menu/{id}":                                "permission:menu:save",
		"DELETE /admin/menu":                                  "permission:menu:delete",
		"GET /admin/sso-provider/list":                        "security:ssoProvider:list",
		"POST /admin/sso-provider":                            "security:ssoProvider:save",
		"PUT /admin/sso-provider/{id}":                        "security:ssoProvider:update",
		"DELETE /admin/sso-provider":                          "security:ssoProvider:delete",
		"GET /admin/sso-user-binding/list":                    "security:ssoUserBinding:list",
		"GET /admin/sso-user-binding/{id}":                    "security:ssoUserBinding:detail",
		"GET /admin/sso-user-binding/user/{id}":               "security:ssoUserBinding:user",
		"DELETE /admin/sso-user-binding/{id}":                 "security:ssoUserBinding:unbind",
		"GET /admin/department/list":                          "permission:department:index",
		"POST /admin/department":                              "permission:department:save",
		"PUT /admin/department/{id}":                          "permission:department:update",
		"DELETE /admin/department":                            "permission:department:delete",
		"GET /admin/position/list":                            "permission:position:index",
		"POST /admin/position":                                "permission:position:save",
		"PUT /admin/position/{id}":                            "permission:position:update",
		"DELETE /admin/position":                              "permission:position:delete",
		"PUT /admin/position/{id}/data_permission":            "permission:position:data_permission",
		"GET /admin/leader/list":                              "permission:leader:index",
		"POST /admin/leader":                                  "permission:leader:save",
		"PUT /admin/leader/{id}":                              "permission:leader:save",
		"DELETE /admin/leader":                                "permission:leader:delete",
		"GET /admin/attachment/list":                          "dataCenter:attachment:list",
		"POST /admin/attachment/upload":                       "dataCenter:attachment:upload",
		"DELETE /admin/attachment/{id}":                       "dataCenter:attachment:delete",
		"GET /admin/dictionary/list":                          "dataCenter:dictionary:list",
		"GET /admin/dictionary/{id}/items":                    "dataCenter:dictionary:list",
		"PUT /admin/dictionary/{id}":                          "dataCenter:dictionary:update",
		"PUT /admin/dictionary-item/{id}":                     "dataCenter:dictionary:update",
		"GET /admin/user-login-log/list":                      "log:userLogin:list",
		"GET /admin/user-operation-log/list":                  "log:userOperation:list",
		"GET /admin/sso-login-log/list":                       "log:ssoLogin:list",
		"GET /admin/sso-login-log/stats":                      "log:ssoLogin:stats",
	}
}
