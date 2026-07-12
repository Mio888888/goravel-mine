package services

import (
	"fmt"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"goravel/app/models"
)

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
