package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v3"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	goravelgorm "github.com/goravel/framework/database/gorm"
	"gorm.io/gorm"

	"goravel/app/models"
)

type PlatformCasbinService struct {
	ctx      context.Context
	passport *PlatformPassportService
}

func NewPlatformCasbinService() *PlatformCasbinService {
	return &PlatformCasbinService{
		passport: NewPlatformPassportService(),
	}
}

func (s *PlatformCasbinService) WithContext(ctx context.Context) *PlatformCasbinService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	clone.passport = clone.passport.WithContext(clone.ctx)
	return &clone
}

func (s *PlatformCasbinService) Authorize(user models.User, method, path string) (bool, error) {
	if ok, err := s.passport.IsSuperAdmin(user); err != nil || ok {
		return ok, err
	}

	permissions := PlatformPermissionsForRoute(method, path)
	if len(permissions) == 0 {
		return false, nil
	}

	enforcer, err := s.enforcer()
	if err != nil {
		return false, err
	}

	subject := fmt.Sprintf("user:%d", user.ID)
	action := strings.ToUpper(method)
	for _, permission := range permissions {
		allowed, err := enforcer.Enforce(subject, permission, action)
		if err != nil || allowed {
			return allowed, err
		}
	}
	return false, nil
}

func (s *PlatformCasbinService) AuthorizeSensitiveEvidence(user models.User, policyKey, resource string) (bool, error) {
	permission, action, err := NewSensitiveOperationPolicyRegistry().PermissionFor(policyKey, 0, resource)
	if err != nil {
		return false, nil
	}
	if allowed, err := s.authorizePermission(user, "platform:security:control", "POST"); err != nil || allowed {
		return allowed, err
	}
	return s.authorizePermission(user, permission, action)
}

func (s *PlatformCasbinService) authorizePermission(user models.User, permission, action string) (bool, error) {
	if ok, err := s.passport.IsSuperAdmin(user); err != nil || ok {
		return ok, err
	}
	enforcer, err := s.enforcer()
	if err != nil {
		return false, err
	}
	return enforcer.Enforce(fmt.Sprintf("user:%d", user.ID), permission, action)
}

func (s *PlatformCasbinService) enforcer() (casbinAuthorizer, error) {
	return defaultCasbinEnforcerCache.Get("platform:"+PlatformConnection(), s.loadEnforcer)
}

func (s *PlatformCasbinService) loadEnforcer() (casbinAuthorizer, error) {
	query, ok := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query().(*goravelgorm.Query)
	if !ok {
		return nil, fmt.Errorf("unsupported orm query type")
	}

	db := query.Instance().Session(&gorm.Session{})
	gormadapter.TurnOffAutoMigrate(db)
	adapter, err := gormadapter.NewAdapterByDBUseTableName(db, "", "platform_casbin_rule")
	if err != nil {
		return nil, err
	}

	m, err := casbinModel()
	if err != nil {
		return nil, err
	}

	enforcer, err := casbin.NewSyncedEnforcer(m, adapter)
	if err != nil {
		return nil, err
	}
	enforcer.EnableAutoSave(false)

	if err := enforcer.LoadPolicy(); err != nil {
		return nil, err
	}

	return enforcer, nil
}

func PlatformPermissionForRoute(method, path string) string {
	permissions := PlatformPermissionsForRoute(method, path)
	if len(permissions) == 0 {
		return ""
	}
	return permissions[0]
}

func PlatformPermissionsForRoute(method, path string) []string {
	permissions := platformRoutePermissionMap()[strings.ToUpper(method)+" "+path]
	if len(permissions) == 0 {
		return nil
	}
	return permissions
}

func platformRoutePermissionMap() map[string][]string {
	p := platformRoutePermissions
	return map[string][]string{
		"GET /admin/platform/tenant-plan/list":                         p("platform:tenantPlan:list"),
		"GET /admin/platform/tenant-plan/options":                      p("platform:tenantPlan:list"),
		"POST /admin/platform/tenant-plan":                             p("platform:tenantPlan:save"),
		"PUT /admin/platform/tenant-plan/{id}":                         p("platform:tenantPlan:update"),
		"DELETE /admin/platform/tenant-plan":                           p("platform:tenantPlan:delete"),
		"GET /admin/platform/tenant/list":                              p("platform:tenant:list"),
		"GET /admin/platform/tenant/permission-catalog":                {"platform:tenant:permissions", "platform:tenantPlan:save", "platform:tenantPlan:update"},
		"GET /admin/platform/tenant/{id}/usage":                        p("platform:tenant:usage"),
		"GET /admin/platform/tenant/{id}/governance":                   {"platform:tenant:governance", "platform:tenant:destroy"},
		"PUT /admin/platform/tenant/{id}/governance":                   p("platform:tenant:governance"),
		"GET /admin/platform/tenant/{id}/permissions":                  p("platform:tenant:permissions"),
		"PUT /admin/platform/tenant/{id}/permissions":                  p("platform:tenant:permissions"),
		"POST /admin/platform/tenant/{id}/permissions/plan-diff":       p("platform:tenant:updatePlan"),
		"POST /admin/platform/tenant":                                  p("platform:tenant:save"),
		"PUT /admin/platform/tenant/{id}":                              p("platform:tenant:update"),
		"PUT /admin/platform/tenant/{id}/plan":                         p("platform:tenant:updatePlan"),
		"PUT /admin/platform/tenant/{id}/suspend":                      p("platform:tenant:suspend"),
		"PUT /admin/platform/tenant/{id}/resume":                       p("platform:tenant:resume"),
		"PUT /admin/platform/tenant/{id}/archive":                      p("platform:tenant:archive"),
		"DELETE /admin/platform/tenant":                                p("platform:tenant:destroy"),
		"POST /admin/platform/tenant/{id}/exports":                     p("platform:tenant:export"),
		"GET /admin/platform/tenant/{id}/exports/{run_id}":             p("platform:tenant:export"),
		"GET /admin/platform/tenant/{id}/exports/{run_id}/download":    p("platform:tenant:export"),
		"GET /admin/platform/user/list":                                p("platform:user:list"),
		"POST /admin/platform/user":                                    p("platform:user:save"),
		"PUT /admin/platform/user/{id}":                                p("platform:user:update"),
		"DELETE /admin/platform/user":                                  p("platform:user:delete"),
		"PUT /admin/platform/user/password":                            p("platform:user:password"),
		"GET /admin/platform/user/{id}/roles":                          p("platform:user:getRole"),
		"PUT /admin/platform/user/{id}/roles":                          p("platform:user:setRole"),
		"POST /admin/platform/attachment/upload":                       {"platform:attachment:upload", "platform:user:save", "platform:user:update"},
		"GET /admin/platform/role/list":                                p("platform:role:list"),
		"POST /admin/platform/role":                                    p("platform:role:save"),
		"PUT /admin/platform/role/{id}":                                p("platform:role:update"),
		"DELETE /admin/platform/role":                                  p("platform:role:delete"),
		"GET /admin/platform/role/{id}/permissions":                    p("platform:role:getMenu"),
		"PUT /admin/platform/role/{id}/permissions":                    p("platform:role:setMenu"),
		"GET /admin/platform/menu/list":                                p("platform:menu:list"),
		"POST /admin/platform/menu":                                    p("platform:menu:create"),
		"PUT /admin/platform/menu/{id}":                                p("platform:menu:save"),
		"DELETE /admin/platform/menu":                                  p("platform:menu:delete"),
		"GET /admin/platform/dictionary/list":                          p("platform:dictionary:list"),
		"GET /admin/platform/dictionary/options":                       p("platform:dictionary:list"),
		"GET /admin/platform/dictionary/{id}":                          p("platform:dictionary:list"),
		"POST /admin/platform/dictionary":                              p("platform:dictionary:save"),
		"PUT /admin/platform/dictionary/{id}":                          p("platform:dictionary:update"),
		"DELETE /admin/platform/dictionary":                            p("platform:dictionary:delete"),
		"POST /admin/platform/dictionary/dispatch":                     p("platform:dictionary:dispatch"),
		"POST /admin/platform/dictionary/dispatch/{id}":                p("platform:dictionary:dispatch"),
		"GET /admin/platform/storage-config/list":                      p("platform:storageConfig:list"),
		"POST /admin/platform/storage-config":                          p("platform:storageConfig:save"),
		"PUT /admin/platform/storage-config/{id}":                      p("platform:storageConfig:update"),
		"DELETE /admin/platform/storage-config":                        p("platform:storageConfig:delete"),
		"GET /admin/platform/scheduled-task/list":                      p("platform:scheduledTask:list"),
		"GET /admin/platform/scheduled-task/tenant-options":            {"platform:scheduledTask:list", "platform:scheduledTask:save", "platform:scheduledTask:update"},
		"GET /admin/platform/scheduled-task/{id}":                      p("platform:scheduledTask:list"),
		"POST /admin/platform/scheduled-task":                          p("platform:scheduledTask:save"),
		"PUT /admin/platform/scheduled-task/{id}":                      p("platform:scheduledTask:update"),
		"DELETE /admin/platform/scheduled-task":                        p("platform:scheduledTask:delete"),
		"PUT /admin/platform/scheduled-task/{id}/enable":               p("platform:scheduledTask:update"),
		"PUT /admin/platform/scheduled-task/{id}/disable":              p("platform:scheduledTask:update"),
		"POST /admin/platform/scheduled-task/{id}/run":                 p("platform:scheduledTask:run"),
		"GET /admin/platform/scheduled-task-log/list":                  p("platform:scheduledTask:log"),
		"GET /admin/platform/queue/failed-jobs":                        p("platform:queueFailedJob:list"),
		"POST /admin/platform/queue/failed-jobs/retry":                 p("platform:queueFailedJob:retry"),
		"DELETE /admin/platform/queue/failed-jobs":                     p("platform:queueFailedJob:delete"),
		"GET /admin/platform/observability/slow-requests":              p("platform:observability:list"),
		"GET /admin/platform/module-lifecycle/state":                   p("platform:moduleLifecycle:list"),
		"GET /admin/platform/module-lifecycle/runs":                    p("platform:moduleLifecycle:list"),
		"GET /admin/platform/module-lifecycle/steps":                   p("platform:moduleLifecycle:log"),
		"GET /admin/platform/module-lifecycle/locks":                   p("platform:moduleLifecycle:list"),
		"GET /admin/platform/module-lifecycle/diff":                    p("platform:moduleLifecycle:list"),
		"POST /admin/platform/module-lifecycle/locks/release-stale":    p("platform:moduleLifecycle:execute"),
		"POST /admin/platform/module-lifecycle/execute":                p("platform:moduleLifecycle:execute"),
		"POST /admin/platform/security/reauth-token":                   platformSensitiveEvidencePermissions(),
		"POST /admin/platform/security/approvals":                      platformSensitiveEvidencePermissions(),
		"GET /admin/platform/security/approvals/{approval_id}":         platformSensitiveEvidencePermissions(),
		"PUT /admin/platform/security/approvals/{approval_id}/approve": platformSensitiveEvidencePermissions(),
		"GET /admin/platform/reference-case/list":                      p("platform:referenceCase:list"),
		"POST /admin/platform/reference-case":                          p("platform:referenceCase:save"),
		"PUT /admin/platform/reference-case/{id}":                      p("platform:referenceCase:update"),
		"DELETE /admin/platform/reference-case":                        p("platform:referenceCase:delete"),
	}
}

func platformSensitiveEvidencePermissions() []string {
	return []string{
		"platform:security:control", "platform:security:mfa", "platform:moduleLifecycle:execute",
		"platform:tenant:destroy", "platform:tenant:export", "platform:tenant:permissions", "platform:tenant:updatePlan",
		"platform:tenant:governance", "platform:tenant:suspend", "platform:tenant:resume", "platform:tenant:archive",
		"platform:user:password", "platform:user:setRole", "platform:role:setMenu",
		"platform:storageConfig:save", "platform:storageConfig:update", "platform:storageConfig:delete",
	}
}

func platformRoutePermissions(permission string) []string {
	return []string{permission}
}
