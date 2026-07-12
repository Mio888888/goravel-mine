package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"goravel/app/models"
)

type UserInfo struct {
	ID             uint64           `json:"id"`
	Username       string           `json:"username"`
	Nickname       string           `json:"nickname"`
	Avatar         string           `json:"avatar"`
	Signed         string           `json:"signed"`
	Dashboard      string           `json:"dashboard"`
	BackendSetting models.JSONMap   `json:"backend_setting,omitempty"`
	Phone          string           `json:"phone"`
	Email          string           `json:"email"`
	Departments    []DepartmentInfo `json:"departments"`
	Positions      []PositionInfo   `json:"positions"`
	Roles          []RoleInfo       `json:"roles"`
}

type DepartmentInfo struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

type PositionInfo struct {
	ID     uint64 `json:"id"`
	DeptID uint64 `json:"dept_id"`
	Name   string `json:"name"`
}

type RoleInfo struct {
	ID   uint64 `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type ProfileUpdate struct {
	Nickname                string         `json:"nickname"`
	Avatar                  string         `json:"avatar"`
	Signed                  string         `json:"signed"`
	BackendSetting          models.JSONMap `json:"backend_setting"`
	OldPassword             string         `json:"old_password"`
	NewPassword             string         `json:"new_password"`
	NewPasswordConfirmation string         `json:"new_password_confirmation"`
}

type TokenInfo struct {
	UserID   uint64
	TenantID uint64
}

func (s *PassportService) UserIDFromAuthorization(authorization, tokenType string) (uint64, error) {
	tokenInfo, err := s.TokenInfoFromAuthorization(authorization, tokenType)
	if err != nil {
		return 0, err
	}
	return tokenInfo.UserID, nil
}

func (s *PassportService) TokenInfoFromAuthorization(authorization, tokenType string) (TokenInfo, error) {
	tokenText := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	if tokenText == "" || tokenText == authorization {
		return TokenInfo{}, ErrUnauthorized
	}

	claims := jwt.MapClaims{}
	secret, err := JWTSecret()
	if err != nil {
		return TokenInfo{}, err
	}

	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return TokenInfo{}, ErrUnauthorized
	}

	if claims["type"] != tokenType {
		return TokenInfo{}, ErrUnauthorized
	}

	userID, err := claimUint64(claims["uid"])
	if err != nil || userID == 0 {
		return TokenInfo{}, ErrUnauthorized
	}
	tenantID, err := claimUint64(claims["tid"])
	if err != nil || tenantID == 0 {
		return TokenInfo{}, ErrUnauthorized
	}

	return TokenInfo{UserID: userID, TenantID: tenantID}, nil
}

func (s *PassportService) FormatUserInfo(user models.User) (UserInfo, error) {
	if info, ok := cachedCurrentUserInfo(s.connection, user.ID); ok {
		return info, nil
	}

	info, err := s.buildUserInfo(user)
	if err != nil {
		return UserInfo{}, err
	}

	cacheCurrentUserInfo(s.connection, info)
	return info, nil
}

func (s *PassportService) buildUserInfo(user models.User) (UserInfo, error) {
	roles, err := s.UserRoles(user.ID)
	if err != nil {
		return UserInfo{}, err
	}

	departments, err := s.UserDepartments(user.ID)
	if err != nil {
		return UserInfo{}, err
	}

	positions, err := s.UserPositions(user.ID)
	if err != nil {
		return UserInfo{}, err
	}

	backendSetting := user.BackendSetting
	if len(backendSetting) == 0 {
		backendSetting = nil
	}

	return UserInfo{
		ID:             user.ID,
		Username:       user.Username,
		Nickname:       user.Nickname,
		Avatar:         user.Avatar,
		Signed:         user.Signed,
		Dashboard:      user.Dashboard,
		BackendSetting: backendSetting,
		Phone:          user.Phone,
		Email:          user.Email,
		Departments:    departments,
		Positions:      positions,
		Roles:          roles,
	}, nil
}

func (s *PassportService) UserRoles(userID uint64) ([]RoleInfo, error) {
	roles := make([]RoleInfo, 0)
	err := s.orm().Query().
		Table("role").
		Select("role.id", "role.code", "role.name").
		Join("JOIN user_belongs_role ubr ON ubr.role_id = role.id").
		Where("ubr.user_id", userID).
		Where("role.status", 1).
		OrderBy("role.sort").
		OrderBy("role.id").
		Scan(&roles)

	return roles, err
}

func (s *PassportService) UserDepartments(userID uint64) ([]DepartmentInfo, error) {
	departments := make([]DepartmentInfo, 0)
	err := s.orm().Query().
		Table("department").
		Select("department.id", "department.name").
		Join("JOIN user_dept ud ON ud.dept_id = department.id").
		Where("ud.user_id", userID).
		WhereNull("department.deleted_at").
		WhereNull("ud.deleted_at").
		OrderBy("department.id").
		Scan(&departments)

	return departments, err
}

func (s *PassportService) UserPositions(userID uint64) ([]PositionInfo, error) {
	positions := make([]PositionInfo, 0)
	err := s.orm().Query().
		Table("position").
		Select("position.id", "position.dept_id", "position.name").
		Join("JOIN user_position up ON up.position_id = position.id").
		Where("up.user_id", userID).
		WhereNull("position.deleted_at").
		WhereNull("up.deleted_at").
		OrderBy("position.id").
		Scan(&positions)

	return positions, err
}

func (s *PassportService) IsSuperAdmin(user models.User) (bool, error) {
	if user.ID == 1 {
		return true, nil
	}

	roles, err := s.UserRoles(user.ID)
	if err != nil {
		return false, err
	}

	for _, role := range roles {
		if role.Code == "SuperAdmin" {
			return true, nil
		}
	}

	return false, nil
}

func (s *PassportService) UpdateProfile(userID uint64, input ProfileUpdate) error {
	values, err := profileUpdateValues(input)
	if err != nil {
		return err
	}
	if err := s.appendPasswordUpdate(userID, input, values); err != nil {
		return err
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Model(&models.User{}).
			Where("id", userID).
			Update(values)
		if err != nil {
			return err
		}
		if hash, ok := values["password"].(string); ok && hash != "" {
			return NewPasswordHistoryService(s.connection, "user_password_history").WithContext(s.ctx).RecordWithQuery(tx, userID, hash)
		}
		return nil
	}); err != nil {
		return err
	}
	InvalidateCurrentUserInfoForConnection(s.connection, userID)
	return nil
}

func profileUpdateValues(input ProfileUpdate) (map[string]any, error) {
	values := map[string]any{"updated_at": time.Now()}
	if input.Nickname != "" {
		values["nickname"] = input.Nickname
	}
	if input.Avatar != "" {
		values["avatar"] = input.Avatar
	}
	if input.Signed != "" {
		values["signed"] = input.Signed
	}
	if input.BackendSetting != nil {
		backendSetting, err := json.Marshal(input.BackendSetting)
		if err != nil {
			return nil, err
		}
		values["backend_setting"] = string(backendSetting)
	}
	return values, nil
}

func (s *PassportService) ensureTokenTenant(tokenTenantID uint64) error {
	if s.tenant.ID == 0 || s.tenant.ID != tokenTenantID {
		return ErrUnauthorized
	}
	return nil
}

func claimUint64(value any) (uint64, error) {
	switch v := value.(type) {
	case float64:
		return uint64(v), nil
	case int:
		return uint64(v), nil
	case int64:
		return uint64(v), nil
	case uint64:
		return v, nil
	case string:
		return strconv.ParseUint(v, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported uid claim type %T", value)
	}
}

func IsProfileValidationError(err error) bool {
	return errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrBusinessRule)
}
