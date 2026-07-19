package services

import (
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"goravel/app/http/request"
	"goravel/app/models"
)

func (s *PermissionAdminService) ListRoles(filters map[string]string, page, pageSize int) (request.PageResult[models.Role], error) {
	query := s.orm().Query().Table("role")
	query = applyStringFilter(query, "name", filters["name"])
	query = applyStringFilter(query, "code", filters["code"])
	if filters["status"] != "" {
		query = query.Where("status", filters["status"])
	}

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
