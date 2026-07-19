package application

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/beevik/etree"
	"github.com/golang-jwt/jwt/v5"
	contractscache "github.com/goravel/framework/contracts/cache"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	dsig "github.com/russellhaering/goxmldsig"
	"goravel/app/facades"
	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
	ssoservice "goravel/app/services/access/sso"
	"io"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Source: sso.go
func evalSSOCondition(condition string, claims map[string]any) bool {
	return ssoservice.EvalCondition(condition, claims)
}

// Source: sso_audit_format.go
func formatSSOBindingRows(rows []SSOUserBindingRow) []SSOUserBindingRow {
	out := make([]SSOUserBindingRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, formatSSOBindingRow(row))
	}
	return out
}

func formatSSOBindingRow(row SSOUserBindingRow) SSOUserBindingRow {
	row.FirstLoginAt = formatMaybeLogTime(row.FirstLoginAt)
	row.LastLoginAt = formatMaybeLogTime(row.LastLoginAt)
	row.TokenExpiresAt = formatMaybeLogTime(row.TokenExpiresAt)
	row.CreatedAt = formatMaybeLogTime(row.CreatedAt)
	row.UpdatedAt = formatMaybeLogTime(row.UpdatedAt)
	return row
}

func formatSSOLoginLogRows(rows []SSOLoginLogRow) []SSOLoginLogRow {
	out := make([]SSOLoginLogRow, 0, len(rows))
	for _, row := range rows {
		row.LoginAt = formatMaybeLogTime(row.LoginAt)
		out = append(out, row)
	}
	return out
}

func formatMaybeLogTime(value string) string {
	if value == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return formatLogTime(parsed)
	}
	if parsed, err := time.Parse("2006-01-02T15:04:05Z07:00", value); err == nil {
		return formatLogTime(parsed)
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05 -0700 MST", value); err == nil {
		return formatLogTime(parsed)
	}
	return strings.TrimSuffix(strings.ReplaceAll(value, "T", " "), "Z")
}

func detectDeviceType(userAgent string) string {
	lower := strings.ToLower(userAgent)
	switch {
	case strings.Contains(lower, "ipad") || strings.Contains(lower, "tablet"):
		return "tablet"
	case strings.Contains(lower, "mobile") || strings.Contains(lower, "iphone") || strings.Contains(lower, "android"):
		return "mobile"
	case strings.TrimSpace(lower) == "":
		return "unknown"
	default:
		return "desktop"
	}
}

func roundFloat(value float64, places int) float64 {
	factor := math.Pow(10, float64(places))
	return math.Round(value*factor) / factor
}

func ssoFailureMessage(err error) string {
	switch {
	case errors.Is(err, ErrSSONotConfigured):
		return "SSO 未配置或已停用"
	case errors.Is(err, ErrSSOTokenInvalid):
		return "SSO Token 无效"
	case errors.Is(err, ErrUnauthorized):
		return "未登录或登录已过期"
	case errors.Is(err, ErrUserDisabled):
		return "用户已停用"
	case errors.Is(err, ErrQuotaExceeded):
		return "租户配额已用尽"
	case errors.Is(err, ErrSubscriptionInactive):
		return "租户订阅不可用"
	default:
		var businessErr BusinessError
		if errors.As(err, &businessErr) {
			return businessErr.Message
		}
		return "服务器错误"
	}
}

// Source: sso_audit_query.go
func (s *SSOAuditService) bindingRowsQuery() contractsorm.Query {
	return s.orm().Query().
		Table("sso_user_binding").
		Select(
			"sso_user_binding.id",
			"sso_user_binding.user_id",
			`"user".username`,
			`"user".nickname`,
			"sso_user_binding.provider_id",
			"sso_provider.name AS provider_name",
			"sso_provider.display_name AS provider_display_name",
			"sso_provider.type AS provider_type",
			"sso_provider.scene AS provider_scene",
			"sso_user_binding.sso_user_id",
			"sso_user_binding.sso_email",
			"sso_user_binding.sso_username",
			"sso_user_binding.sso_avatar",
			"sso_user_binding.login_count",
			"sso_user_binding.first_login_at",
			"sso_user_binding.last_login_at",
			"sso_user_binding.token_expires_at",
			"sso_user_binding.created_at",
			"sso_user_binding.updated_at",
		).
		Join(`LEFT JOIN "user" ON "user".id = sso_user_binding.user_id`).
		Join("LEFT JOIN sso_provider ON sso_provider.id = sso_user_binding.provider_id")
}

func (s *SSOAuditService) loginRowsQuery() contractsorm.Query {
	return s.orm().Query().
		Table("sso_login_log").
		Select(
			"sso_login_log.id",
			"sso_login_log.user_id",
			`"user".username`,
			"sso_login_log.provider_id",
			"sso_provider.name AS provider_name",
			"sso_provider.display_name AS provider_display_name",
			"sso_provider.type AS provider_type",
			"sso_provider.scene AS provider_scene",
			"sso_login_log.binding_id",
			"sso_login_log.sso_user_id",
			"sso_login_log.sso_email",
			"sso_login_log.status",
			"sso_login_log.failure_reason",
			"sso_login_log.ip",
			"sso_login_log.user_agent",
			"sso_login_log.device_type",
			"sso_login_log.login_at",
		).
		Join(`LEFT JOIN "user" ON "user".id = sso_login_log.user_id`).
		Join("LEFT JOIN sso_provider ON sso_provider.id = sso_login_log.provider_id")
}

func ssoBindingFilters(query contractsorm.Query, filters map[string]string) contractsorm.Query {
	query = query.Scopes(scopes.Equal("sso_user_binding.user_id", filters["user_id"]))
	query = query.Scopes(scopes.Equal("sso_user_binding.provider_id", filters["provider_id"]))
	query = query.Scopes(scopes.Contains("sso_user_binding.sso_user_id", filters["sso_user_id"]))
	query = query.Scopes(scopes.Contains("sso_user_binding.sso_email", filters["sso_email"]))
	query = query.Scopes(scopes.Contains("sso_user_binding.sso_username", filters["sso_username"]))
	query = query.Scopes(scopes.Contains("sso_provider.name", filters["provider_name"]))
	return query.Scopes(scopes.Contains(`"user".username`, filters["username"]))
}

func ssoLoginLogFilters(query contractsorm.Query, filters map[string]string) contractsorm.Query {
	query = query.Scopes(scopes.Equal("sso_login_log.user_id", filters["user_id"]))
	query = query.Scopes(scopes.Equal("sso_login_log.provider_id", filters["provider_id"]))
	query = query.Scopes(scopes.Equal("sso_login_log.status", filters["status"]))
	query = query.Scopes(scopes.Contains("sso_login_log.sso_user_id", filters["sso_user_id"]))
	query = query.Scopes(scopes.Contains("sso_login_log.sso_email", filters["sso_email"]))
	query = query.Scopes(scopes.Contains("sso_provider.name", filters["provider_name"]))
	query = query.Scopes(scopes.Contains(`"user".username`, filters["username"]))
	return query.Scopes(
		scopes.GreaterThanOrEqual("sso_login_log.login_at", filters["start_date"]),
		scopes.LessThanOrEqual("sso_login_log.login_at", filters["end_date"]),
	)
}

// Source: sso_audit_service.go
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

// Source: sso_audit_types.go
const (
	ssoLogStatusSuccess int16 = 1
	ssoLogStatusFailure int16 = 2
)

type SSOUserBindingRow struct {
	ID                  uint64 `json:"id"`
	UserID              uint64 `json:"user_id"`
	Username            string `json:"username"`
	Nickname            string `json:"nickname"`
	ProviderID          uint64 `json:"provider_id"`
	ProviderName        string `json:"provider_name"`
	ProviderDisplayName string `json:"provider_display_name"`
	ProviderType        string `json:"provider_type"`
	ProviderScene       string `json:"provider_scene"`
	SSOUserID           string `json:"sso_user_id"`
	SSOEmail            string `json:"sso_email"`
	SSOUsername         string `json:"sso_username"`
	SSOAvatar           string `json:"sso_avatar"`
	LoginCount          int    `json:"login_count"`
	FirstLoginAt        string `json:"first_login_at"`
	LastLoginAt         string `json:"last_login_at"`
	TokenExpiresAt      string `json:"token_expires_at"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type SSOLoginLogRow struct {
	ID                  uint64 `json:"id"`
	UserID              uint64 `json:"user_id"`
	Username            string `json:"username"`
	ProviderID          uint64 `json:"provider_id"`
	ProviderName        string `json:"provider_name"`
	ProviderDisplayName string `json:"provider_display_name"`
	ProviderType        string `json:"provider_type"`
	ProviderScene       string `json:"provider_scene"`
	BindingID           uint64 `json:"binding_id"`
	SSOUserID           string `json:"sso_user_id"`
	SSOEmail            string `json:"sso_email"`
	Status              int16  `json:"status"`
	FailureReason       string `json:"failure_reason"`
	IP                  string `json:"ip"`
	UserAgent           string `json:"user_agent"`
	DeviceType          string `json:"device_type"`
	LoginAt             string `json:"login_at"`
}

type SSOLoginStats struct {
	Total        int64                   `json:"total"`
	SuccessCount int64                   `json:"success_count"`
	FailCount    int64                   `json:"fail_count"`
	SuccessRate  float64                 `json:"success_rate"`
	Providers    []SSOProviderLogStatRow `json:"providers"`
}

type SSOProviderLogStatRow struct {
	ProviderID          uint64 `json:"provider_id"`
	ProviderName        string `json:"provider_name"`
	ProviderDisplayName string `json:"provider_display_name"`
	Total               int64  `json:"total"`
	SuccessCount        int64  `json:"success_count"`
	FailCount           int64  `json:"fail_count"`
}

type ssoBindingInput struct {
	UserID       uint64
	ProviderID   uint64
	SSOUserID    string
	SSOEmail     string
	SSOUsername  string
	SSOAvatar    string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type ssoLogInput struct {
	UserID        uint64
	ProviderID    uint64
	BindingID     uint64
	SSOUserID     string
	SSOEmail      string
	Status        int16
	FailureReason string
	IP            string
	UserAgent     string
}

// Source: sso_authorization_transaction_service.go
const (
	ssoAuthorizationTransactionPrefix     = "sso:authorization:transaction:"
	ssoAuthorizationTransactionLockPrefix = "sso:authorization:transaction:lock:"
	ssoAuthorizationTransactionUsedPrefix = "sso:authorization:transaction:used:"
	ssoAuthorizationVerifiedPrefix        = "sso:authorization:verified:"
	ssoAuthorizationTransactionMaxTTL     = 5 * time.Minute
	ssoAuthorizationTransactionLockTTL    = 30 * time.Second
)

var (
	ErrSSOAuthorizationTransactionInvalid = errors.New("sso authorization transaction is invalid")
	ErrSSOAuthorizationTransactionExpired = errors.New("sso authorization transaction has expired")
	ErrSSOAuthorizationTransactionReused  = errors.New("sso authorization transaction has already been used")
)

var ssoAuthorizationTransactionCache = func() contractscache.Driver {
	return facades.Cache()
}

var ssoAuthorizationTransactionNow = time.Now

type SSOAuthorizationTransaction struct {
	ID           string    `json:"id"`
	TenantCode   string    `json:"tenant_code"`
	Provider     string    `json:"provider"`
	Scene        string    `json:"scene"`
	State        string    `json:"state"`
	Nonce        string    `json:"nonce"`
	CodeVerifier string    `json:"code_verifier"`
	RedirectURI  string    `json:"redirect_uri"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type SSOAuthorizationResult struct {
	TransactionID    string `json:"transaction_id"`
	State            string `json:"state"`
	AuthorizationURL string `json:"authorization_url"`
}

type ssoVerifiedAuthorization struct {
	TenantCode string    `json:"tenant_code"`
	ProviderID uint64    `json:"provider_id"`
	Provider   string    `json:"provider"`
	Scene      string    `json:"scene"`
	Claims     ssoClaims `json:"claims"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type SSOAuthorizationTransactionService struct{}

func NewSSOAuthorizationTransactionService() *SSOAuthorizationTransactionService {
	return &SSOAuthorizationTransactionService{}
}

func (s *SSOAuthorizationTransactionService) Create(tenant Tenant, provider SSOProvider) (SSOAuthorizationResult, error) {
	provider = withSSODiscoveryDefaults(provider)
	if tenant.ID == 0 || strings.TrimSpace(tenant.Code) == "" ||
		strings.TrimSpace(provider.Name) == "" || strings.TrimSpace(provider.AuthorizationEndpoint) == "" ||
		strings.TrimSpace(provider.ClientID) == "" || strings.TrimSpace(provider.RedirectURI) == "" {
		return SSOAuthorizationResult{}, ErrSSOAuthorizationTransactionInvalid
	}

	id, err := newSSOAuthorizationRandomValue(24)
	if err != nil {
		return SSOAuthorizationResult{}, err
	}
	state, err := newSSOAuthorizationRandomValue(32)
	if err != nil {
		return SSOAuthorizationResult{}, err
	}
	transaction := SSOAuthorizationTransaction{
		ID:          id,
		TenantCode:  tenant.Code,
		Provider:    provider.Name,
		Scene:       normalizeSSOScene(provider.Scene),
		State:       state,
		RedirectURI: provider.RedirectURI,
		ExpiresAt:   ssoAuthorizationTransactionNow().Add(ssoAuthorizationTransactionMaxTTL),
	}
	if provider.EnableNonce || provider.Type == "oidc" {
		transaction.Nonce, err = newSSOAuthorizationRandomValue(32)
		if err != nil {
			return SSOAuthorizationResult{}, err
		}
	}
	if provider.EnablePKCE || provider.Type == "oidc" {
		transaction.CodeVerifier, err = newSSOAuthorizationRandomValue(48)
		if err != nil {
			return SSOAuthorizationResult{}, err
		}
	}
	if err := s.Store(transaction); err != nil {
		return SSOAuthorizationResult{}, err
	}

	authorizationURL, err := s.authorizationURL(provider, transaction)
	if err != nil {
		_ = ssoAuthorizationTransactionCache().Forget(ssoAuthorizationTransactionKey(transaction.ID))
		return SSOAuthorizationResult{}, err
	}
	return SSOAuthorizationResult{
		TransactionID:    transaction.ID,
		State:            transaction.State,
		AuthorizationURL: authorizationURL,
	}, nil
}

func (s *SSOAuthorizationTransactionService) Store(transaction SSOAuthorizationTransaction) error {
	transaction = normalizeSSOAuthorizationTransaction(transaction)
	if !validSSOAuthorizationTransaction(transaction) {
		return ErrSSOAuthorizationTransactionInvalid
	}
	ttl := time.Until(transaction.ExpiresAt)
	if ttl <= 0 || ttl > ssoAuthorizationTransactionMaxTTL {
		return ErrSSOAuthorizationTransactionExpired
	}
	raw, err := json.Marshal(transaction)
	if err != nil {
		return err
	}
	return ssoAuthorizationTransactionCache().Put(ssoAuthorizationTransactionKey(transaction.ID), string(raw), ttl)
}

func (s *SSOAuthorizationTransactionService) Load(tenant Tenant, transactionID string) (SSOAuthorizationTransaction, error) {
	transactionID = strings.TrimSpace(transactionID)
	if tenant.ID == 0 || strings.TrimSpace(tenant.Code) == "" || transactionID == "" {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	raw := ssoAuthorizationTransactionCache().GetString(ssoAuthorizationTransactionKey(transactionID))
	transaction, err := parseSSOAuthorizationTransaction(raw)
	if err != nil {
		return SSOAuthorizationTransaction{}, err
	}
	if transaction.TenantCode != tenant.Code {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	return transaction, nil
}

func (s *SSOAuthorizationTransactionService) ValidateCallback(tenant Tenant, transactionID, state string) (SSOAuthorizationTransaction, error) {
	transaction, err := s.Load(tenant, transactionID)
	if err != nil {
		return SSOAuthorizationTransaction{}, err
	}
	if !secureSSOAuthorizationEqual(transaction.State, strings.TrimSpace(state)) {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	return transaction, nil
}

func (s *SSOAuthorizationTransactionService) LoadVerified(tenant Tenant, transaction SSOAuthorizationTransaction) (ssoVerifiedAuthorization, bool) {
	raw := ssoAuthorizationTransactionCache().GetString(ssoAuthorizationVerifiedKey(transaction.ID))
	verified := ssoVerifiedAuthorization{}
	if json.Unmarshal([]byte(raw), &verified) != nil || verified.TenantCode != tenant.Code ||
		verified.Provider != transaction.Provider || verified.Scene != transaction.Scene ||
		verified.ProviderID == 0 || !ssoAuthorizationTransactionNow().Before(verified.ExpiresAt) {
		return ssoVerifiedAuthorization{}, false
	}
	return verified, true
}

func (s *SSOAuthorizationTransactionService) StoreVerified(transaction SSOAuthorizationTransaction, verified ssoVerifiedAuthorization) error {
	verified.ExpiresAt = transaction.ExpiresAt
	raw, err := json.Marshal(verified)
	if err != nil {
		return err
	}
	ttl := time.Until(transaction.ExpiresAt)
	if ttl <= 0 {
		return ErrSSOAuthorizationTransactionExpired
	}
	return ssoAuthorizationTransactionCache().Put(ssoAuthorizationVerifiedKey(transaction.ID), string(raw), ttl)
}

func (s *SSOAuthorizationTransactionService) ForgetVerified(transactionID string) {
	_ = ssoAuthorizationTransactionCache().Forget(ssoAuthorizationVerifiedKey(transactionID))
}

func (s *SSOAuthorizationTransactionService) VerifyAndConsumeCallback(
	tenant Tenant,
	transactionID, state string,
	verify func(SSOAuthorizationTransaction) error,
) (SSOAuthorizationTransaction, error) {
	transactionID = strings.TrimSpace(transactionID)
	if tenant.ID == 0 || strings.TrimSpace(tenant.Code) == "" || transactionID == "" || verify == nil {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	cache := ssoAuthorizationTransactionCache()
	lock := cache.Lock(ssoAuthorizationTransactionLockKey(transactionID), ssoAuthorizationTransactionLockTTL)
	if !lock.Get() {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionReused
	}
	defer lock.Release()
	transaction, err := s.Load(tenant, transactionID)
	if err != nil {
		if errors.Is(err, ErrSSOAuthorizationTransactionInvalid) {
			if cache.Has(ssoAuthorizationTransactionUsedKey(transactionID)) {
				return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionReused
			}
		}
		return SSOAuthorizationTransaction{}, err
	}
	if !secureSSOAuthorizationEqual(transaction.State, strings.TrimSpace(state)) {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	if err := verify(transaction); err != nil {
		return SSOAuthorizationTransaction{}, err
	}
	if !cache.Forget(ssoAuthorizationTransactionKey(transaction.ID)) {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionReused
	}
	usedTTL := time.Until(transaction.ExpiresAt)
	if usedTTL > 0 {
		_ = cache.Put(ssoAuthorizationTransactionUsedKey(transaction.ID), "1", usedTTL)
	}
	return transaction, nil
}

func (s *SSOAuthorizationTransactionService) authorizationURL(provider SSOProvider, transaction SSOAuthorizationTransaction) (string, error) {
	endpoint, err := ssoEndpointURL(provider.AuthorizationEndpoint)
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("response_type", "code")
	query.Set("client_id", provider.ClientID)
	query.Set("redirect_uri", transaction.RedirectURI)
	query.Set("state", transaction.State)
	if scope := strings.TrimSpace(provider.Scope); scope != "" {
		query.Set("scope", scope)
	}
	if provider.EnableNonce || provider.Type == "oidc" {
		query.Set("nonce", transaction.Nonce)
	}
	if provider.EnablePKCE || provider.Type == "oidc" {
		query.Set("code_challenge", ssoPKCEChallenge(transaction.CodeVerifier))
		query.Set("code_challenge_method", "S256")
	}
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func parseSSOAuthorizationTransaction(raw string) (SSOAuthorizationTransaction, error) {
	transaction := SSOAuthorizationTransaction{}
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &transaction) != nil {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	transaction = normalizeSSOAuthorizationTransaction(transaction)
	if !validSSOAuthorizationTransaction(transaction) {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	if !ssoAuthorizationTransactionNow().Before(transaction.ExpiresAt) {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionExpired
	}
	return transaction, nil
}

func normalizeSSOAuthorizationTransaction(transaction SSOAuthorizationTransaction) SSOAuthorizationTransaction {
	transaction.ID = strings.TrimSpace(transaction.ID)
	transaction.TenantCode = strings.TrimSpace(transaction.TenantCode)
	transaction.Provider = strings.TrimSpace(transaction.Provider)
	transaction.Scene = normalizeSSOScene(transaction.Scene)
	transaction.State = strings.TrimSpace(transaction.State)
	transaction.Nonce = strings.TrimSpace(transaction.Nonce)
	transaction.CodeVerifier = strings.TrimSpace(transaction.CodeVerifier)
	transaction.RedirectURI = strings.TrimSpace(transaction.RedirectURI)
	return transaction
}

func validSSOAuthorizationTransaction(transaction SSOAuthorizationTransaction) bool {
	return transaction.ID != "" && transaction.TenantCode != "" && transaction.Provider != "" &&
		transaction.State != "" && transaction.RedirectURI != "" && !transaction.ExpiresAt.IsZero()
}

func newSSOAuthorizationRandomValue(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func ssoPKCEChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func secureSSOAuthorizationEqual(expected, actual string) bool {
	if expected == "" || actual == "" || len(expected) != len(actual) {
		return false
	}
	var different byte
	for index := range expected {
		different |= expected[index] ^ actual[index]
	}
	return different == 0
}

func ssoAuthorizationTransactionKey(id string) string {
	return ssoAuthorizationTransactionPrefix + id
}

func ssoAuthorizationVerifiedKey(id string) string {
	return ssoAuthorizationVerifiedPrefix + id
}

func ssoAuthorizationTransactionLockKey(id string) string {
	return ssoAuthorizationTransactionLockPrefix + id
}

func ssoAuthorizationTransactionUsedKey(id string) string {
	return ssoAuthorizationTransactionUsedPrefix + id
}

// Source: sso_mapping_service.go
type ssoMappingResult struct {
	RoleCodes  []string
	DataPolicy *DataPolicy
}

func applySSOMappings(ctx context.Context, connection string, userID uint64, provider SSOProvider, claims ssoClaims) error {
	result := resolveSSOMappings(provider, claims)
	return OrmForConnectionWithContext(ctx, connection).Transaction(func(tx contractsorm.Query) error {
		if result.RoleCodes != nil {
			if err := syncMappedUserRoles(tx, userID, result.RoleCodes); err != nil {
				return err
			}
		}
		if result.DataPolicy != nil {
			if err := syncMappedUserDataPolicy(tx, userID, *result.DataPolicy); err != nil {
				return err
			}
		}
		return nil
	})
}

func resolveSSOMappings(provider SSOProvider, claims ssoClaims) ssoMappingResult {
	return ssoMappingResult{
		RoleCodes:  resolveRoleMapping(provider.RoleMapping, claims.Raw),
		DataPolicy: resolveDataPermissionMapping(provider.DataPermissionMapping, claims.Raw),
	}
}

func resolveRoleMapping(mapping models.JSONMap, claims map[string]any) []string {
	if len(mapping) == 0 {
		return nil
	}
	roleCodes := make([]string, 0)
	claimValues := claimStrings(claims, jsonString(mapping, "claim"))
	rules, _ := asJSONMap(mapping["mapping"])
	for key, raw := range rules {
		if !claimValueMatches(claimValues, key) {
			continue
		}
		roleCodes = append(roleCodes, rolesFromMappingValue(raw, claims)...)
	}
	if len(roleCodes) == 0 {
		roleCodes = roleCodesFromAny(mapping["default"])
	}
	return uniqueStrings(roleCodes)
}

func rolesFromMappingValue(value any, claims map[string]any) []string {
	if rule, ok := asJSONMap(value); ok {
		if condition := strings.TrimSpace(jsonString(rule, "condition")); condition != "" && !evalSSOCondition(condition, claims) {
			return nil
		}
		return roleCodesFromAny(rule["roles"])
	}
	return roleCodesFromAny(value)
}

func resolveDataPermissionMapping(mapping models.JSONMap, claims map[string]any) *DataPolicy {
	if len(mapping) == 0 {
		return nil
	}
	claimValues := claimStrings(claims, jsonString(mapping, "claim"))
	rules, _ := asJSONMap(mapping["mapping"])
	for key, raw := range rules {
		if !claimValueMatches(claimValues, key) {
			continue
		}
		if policy, ok := dataPolicyFromMappingValue(raw, claims); ok {
			return &policy
		}
	}
	if policyType := PolicyType(strings.TrimSpace(jsonString(mapping, "default"))); policyType != "" {
		return &DataPolicy{Type: policyType}
	}
	return nil
}

func dataPolicyFromMappingValue(value any, claims map[string]any) (DataPolicy, bool) {
	if policyType, ok := value.(string); ok {
		policyType = strings.TrimSpace(policyType)
		return DataPolicy{Type: PolicyType(policyType)}, policyType != ""
	}
	rule, ok := asJSONMap(value)
	if !ok {
		return DataPolicy{}, false
	}
	if condition := strings.TrimSpace(jsonString(rule, "condition")); condition != "" && !evalSSOCondition(condition, claims) {
		return DataPolicy{}, false
	}
	policyType := PolicyType(strings.TrimSpace(jsonString(rule, "policy_type")))
	if policyType == "" {
		return DataPolicy{}, false
	}
	return DataPolicy{Type: policyType, DeptIDs: uint64SliceFromAny(rule["value"])}, true
}

func syncMappedUserRoles(tx contractsorm.Query, userID uint64, roleCodes []string) error {
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
		if err := attachMappedUserRole(tx, userID, subject, code); err != nil {
			return err
		}
	}
	return nil
}

func attachMappedUserRole(tx contractsorm.Query, userID uint64, subject, code string) error {
	var role models.Role
	if err := tx.Table("role").Where("code", code).First(&role); err != nil {
		return err
	}
	now := time.Now()
	if err := tx.Table("user_belongs_role").Create(&models.UserBelongsRole{
		UserID: userID, RoleID: role.ID,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		return err
	}
	return addCasbinRule(tx, "g", subject, "role:"+role.Code, "")
}

func syncMappedUserDataPolicy(tx contractsorm.Query, userID uint64, policy DataPolicy) error {
	if policy.Type == PolicyCustomFunc {
		return BusinessError{Message: "自定义数据权限函数未注册"}
	}
	deptIDs := policy.DeptIDs
	if deptIDs == nil {
		deptIDs = []uint64{}
	}
	encoded, err := json.Marshal(deptIDs)
	if err != nil {
		return err
	}
	_, err = tx.Table("data_permission_policy").Where("user_id", userID).Delete()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO data_permission_policy (user_id, policy_type, is_default, value, created_at, updated_at)
		VALUES (?, ?, true, ?::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, userID, string(policy.Type), string(encoded))
	return err
}

func claimStrings(claims map[string]any, key string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	return stringsFromAny(claims[key])
}

func claimValueMatches(values []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func roleCodesFromAny(value any) []string {
	return uniqueStrings(stringsFromAny(value))
}

func stringsFromAny(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if typed = strings.TrimSpace(typed); typed != "" {
			return []string{typed}
		}
	case []string:
		return uniqueStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, stringsFromAny(item)...)
		}
		return uniqueStrings(out)
	default:
		text := strings.TrimSpace(fmt.Sprint(typed))
		if text != "" {
			return []string{text}
		}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func uint64SliceFromAny(value any) []uint64 {
	switch typed := value.(type) {
	case []any:
		out := make([]uint64, 0, len(typed))
		for _, item := range typed {
			if id := uint64FromAny(item); id > 0 {
				out = append(out, id)
			}
		}
		return compactPositiveIDs(out)
	case []uint64:
		return compactPositiveIDs(typed)
	case []int:
		out := make([]uint64, 0, len(typed))
		for _, item := range typed {
			if item > 0 {
				out = append(out, uint64(item))
			}
		}
		return out
	default:
		if id := uint64FromAny(value); id > 0 {
			return []uint64{id}
		}
	}
	return nil
}

func uint64FromAny(value any) uint64 {
	switch typed := value.(type) {
	case float64:
		return uint64(typed)
	case int:
		return uint64(typed)
	case int64:
		return uint64(typed)
	case uint64:
		return typed
	case string:
		parsed, _ := strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
		return parsed
	}
	return 0
}

// Source: sso_protocol_service.go
const ssoHTTPTimeout = 10 * time.Second

var ssoAllowLoopbackEndpoints bool

func AllowLoopbackSSOEndpointsForTesting() func() {
	previous := ssoAllowLoopbackEndpoints
	ssoAllowLoopbackEndpoints = true
	return func() {
		ssoAllowLoopbackEndpoints = previous
	}
}

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string   `json:"kty"`
	Use string   `json:"use"`
	Kid string   `json:"kid"`
	Alg string   `json:"alg"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5C []string `json:"x5c"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
}

type oidcDiscoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

func verifySSOClaims(provider SSOProvider, payload SSOLoginPayload) (ssoClaims, error) {
	provider = withSSODiscoveryDefaults(provider)
	switch provider.Type {
	case "oidc":
		return verifyOIDCClaims(provider, payload)
	case "oauth2":
		return verifyOAuth2Claims(provider, payload)
	case "saml":
		return verifySAMLClaims(provider, payload)
	default:
		return ssoClaims{}, ErrSSOTokenInvalid
	}
}

func withSSODiscoveryDefaults(provider SSOProvider) SSOProvider {
	if strings.TrimSpace(provider.DiscoveryURL) == "" {
		return provider
	}
	body, err := fetchURL(provider.DiscoveryURL)
	if err != nil {
		return provider
	}
	var doc oidcDiscoveryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return provider
	}
	if provider.Issuer == "" {
		provider.Issuer = doc.Issuer
	}
	if provider.AuthorizationEndpoint == "" {
		provider.AuthorizationEndpoint = doc.AuthorizationEndpoint
	}
	if provider.TokenEndpoint == "" {
		provider.TokenEndpoint = doc.TokenEndpoint
	}
	if provider.UserinfoEndpoint == "" {
		provider.UserinfoEndpoint = doc.UserinfoEndpoint
	}
	if provider.JWKSURI == "" {
		provider.JWKSURI = doc.JWKSURI
	}
	return provider
}

func verifyOIDCClaims(provider SSOProvider, payload SSOLoginPayload) (ssoClaims, error) {
	tokenText := strings.TrimSpace(payload.IDToken)
	if tokenText == "" && strings.TrimSpace(payload.Code) != "" {
		exchanged, err := exchangeOAuthCode(provider, payload)
		if err != nil {
			return ssoClaims{}, err
		}
		tokenText = exchanged.IDToken
	}
	if tokenText == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	return verifyIDToken(provider, payload, tokenText)
}

func verifyOAuth2Claims(provider SSOProvider, payload SSOLoginPayload) (ssoClaims, error) {
	tokenText := strings.TrimSpace(payload.IDToken)
	accessToken := ""
	if tokenText == "" && strings.TrimSpace(payload.Code) != "" {
		exchanged, err := exchangeOAuthCode(provider, payload)
		if err != nil {
			return ssoClaims{}, err
		}
		tokenText = exchanged.IDToken
		accessToken = exchanged.AccessToken
	}
	if tokenText != "" {
		return verifyIDToken(provider, payload, tokenText)
	}
	if accessToken != "" {
		return fetchUserInfoClaims(provider, accessToken)
	}
	return ssoClaims{}, ErrSSOTokenInvalid
}

func fetchUserInfoClaims(provider SSOProvider, accessToken string) (ssoClaims, error) {
	if provider.UserinfoEndpoint == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	endpoint, err := ssoEndpointURL(provider.UserinfoEndpoint)
	if err != nil {
		return ssoClaims{}, err
	}
	client := ssoHTTPClient()
	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := client.Do(req)
	if err != nil {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	var claims map[string]any
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&claims); err != nil {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	subject := strings.TrimSpace(jsonString(claims, "sub"))
	if subject == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	return ssoClaims{
		Subject: subject,
		Email:   jsonString(claims, "email"),
		Name:    jsonString(claims, "name"),
		Issuer:  provider.Issuer,
		Raw:     claims,
	}, nil
}

func verifyIDToken(provider SSOProvider, payload SSOLoginPayload, tokenText string) (ssoClaims, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) {
		if method, ok := token.Method.(*jwt.SigningMethodHMAC); ok {
			if method != jwt.SigningMethodHS256 || provider.JWTSecret == "" {
				return nil, ErrSSOTokenInvalid
			}
			return []byte(provider.JWTSecret), nil
		}
		if _, ok := token.Method.(*jwt.SigningMethodRSA); ok {
			key, err := publicKeyFromJWKS(provider, fmt.Sprint(token.Header["kid"]))
			if err != nil {
				return nil, err
			}
			return key, nil
		}
		return nil, ErrSSOTokenInvalid
	}, jwt.WithValidMethods([]string{"HS256", "RS256", "RS384", "RS512"}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if provider.Issuer != "" && claimsString(claims, "iss") != provider.Issuer {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if provider.Audience != "" && !audienceMatches(claims["aud"], provider.Audience) {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if provider.EnableNonce {
		nonce := strings.TrimSpace(payload.Nonce)
		if nonce == "" || claimsString(claims, "nonce") != nonce {
			return ssoClaims{}, ErrSSOTokenInvalid
		}
	}
	subject := strings.TrimSpace(claimsString(claims, "sub"))
	if subject == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	return ssoClaims{
		Subject: subject,
		Email:   claimsString(claims, "email"),
		Name:    claimsString(claims, "name"),
		Issuer:  claimsString(claims, "iss"),
		Raw:     mapClaims(claims),
	}, nil
}

func exchangeOAuthCode(provider SSOProvider, payload SSOLoginPayload) (tokenResponse, error) {
	if provider.TokenEndpoint == "" || provider.ClientID == "" {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	if strings.TrimSpace(payload.Code) == "" {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	endpoint, err := ssoEndpointURL(provider.TokenEndpoint)
	if err != nil {
		return tokenResponse{}, err
	}
	redirectURI := strings.TrimSpace(provider.RedirectURI)
	if redirectURI == "" {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	if requestedRedirect := strings.TrimSpace(payload.RedirectURI); requestedRedirect != "" && requestedRedirect != redirectURI {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	codeVerifier := strings.TrimSpace(payload.CodeVerifier)
	if (provider.EnablePKCE || provider.Type == "oidc") && codeVerifier == "" {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(payload.Code))
	form.Set("client_id", provider.ClientID)
	if provider.ClientSecret != "" {
		form.Set("client_secret", provider.ClientSecret)
	}
	form.Set("redirect_uri", redirectURI)
	if provider.EnablePKCE || provider.Type == "oidc" {
		form.Set("code_verifier", codeVerifier)
	}

	client := ssoHTTPClient()
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	var payloadRes tokenResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&payloadRes); err != nil {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	if payloadRes.IDToken == "" && payloadRes.AccessToken == "" {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	return payloadRes, nil
}

func publicKeyFromJWKS(provider SSOProvider, kid string) (*rsa.PublicKey, error) {
	rawJWKS := strings.TrimSpace(provider.JWKSJSON)
	if rawJWKS == "" {
		if provider.JWKSURI == "" {
			return nil, ErrSSOTokenInvalid
		}
		body, err := fetchURL(provider.JWKSURI)
		if err != nil {
			return nil, err
		}
		rawJWKS = string(body)
	}
	var doc jwksDocument
	if err := json.Unmarshal([]byte(rawJWKS), &doc); err != nil {
		return nil, ErrSSOTokenInvalid
	}
	for _, key := range doc.Keys {
		if key.Kty != "RSA" || (kid != "" && key.Kid != kid) {
			continue
		}
		if len(key.X5C) > 0 {
			certDER, err := base64.StdEncoding.DecodeString(key.X5C[0])
			if err != nil {
				return nil, ErrSSOTokenInvalid
			}
			cert, err := x509.ParseCertificate(certDER)
			if err != nil {
				return nil, ErrSSOTokenInvalid
			}
			if rsaKey, ok := cert.PublicKey.(*rsa.PublicKey); ok {
				return rsaKey, nil
			}
		}
		rsaKey, err := jwkRSAPublicKey(key)
		if err == nil {
			return rsaKey, nil
		}
	}
	return nil, ErrSSOTokenInvalid
}

func jwkRSAPublicKey(key jwkKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, err
	}
	e := new(big.Int).SetBytes(eBytes).Int64()
	if e <= 0 {
		return nil, ErrSSOTokenInvalid
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(e)}, nil
}

func fetchURL(uri string) ([]byte, error) {
	endpoint, err := ssoEndpointURL(uri)
	if err != nil {
		return nil, err
	}
	client := ssoHTTPClient()
	res, err := client.Get(endpoint.String())
	if err != nil {
		return nil, ErrSSOTokenInvalid
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, ErrSSOTokenInvalid
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, ErrSSOTokenInvalid
	}
	return body, nil
}

func ssoEndpointURL(uri string) (*url.URL, error) {
	return safeOutboundURL(uri, ssoOutboundHTTPPolicy())
}

func allowHTTPSSOEndpoint(ips []net.IP) bool {
	if !allowLoopbackSSOEndpoints() || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return false
		}
	}
	return true
}

func allowLoopbackSSOEndpoints() bool {
	return ssoAllowLoopbackEndpoints ||
		(os.Getenv("APP_ENV") == "testing" && os.Getenv("SSO_TEST_ALLOW_LOOPBACK") == "true")
}

func ssoHTTPClient() http.Client {
	return safeOutboundHTTPClient(ssoHTTPTimeout, ssoOutboundHTTPPolicy())
}

func ssoOutboundHTTPPolicy() outboundHTTPPolicy {
	return outboundHTTPPolicy{
		invalidURL:     func() error { return ErrSSOTokenInvalid },
		unresolvedHost: func() error { return ErrSSOTokenInvalid },
		invalidAddress: func() error { return ErrSSOTokenInvalid },
		validateURL: func(endpoint *url.URL) error {
			if endpoint.Scheme == "" || endpoint.Host == "" ||
				(endpoint.Scheme != "https" && endpoint.Scheme != "http") ||
				endpoint.User != nil || strings.TrimSpace(endpoint.Hostname()) == "" {
				return ErrSSOTokenInvalid
			}
			return nil
		},
		validateTarget: func(endpoint *url.URL, ips []net.IP) error {
			if len(ips) == 0 {
				return ErrSSOTokenInvalid
			}
			if endpoint.Scheme == "http" && !allowHTTPSSOEndpoint(ips) {
				return ErrSSOTokenInvalid
			}
			for _, ip := range ips {
				if isPrivateSSOEndpointIP(ip) {
					return ErrSSOTokenInvalid
				}
			}
			return nil
		},
	}
}

func isPrivateSSOEndpointIP(ip net.IP) bool {
	if allowLoopbackSSOEndpoints() && ip.IsLoopback() {
		return false
	}
	return isPrivateOutboundIP(ip)
}

func verifySAMLClaims(provider SSOProvider, payload SSOLoginPayload) (ssoClaims, error) {
	raw := strings.TrimSpace(payload.SAMLResponse)
	if raw == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && strings.Contains(string(decoded), "<") {
		raw = string(decoded)
	}
	certs, err := parseSAMLCertificates(provider.SAMLCertificate)
	if err != nil || len(certs) == 0 {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromString(raw); err != nil {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	validator := dsig.NewDefaultValidationContext(&dsig.MemoryX509CertificateStore{Roots: certs})
	validator.IdAttribute = "ID"
	validated, err := validator.Validate(doc.Root())
	if err != nil {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if provider.Issuer != "" && samlFirstText(validated, "Issuer") != provider.Issuer {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if provider.SAMLEntityID != "" && !samlAudienceMatches(validated, provider.SAMLEntityID) {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if !samlConditionsValid(validated, time.Now()) {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	subject := strings.TrimSpace(samlFirstText(validated, "NameID"))
	if subject == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	rawClaims := samlAttributes(validated)
	rawClaims["sub"] = subject
	rawClaims["subject"] = subject
	rawClaims["email"] = samlAttribute(validated, "email")
	rawClaims["name"] = samlAttribute(validated, "name")
	rawClaims["iss"] = samlFirstText(validated, "Issuer")
	return ssoClaims{
		Subject: subject,
		Email:   jsonString(rawClaims, "email"),
		Name:    jsonString(rawClaims, "name"),
		Issuer:  jsonString(rawClaims, "iss"),
		Raw:     rawClaims,
	}, nil
}

func mapClaims(claims jwt.MapClaims) map[string]any {
	out := make(map[string]any, len(claims))
	for key, value := range claims {
		out[key] = value
	}
	return out
}

func parseSAMLCertificates(value string) ([]*x509.Certificate, error) {
	certs := make([]*x509.Certificate, 0)
	rest := []byte(strings.TrimSpace(value))
	for len(rest) > 0 {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	if len(certs) > 0 {
		return certs, nil
	}
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return []*x509.Certificate{cert}, nil
}

func claimsString(claims jwt.MapClaims, key string) string {
	value, ok := claims[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func samlFirstText(root *etree.Element, tag string) string {
	for _, el := range root.FindElements(".//*") {
		if localName(el.Tag) == tag {
			return strings.TrimSpace(el.Text())
		}
	}
	return ""
}

func samlAttribute(root *etree.Element, name string) string {
	for _, attr := range root.FindElements(".//*") {
		if localName(attr.Tag) != "Attribute" || attr.SelectAttrValue("Name", "") != name {
			continue
		}
		for _, child := range attr.ChildElements() {
			if localName(child.Tag) == "AttributeValue" {
				return strings.TrimSpace(child.Text())
			}
		}
	}
	return ""
}

func samlAttributes(root *etree.Element) map[string]any {
	out := map[string]any{}
	for _, attr := range root.FindElements(".//*") {
		if localName(attr.Tag) != "Attribute" {
			continue
		}
		name := strings.TrimSpace(attr.SelectAttrValue("Name", ""))
		if name == "" {
			continue
		}
		values := make([]string, 0)
		for _, child := range attr.ChildElements() {
			if localName(child.Tag) == "AttributeValue" {
				if value := strings.TrimSpace(child.Text()); value != "" {
					values = append(values, value)
				}
			}
		}
		if len(values) == 1 {
			out[name] = values[0]
		} else if len(values) > 1 {
			out[name] = values
		}
	}
	return out
}

func samlAudienceMatches(root *etree.Element, expected string) bool {
	for _, el := range root.FindElements(".//*") {
		if localName(el.Tag) == "Audience" && strings.TrimSpace(el.Text()) == expected {
			return true
		}
	}
	return false
}

func samlConditionsValid(root *etree.Element, now time.Time) bool {
	for _, el := range root.FindElements(".//*") {
		if localName(el.Tag) != "Conditions" {
			continue
		}
		if notBefore := el.SelectAttrValue("NotBefore", ""); notBefore != "" {
			parsed, err := time.Parse(time.RFC3339, notBefore)
			if err != nil || now.Before(parsed.Add(-time.Minute)) {
				return false
			}
		}
		if notOnOrAfter := el.SelectAttrValue("NotOnOrAfter", ""); notOnOrAfter != "" {
			parsed, err := time.Parse(time.RFC3339, notOnOrAfter)
			if err != nil || !now.Before(parsed.Add(time.Minute)) {
				return false
			}
		}
	}
	return true
}

func localName(tag string) string {
	if idx := strings.LastIndex(tag, ":"); idx >= 0 {
		return tag[idx+1:]
	}
	return tag
}

// Source: sso_provider_service.go
const DefaultSSOScene = "admin"

type SSOProvider = models.SSOProvider

type SSOProviderPayload struct {
	Name                  string         `json:"name"`
	DisplayName           string         `json:"display_name"`
	Scene                 string         `json:"scene"`
	Type                  string         `json:"type"`
	Enabled               *bool          `json:"enabled"`
	Issuer                string         `json:"issuer"`
	Audience              string         `json:"audience"`
	JWTSecret             string         `json:"jwt_secret"`
	DiscoveryURL          string         `json:"discovery_url"`
	AuthorizationEndpoint string         `json:"authorization_endpoint"`
	TokenEndpoint         string         `json:"token_endpoint"`
	UserinfoEndpoint      string         `json:"userinfo_endpoint"`
	JWKSURI               string         `json:"jwks_uri"`
	JWKSJSON              string         `json:"jwks_json"`
	ClientID              string         `json:"client_id"`
	ClientSecret          string         `json:"client_secret"`
	Scope                 string         `json:"scope"`
	RedirectURI           string         `json:"redirect_uri"`
	EnablePKCE            *bool          `json:"enable_pkce"`
	EnableNonce           *bool          `json:"enable_nonce"`
	AutoCreate            *bool          `json:"auto_create"`
	Icon                  string         `json:"icon"`
	ButtonColor           string         `json:"button_color"`
	DisplayOrder          int            `json:"display_order"`
	SAMLEntrypoint        string         `json:"saml_entrypoint"`
	SAMLEntityID          string         `json:"saml_entity_id"`
	SAMLCertificate       string         `json:"saml_certificate"`
	RoleMapping           models.JSONMap `json:"role_mapping"`
	DataPermissionMapping models.JSONMap `json:"data_permission_mapping"`
	Remark                string         `json:"remark"`
}

type PublicSSOProvider struct {
	Name                  string `json:"name"`
	DisplayName           string `json:"display_name"`
	Scene                 string `json:"scene"`
	Type                  string `json:"type"`
	Issuer                string `json:"issuer"`
	DiscoveryURL          string `json:"discovery_url"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	ClientID              string `json:"client_id"`
	Scope                 string `json:"scope"`
	RedirectURI           string `json:"redirect_uri"`
	EnablePKCE            bool   `json:"enable_pkce"`
	EnableNonce           bool   `json:"enable_nonce"`
	SAMLEntrypoint        string `json:"saml_entrypoint"`
	SAMLEntityID          string `json:"saml_entity_id"`
	Icon                  string `json:"icon"`
	ButtonColor           string `json:"button_color"`
	Enabled               bool   `json:"enabled"`
}

type SSOProviderService struct {
	ctx        context.Context
	connection string
}

func NewSSOProviderServiceForTenant(tenant Tenant) *SSOProviderService {
	return &SSOProviderService{connection: TenantConnectionName(tenant)}
}

func (s *SSOProviderService) WithContext(ctx context.Context) *SSOProviderService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *SSOProviderService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func (s *SSOProviderService) List(filters map[string]string, page, pageSize int) (request.PageResult[SSOProvider], error) {
	query := s.orm().Query().Table("sso_provider")
	query = query.Scopes(scopes.Contains("name", filters["name"]))
	query = query.Scopes(scopes.Contains("display_name", filters["display_name"]))
	query = query.Scopes(scopes.Contains("scene", filters["scene"]))
	query = query.Scopes(scopes.EqualIfPresent("type", filters["type"]))
	query = query.Scopes(scopes.EqualIfPresent("enabled", filters["enabled"]))
	return request.Paginate[SSOProvider](query.OrderBy("display_order").OrderByDesc("id"), page, pageSize)
}

func (s *SSOProviderService) PublicProviders(scene string) ([]PublicSSOProvider, error) {
	scene = normalizeSSOScene(scene)
	providers := make([]SSOProvider, 0)
	err := s.orm().
		Query().
		Table("sso_provider").
		Where("scene", scene).
		Where("enabled", true).
		OrderBy("display_order").
		OrderBy("id").
		Get(&providers)
	if err != nil {
		return nil, err
	}
	out := make([]PublicSSOProvider, 0, len(providers))
	for _, provider := range providers {
		provider = withSSODiscoveryDefaults(provider)
		out = append(out, PublicSSOProvider{
			Name:                  provider.Name,
			DisplayName:           provider.DisplayName,
			Scene:                 provider.Scene,
			Type:                  provider.Type,
			Issuer:                provider.Issuer,
			DiscoveryURL:          provider.DiscoveryURL,
			AuthorizationEndpoint: provider.AuthorizationEndpoint,
			ClientID:              provider.ClientID,
			Scope:                 provider.Scope,
			RedirectURI:           provider.RedirectURI,
			EnablePKCE:            provider.EnablePKCE,
			EnableNonce:           provider.EnableNonce,
			SAMLEntrypoint:        provider.SAMLEntrypoint,
			SAMLEntityID:          provider.SAMLEntityID,
			Icon:                  provider.Icon,
			ButtonColor:           provider.ButtonColor,
			Enabled:               provider.Enabled,
		})
	}
	return out, nil
}

func (s *SSOProviderService) Create(input SSOProviderPayload, operatorID uint64) (SSOProvider, error) {
	provider := input.Provider()
	provider.AuditColumns = models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID}
	provider = withSSORotationMetadata(provider, time.Now())
	if err := validateSSOProvider(provider); err != nil {
		return SSOProvider{}, err
	}
	if err := s.ensureUniqueNameScene(0, provider.Name, provider.Scene); err != nil {
		return SSOProvider{}, err
	}
	roleMapping := provider.RoleMapping
	dataPermissionMapping := provider.DataPermissionMapping
	provider.RoleMapping = nil
	provider.DataPermissionMapping = nil
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		if err := tx.Table("sso_provider").Create(&provider); err != nil {
			return err
		}
		provider.RoleMapping = roleMapping
		provider.DataPermissionMapping = dataPermissionMapping
		return s.normalizeJSONColumnsWithQuery(tx, provider.ID, provider)
	}); err != nil {
		return SSOProvider{}, err
	}
	provider.RoleMapping = roleMapping
	provider.DataPermissionMapping = dataPermissionMapping
	return provider, nil
}

func (s *SSOProviderService) Update(id uint64, input SSOProviderPayload, operatorID uint64) (SSOProvider, error) {
	provider := input.Provider()
	provider.ID = id
	if err := validateSSOProvider(provider); err != nil {
		return SSOProvider{}, err
	}
	if err := s.ensureUniqueNameScene(id, provider.Name, provider.Scene); err != nil {
		return SSOProvider{}, err
	}
	values := map[string]any{
		"name":                   provider.Name,
		"display_name":           provider.DisplayName,
		"scene":                  provider.Scene,
		"type":                   provider.Type,
		"enabled":                provider.Enabled,
		"issuer":                 provider.Issuer,
		"audience":               provider.Audience,
		"discovery_url":          provider.DiscoveryURL,
		"authorization_endpoint": provider.AuthorizationEndpoint,
		"token_endpoint":         provider.TokenEndpoint,
		"userinfo_endpoint":      provider.UserinfoEndpoint,
		"jwks_uri":               provider.JWKSURI,
		"jwks_json":              provider.JWKSJSON,
		"client_id":              provider.ClientID,
		"scope":                  provider.Scope,
		"redirect_uri":           provider.RedirectURI,
		"enable_pkce":            provider.EnablePKCE,
		"enable_nonce":           provider.EnableNonce,
		"auto_create":            provider.AutoCreate,
		"icon":                   provider.Icon,
		"button_color":           provider.ButtonColor,
		"display_order":          provider.DisplayOrder,
		"saml_entrypoint":        provider.SAMLEntrypoint,
		"saml_entity_id":         provider.SAMLEntityID,
		"saml_certificate":       provider.SAMLCertificate,
		"updated_by":             operatorID,
		"updated_at":             time.Now(),
		"remark":                 provider.Remark,
	}
	now := time.Now()
	if provider.JWTSecret != "" {
		values["jwt_secret"] = provider.JWTSecret
		values["jwt_secret_rotated_at"] = now
	}
	if provider.ClientSecret != "" {
		values["client_secret"] = provider.ClientSecret
		values["client_secret_rotated_at"] = now
	}
	result, err := s.orm().Query().Table("sso_provider").Where("id", id).Update(values)
	if err != nil {
		return SSOProvider{}, err
	}
	if result.RowsAffected == 0 {
		return SSOProvider{}, BusinessError{Message: "SSO Provider 不存在"}
	}
	if err := s.normalizeJSONColumns(id, provider); err != nil {
		return SSOProvider{}, err
	}
	provider.ID = id
	return provider, nil
}

func withSSORotationMetadata(provider SSOProvider, now time.Time) SSOProvider {
	if provider.JWTSecret != "" {
		provider.JWTSecretRotatedAt = now
	}
	if provider.ClientSecret != "" {
		provider.ClientSecretRotatedAt = now
	}
	return provider
}

func (s *SSOProviderService) Delete(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.orm().Query().Table("sso_provider").WhereIn("id", uint64Any(ids)).Delete()
	return err
}

func (s *SSOProviderService) EnabledProvider(name string) (SSOProvider, error) {
	return s.EnabledProviderForScene(name, "")
}

func (s *SSOProviderService) EnabledProviderForScene(name, scene string) (SSOProvider, error) {
	var provider SSOProvider
	query := s.orm().
		Query().
		Table("sso_provider").
		Where("name", strings.TrimSpace(name)).
		Where("enabled", true)
	if strings.TrimSpace(scene) != "" {
		query = query.Where("scene", normalizeSSOScene(scene))
	} else {
		count, err := query.Count()
		if err != nil {
			return SSOProvider{}, err
		}
		if count != 1 {
			return SSOProvider{}, ErrSSONotConfigured
		}
	}
	err := query.First(&provider)
	if err != nil || provider.ID == 0 {
		return SSOProvider{}, ErrSSONotConfigured
	}
	return provider, nil
}

func (p SSOProviderPayload) Provider() SSOProvider {
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	enablePKCE := true
	if p.EnablePKCE != nil {
		enablePKCE = *p.EnablePKCE
	}
	enableNonce := true
	if p.EnableNonce != nil {
		enableNonce = *p.EnableNonce
	}
	autoCreate := false
	if p.AutoCreate != nil {
		autoCreate = *p.AutoCreate
	}
	provider := SSOProvider{
		Name:                  strings.TrimSpace(p.Name),
		DisplayName:           strings.TrimSpace(p.DisplayName),
		Scene:                 normalizeSSOScene(p.Scene),
		Type:                  normalizeSSOType(p.Type),
		Enabled:               enabled,
		Issuer:                strings.TrimSpace(p.Issuer),
		Audience:              strings.TrimSpace(p.Audience),
		JWTSecret:             p.JWTSecret,
		DiscoveryURL:          strings.TrimSpace(p.DiscoveryURL),
		AuthorizationEndpoint: strings.TrimSpace(p.AuthorizationEndpoint),
		TokenEndpoint:         strings.TrimSpace(p.TokenEndpoint),
		UserinfoEndpoint:      strings.TrimSpace(p.UserinfoEndpoint),
		JWKSURI:               strings.TrimSpace(p.JWKSURI),
		JWKSJSON:              strings.TrimSpace(p.JWKSJSON),
		ClientID:              strings.TrimSpace(p.ClientID),
		ClientSecret:          p.ClientSecret,
		Scope:                 strings.TrimSpace(p.Scope),
		RedirectURI:           strings.TrimSpace(p.RedirectURI),
		EnablePKCE:            enablePKCE,
		EnableNonce:           enableNonce,
		AutoCreate:            autoCreate,
		Icon:                  strings.TrimSpace(p.Icon),
		ButtonColor:           strings.TrimSpace(p.ButtonColor),
		DisplayOrder:          p.DisplayOrder,
		SAMLEntrypoint:        strings.TrimSpace(p.SAMLEntrypoint),
		SAMLEntityID:          strings.TrimSpace(p.SAMLEntityID),
		SAMLCertificate:       strings.TrimSpace(p.SAMLCertificate),
		RoleMapping:           p.RoleMapping,
		DataPermissionMapping: p.DataPermissionMapping,
		Remark:                strings.TrimSpace(p.Remark),
	}
	if provider.DisplayName == "" {
		provider.DisplayName = provider.Name
	}
	if provider.Scope == "" && provider.Type == "oidc" {
		provider.Scope = "openid profile email"
	}
	return provider
}

func validateSSOProvider(provider SSOProvider) error {
	if provider.Name == "" || provider.DisplayName == "" {
		return BusinessError{Message: "SSO Provider 名称不能为空"}
	}
	if provider.Type != "oidc" && provider.Type != "oauth2" && provider.Type != "saml" {
		return BusinessError{Message: "SSO Provider 类型无效"}
	}
	return nil
}

func (s *SSOProviderService) ensureUniqueNameScene(id uint64, name, scene string) error {
	query := s.orm().Query().Table("sso_provider").Where("name", name).Where("scene", scene)
	if id > 0 {
		query = query.Where("id <> ?", id)
	}
	count, err := query.Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return BusinessError{Message: "同一场景下 Provider 名称不能重复"}
	}
	return nil
}

func (s *SSOProviderService) normalizeJSONColumns(id uint64, provider SSOProvider) error {
	return s.normalizeJSONColumnsWithQuery(s.orm().Query(), id, provider)
}

func (s *SSOProviderService) normalizeJSONColumnsWithQuery(query contractsorm.Query, id uint64, provider SSOProvider) error {
	roleMapping, err := json.Marshal(nullIfEmpty(provider.RoleMapping))
	if err != nil {
		return err
	}
	dataPermissionMapping, err := json.Marshal(nullIfEmpty(provider.DataPermissionMapping))
	if err != nil {
		return err
	}
	_, err = query.Exec(
		"UPDATE sso_provider SET role_mapping = ?::jsonb, data_permission_mapping = ?::jsonb WHERE id = ?",
		string(roleMapping), string(dataPermissionMapping), id,
	)
	return err
}

func normalizeSSOScene(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DefaultSSOScene
	}
	return value
}

func normalizeSSOType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "oidc"
	}
	return value
}
