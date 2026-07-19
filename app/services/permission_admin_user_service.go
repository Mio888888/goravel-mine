package services

import (
	"encoding/json"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/http/request"
	"goravel/app/models"
)

func (s *PermissionAdminService) ListUsers(filters map[string]string, page, pageSize int, currentUserID uint64) (request.PageResult[UserRow], error) {
	query := s.orm().Query().Table(`"user"`).Where("user_type", "100")
	var err error
	query, err = s.applyUserDataScope(query, currentUserID)
	if err != nil {
		return request.PageResult[UserRow]{}, err
	}
	query = applyStringFilter(query, "username", filters["username"])
	query = applyStringFilter(query, "nickname", filters["nickname"])
	query = applyStringFilter(query, "phone", filters["phone"])
	query = applyStringFilter(query, "email", filters["email"])
	if filters["status"] != "" {
		query = query.Where("status", filters["status"])
	}

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
