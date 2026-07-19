package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
)

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
