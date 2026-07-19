package application

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"github.com/goravel/framework/support/collect"
	permissioncontract "goravel/app/contracts/permission"
	"goravel/app/facades"
	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
	authservice "goravel/app/services/access/auth"
	casbincache "goravel/app/services/access/casbin"
	permissionservice "goravel/app/services/access/permission"
	"goravel/app/support/apperror"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Source: casbin_cache.go
type CasbinEnforcerCacheMetrics = casbincache.CasbinEnforcerCacheMetrics

var InvalidateCasbinEnforcer = casbincache.Invalidate
var ResetCasbinEnforcerCacheForTest = casbincache.Reset
var CasbinEnforcerCacheSnapshot = casbincache.Snapshot

// Source: casbin_policy_loader.go
type casbinPolicyRow struct {
	ID    uint64 `gorm:"column:id"`
	Ptype string `gorm:"column:ptype"`
	V0    string `gorm:"column:v0"`
	V1    string `gorm:"column:v1"`
	V2    string `gorm:"column:v2"`
	V3    string `gorm:"column:v3"`
	V4    string `gorm:"column:v4"`
	V5    string `gorm:"column:v5"`
}

func loadCasbinEnforcer(query contractsorm.Query, table string) (casbincache.Authorizer, error) {
	var rules []casbinPolicyRow
	if err := query.Table(table).
		SelectRaw(`id,
			COALESCE(ptype, '') AS ptype,
			COALESCE(v0, '') AS v0,
			COALESCE(v1, '') AS v1,
			COALESCE(v2, '') AS v2,
			COALESCE(v3, '') AS v3,
			COALESCE(v4, '') AS v4,
			COALESCE(v5, '') AS v5`).
		OrderBy("id").Get(&rules); err != nil {
		return nil, err
	}

	casbinPolicy, err := casbinModel()
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		line, err := casbinPolicyLine(rule)
		if err != nil {
			return nil, fmt.Errorf("load %s policy %d: %w", table, rule.ID, err)
		}
		if err := persist.LoadPolicyArray(line, casbinPolicy); err != nil {
			return nil, fmt.Errorf("load %s policy %d: %w", table, rule.ID, err)
		}
	}

	enforcer, err := casbin.NewSyncedEnforcer(casbinPolicy)
	if err != nil {
		return nil, err
	}
	enforcer.EnableAutoSave(false)
	if err := enforcer.BuildRoleLinks(); err != nil {
		return nil, err
	}

	return enforcer, nil
}

func casbinPolicyLine(rule casbinPolicyRow) ([]string, error) {
	if rule.Ptype == "" {
		return nil, fmt.Errorf("ptype is empty")
	}
	line := []string{rule.Ptype, rule.V0, rule.V1, rule.V2, rule.V3, rule.V4, rule.V5}
	for len(line) > 1 && line[len(line)-1] == "" {
		line = line[:len(line)-1]
	}
	return line, nil
}

// Source: casbin_service.go
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

func (s *CasbinService) enforcer() (casbincache.Authorizer, error) {
	return casbincache.Get("tenant:"+s.connection, s.loadEnforcer)
}

func (s *CasbinService) loadEnforcer() (casbincache.Authorizer, error) {
	return loadCasbinEnforcer(s.orm().Query(), "casbin_rule")
}

func casbinModel() (model.Model, error) {
	path := facades.Config().GetString("casbin.model")
	if !filepath.IsAbs(path) {
		path = resolveCasbinModelPath(facades.App().BasePath(path), path)
	}

	return model.NewModelFromFile(path)
}

func resolveCasbinModelPath(basePath, configuredPath string) string {
	if _, err := os.Stat(basePath); err == nil {
		return basePath
	}
	cwd, err := os.Getwd()
	if err != nil {
		return basePath
	}
	for {
		candidate := filepath.Join(cwd, configuredPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return candidate
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return basePath
		}
		cwd = parent
	}
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

// Source: data_permission.go
type PolicyType = permissioncontract.PolicyType

const (
	PolicyAll        = permissioncontract.PolicyAll
	PolicyDeptSelf   = permissioncontract.PolicyDeptSelf
	PolicyDeptTree   = permissioncontract.PolicyDeptTree
	PolicySelf       = permissioncontract.PolicySelf
	PolicyCustomDept = permissioncontract.PolicyCustomDept
	PolicyCustomFunc = permissioncontract.PolicyCustomFunc
)

type DataPolicy = permissioncontract.DataPolicy
type DataScopeContext = permissioncontract.DataScopeContext
type DataScope = permissioncontract.DataScope

var ResolveDataPolicy = permissioncontract.ResolveDataPolicy
var BuildDataScope = permissioncontract.BuildDataScope

// Source: permission.go
type menuDeleteRow = permissionservice.MenuDeleteRow

func collectMenuDeleteTargets(rows []menuDeleteRow, roots []uint64) ([]uint64, []string) {
	return permissionservice.CollectMenuDeleteTargets(rows, roots)
}

// Source: permission_admin_casbin_service.go
func (s *PermissionAdminService) addCasbinRule(ptype, v0, v1, v2 string) error {
	return addCasbinRule(s.orm().Query(), ptype, v0, v1, v2)
}

func addCasbinRule(query contractsorm.Query, ptype, v0, v1, v2 string) error {
	now := time.Now()
	return query.Create(&models.CasbinRule{
		Ptype: ptype, V0: v0, V1: v1, V2: v2,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	})
}

// Source: permission_admin_menu_service.go
func (s *PermissionAdminService) ListMenus() ([]AdminMenuItem, error) {
	menus := make([]AdminMenuItem, 0)
	err := s.orm().Query().Table("menu").Where("status", 1).
		OrderBy("sort").OrderBy("id").Scan(&menus)
	if err != nil {
		return nil, err
	}
	if s.tenant.ID != 0 {
		menus = FilterAdminMenusByTenantPermissions(s.tenant, menus)
	}
	return buildAdminMenuTree(menus, 0), nil
}

func (s *PermissionAdminService) CreateMenu(input MenuPayload, operatorID uint64) error {
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		menuID, err := s.saveMenu(tx, 0, input, operatorID)
		if err != nil {
			return err
		}
		return s.syncButtonPermissions(tx, menuID, input.BtnPermission, operatorID)
	}); err != nil {
		return err
	}
	InvalidateAllCurrentUserInfo()
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PermissionAdminService) UpdateMenu(id uint64, input MenuPayload, operatorID uint64) error {
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := s.saveMenu(tx, id, input, operatorID)
		if err != nil {
			return err
		}
		return s.syncButtonPermissions(tx, id, input.BtnPermission, operatorID)
	}); err != nil {
		return err
	}
	InvalidateAllCurrentUserInfo()
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PermissionAdminService) DeleteMenus(ids []uint64) error {
	menuIDs, menuNames, err := s.deletedMenuTargets(ids)
	if err != nil {
		return err
	}
	if len(menuIDs) == 0 {
		return nil
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("role_belongs_menu").WhereIn("menu_id", uint64Any(menuIDs)).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Table("casbin_rule").Where("ptype", "p").WhereIn("v1", stringAny(menuNames)).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Table("menu").WhereIn("id", uint64Any(menuIDs)).Delete()
		return err
	}); err != nil {
		return err
	}
	InvalidateAllCurrentUserInfo()
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PermissionAdminService) deletedMenuTargets(ids []uint64) ([]uint64, []string, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}

	rows := make([]menuDeleteRow, 0)
	err := s.orm().Query().Table("menu").
		Select("id", "parent_id", "name").
		Scan(&rows)
	if err != nil {
		return nil, nil, err
	}

	menuIDs, menuNames := collectMenuDeleteTargets(rows, ids)
	return menuIDs, menuNames, nil
}

func (s *PermissionAdminService) saveMenu(tx contractsorm.Query, id uint64, input MenuPayload, operatorID uint64) (uint64, error) {
	if input.Meta == nil {
		input.Meta = models.JSONMap{}
	}
	if input.Status == 0 {
		input.Status = 1
	}
	encodedMeta, err := json.Marshal(input.Meta)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		menu := models.Menu{
			ParentID: input.ParentID, Name: input.Name, Meta: nil,
			Path: input.Path, Component: input.Component, Redirect: input.Redirect,
			Status: input.Status, Sort: input.Sort, Remark: input.Remark,
			AuditColumns: models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID},
		}
		if err := tx.Create(&menu); err != nil {
			return 0, err
		}
		_, err := tx.Exec("UPDATE menu SET meta = ?::jsonb WHERE id = ?", string(encodedMeta), menu.ID)
		if err != nil {
			return 0, err
		}
		return menu.ID, nil
	}

	_, err = tx.Table("menu").Where("id", id).Update(map[string]any{
		"parent_id": input.ParentID, "name": input.Name,
		"path": input.Path, "component": input.Component, "redirect": input.Redirect,
		"status": input.Status, "sort": input.Sort, "remark": input.Remark,
		"updated_by": operatorID, "updated_at": time.Now(),
	})
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec("UPDATE menu SET meta = ?::jsonb WHERE id = ?", string(encodedMeta), id)
	return id, err
}

func (s *PermissionAdminService) syncButtonPermissions(
	tx contractsorm.Query,
	parentID uint64,
	buttons []MenuPayload,
	operatorID uint64,
) error {
	if buttons == nil {
		return nil
	}
	_, err := tx.Exec(
		"DELETE FROM menu WHERE parent_id = ? AND meta->>'type' = ?",
		parentID,
		"B",
	)
	if err != nil {
		return err
	}
	for _, button := range buttons {
		button.ParentID = parentID
		if button.Meta == nil {
			button.Meta = models.JSONMap{"type": "B"}
		}
		if _, ok := button.Meta["type"]; !ok {
			button.Meta["type"] = "B"
		}
		if _, err := s.saveMenu(tx, 0, button, operatorID); err != nil {
			return err
		}
	}
	return nil
}

func buildAdminMenuTree(menus []AdminMenuItem, parentID uint64) []AdminMenuItem {
	tree := make([]AdminMenuItem, 0)
	for _, menu := range menus {
		if menu.ParentID != parentID {
			continue
		}
		if menu.Meta == nil {
			menu.Meta = models.JSONMap{}
		}
		menu.Children = buildAdminMenuTree(menus, menu.ID)
		tree = append(tree, menu)
	}
	return tree
}

// Source: permission_admin_role_service.go
func (s *PermissionAdminService) ListRoles(filters map[string]string, page, pageSize int) (request.PageResult[models.Role], error) {
	query := s.orm().Query().Table("role")
	query = query.Scopes(scopes.Contains("name", filters["name"]))
	query = query.Scopes(scopes.Contains("code", filters["code"]))
	query = query.Scopes(scopes.EqualIfPresent("status", filters["status"]))

	return request.Paginate[models.Role](query.OrderBy("sort").OrderBy("id"), page, pageSize)
}

func (s *PermissionAdminService) CreateRole(input RolePayload, operatorID uint64) error {
	if s.tenant.ID != 0 {
		if err := NewTenantRuntimeService().WithContext(s.ctx).EnsureResourceQuota(s.tenant, "roles", 1); err != nil {
			return err
		}
	}
	role := models.Role{
		Name: input.Name, Code: input.Code, Status: statusOrDefault(input.Status),
		Sort: input.Sort, Remark: input.Remark,
		AuditColumns: models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID},
	}
	if err := s.orm().Query().Create(&role); err != nil {
		return err
	}
	InvalidateAllCurrentUserInfo()
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PermissionAdminService) UpdateRole(id uint64, input RolePayload, operatorID uint64) error {
	var oldRole models.Role
	if err := s.orm().Query().Where("id", id).First(&oldRole); err != nil {
		return err
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("role").Where("id", id).Update(map[string]any{
			"name": input.Name, "code": input.Code, "status": statusOrDefault(input.Status),
			"sort": input.Sort, "remark": input.Remark, "updated_by": operatorID,
			"updated_at": time.Now(),
		})
		if err != nil {
			return err
		}
		if oldRole.Code == input.Code {
			return nil
		}
		_, err = tx.Table("casbin_rule").
			Where("v0", "role:"+oldRole.Code).
			Update("v0", "role:"+input.Code)
		if err != nil {
			return err
		}
		_, err = tx.Table("casbin_rule").
			Where("ptype", "g").
			Where("v1", "role:"+oldRole.Code).
			Update("v1", "role:"+input.Code)
		return err
	}); err != nil {
		return err
	}
	InvalidateAllCurrentUserInfo()
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PermissionAdminService) DeleteRoles(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	count, err := s.orm().Query().Table("role").
		WhereIn("id", uint64Any(ids)).
		Where("code", "SuperAdmin").
		Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return BusinessError{Message: "不能删除超级管理员角色"}
	}
	rows := make([]struct {
		Code string `gorm:"column:code"`
	}, 0, len(ids))
	if err := s.orm().Query().Table("role").
		Select("code").
		WhereIn("id", uint64Any(ids)).
		Scan(&rows); err != nil {
		return err
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("role").WhereIn("id", uint64Any(ids)).Delete()
		if err != nil {
			return err
		}
		for _, row := range rows {
			subject := "role:" + row.Code
			_, err = tx.Table("casbin_rule").Where("v0", subject).Delete()
			if err != nil {
				return err
			}
			_, err = tx.Table("casbin_rule").Where("ptype", "g").Where("v1", subject).Delete()
			if err != nil {
				return err
			}
		}
		_, err = tx.Table("role_belongs_menu").WhereIn("role_id", uint64Any(ids)).Delete()
		return err
	}); err != nil {
		return err
	}
	InvalidateAllCurrentUserInfo()
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PermissionAdminService) RolePermissions(roleID uint64) ([]RolePermission, error) {
	permissions := make([]RolePermission, 0)
	err := s.orm().Query().
		Table("menu").
		Select("menu.id", "menu.name").
		Join("JOIN role_belongs_menu rbm ON rbm.menu_id = menu.id").
		Where("rbm.role_id", roleID).
		OrderBy("menu.sort").
		OrderBy("menu.id").
		Scan(&permissions)
	return permissions, err
}

func (s *PermissionAdminService) SyncRolePermissions(roleID uint64, permissions []string) error {
	if s.tenant.ID != 0 {
		if err := ValidateTenantRolePermissions(s.tenant, permissions); err != nil {
			return err
		}
	}
	var role models.Role
	if err := s.orm().Query().Where("id", roleID).First(&role); err != nil {
		return err
	}
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("role_belongs_menu").Where("role_id", roleID).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Table("casbin_rule").Where("ptype", "p").Where("v0", "role:"+role.Code).Delete()
		if err != nil {
			return err
		}

		for _, permission := range permissions {
			var menu models.Menu
			if err := tx.Where("name", permission).First(&menu); err != nil {
				return err
			}
			now := time.Now()
			err := tx.Create(&models.RoleBelongsMenu{
				RoleID: roleID, MenuID: menu.ID,
				Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
			})
			if err != nil {
				return err
			}
			if err := addCasbinRule(tx, "p", "role:"+role.Code, permission, "*"); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	InvalidateAllCurrentUserInfo()
	s.invalidateCasbinEnforcer()
	return nil
}

// Source: permission_admin_types.go
const defaultPassword = authservice.DefaultPassword

var ErrBusinessRule = apperror.ErrBusinessRule

type BusinessError = apperror.BusinessError

type PermissionAdminService struct {
	ctx        context.Context
	connection string
	tenant     Tenant
}

func (s *PermissionAdminService) invalidateCasbinEnforcer() {
	casbincache.Invalidate("tenant:" + s.connection)
}

type UserPayload struct {
	Username       string         `json:"username"`
	Password       string         `json:"password"`
	UserType       any            `json:"user_type"`
	Nickname       string         `json:"nickname"`
	Phone          string         `json:"phone"`
	Email          string         `json:"email"`
	Avatar         string         `json:"avatar"`
	Signed         string         `json:"signed"`
	Dashboard      string         `json:"dashboard"`
	Status         int8           `json:"status"`
	Remark         string         `json:"remark"`
	BackendSetting models.JSONMap `json:"backend_setting"`
	Department     []any          `json:"department"`
	Position       []any          `json:"position"`
}

type RolePayload struct {
	Name   string `json:"name"`
	Code   string `json:"code"`
	Status int8   `json:"status"`
	Sort   int16  `json:"sort"`
	Remark string `json:"remark"`
}

type MenuPayload struct {
	ParentID      uint64         `json:"parent_id"`
	Name          string         `json:"name"`
	Meta          models.JSONMap `json:"meta"`
	Path          string         `json:"path"`
	Component     string         `json:"component"`
	Redirect      string         `json:"redirect"`
	Status        int8           `json:"status"`
	Sort          int16          `json:"sort"`
	Remark        string         `json:"remark"`
	BtnPermission []MenuPayload  `json:"btnPermission"`
}

type AdminMenuItem struct {
	ID        uint64          `gorm:"column:id" json:"id"`
	ParentID  uint64          `gorm:"column:parent_id" json:"parent_id"`
	Name      string          `gorm:"column:name" json:"name"`
	Meta      models.JSONMap  `gorm:"column:meta;type:jsonb" json:"meta"`
	Path      string          `gorm:"column:path" json:"path"`
	Component string          `gorm:"column:component" json:"component"`
	Redirect  string          `gorm:"column:redirect" json:"redirect"`
	Status    int8            `gorm:"column:status" json:"status"`
	Sort      int16           `gorm:"column:sort" json:"sort"`
	Remark    string          `gorm:"column:remark" json:"remark"`
	Children  []AdminMenuItem `gorm:"-" json:"children"`
}

type UserRow struct {
	models.User
	Roles []RoleInfo `gorm:"-" json:"roles"`
}

type RolePermission struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

func NewPermissionAdminService() *PermissionAdminService {
	return &PermissionAdminService{}
}

func NewPermissionAdminServiceForTenant(tenant Tenant) *PermissionAdminService {
	return &PermissionAdminService{connection: TenantConnectionName(tenant), tenant: tenant}
}

func (s *PermissionAdminService) WithContext(ctx context.Context) *PermissionAdminService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *PermissionAdminService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func userTypeString(value any) string {
	switch v := value.(type) {
	case string:
		if v != "" {
			return v
		}
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	}
	return "100"
}

func statusOrDefault(status int8) int8 {
	if status == 0 {
		return 1
	}
	return status
}

func mapOrEmpty(value models.JSONMap) models.JSONMap {
	if value == nil {
		return models.JSONMap{}
	}
	return value
}

func addNonEmpty(values map[string]any, column, value string) {
	if strings.TrimSpace(value) != "" {
		values[column] = value
	}
}

func uint64Any(values []uint64) []any {
	return collect.Map(values, func(value uint64, _ int) any {
		return value
	})
}

// Source: permission_admin_user_relation_service.go
func (s *PermissionAdminService) syncUserDepartments(tx contractsorm.Query, userID uint64, values []any) error {
	ids := payloadIDs(values, "id")
	_, err := tx.Table("user_dept").Where("user_id", userID).Delete()
	if err != nil {
		return err
	}
	for _, id := range ids {
		now := time.Now()
		err := tx.Create(&models.UserDept{
			UserID: userID, DeptID: id,
			Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *PermissionAdminService) syncUserPositions(tx contractsorm.Query, userID uint64, values []any) error {
	ids := payloadIDs(values, "id")
	_, err := tx.Table("user_position").Where("user_id", userID).Delete()
	if err != nil {
		return err
	}
	for _, id := range ids {
		now := time.Now()
		err := tx.Create(&models.UserPosition{
			UserID: userID, PositionID: id,
			Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *PermissionAdminService) SyncUserRoles(userID uint64, roleCodes []string) error {
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		return s.syncUserRolesInTransaction(tx, userID, roleCodes)
	}); err != nil {
		return err
	}
	InvalidateCurrentUserInfoForConnection(s.connection, userID)
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PermissionAdminService) syncUserRolesInTransaction(
	tx contractsorm.Query,
	userID uint64,
	roleCodes []string,
) error {
	_, err := tx.Table("user_belongs_role").Where("user_id", userID).Delete()
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("user:%d", userID)
	_, err = tx.Table("casbin_rule").Where("ptype", "g").Where("v0", subject).Delete()
	if err != nil {
		return err
	}

	for _, code := range roleCodes {
		if err := s.attachUserRole(tx, userID, subject, code); err != nil {
			return err
		}
	}
	return nil
}

func (s *PermissionAdminService) attachUserRole(
	tx contractsorm.Query,
	userID uint64,
	subject string,
	code string,
) error {
	var role models.Role
	if err := tx.Where("code", code).First(&role); err != nil {
		return err
	}
	now := time.Now()
	err := tx.Create(&models.UserBelongsRole{
		UserID: userID, RoleID: role.ID,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	})
	if err != nil {
		return err
	}
	return addCasbinRule(tx, "g", subject, "role:"+role.Code, "")
}

// Source: permission_admin_user_scope_service.go
func (s *PermissionAdminService) applyUserDataScope(query contractsorm.Query, currentUserID uint64) (contractsorm.Query, error) {
	if currentUserID == 1 {
		return query, nil
	}
	policy, err := s.resolveUserListPolicy(currentUserID)
	if err != nil {
		return query, err
	}
	scope, err := BuildDataScope(policy, DataScopeContext{
		UserID: currentUserID, DeptColumn: "ud.dept_id", OwnerColumn: `"user".id`,
	})
	if err != nil {
		return query, err
	}
	if scope.Condition == "" {
		return query, nil
	}
	query = query.Join("LEFT JOIN user_dept ud ON ud.user_id = \"user\".id AND ud.deleted_at IS NULL")
	return query.Where(scope.Condition, scope.Args...), nil
}

func (s *PermissionAdminService) resolveUserListPolicy(userID uint64) (DataPolicy, error) {
	userPolicy, err := s.policyByOwner("user_id", userID)
	if err != nil {
		return DataPolicy{}, err
	}
	positionPolicies, err := s.positionPolicies(userID)
	if err != nil {
		return DataPolicy{}, err
	}
	return s.resolveDepartmentPolicy(ResolveDataPolicy(userPolicy, positionPolicies), userID)
}

func (s *PermissionAdminService) resolveDepartmentPolicy(policy DataPolicy, userID uint64) (DataPolicy, error) {
	switch policy.Type {
	case PolicyDeptSelf:
		deptIDs, err := s.userDepartmentIDs(userID)
		if err != nil {
			return DataPolicy{}, err
		}
		policy.DeptIDs = deptIDs
	case PolicyDeptTree:
		deptIDs, err := s.userDepartmentIDs(userID)
		if err != nil {
			return DataPolicy{}, err
		}
		policy.DeptIDs, err = s.departmentTreeIDs(deptIDs)
		if err != nil {
			return DataPolicy{}, err
		}
	}

	return policy, nil
}

func (s *PermissionAdminService) policyByOwner(column string, id uint64) (DataPolicy, error) {
	var row struct {
		PolicyType string           `gorm:"column:policy_type"`
		Value      models.JSONSlice `gorm:"column:value;type:jsonb"`
	}
	err := s.orm().Query().Table("data_permission_policy").
		Select("policy_type", "value").
		Where(column, id).
		WhereNull("deleted_at").
		OrderByDesc("is_default").
		OrderBy("id").
		First(&row)
	if err != nil {
		return DataPolicy{}, nil
	}
	return DataPolicy{Type: PolicyType(row.PolicyType), DeptIDs: jsonSliceUint64(row.Value)}, nil
}

func (s *PermissionAdminService) positionPolicies(userID uint64) ([]DataPolicy, error) {
	rows := make([]struct {
		PolicyType string           `gorm:"column:policy_type"`
		Value      models.JSONSlice `gorm:"column:value;type:jsonb"`
	}, 0)
	err := s.orm().Query().Table("data_permission_policy").
		Select("data_permission_policy.policy_type", "data_permission_policy.value").
		Join("JOIN user_position up ON up.position_id = data_permission_policy.position_id").
		Where("up.user_id", userID).
		WhereNull("up.deleted_at").
		WhereNull("data_permission_policy.deleted_at").
		OrderByDesc("data_permission_policy.is_default").
		OrderBy("data_permission_policy.id").
		Scan(&rows)
	if err != nil {
		return nil, err
	}
	policies := make([]DataPolicy, 0, len(rows))
	for _, row := range rows {
		policies = append(policies, DataPolicy{Type: PolicyType(row.PolicyType), DeptIDs: jsonSliceUint64(row.Value)})
	}
	return policies, nil
}

func (s *PermissionAdminService) userDepartmentIDs(userID uint64) ([]uint64, error) {
	rows := make([]struct {
		DeptID uint64 `gorm:"column:dept_id"`
	}, 0)
	err := s.orm().Query().Table("user_dept").
		Select("dept_id").
		Where("user_id", userID).
		WhereNull("deleted_at").
		Scan(&rows)
	if err != nil {
		return nil, err
	}
	deptIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		deptIDs = append(deptIDs, row.DeptID)
	}
	return deptIDs, nil
}

func (s *PermissionAdminService) departmentTreeIDs(rootIDs []uint64) ([]uint64, error) {
	seen := make(map[uint64]bool, len(rootIDs))
	queue := make([]uint64, 0, len(rootIDs))
	for _, id := range rootIDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		queue = append(queue, id)
	}

	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]

		children := make([]struct {
			ID uint64 `gorm:"column:id"`
		}, 0)
		err := s.orm().Query().Table("department").
			Select("id").
			Where("parent_id", parentID).
			WhereNull("deleted_at").
			Scan(&children)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if seen[child.ID] {
				continue
			}
			seen[child.ID] = true
			queue = append(queue, child.ID)
		}
	}

	deptIDs := make([]uint64, 0, len(seen))
	for id := range seen {
		deptIDs = append(deptIDs, id)
	}
	return deptIDs, nil
}

func jsonSliceUint64(values models.JSONSlice) []uint64 {
	out := make([]uint64, 0, len(values))
	for _, value := range values {
		switch v := value.(type) {
		case float64:
			out = append(out, uint64(v))
		case int:
			out = append(out, uint64(v))
		case uint64:
			out = append(out, v)
		}
	}
	return out
}

// Source: permission_admin_user_service.go
func (s *PermissionAdminService) ListUsers(filters map[string]string, page, pageSize int, currentUserID uint64) (request.PageResult[UserRow], error) {
	query := s.orm().Query().Table(`"user"`).Where("user_type", "100")
	var err error
	query, err = s.applyUserDataScope(query, currentUserID)
	if err != nil {
		return request.PageResult[UserRow]{}, err
	}
	query = query.Scopes(scopes.Contains("username", filters["username"]))
	query = query.Scopes(scopes.Contains("nickname", filters["nickname"]))
	query = query.Scopes(scopes.Contains("phone", filters["phone"]))
	query = query.Scopes(scopes.Contains("email", filters["email"]))
	query = query.Scopes(scopes.EqualIfPresent("status", filters["status"]))

	result, err := request.Paginate[UserRow](query.OrderByDesc("id"), page, pageSize)
	if err != nil {
		return request.PageResult[UserRow]{}, err
	}
	passport := (&PassportService{connection: s.connection}).WithContext(s.ctx)
	for i := range result.List {
		roles, err := passport.UserRoles(result.List[i].ID)
		if err != nil {
			return request.PageResult[UserRow]{}, err
		}
		result.List[i].Roles = roles
	}

	return result, nil
}

func (s *PermissionAdminService) CreateUser(input UserPayload, operatorID uint64) error {
	if s.tenant.ID != 0 {
		if err := NewTenantRuntimeService().WithContext(s.ctx).EnsureResourceQuota(s.tenant, "users", 1); err != nil {
			return err
		}
	}
	password, err := InitialPassword(input.Password)
	if err != nil {
		return err
	}
	hash, err := makePasswordHash(password)
	if err != nil {
		return err
	}

	user := models.User{
		Username:       input.Username,
		Password:       hash,
		UserType:       userTypeString(input.UserType),
		Nickname:       input.Nickname,
		Phone:          input.Phone,
		Email:          input.Email,
		Avatar:         input.Avatar,
		Signed:         input.Signed,
		Dashboard:      input.Dashboard,
		Status:         statusOrDefault(input.Status),
		BackendSetting: nil,
		AuditColumns:   models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID},
		Remark:         input.Remark,
	}
	if user.Dashboard == "" {
		user.Dashboard = "dashboard:workbench"
	}

	encoded, err := json.Marshal(mapOrEmpty(input.BackendSetting))
	if err != nil {
		return err
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		if err := tx.Create(&user); err != nil {
			return err
		}
		if err := s.syncUserDepartments(tx, user.ID, input.Department); err != nil {
			return err
		}
		if err := s.syncUserPositions(tx, user.ID, input.Position); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE "user" SET backend_setting = ?::jsonb WHERE id = ?`, string(encoded), user.ID)
		if err != nil {
			return err
		}
		return NewPasswordHistoryService(s.connection, "user_password_history").WithContext(s.ctx).RecordWithQuery(tx, user.ID, hash)
	}); err != nil {
		return err
	}
	InvalidateCurrentUserInfoForConnection(s.connection, user.ID)
	return nil
}

func (s *PermissionAdminService) UpdateUser(id uint64, input UserPayload, operatorID uint64) error {
	if input.Password != "" {
		return ErrSensitiveOperationPolicy
	}
	values := map[string]any{"updated_by": operatorID, "updated_at": time.Now()}
	addNonEmpty(values, "nickname", input.Nickname)
	addNonEmpty(values, "phone", input.Phone)
	addNonEmpty(values, "email", input.Email)
	addNonEmpty(values, "avatar", input.Avatar)
	addNonEmpty(values, "signed", input.Signed)
	addNonEmpty(values, "dashboard", input.Dashboard)
	addNonEmpty(values, "remark", input.Remark)
	if input.Status != 0 {
		values["status"] = input.Status
	}
	var encodedSetting []byte
	if input.BackendSetting != nil {
		var err error
		encodedSetting, err = json.Marshal(input.BackendSetting)
		if err != nil {
			return err
		}
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table(`"user"`).Where("id", id).Update(values)
		if err != nil {
			return err
		}
		if input.Department != nil {
			if err := s.syncUserDepartments(tx, id, input.Department); err != nil {
				return err
			}
		}
		if input.Position != nil {
			if err := s.syncUserPositions(tx, id, input.Position); err != nil {
				return err
			}
		}
		if input.BackendSetting != nil {
			_, err = tx.Exec(`UPDATE "user" SET backend_setting = ?::jsonb WHERE id = ?`, string(encodedSetting), id)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	InvalidateCurrentUserInfoForConnection(s.connection, id)
	return nil
}

func (s *PermissionAdminService) DeleteUsers(ids []uint64, currentUserID uint64) error {
	for _, id := range ids {
		if id == currentUserID || id == 1 {
			return BusinessError{Message: "不能删除当前管理员"}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := s.orm().Query().Table(`"user"`).WhereIn("id", uint64Any(ids)).Delete()
	if err != nil {
		return err
	}
	InvalidateCurrentUserInfoForConnection(s.connection, ids...)
	return nil
}

func (s *PermissionAdminService) ResetPassword(userID uint64) error {
	password, err := InitialPassword("")
	if err != nil {
		return err
	}
	if err := NewPasswordHistoryService(s.connection, "user_password_history").WithContext(s.ctx).ValidateReuse(userID, password); err != nil {
		return err
	}
	hash, err := makePasswordHash(password)
	if err != nil {
		return err
	}
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err = tx.Table(`"user"`).Where("id", userID).Update(map[string]any{
			"password": hash, "updated_at": time.Now(),
		})
		if err != nil {
			return err
		}
		return NewPasswordHistoryService(s.connection, "user_password_history").WithContext(s.ctx).RecordWithQuery(tx, userID, hash)
	}); err != nil {
		return err
	}
	InvalidateCurrentUserInfoForConnection(s.connection, userID)
	return nil
}

func (s *PermissionAdminService) UserRoles(userID uint64) ([]RoleInfo, error) {
	return (&PassportService{connection: s.connection}).WithContext(s.ctx).UserRoles(userID)
}

// Source: platform_casbin_service.go
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

func (s *PlatformCasbinService) enforcer() (casbincache.Authorizer, error) {
	return casbincache.Get("platform:"+PlatformConnection(), s.loadEnforcer)
}

func (s *PlatformCasbinService) loadEnforcer() (casbincache.Authorizer, error) {
	query := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query()
	return loadCasbinEnforcer(query, "platform_casbin_rule")
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

// Source: platform_permission_admin_service.go
type PlatformPermissionAdminService struct {
	ctx context.Context
}

func NewPlatformPermissionAdminService() *PlatformPermissionAdminService {
	return &PlatformPermissionAdminService{}
}

func (s *PlatformPermissionAdminService) WithContext(ctx context.Context) *PlatformPermissionAdminService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *PlatformPermissionAdminService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func (s *PlatformPermissionAdminService) invalidateCasbinEnforcer() {
	InvalidateCasbinEnforcer("platform:" + PlatformConnection())
}

func (s *PlatformPermissionAdminService) ListUsers(filters map[string]string, page, pageSize int) (request.PageResult[UserRow], error) {
	query := s.orm().Query().Table("platform_user").Where("user_type", "900")
	query = query.Scopes(scopes.Contains("username", filters["username"]))
	query = query.Scopes(scopes.Contains("nickname", filters["nickname"]))
	query = query.Scopes(scopes.Contains("phone", filters["phone"]))
	query = query.Scopes(scopes.Contains("email", filters["email"]))
	query = query.Scopes(scopes.EqualIfPresent("status", filters["status"]))

	result, err := request.Paginate[UserRow](query.OrderByDesc("id"), page, pageSize)
	if err != nil {
		return request.PageResult[UserRow]{}, err
	}
	passport := NewPlatformPassportService().WithContext(s.ctx)
	for i := range result.List {
		roles, err := passport.UserRoles(result.List[i].ID)
		if err != nil {
			return request.PageResult[UserRow]{}, err
		}
		result.List[i].Roles = roles
	}

	return result, nil
}

func (s *PlatformPermissionAdminService) CreateUser(input UserPayload, operatorID uint64) error {
	password, err := InitialPassword(input.Password)
	if err != nil {
		return err
	}
	hash, err := makePasswordHash(password)
	if err != nil {
		return err
	}

	user := models.User{
		Username:       input.Username,
		Password:       hash,
		UserType:       "900",
		Nickname:       input.Nickname,
		Phone:          input.Phone,
		Email:          input.Email,
		Avatar:         input.Avatar,
		Signed:         input.Signed,
		Dashboard:      input.Dashboard,
		Status:         statusOrDefault(input.Status),
		BackendSetting: nil,
		AuditColumns:   models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID},
		Remark:         input.Remark,
	}
	if user.Dashboard == "" {
		user.Dashboard = "platform:tenant"
	}

	encoded, err := json.Marshal(mapOrEmpty(input.BackendSetting))
	if err != nil {
		return err
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		if err := tx.Table("platform_user").Create(&user); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE platform_user SET backend_setting = ?::jsonb WHERE id = ?`, string(encoded), user.ID)
		if err != nil {
			return err
		}
		return PlatformPasswordHistoryService().WithContext(s.ctx).RecordWithQuery(tx, user.ID, hash)
	}); err != nil {
		return err
	}
	return nil
}

func (s *PlatformPermissionAdminService) UpdateUser(id uint64, input UserPayload, operatorID uint64) error {
	if input.Password != "" {
		return ErrSensitiveOperationPolicy
	}
	values := map[string]any{"updated_by": operatorID, "updated_at": time.Now()}
	addNonEmpty(values, "nickname", input.Nickname)
	addNonEmpty(values, "phone", input.Phone)
	addNonEmpty(values, "email", input.Email)
	addNonEmpty(values, "avatar", input.Avatar)
	addNonEmpty(values, "signed", input.Signed)
	addNonEmpty(values, "dashboard", input.Dashboard)
	addNonEmpty(values, "remark", input.Remark)
	if input.Status != 0 {
		values["status"] = input.Status
	}
	var encodedSetting []byte
	if input.BackendSetting != nil {
		var err error
		encodedSetting, err = json.Marshal(input.BackendSetting)
		if err != nil {
			return err
		}
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_user").Where("id", id).Update(values)
		if err != nil {
			return err
		}
		if input.BackendSetting != nil {
			_, err = tx.Exec(`UPDATE platform_user SET backend_setting = ?::jsonb WHERE id = ?`, string(encodedSetting), id)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *PlatformPermissionAdminService) DeleteUsers(ids []uint64, currentUserID uint64) error {
	for _, id := range ids {
		if id == currentUserID || id == 1 {
			return BusinessError{Message: "不能删除平台超级管理员"}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_user").WhereIn("id", uint64Any(ids)).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Table("platform_user_belongs_role").WhereIn("user_id", uint64Any(ids)).Delete()
		if err != nil {
			return err
		}
		for _, id := range ids {
			_, err = tx.Table("platform_casbin_rule").Where("ptype", "g").Where("v0", fmt.Sprintf("user:%d", id)).Delete()
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PlatformPermissionAdminService) ResetPassword(userID uint64) error {
	password, err := InitialPassword("")
	if err != nil {
		return err
	}
	if err := PlatformPasswordHistoryService().WithContext(s.ctx).ValidateReuse(userID, password); err != nil {
		return err
	}
	hash, err := makePasswordHash(password)
	if err != nil {
		return err
	}
	return s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err = tx.Table("platform_user").Where("id", userID).Update(map[string]any{
			"password": hash, "updated_at": time.Now(),
		})
		if err != nil {
			return err
		}
		return PlatformPasswordHistoryService().WithContext(s.ctx).RecordWithQuery(tx, userID, hash)
	})
}

func (s *PlatformPermissionAdminService) UserRoles(userID uint64) ([]RoleInfo, error) {
	return NewPlatformPassportService().WithContext(s.ctx).UserRoles(userID)
}

func (s *PlatformPermissionAdminService) SyncUserRoles(userID uint64, roleCodes []string) error {
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_user_belongs_role").Where("user_id", userID).Delete()
		if err != nil {
			return err
		}
		subject := fmt.Sprintf("user:%d", userID)
		_, err = tx.Table("platform_casbin_rule").Where("ptype", "g").Where("v0", subject).Delete()
		if err != nil {
			return err
		}
		for _, code := range roleCodes {
			if err := s.attachUserRole(tx, userID, subject, code); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PlatformPermissionAdminService) attachUserRole(tx contractsorm.Query, userID uint64, subject, code string) error {
	var role models.Role
	if err := tx.Table("platform_role").Where("code", code).First(&role); err != nil {
		return err
	}
	now := time.Now()
	if err := tx.Table("platform_user_belongs_role").Create(&models.UserBelongsRole{
		UserID: userID, RoleID: role.ID,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		return err
	}
	return addPlatformCasbinRule(tx, "g", subject, "role:"+role.Code, "")
}

func (s *PlatformPermissionAdminService) ListRoles(filters map[string]string, page, pageSize int) (request.PageResult[models.Role], error) {
	query := s.orm().Query().Table("platform_role")
	query = query.Scopes(scopes.Contains("name", filters["name"]))
	query = query.Scopes(scopes.Contains("code", filters["code"]))
	query = query.Scopes(scopes.EqualIfPresent("status", filters["status"]))

	result, err := request.Paginate[models.Role](query.OrderBy("sort").OrderBy("id"), page, pageSize)
	if err != nil {
		return request.PageResult[models.Role]{}, err
	}

	return result, nil
}

func (s *PlatformPermissionAdminService) CreateRole(input RolePayload, operatorID uint64) error {
	role := models.Role{
		Name: input.Name, Code: input.Code, Status: statusOrDefault(input.Status),
		Sort: input.Sort, Remark: input.Remark,
		AuditColumns: models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID},
	}
	return s.orm().Query().Table("platform_role").Create(&role)
}

func (s *PlatformPermissionAdminService) UpdateRole(id uint64, input RolePayload, operatorID uint64) error {
	var oldRole models.Role
	if err := s.orm().Query().Table("platform_role").Where("id", id).First(&oldRole); err != nil {
		return err
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_role").Where("id", id).Update(map[string]any{
			"name": input.Name, "code": input.Code, "status": statusOrDefault(input.Status),
			"sort": input.Sort, "remark": input.Remark, "updated_by": operatorID,
			"updated_at": time.Now(),
		})
		if err != nil || oldRole.Code == input.Code {
			return err
		}
		_, err = tx.Table("platform_casbin_rule").
			Where("v0", "role:"+oldRole.Code).
			Update("v0", "role:"+input.Code)
		if err != nil {
			return err
		}
		_, err = tx.Table("platform_casbin_rule").
			Where("ptype", "g").
			Where("v1", "role:"+oldRole.Code).
			Update("v1", "role:"+input.Code)
		return err
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PlatformPermissionAdminService) DeleteRoles(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	count, err := s.orm().Query().Table("platform_role").
		WhereIn("id", uint64Any(ids)).
		Where("code", "PlatformSuperAdmin").
		Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return BusinessError{Message: "不能删除平台超级管理员角色"}
	}
	rows := make([]struct {
		Code string `gorm:"column:code"`
	}, 0, len(ids))
	if err := s.orm().Query().Table("platform_role").
		Select("code").
		WhereIn("id", uint64Any(ids)).
		Scan(&rows); err != nil {
		return err
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_role").WhereIn("id", uint64Any(ids)).Delete()
		if err != nil {
			return err
		}
		for _, row := range rows {
			subject := "role:" + row.Code
			_, err = tx.Table("platform_casbin_rule").Where("v0", subject).Delete()
			if err != nil {
				return err
			}
			_, err = tx.Table("platform_casbin_rule").Where("ptype", "g").Where("v1", subject).Delete()
			if err != nil {
				return err
			}
		}
		_, err = tx.Table("platform_role_belongs_menu").WhereIn("role_id", uint64Any(ids)).Delete()
		return err
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PlatformPermissionAdminService) RolePermissions(roleID uint64) ([]RolePermission, error) {
	permissions := make([]RolePermission, 0)
	err := s.orm().Query().
		Table("platform_menu").
		Select("platform_menu.id", "platform_menu.name").
		Join("JOIN platform_role_belongs_menu rbm ON rbm.menu_id = platform_menu.id").
		Where("rbm.role_id", roleID).
		OrderBy("platform_menu.sort").
		OrderBy("platform_menu.id").
		Scan(&permissions)
	return permissions, err
}

func (s *PlatformPermissionAdminService) SyncRolePermissions(roleID uint64, permissions []string) error {
	var role models.Role
	if err := s.orm().Query().Table("platform_role").Where("id", roleID).First(&role); err != nil {
		return err
	}
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_role_belongs_menu").Where("role_id", roleID).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Table("platform_casbin_rule").Where("ptype", "p").Where("v0", "role:"+role.Code).Delete()
		if err != nil {
			return err
		}

		for _, permission := range permissions {
			var menu models.Menu
			if err := tx.Table("platform_menu").Where("name", permission).First(&menu); err != nil {
				return err
			}
			now := time.Now()
			err := tx.Table("platform_role_belongs_menu").Create(&models.RoleBelongsMenu{
				RoleID: roleID, MenuID: menu.ID,
				Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
			})
			if err != nil {
				return err
			}
			if err := addPlatformCasbinRule(tx, "p", "role:"+role.Code, permission, "*"); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func addPlatformCasbinRule(query contractsorm.Query, ptype, v0, v1, v2 string) error {
	now := time.Now()
	return query.Table("platform_casbin_rule").Create(&models.CasbinRule{
		Ptype: ptype, V0: v0, V1: v1, V2: v2,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	})
}

// Source: platform_permission_menu_service.go
func (s *PlatformPermissionAdminService) ListMenus() ([]AdminMenuItem, error) {
	menus := make([]AdminMenuItem, 0)
	err := s.orm().Query().Table("platform_menu").Where("status", 1).
		OrderBy("sort").OrderBy("id").Scan(&menus)
	if err != nil {
		return nil, err
	}
	return buildAdminMenuTree(menus, 0), nil
}

func (s *PlatformPermissionAdminService) CreateMenu(input MenuPayload, operatorID uint64) error {
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		menuID, err := s.saveMenu(tx, 0, input, operatorID)
		if err != nil {
			return err
		}
		return s.syncButtonPermissions(tx, menuID, input.BtnPermission, operatorID)
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PlatformPermissionAdminService) UpdateMenu(id uint64, input MenuPayload, operatorID uint64) error {
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := s.saveMenu(tx, id, input, operatorID)
		if err != nil {
			return err
		}
		return s.syncButtonPermissions(tx, id, input.BtnPermission, operatorID)
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PlatformPermissionAdminService) DeleteMenus(ids []uint64) error {
	menuIDs, menuNames, err := s.deletedMenuTargets(ids)
	if err != nil {
		return err
	}
	if len(menuIDs) == 0 {
		return nil
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_role_belongs_menu").WhereIn("menu_id", uint64Any(menuIDs)).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Table("platform_casbin_rule").Where("ptype", "p").WhereIn("v1", stringAny(menuNames)).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Table("platform_menu").WhereIn("id", uint64Any(menuIDs)).Delete()
		return err
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PlatformPermissionAdminService) deletedMenuTargets(ids []uint64) ([]uint64, []string, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}

	rows := make([]menuDeleteRow, 0)
	err := s.orm().Query().Table("platform_menu").
		Select("id", "parent_id", "name").
		Scan(&rows)
	if err != nil {
		return nil, nil, err
	}

	menuIDs, menuNames := collectMenuDeleteTargets(rows, ids)
	return menuIDs, menuNames, nil
}

func (s *PlatformPermissionAdminService) saveMenu(tx contractsorm.Query, id uint64, input MenuPayload, operatorID uint64) (uint64, error) {
	if input.Meta == nil {
		input.Meta = models.JSONMap{}
	}
	if input.Status == 0 {
		input.Status = 1
	}
	encodedMeta, err := json.Marshal(input.Meta)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		menu := models.Menu{
			ParentID: input.ParentID, Name: input.Name, Meta: nil,
			Path: input.Path, Component: input.Component, Redirect: input.Redirect,
			Status: input.Status, Sort: input.Sort, Remark: input.Remark,
			AuditColumns: models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID},
		}
		if err := tx.Table("platform_menu").Create(&menu); err != nil {
			return 0, err
		}
		_, err := tx.Exec("UPDATE platform_menu SET meta = ?::jsonb WHERE id = ?", string(encodedMeta), menu.ID)
		if err != nil {
			return 0, err
		}
		return menu.ID, nil
	}

	_, err = tx.Table("platform_menu").Where("id", id).Update(map[string]any{
		"parent_id": input.ParentID, "name": input.Name,
		"path": input.Path, "component": input.Component, "redirect": input.Redirect,
		"status": input.Status, "sort": input.Sort, "remark": input.Remark,
		"updated_by": operatorID, "updated_at": time.Now(),
	})
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec("UPDATE platform_menu SET meta = ?::jsonb WHERE id = ?", string(encodedMeta), id)
	return id, err
}

func (s *PlatformPermissionAdminService) syncButtonPermissions(
	tx contractsorm.Query,
	parentID uint64,
	buttons []MenuPayload,
	operatorID uint64,
) error {
	if buttons == nil {
		return nil
	}
	_, err := tx.Exec(
		"DELETE FROM platform_menu WHERE parent_id = ? AND meta->>'type' = ?",
		parentID,
		"B",
	)
	if err != nil {
		return err
	}
	for _, button := range buttons {
		button.ParentID = parentID
		if button.Meta == nil {
			button.Meta = models.JSONMap{"type": "B"}
		}
		if _, ok := button.Meta["type"]; !ok {
			button.Meta["type"] = "B"
		}
		if _, err := s.saveMenu(tx, 0, button, operatorID); err != nil {
			return err
		}
	}
	return nil
}
