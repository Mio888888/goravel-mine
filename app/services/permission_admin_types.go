package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/models"
)

const defaultPassword = "123456"

var ErrBusinessRule = errors.New("business rule violation")

type BusinessError struct {
	Message string
}

func (e BusinessError) Error() string {
	return e.Message
}

func (e BusinessError) Unwrap() error {
	return ErrBusinessRule
}

type PermissionAdminService struct {
	ctx        context.Context
	connection string
	tenant     Tenant
}

func (s *PermissionAdminService) invalidateCasbinEnforcer() {
	InvalidateCasbinEnforcer("tenant:" + s.connection)
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

func applyStringFilter(query contractsorm.Query, column, value string) contractsorm.Query {
	if value == "" {
		return query
	}
	return query.Where(column+" LIKE ?", "%"+value+"%")
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
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
