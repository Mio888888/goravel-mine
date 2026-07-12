package services

import (
	"context"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/models"
)

type OrgAdminService struct {
	ctx        context.Context
	connection string
}

type DepartmentPayload struct {
	Name            string `json:"name"`
	ParentID        uint64 `json:"parent_id"`
	DepartmentUsers []any  `json:"department_users"`
	Leader          []any  `json:"leader"`
}

type DepartmentRow struct {
	ID              uint64           `gorm:"column:id" json:"id"`
	Name            string           `gorm:"column:name" json:"name"`
	ParentID        uint64           `gorm:"column:parent_id" json:"parent_id"`
	Children        []DepartmentRow  `gorm:"-" json:"children"`
	Positions       []PositionRow    `gorm:"-" json:"positions"`
	DepartmentUsers []DepartmentUser `gorm:"-" json:"department_users"`
	Leader          []DepartmentUser `gorm:"-" json:"leader"`
}

type DepartmentUser struct {
	ID       uint64 `gorm:"column:id" json:"id"`
	Username string `gorm:"column:username" json:"username"`
	Nickname string `gorm:"column:nickname" json:"nickname"`
	Avatar   string `gorm:"column:avatar" json:"avatar"`
	Phone    string `gorm:"column:phone" json:"phone"`
	Email    string `gorm:"column:email" json:"email"`
}

type PositionPayload struct {
	Name       string           `json:"name"`
	DeptID     uint64           `json:"dept_id"`
	PolicyType PolicyType       `json:"policy_type"`
	Value      models.JSONSlice `json:"value"`
}

type PositionRow struct {
	ID         uint64           `gorm:"column:id" json:"id"`
	Name       string           `gorm:"column:name" json:"name"`
	DeptID     uint64           `gorm:"column:dept_id" json:"dept_id"`
	DeptName   string           `gorm:"column:dept_name" json:"dept_name"`
	PolicyType string           `gorm:"column:policy_type" json:"policy_type,omitempty"`
	Value      models.JSONSlice `gorm:"column:value;type:jsonb" json:"value,omitempty"`
	Policy     *PositionPolicy  `gorm:"-" json:"policy,omitempty"`
}

type PositionPolicy struct {
	PolicyType string           `json:"policy_type"`
	Value      models.JSONSlice `json:"value"`
}

type LeaderPayload struct {
	DeptID  uint64 `json:"dept_id"`
	UserID  []any  `json:"user_id"`
	UserIDs []any  `json:"user_ids"`
}

type LeaderRow struct {
	DeptID   uint64           `gorm:"column:dept_id" json:"dept_id"`
	UserID   uint64           `gorm:"column:user_id" json:"user_id"`
	DeptName string           `gorm:"column:dept_name" json:"dept_name"`
	Username string           `gorm:"column:username" json:"username"`
	Nickname string           `gorm:"column:nickname" json:"nickname"`
	Phone    string           `gorm:"column:phone" json:"phone"`
	Email    string           `gorm:"column:email" json:"email"`
	User     DepartmentUser   `gorm:"-" json:"user"`
	Users    []DepartmentUser `gorm:"-" json:"users,omitempty"`
}

func NewOrgAdminService() *OrgAdminService {
	return &OrgAdminService{}
}

func NewOrgAdminServiceForTenant(tenant Tenant) *OrgAdminService {
	return &OrgAdminService{connection: TenantConnectionName(tenant)}
}

func (s *OrgAdminService) WithContext(ctx context.Context) *OrgAdminService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *OrgAdminService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}
