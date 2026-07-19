package services

import (
	"context"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/http/request"
	"goravel/app/models"
)

type SSOAuditService struct {
	ctx        context.Context
	connection string
}

func NewSSOAuditServiceForTenant(tenant Tenant) *SSOAuditService {
	return &SSOAuditService{connection: TenantConnectionName(tenant)}
}

func NewSSOAuditServiceForConnection(connection string) *SSOAuditService {
	return &SSOAuditService{connection: connection}
}

func (s *SSOAuditService) WithContext(ctx context.Context) *SSOAuditService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *SSOAuditService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func (s *SSOAuditService) ListBindings(filters map[string]string, page, pageSize int) (request.PageResult[SSOUserBindingRow], error) {
	result, err := request.Paginate[SSOUserBindingRow](ssoBindingFilters(s.bindingRowsQuery(), filters).OrderByDesc("sso_user_binding.id"), page, pageSize)
	if err != nil {
		return request.PageResult[SSOUserBindingRow]{}, err
	}
	result.List = formatSSOBindingRows(result.List)
	return result, nil
}

func (s *SSOAuditService) Binding(id uint64) (SSOUserBindingRow, error) {
	var row SSOUserBindingRow
	err := s.bindingRowsQuery().Where("sso_user_binding.id", id).First(&row)
	return formatSSOBindingRow(row), err
}

func (s *SSOAuditService) UserBindings(userID uint64) ([]SSOUserBindingRow, error) {
	rows := make([]SSOUserBindingRow, 0)
	err := s.bindingRowsQuery().
		Where("sso_user_binding.user_id", userID).
		OrderByDesc("sso_user_binding.id").
		Scan(&rows)
	if err != nil {
		return nil, err
	}
	return formatSSOBindingRows(rows), nil
}

func (s *SSOAuditService) BoundUser(providerID uint64, ssoUserID string) (models.User, error) {
	var user models.User
	err := s.orm().Query().
		Table(`"user"`).
		Select(`"user".*`).
		Join("JOIN sso_user_binding ON sso_user_binding.user_id = \"user\".id").
		Where("sso_user_binding.provider_id", providerID).
		Where("sso_user_binding.sso_user_id", ssoUserID).
		First(&user)
	return user, err
}

func (s *SSOAuditService) DeleteBinding(id uint64) error {
	if id == 0 {
		return nil
	}
	result, err := s.orm().Query().Table("sso_user_binding").Where("id", id).Delete()
	if err != nil {
		return err
	}
	if result.RowsAffected == 0 {
		return BusinessError{Message: "SSO 用户绑定不存在"}
	}
	return nil
}

func (s *SSOAuditService) ListLoginLogs(filters map[string]string, page, pageSize int) (request.PageResult[SSOLoginLogRow], error) {
	result, err := request.Paginate[SSOLoginLogRow](ssoLoginLogFilters(s.loginRowsQuery(), filters).OrderByDesc("sso_login_log.id"), page, pageSize)
	if err != nil {
		return request.PageResult[SSOLoginLogRow]{}, err
	}
	result.List = formatSSOLoginLogRows(result.List)
	return result, nil
}

func (s *SSOAuditService) LoginStats(filters map[string]string) (SSOLoginStats, error) {
	query := ssoLoginLogFilters(s.loginStatsBaseQuery(), filters)
	total, err := query.Count()
	if err != nil {
		return SSOLoginStats{}, err
	}
	success, err := ssoLoginLogFilters(s.loginStatsBaseQuery(), filters).
		Where("sso_login_log.status", ssoLogStatusSuccess).
		Count()
	if err != nil {
		return SSOLoginStats{}, err
	}
	providers := make([]SSOProviderLogStatRow, 0)
	err = ssoLoginLogFilters(s.loginStatsBaseQuery().
		Select(
			"sso_login_log.provider_id",
			"sso_provider.name AS provider_name",
			"sso_provider.display_name AS provider_display_name",
			"COUNT(*) AS total",
			"SUM(CASE WHEN sso_login_log.status = 1 THEN 1 ELSE 0 END) AS success_count",
			"SUM(CASE WHEN sso_login_log.status = 2 THEN 1 ELSE 0 END) AS fail_count",
		), filters).
		GroupBy("sso_login_log.provider_id", "sso_provider.name", "sso_provider.display_name").
		OrderByDesc("total").
		Scan(&providers)
	if err != nil {
		return SSOLoginStats{}, err
	}
	rate := float64(0)
	if total > 0 {
		rate = float64(success) / float64(total) * 100
	}
	return SSOLoginStats{
		Total: total, SuccessCount: success, FailCount: total - success,
		SuccessRate: roundFloat(rate, 2), Providers: providers,
	}, nil
}

func (s *SSOAuditService) loginStatsBaseQuery() contractsorm.Query {
	return s.orm().Query().
		Table("sso_login_log").
		Join(`LEFT JOIN "user" ON "user".id = sso_login_log.user_id`).
		Join("LEFT JOIN sso_provider ON sso_provider.id = sso_login_log.provider_id")
}

func (s *SSOAuditService) UpsertBinding(input ssoBindingInput) (models.SSOUserBinding, error) {
	now := time.Now()
	var binding models.SSOUserBinding
	err := s.orm().Query().
		Table("sso_user_binding").
		Where("provider_id", input.ProviderID).
		Where("sso_user_id", input.SSOUserID).
		First(&binding)
	if err != nil {
		return models.SSOUserBinding{}, err
	}
	if binding.ID != 0 {
		values := map[string]any{
			"user_id":       input.UserID,
			"sso_email":     input.SSOEmail,
			"sso_username":  input.SSOUsername,
			"sso_avatar":    input.SSOAvatar,
			"access_token":  input.AccessToken,
			"refresh_token": input.RefreshToken,
			"last_login_at": now,
			"login_count":   binding.LoginCount + 1,
			"updated_at":    now,
		}
		if !input.ExpiresAt.IsZero() {
			values["token_expires_at"] = input.ExpiresAt
		}
		_, err = s.orm().Query().Table("sso_user_binding").Where("id", binding.ID).Update(values)
		if err != nil {
			return models.SSOUserBinding{}, err
		}
		binding.UserID = input.UserID
		binding.SSOEmail = input.SSOEmail
		binding.SSOUsername = input.SSOUsername
		binding.SSOAvatar = input.SSOAvatar
		binding.LastLoginAt = now
		binding.LoginCount++
		return binding, nil
	}
	binding = models.SSOUserBinding{
		UserID: input.UserID, ProviderID: input.ProviderID, SSOUserID: input.SSOUserID,
		SSOEmail: input.SSOEmail, SSOUsername: input.SSOUsername, SSOAvatar: input.SSOAvatar,
		AccessToken: input.AccessToken, RefreshToken: input.RefreshToken, TokenExpiresAt: input.ExpiresAt,
		FirstLoginAt: now, LastLoginAt: now, LoginCount: 1,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}
	if err := s.orm().Query().Create(&binding); err != nil {
		return models.SSOUserBinding{}, err
	}
	return binding, nil
}

func (s *SSOAuditService) Log(input ssoLogInput) error {
	if input.ProviderID == 0 {
		return nil
	}
	if input.Status == 0 {
		input.Status = ssoLogStatusSuccess
	}
	return s.orm().Query().Create(&models.SSOLoginLog{
		UserID: input.UserID, ProviderID: input.ProviderID, BindingID: input.BindingID,
		SSOUserID: input.SSOUserID, SSOEmail: input.SSOEmail, Status: input.Status,
		FailureReason: input.FailureReason, IP: input.IP, UserAgent: input.UserAgent,
		DeviceType: detectDeviceType(input.UserAgent), LoginAt: time.Now(),
	})
}
