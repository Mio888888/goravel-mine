package auth

import (
	"context"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	tenantcontract "goravel/app/contracts/tenant"
	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/support/apperror"
)

type Tenant = tenantcontract.Tenant

type PasswordHistoryService struct {
	ctx        context.Context
	connection string
	table      string
}

func NewPasswordHistoryService(connection, table string) *PasswordHistoryService {
	return &PasswordHistoryService{connection: connection, table: table}
}

func TenantPasswordHistoryService(tenant Tenant) *PasswordHistoryService {
	return NewPasswordHistoryService(tenantcontract.ConnectionName(tenant), "user_password_history")
}

func PlatformPasswordHistoryService() *PasswordHistoryService {
	return NewPasswordHistoryService(PlatformConnection(), "platform_user_password_history")
}

func (s *PasswordHistoryService) WithContext(ctx context.Context) *PasswordHistoryService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *PasswordHistoryService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func (s *PasswordHistoryService) ValidateReuse(userID uint64, password string) error {
	limit := facades.Config().GetInt("security.password.history_limit", 0)
	if limit <= 0 || password == "" {
		return nil
	}
	rows := make([]models.UserPasswordHistory, 0, limit)
	err := s.orm().Query().Table(s.table).Where("user_id", userID).OrderByDesc("id").Limit(limit).Get(&rows)
	if err != nil {
		return err
	}
	if passwordMatchesHistory(rows, password) {
		return apperror.BusinessError{Message: "不能使用最近使用过的密码"}
	}
	return nil
}

func (s *PasswordHistoryService) Record(userID uint64, passwordHash string) error {
	return s.RecordWithQuery(s.orm().Query(), userID, passwordHash)
}

func (s *PasswordHistoryService) RecordWithQuery(query contractsorm.Query, userID uint64, passwordHash string) error {
	return s.RecordWithQueryAt(query, userID, passwordHash, time.Now())
}

func (s *PasswordHistoryService) RecordWithQueryAt(query contractsorm.Query, userID uint64, passwordHash string, changedAt time.Time) error {
	if passwordHash == "" {
		return nil
	}
	if changedAt.IsZero() {
		changedAt = time.Now()
	}
	return query.Table(s.table).Create(&models.UserPasswordHistory{
		UserID:     userID,
		Password:   passwordHash,
		Timestamps: models.Timestamps{CreatedAt: changedAt, UpdatedAt: changedAt},
	})
}

func (s *PasswordHistoryService) SeedIfMissing(user models.User) error {
	userID := user.ID
	passwordHash := user.Password
	if passwordHash == "" {
		return nil
	}
	count, err := s.orm().Query().Table(s.table).Where("user_id", userID).Count()
	if err != nil || count > 0 {
		return err
	}
	return s.RecordWithQueryAt(s.orm().Query(), userID, passwordHash, passwordChangedAt(user))
}

func passwordMatchesHistory(rows []models.UserPasswordHistory, password string) bool {
	for _, row := range rows {
		if PasswordHashMatches(row.Password, password) {
			return true
		}
	}
	return false
}

func passwordChangedAt(user models.User) time.Time {
	if !user.UpdatedAt.IsZero() {
		return user.UpdatedAt
	}
	if !user.CreatedAt.IsZero() {
		return user.CreatedAt
	}
	return time.Time{}
}

func (s *PasswordHistoryService) CheckMaxAge(userID uint64) error {
	maxAge := facades.Config().GetInt("security.password.max_age_days", 0)
	if maxAge <= 0 {
		return nil
	}
	var row models.UserPasswordHistory
	err := s.orm().Query().Table(s.table).Where("user_id", userID).OrderByDesc("id").First(&row)
	if err != nil || row.CreatedAt.IsZero() {
		return nil
	}
	if time.Since(row.CreatedAt) > time.Duration(maxAge)*24*time.Hour {
		return apperror.BusinessError{Message: "密码已过期，请修改密码"}
	}
	return nil
}
