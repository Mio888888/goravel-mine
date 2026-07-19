package application

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	contractscache "github.com/goravel/framework/contracts/cache"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"github.com/goravel/framework/support/env"
	authcontract "goravel/app/contracts/auth"
	"goravel/app/facades"
	"goravel/app/http/response"
	"goravel/app/models"
	authservice "goravel/app/services/access/auth"
	"goravel/database/seeders"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Source: auth.go
const (
	tenantTokenSubject = authservice.TenantTokenSubject
)

var (
	ErrAccountLocked              = authcontract.ErrAccountLocked
	ErrLoginRiskBlocked           = authcontract.ErrLoginRiskBlocked
	ErrPasswordChangeTokenInvalid = authservice.ErrPasswordChangeTokenInvalid
)

type CaptchaService = authservice.CaptchaService
type CaptchaResult = authservice.CaptchaResult
type TokenInfo = authservice.TokenInfo
type jwtTokenRequirements = authservice.TokenRequirements
type LoginSignal = authservice.LoginSignal

var loginSecurityCache = func() contractscache.Driver {
	return facades.Cache()
}

var NewCaptchaService = authservice.NewCaptchaService
var CSRFEnabled = authservice.CSRFEnabled
var SecuritySameSite = authservice.SecuritySameSite
var CSRFCookieSecure = authservice.CSRFCookieSecure
var NewCSRFToken = authservice.NewCSRFToken
var CSRFTokenValid = authservice.CSRFTokenValid
var CSRFOriginAllowed = authservice.CSRFOriginAllowed
var JWTSecret = authservice.JWTSecret
var AccessTokenTTLSeconds = authservice.AccessTokenTTLSeconds
var RefreshTokenTTLSeconds = authservice.RefreshTokenTTLSeconds
var ValidatePasswordPolicy = authservice.ValidatePasswordPolicy
var InitialPassword = authservice.InitialPassword
var CheckLoginLockout = authservice.CheckLoginLockout
var CheckLoginRisk = authservice.CheckLoginRisk
var RecordLoginFailure = authservice.RecordLoginFailure
var RecordLoginRiskFailure = authservice.RecordLoginRiskFailure
var RecordLoginSuccess = authservice.RecordLoginSuccess
var RecordLoginRiskSuccess = authservice.RecordLoginRiskSuccess
var LoginRiskUserAgentChanged = authservice.LoginRiskUserAgentChanged

func init() {
	authservice.ConfigureDependencies(
		OrmForConnectionWithContext,
		PlatformConnection,
	)
	authservice.ConfigureSecurityCache(func() contractscache.Driver {
		return loginSecurityCache()
	})
}

func IssuePasswordChangeChallenge(scope string, userID uint64) (string, error) {
	return authservice.IssuePasswordChangeChallenge(scope, userID)
}

func ConsumePasswordChangeChallenge(scope, token string) (uint64, error) {
	return authservice.ConsumePasswordChangeChallenge(scope, token)
}

func PasswordChangeChallengeUserID(scope, token string) (uint64, error) {
	return authservice.PasswordChangeChallengeUserID(scope, token)
}

func ForgetPasswordChangeChallenge(scope, token string) {
	authservice.ForgetPasswordChangeChallenge(scope, token)
}

func blacklistToken(token string, ttl time.Duration) error {
	return authservice.BlacklistToken(token, ttl)
}

func tokenBlacklisted(token string) bool {
	return authservice.TokenBlacklisted(token)
}

func issueApplicationToken(subject string, userID, tenantID uint64, tokenType string, ttlSeconds int) (string, error) {
	return authservice.IssueApplicationToken(subject, userID, tenantID, tokenType, ttlSeconds)
}

func parseApplicationToken(authorization string, requirements jwtTokenRequirements) (TokenInfo, error) {
	return authservice.ParseApplicationToken(authorization, requirements)
}

func tokenBlacklistTTL(ttlSeconds int) time.Duration {
	return authservice.TokenBlacklistTTL(ttlSeconds)
}

func bearerToken(authorization string) string {
	return authservice.BearerToken(authorization)
}

func makeSecretHash(value string) (string, error) {
	return authservice.MakeSecretHash(value)
}

func secretHashMatches(hash, value string) bool {
	return authservice.SecretHashMatches(hash, value)
}

func makePasswordHash(password string) (string, error) {
	return authservice.MakePasswordHash(password)
}

func passwordHashMatches(hash, password string) bool {
	return authservice.PasswordHashMatches(hash, password)
}

func firstLoginSignal(signals []LoginSignal) LoginSignal {
	return authservice.FirstLoginSignal(signals)
}

type PasswordHistoryService = authservice.PasswordHistoryService

func NewPasswordHistoryService(connection, table string) *PasswordHistoryService {
	return authservice.NewPasswordHistoryService(connection, table)
}

func TenantPasswordHistoryService(tenant Tenant) *PasswordHistoryService {
	return authservice.TenantPasswordHistoryService(tenant)
}

func PlatformPasswordHistoryService() *PasswordHistoryService {
	return authservice.PlatformPasswordHistoryService()
}

func minutesDuration(configKey string, fallback int) time.Duration {
	return authservice.MinutesDuration(configKey, fallback)
}

func loginIdentity(scope, username string) string {
	return authservice.LoginIdentity(scope, username)
}

// Source: current_user_cache.go
const currentUserInfoCacheTTL = 2 * time.Minute

func currentUserInfoCacheKey(connection string, userID uint64) string {
	return fmt.Sprintf(
		"current_user_info:%d:%s:%d:%d",
		currentUserInfoCacheVersion(),
		connection,
		currentUserInfoVersionForUser(connection, userID),
		userID,
	)
}

func currentUserInfoCacheVersion() int64 {
	return facades.Cache().GetInt64("current_user_info_version", 0)
}

func currentUserInfoVersionKey(connection string, userID uint64) string {
	return fmt.Sprintf("current_user_info_user_version:%s:%d", connection, userID)
}

func currentUserInfoVersionForUser(connection string, userID uint64) int64 {
	return facades.Cache().GetInt64(currentUserInfoVersionKey(connection, userID), 0)
}

func cachedCurrentUserInfo(connection string, userID uint64) (UserInfo, bool) {
	raw := facades.Cache().GetString(currentUserInfoCacheKey(connection, userID))
	if raw == "" {
		return UserInfo{}, false
	}

	var info UserInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		InvalidateCurrentUserInfoForConnection(connection, userID)
		return UserInfo{}, false
	}

	return info, true
}

func cacheCurrentUserInfo(connection string, info UserInfo) {
	raw, err := json.Marshal(info)
	if err != nil {
		return
	}
	_ = facades.Cache().Put(currentUserInfoCacheKey(connection, info.ID), string(raw), currentUserInfoCacheTTL)
}

func InvalidateCurrentUserInfo(userIDs ...uint64) {
	InvalidateAllCurrentUserInfo()
}

func InvalidateCurrentUserInfoForConnection(connection string, userIDs ...uint64) {
	for _, userID := range userIDs {
		if userID == 0 {
			continue
		}
		key := currentUserInfoVersionKey(connection, userID)
		if _, err := facades.Cache().Increment(key); err != nil {
			_ = facades.Cache().Put(key, int64(2), currentUserInfoCacheTTL)
		}
	}
}

func InvalidateAllCurrentUserInfo() {
	if _, err := facades.Cache().Increment("current_user_info_version"); err != nil {
		_ = facades.Cache().Put("current_user_info_version", int64(2), currentUserInfoCacheTTL)
	}
}

// Source: current_user_password.go
func (s *PassportService) appendPasswordUpdate(userID uint64, input ProfileUpdate, values map[string]any) error {
	if input.NewPassword == "" {
		return nil
	}
	if input.NewPassword != input.NewPasswordConfirmation {
		return ErrInvalidCredentials
	}
	if err := ValidatePasswordPolicy(input.NewPassword); err != nil {
		return err
	}
	if err := NewPasswordHistoryService(s.connection, "user_password_history").WithContext(s.ctx).ValidateReuse(userID, input.NewPassword); err != nil {
		return err
	}

	var user models.User
	if err := s.orm().Query().Where("id", userID).First(&user); err != nil {
		return ErrUnauthorized
	}
	if !passwordHashMatches(user.Password, input.OldPassword) {
		return ErrInvalidCredentials
	}

	password, err := makePasswordHash(input.NewPassword)
	if err != nil {
		return err
	}
	values["password"] = password

	return nil
}

func ProfileValidationMessage(err error) string {
	if errors.Is(err, ErrBusinessRule) {
		return err.Error()
	}
	return "旧密码错误或新密码不一致"
}

// Source: current_user_service.go
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

func (s *PassportService) UserIDFromAuthorization(authorization, tokenType string) (uint64, error) {
	tokenInfo, err := s.TokenInfoFromAuthorization(authorization, tokenType)
	if err != nil {
		return 0, err
	}
	return tokenInfo.UserID, nil
}

func (s *PassportService) TokenInfoFromAuthorization(authorization, tokenType string) (TokenInfo, error) {
	return parseApplicationToken(authorization, jwtTokenRequirements{
		Subject:       tenantTokenSubject,
		Type:          tokenType,
		RequireTenant: true,
	})
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

func IsProfileValidationError(err error) bool {
	return errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrBusinessRule)
}

// Source: mfa_service.go
const (
	defaultTOTPStepSeconds   = 30
	defaultTOTPDigits        = 6
	defaultTOTPSkew          = 1
	encryptedMFASecretPrefix = "enc:"
)

var (
	ErrMFAAlreadyEnabled = BusinessError{Message: "MFA 已启用，请先验证后重置"}
	ErrMFAInvalidCode    = BusinessError{Message: "MFA 验证码无效"}
	ErrMFANotEnabled     = BusinessError{Message: "MFA 未启用"}
	ErrMFATokenInvalid   = BusinessError{Message: "MFA Token 无效或已过期"}
	mfaSecretEncrypt     = func(secret string) (string, error) {
		return facades.Crypt().EncryptString(secret)
	}
	mfaSecretDecrypt = func(payload string) (string, error) {
		return facades.Crypt().DecryptString(payload)
	}
)

type MFAService struct {
	ctx        context.Context
	connection string
	table      string
}

type MFASetupResult struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

func NewMFAServiceForTenant(tenant Tenant) *MFAService {
	return &MFAService{connection: TenantConnectionName(tenant), table: "user_mfa"}
}

func NewPlatformMFAService() *MFAService {
	return &MFAService{connection: PlatformConnection(), table: "platform_user_mfa"}
}

func (s *MFAService) WithContext(ctx context.Context) *MFAService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *MFAService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(buf), "="), nil
}

func MFATOTPEnabled() bool {
	return facades.Config().GetBool("security.mfa.totp_enabled", false)
}

func DecodeTOTPSecret(secret string) ([]byte, error) {
	value := strings.ToUpper(strings.TrimSpace(secret))
	if value == "" {
		return nil, BusinessError{Message: "MFA 密钥不能为空"}
	}
	for len(value)%8 != 0 {
		value += "="
	}
	return base32.StdEncoding.DecodeString(value)
}

func GenerateTOTPCode(secret []byte, now time.Time, stepSeconds, digits int) (string, error) {
	if stepSeconds <= 0 {
		stepSeconds = defaultTOTPStepSeconds
	}
	if digits <= 0 {
		digits = defaultTOTPDigits
	}
	counter := uint64(now.Unix() / int64(stepSeconds))
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, secret)
	if _, err := mac.Write(buf[:]); err != nil {
		return "", err
	}
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binCode := (uint32(sum[offset])&0x7f)<<24 |
		(uint32(sum[offset+1])&0xff)<<16 |
		(uint32(sum[offset+2])&0xff)<<8 |
		(uint32(sum[offset+3]) & 0xff)
	mod := uint32(math.Pow10(digits))
	return fmt.Sprintf("%0*d", digits, binCode%mod), nil
}

func VerifyTOTPCode(secret []byte, code string, now time.Time, stepSeconds, digits, skew int) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	if stepSeconds <= 0 {
		stepSeconds = defaultTOTPStepSeconds
	}
	if digits <= 0 {
		digits = defaultTOTPDigits
	}
	if skew < 0 {
		skew = defaultTOTPSkew
	}
	for offset := -skew; offset <= skew; offset++ {
		candidate, err := GenerateTOTPCode(secret, now.Add(time.Duration(offset*stepSeconds)*time.Second), stepSeconds, digits)
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func (s *MFAService) Setup(userID uint64, username string) (MFASetupResult, error) {
	if !MFATOTPEnabled() {
		return MFASetupResult{}, BusinessError{Message: "MFA/TOTP 未启用"}
	}
	if err := ValidateMFASetupAllowed(s.Enabled(userID)); err != nil {
		return MFASetupResult{}, err
	}
	secret, err := GenerateTOTPSecret()
	if err != nil {
		return MFASetupResult{}, err
	}
	storedSecret, err := encryptMFASecret(secret)
	if err != nil {
		return MFASetupResult{}, err
	}
	now := time.Now()
	values := map[string]any{
		"user_id":        userID,
		"secret":         storedSecret,
		"enabled":        false,
		"recovery_codes": models.JSONSlice{},
		"updated_at":     now,
	}
	if s.hasUserMFA(userID) {
		if _, err := s.orm().Query().Table(s.table).Where("user_id", userID).Update(values); err != nil {
			return MFASetupResult{}, err
		}
	} else {
		row := models.UserMFA{
			UserID:        userID,
			Secret:        storedSecret,
			Enabled:       false,
			RecoveryCodes: models.JSONSlice{},
		}
		if err := s.orm().Query().Table(s.table).Create(&row); err != nil {
			return MFASetupResult{}, err
		}
	}
	return MFASetupResult{Secret: secret, URI: s.provisioningURI(username, secret)}, nil
}

func (s *MFAService) Confirm(userID uint64, code string) ([]string, error) {
	if !MFATOTPEnabled() {
		return nil, BusinessError{Message: "MFA/TOTP 未启用"}
	}
	row, err := s.find(userID)
	if err != nil {
		return nil, err
	}
	if !verifySecretCode(row.Secret, code) {
		return nil, ErrMFAInvalidCode
	}
	count := facades.Config().GetInt("security.mfa.recovery_codes", 8)
	codes, hashes, err := GenerateMFARecoveryCodes(count)
	if err != nil {
		return nil, err
	}
	_, err = s.orm().Query().Table(s.table).Where("user_id", userID).Update(map[string]any{
		"enabled":        true,
		"recovery_codes": models.JSONSlice(stringSliceAny(hashes)),
		"confirmed_at":   time.Now(),
		"updated_at":     time.Now(),
	})
	return codes, err
}

func (s *MFAService) Disable(userID uint64) error {
	return s.DisableWithCode(userID, "")
}

func (s *MFAService) DisableWithCode(userID uint64, code string) error {
	if err := ValidateMFADisableCode(code); err != nil {
		return err
	}
	if err := s.Verify(userID, code); err != nil {
		return err
	}
	_, err := s.orm().Query().Table(s.table).Where("user_id", userID).Update(map[string]any{
		"enabled":        false,
		"recovery_codes": models.JSONSlice{},
		"updated_at":     time.Now(),
	})
	return err
}

func (s *MFAService) DisableSensitive(userID, tenantID uint64, code string, evidence SensitiveOperationEvidence) error {
	guard := NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(s.ctx, "mfa.disable", userID, tenantID, SensitiveOperationPlanSelector{
		Resource: fmt.Sprintf("mfa:user:%d:disable", userID),
	})
	if err != nil {
		return err
	}
	flow := sensitiveMFADisableFlow{
		validateEvidence:    func() error { return guard.Validate(s.ctx, plan, evidence) },
		prepareVerification: func() (func() error, error) { return s.PrepareVerify(userID, code) },
		executeSensitive:    func(mutate func() error) error { return guard.Execute(s.ctx, plan, evidence, mutate) },
		disable: func() error {
			_, err := s.orm().Query().Table(s.table).Where("user_id", userID).Where("enabled", true).Update(map[string]any{
				"enabled": false, "recovery_codes": models.JSONSlice{}, "updated_at": time.Now(),
			})
			return err
		},
	}
	return flow.Execute()
}

type sensitiveMFADisableFlow struct {
	validateEvidence    func() error
	prepareVerification func() (func() error, error)
	executeSensitive    func(func() error) error
	disable             func() error
}

func (flow sensitiveMFADisableFlow) Execute() error {
	if err := flow.validateEvidence(); err != nil {
		return err
	}
	commitVerification, err := flow.prepareVerification()
	if err != nil {
		return err
	}
	return flow.executeSensitive(func() error {
		if err := commitVerification(); err != nil {
			return err
		}
		return flow.disable()
	})
}

func ValidateMFASetupAllowed(enabled bool) error {
	if enabled {
		return ErrMFAAlreadyEnabled
	}
	return nil
}

func ValidateMFADisableCode(code string) error {
	if strings.TrimSpace(code) == "" {
		return ErrMFAInvalidCode
	}
	return nil
}

func (s *MFAService) Enabled(userID uint64) bool {
	if !MFATOTPEnabled() {
		return false
	}
	row, err := s.find(userID)
	return err == nil && row.Enabled
}

func (s *MFAService) Verify(userID uint64, code string) error {
	commit, err := s.PrepareVerify(userID, code)
	if err != nil {
		return err
	}
	return commit()
}

// PrepareVerify validates an MFA code and returns a commit function for successful use.
func (s *MFAService) PrepareVerify(userID uint64, code string) (func() error, error) {
	if !MFATOTPEnabled() {
		return nil, BusinessError{Message: "MFA/TOTP 未启用"}
	}
	row, err := s.find(userID)
	if err != nil {
		return nil, err
	}
	return prepareMFAVerification(
		row,
		code,
		func() error { return s.markUsed(userID) },
		func(hashes []string) error { return s.updateRecoveryCodes(userID, hashes) },
	)
}

func prepareMFAVerification(row models.UserMFA, code string, markUsed func() error, updateRecoveryCodes func([]string) error) (func() error, error) {
	if !row.Enabled {
		return nil, ErrMFANotEnabled
	}
	if verifySecretCode(row.Secret, code) {
		return markUsed, nil
	}
	trimmedCode := strings.TrimSpace(code)
	hashes := jsonSliceStrings(row.RecoveryCodes)
	for i, hash := range hashes {
		if secretHashMatches(hash, trimmedCode) {
			remaining := make([]string, 0, len(hashes)-1)
			remaining = append(remaining, hashes[:i]...)
			remaining = append(remaining, hashes[i+1:]...)
			return func() error { return updateRecoveryCodes(remaining) }, nil
		}
	}
	return nil, ErrMFAInvalidCode
}

func (s *MFAService) IssueChallenge(scope string, userID uint64) (string, error) {
	token := rand.Text()
	err := loginSecurityCache().Put(mfaChallengeKey(scope, token), userID, minutesDuration("security.mfa.challenge_minutes", 5))
	return token, err
}

func (s *MFAService) ConsumeChallenge(scope, token string) (uint64, error) {
	userID, err := s.ChallengeUserID(scope, token)
	if err != nil {
		return 0, err
	}
	s.ForgetChallenge(scope, token)
	return userID, nil
}

func (s *MFAService) ChallengeUserID(scope, token string) (uint64, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, ErrMFATokenInvalid
	}
	key := mfaChallengeKey(scope, token)
	value := loginSecurityCache().Get(key)
	userID, ok := cacheUint64(value)
	if !ok || userID == 0 {
		return 0, ErrMFATokenInvalid
	}
	return userID, nil
}

func (s *MFAService) ForgetChallenge(scope, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	_ = loginSecurityCache().Forget(mfaChallengeKey(scope, token))
}

func GenerateMFARecoveryCodes(count int) ([]string, []string, error) {
	if count <= 0 {
		count = 8
	}
	codes := make([]string, 0, count)
	hashes := make([]string, 0, count)
	seen := make(map[string]struct{}, count)
	for len(codes) < count {
		code, err := randomRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		if _, ok := seen[code]; ok {
			continue
		}
		hash, err := makeSecretHash(code)
		if err != nil {
			return nil, nil, err
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
		hashes = append(hashes, hash)
	}
	return codes, hashes, nil
}

func randomRecoveryCode() (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	encoded := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
	if len(encoded) > 16 {
		encoded = encoded[:16]
	}
	return encoded[:8] + "-" + encoded[8:], nil
}

func (s *MFAService) hasUserMFA(userID uint64) bool {
	count, err := s.orm().Query().Table(s.table).Where("user_id", userID).Count()
	return err == nil && count > 0
}

func (s *MFAService) find(userID uint64) (models.UserMFA, error) {
	var row models.UserMFA
	err := s.orm().Query().Table(s.table).Where("user_id", userID).First(&row)
	if err != nil {
		return models.UserMFA{}, ErrMFANotEnabled
	}
	return row, nil
}

func (s *MFAService) markUsed(userID uint64) error {
	_, err := s.orm().Query().Table(s.table).Where("user_id", userID).Update(map[string]any{
		"last_used_at": time.Now(),
		"updated_at":   time.Now(),
	})
	return err
}

func (s *MFAService) updateRecoveryCodes(userID uint64, hashes []string) error {
	_, err := s.orm().Query().Table(s.table).Where("user_id", userID).Update(map[string]any{
		"recovery_codes": models.JSONSlice(stringSliceAny(hashes)),
		"last_used_at":   time.Now(),
		"updated_at":     time.Now(),
	})
	return err
}

func (s *MFAService) provisioningURI(username, secret string) string {
	issuer := facades.Config().GetString("security.mfa.totp_issuer", "Goravel")
	label := issuer + ":" + username
	return fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=%s", url.PathEscape(label), url.QueryEscape(secret), url.QueryEscape(issuer))
}

func verifySecretCode(secret, code string) bool {
	plainSecret, err := decryptMFASecret(secret)
	if err != nil {
		return false
	}
	raw, err := DecodeTOTPSecret(plainSecret)
	if err != nil {
		return false
	}
	return VerifyTOTPCode(raw, code, time.Now(), defaultTOTPStepSeconds, defaultTOTPDigits, defaultTOTPSkew)
}

func encryptMFASecret(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", BusinessError{Message: "MFA 密钥不能为空"}
	}
	encrypted, err := mfaSecretEncrypt(secret)
	if err != nil {
		return "", err
	}
	return encryptedMFASecretPrefix + encrypted, nil
}

func decryptMFASecret(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, encryptedMFASecretPrefix) {
		return secret, nil
	}
	return mfaSecretDecrypt(strings.TrimPrefix(secret, encryptedMFASecretPrefix))
}

func jsonSliceStrings(items models.JSONSlice) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func stringSliceAny(items []string) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

func cacheUint64(value any) (uint64, bool) {
	switch typed := value.(type) {
	case uint64:
		return typed, true
	case uint:
		return uint64(typed), true
	case int:
		if typed > 0 {
			return uint64(typed), true
		}
	case int64:
		if typed > 0 {
			return uint64(typed), true
		}
	case float64:
		if typed > 0 {
			return uint64(typed), true
		}
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
		if err == nil && parsed > 0 {
			return parsed, true
		}
	}
	return 0, false
}

func mfaChallengeKey(scope, token string) string {
	return "security:mfa:challenge:" + loginIdentity(scope, token)
}

// Source: passport_service.go
var (
	ErrInvalidCredentials = authcontract.ErrInvalidCredentials
	ErrUserDisabled       = authcontract.ErrUserDisabled
	ErrUnauthorized       = authcontract.ErrUnauthorized
	ErrJWTSecretMissing   = authcontract.ErrJWTSecretMissing
)

type PassportService struct {
	ctx        context.Context
	connection string
	tenant     Tenant
}

type LoginResult struct {
	AccessToken            string `json:"access_token"`
	RefreshToken           string `json:"refresh_token"`
	ExpireAt               int    `json:"expire_at"`
	MFARequired            bool   `json:"mfa_required,omitempty"`
	MFAToken               string `json:"mfa_token,omitempty"`
	PasswordChangeRequired bool   `json:"password_change_required,omitempty"`
	PasswordChangeToken    string `json:"password_change_token,omitempty"`
}

func NewPassportService() *PassportService {
	return &PassportService{}
}

func NewPassportServiceForTenant(tenant Tenant) *PassportService {
	return &PassportService{connection: TenantConnectionName(tenant), tenant: tenant}
}

func (s *PassportService) WithContext(ctx context.Context) *PassportService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *PassportService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func (s *PassportService) Orm() contractsorm.Orm {
	return s.orm()
}

func (s *PassportService) Login(username, password, ip, browser, os string) (LoginResult, error) {
	if s.tenant.ID == 0 {
		return LoginResult{}, ErrTenantRequired
	}
	if err := CheckLoginLockout(s.loginSecurityScope(), username); err != nil {
		_ = s.writeLoginLog(username, ip, browser, os, 2, "账号已锁定")
		return LoginResult{}, err
	}
	if err := CheckLoginRisk(s.loginSecurityScope(), username, ip, browser); err != nil {
		_ = s.writeLoginLog(username, ip, browser, os, 2, "登录风险过高")
		return LoginResult{}, err
	}

	var user models.User
	if err := s.orm().Query().
		Where("username", username).
		Where("user_type", "100").
		First(&user); err != nil {
		_ = RecordLoginFailure(s.loginSecurityScope(), username)
		_ = RecordLoginRiskFailure(s.loginSecurityScope(), username, ip, browser)
		_ = s.writeLoginLog(username, ip, browser, os, 2, "用户名或密码错误")
		return LoginResult{}, ErrInvalidCredentials
	}
	if password == externalUserDefaultPass {
		managed, err := s.isLegacySSOManagedUser(user.ID)
		if err != nil {
			return LoginResult{}, err
		}
		if managed {
			_ = RecordLoginFailure(s.loginSecurityScope(), username)
			_ = RecordLoginRiskFailure(s.loginSecurityScope(), username, ip, browser)
			_ = s.writeLoginLog(username, ip, browser, os, 2, "用户名或密码错误")
			return LoginResult{}, ErrInvalidCredentials
		}
	}

	if !passwordHashMatches(user.Password, password) {
		_ = RecordLoginFailure(s.loginSecurityScope(), username)
		_ = RecordLoginRiskFailure(s.loginSecurityScope(), username, ip, browser)
		_ = s.writeLoginLog(username, ip, browser, os, 2, "用户名或密码错误")
		return LoginResult{}, ErrInvalidCredentials
	}
	passwordHistory := TenantPasswordHistoryService(s.tenant).WithContext(s.ctx)
	if err := passwordHistory.SeedIfMissing(user); err != nil {
		return LoginResult{}, err
	}

	if user.Status == 2 {
		_ = s.writeLoginLog(username, ip, browser, os, 2, "用户已停用")
		return LoginResult{}, ErrUserDisabled
	}
	if mfa := NewMFAServiceForTenant(s.tenant).WithContext(s.ctx); mfa.Enabled(user.ID) {
		token, err := mfa.IssueChallenge(s.loginSecurityScope(), user.ID)
		if err != nil {
			return LoginResult{}, err
		}
		_ = s.writeLoginLog(username, ip, browser, os, 2, "等待 MFA 验证")
		return LoginResult{MFARequired: true, MFAToken: token}, nil
	}
	if result, challenged, err := s.passwordChangeChallengeIfExpired(passwordHistory, user.ID, username, ip, browser, os); err != nil || challenged {
		return result, err
	}

	_ = s.writeLoginLog(username, ip, browser, os, 1, "登录成功")
	_ = RecordLoginSuccess(s.loginSecurityScope(), username)
	_ = RecordLoginRiskSuccess(s.loginSecurityScope(), username, ip, browser)

	return s.issueLoginTokens(user.ID)
}

func (s *PassportService) CompleteMFALogin(mfaToken, code, ip, browser, os string) (LoginResult, error) {
	mfa := NewMFAServiceForTenant(s.tenant).WithContext(s.ctx)
	userID, err := mfa.ChallengeUserID(s.loginSecurityScope(), mfaToken)
	if err != nil {
		return LoginResult{}, err
	}
	var user models.User
	if err := s.orm().Query().Where("id", userID).First(&user); err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status == 2 {
		_ = s.writeLoginLog(user.Username, ip, browser, os, 2, "用户已停用")
		return LoginResult{}, ErrUserDisabled
	}
	commitMFA, err := mfa.PrepareVerify(user.ID, code)
	if err != nil {
		_ = RecordLoginFailure(s.loginSecurityScope(), user.Username)
		_ = RecordLoginRiskFailure(s.loginSecurityScope(), user.Username, ip, browser)
		_ = s.writeLoginLog(user.Username, ip, browser, os, 2, "MFA 验证失败")
		return LoginResult{}, err
	}
	consumedUserID, err := mfa.ConsumeChallenge(s.loginSecurityScope(), mfaToken)
	if err != nil || consumedUserID != user.ID {
		return LoginResult{}, ErrMFATokenInvalid
	}
	if err := commitMFA(); err != nil {
		return LoginResult{}, err
	}
	_ = s.writeLoginLog(user.Username, ip, browser, os, 1, "登录成功")
	_ = RecordLoginSuccess(s.loginSecurityScope(), user.Username)
	_ = RecordLoginRiskSuccess(s.loginSecurityScope(), user.Username, ip, browser)
	if result, challenged, err := s.passwordChangeChallengeIfExpired(TenantPasswordHistoryService(s.tenant).WithContext(s.ctx), user.ID, user.Username, ip, browser, os); err != nil || challenged {
		return result, err
	}
	return s.issueLoginTokens(user.ID)
}

func (s *PassportService) CompletePasswordChange(passwordChangeToken string, input ProfileUpdate, ip, browser, os string) (LoginResult, error) {
	userID, err := PasswordChangeChallengeUserID(s.loginSecurityScope(), passwordChangeToken)
	if err != nil {
		return LoginResult{}, err
	}
	var user models.User
	if err := s.orm().Query().Where("id", userID).First(&user); err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status == 2 {
		_ = s.writeLoginLog(user.Username, ip, browser, os, 2, "用户已停用")
		return LoginResult{}, ErrUserDisabled
	}
	if input.NewPassword == "" {
		return LoginResult{}, BusinessError{Message: "新密码不能为空"}
	}
	values := map[string]any{"updated_at": time.Now()}
	if err := s.appendPasswordUpdate(user.ID, input, values); err != nil {
		return LoginResult{}, err
	}
	consumedUserID, err := ConsumePasswordChangeChallenge(s.loginSecurityScope(), passwordChangeToken)
	if err != nil || consumedUserID != user.ID {
		return LoginResult{}, ErrPasswordChangeTokenInvalid
	}
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Model(&models.User{}).Where("id", user.ID).Update(values)
		if err != nil {
			return err
		}
		if hash, ok := values["password"].(string); ok && hash != "" {
			return TenantPasswordHistoryService(s.tenant).WithContext(s.ctx).RecordWithQuery(tx, user.ID, hash)
		}
		return nil
	}); err != nil {
		return LoginResult{}, err
	}
	InvalidateCurrentUserInfoForConnection(s.connection, user.ID)
	_ = s.writeLoginLog(user.Username, ip, browser, os, 1, "密码过期改密后登录成功")
	_ = RecordLoginSuccess(s.loginSecurityScope(), user.Username)
	_ = RecordLoginRiskSuccess(s.loginSecurityScope(), user.Username, ip, browser)
	return s.issueLoginTokens(user.ID)
}

func (s *PassportService) isLegacySSOManagedUser(userID uint64) (bool, error) {
	count, err := s.orm().Query().
		Table("sso_user_binding").
		Where("user_id", userID).
		Count()
	return count > 0, err
}

func (s *PassportService) UserFromAuthorization(authorization string) (models.User, error) {
	if tokenBlacklisted(bearerToken(authorization)) {
		return models.User{}, ErrUnauthorized
	}

	tokenInfo, err := s.TokenInfoFromAuthorization(authorization, "access")
	if err != nil {
		return models.User{}, err
	}
	if err := s.ensureTokenTenant(tokenInfo.TenantID); err != nil {
		return models.User{}, err
	}

	var user models.User
	if err := s.orm().Query().Where("id", tokenInfo.UserID).First(&user); err != nil {
		return models.User{}, ErrUnauthorized
	}

	if user.Status == 2 {
		return models.User{}, ErrUserDisabled
	}

	return user, nil
}

func (s *PassportService) Refresh(authorization string) (LoginResult, error) {
	if tokenBlacklisted(bearerToken(authorization)) {
		return LoginResult{}, ErrUnauthorized
	}

	tokenInfo, err := s.TokenInfoFromAuthorization(authorization, "refresh")
	if err != nil {
		return LoginResult{}, err
	}
	if err := s.ensureTokenTenant(tokenInfo.TenantID); err != nil {
		return LoginResult{}, err
	}

	var user models.User
	if err := s.orm().Query().Where("id", tokenInfo.UserID).First(&user); err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status == 2 {
		return LoginResult{}, ErrUserDisabled
	}

	accessTTL := AccessTokenTTLSeconds()
	refreshTTL := RefreshTokenTTLSeconds()
	accessToken, err := s.buildToken(tokenInfo.UserID, tokenInfo.TenantID, "access", accessTTL)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := s.buildToken(tokenInfo.UserID, tokenInfo.TenantID, "refresh", refreshTTL)
	if err != nil {
		return LoginResult{}, err
	}

	if err := blacklistToken(bearerToken(authorization), tokenBlacklistTTL(refreshTTL)); err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpireAt:     accessTTL,
	}, nil
}

func (s *PassportService) Logout(authorization string) error {
	token := bearerToken(authorization)
	if token == "" {
		return ErrUnauthorized
	}

	tokenInfo, err := s.TokenInfoFromAuthorization(authorization, "access")
	if err != nil {
		return err
	}
	if err := s.ensureTokenTenant(tokenInfo.TenantID); err != nil {
		return err
	}

	return blacklistToken(token, tokenBlacklistTTL(AccessTokenTTLSeconds()))
}

func (s *PassportService) buildToken(userID, tenantID uint64, tokenType string, ttlSeconds int) (string, error) {
	return issueApplicationToken(tenantTokenSubject, userID, tenantID, tokenType, ttlSeconds)
}

func (s *PassportService) issueLoginTokens(userID uint64) (LoginResult, error) {
	accessTTL := AccessTokenTTLSeconds()
	refreshTTL := RefreshTokenTTLSeconds()
	accessToken, err := s.buildToken(userID, s.tenant.ID, "access", accessTTL)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := s.buildToken(userID, s.tenant.ID, "refresh", refreshTTL)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: accessToken, RefreshToken: refreshToken, ExpireAt: accessTTL}, nil
}

func (s *PassportService) passwordChangeChallengeIfExpired(passwordHistory *PasswordHistoryService, userID uint64, username, ip, browser, os string) (LoginResult, bool, error) {
	if err := passwordHistory.CheckMaxAge(userID); err != nil {
		_ = s.writeLoginLog(username, ip, browser, os, 2, "密码已过期")
		if errors.Is(err, ErrBusinessRule) {
			token, challengeErr := IssuePasswordChangeChallenge(s.loginSecurityScope(), userID)
			if challengeErr != nil {
				return LoginResult{}, false, challengeErr
			}
			return LoginResult{PasswordChangeRequired: true, PasswordChangeToken: token}, true, nil
		}
		return LoginResult{}, false, err
	}
	return LoginResult{}, false, nil
}

func (s *PassportService) writeLoginLog(username, ip, browser, os string, status int16, message string) error {
	return s.orm().Query().Create(&models.UserLoginLog{
		Username:  username,
		IP:        ip,
		OS:        os,
		Browser:   browser,
		Status:    status,
		Message:   message,
		LoginTime: time.Now(),
	})
}

func (s *PassportService) loginSecurityScope() string {
	if s.tenant.Code != "" {
		return "tenant:" + s.tenant.Code
	}
	if s.tenant.ID != 0 {
		return fmt.Sprintf("tenant:%d", s.tenant.ID)
	}
	return "tenant"
}

func LoginErrorResult(err error) response.Result {
	switch {
	case errors.Is(err, ErrLoginRiskBlocked):
		return response.Error(response.CodeTooManyRequests, "登录风险过高，请稍后再试", []any{})
	case errors.Is(err, ErrAccountLocked):
		return response.Error(response.CodeTooManyRequests, "账号已锁定，请稍后再试", []any{})
	case errors.Is(err, ErrInvalidCredentials):
		return response.Error(response.CodeUnprocessableEntity, "用户名或密码错误", []any{})
	case errors.Is(err, ErrUserDisabled):
		return response.Error(response.CodeDisabled, "用户已停用", []any{})
	case errors.Is(err, ErrUnauthorized):
		return response.Error(response.CodeUnauthorized, "未登录或登录已过期", []any{})
	case errors.Is(err, ErrJWTSecretMissing):
		return response.Error(response.CodeFail, "JWT 密钥未配置", []any{})
	case errors.Is(err, ErrBusinessRule):
		return response.Error(response.CodeUnprocessableEntity, err.Error(), []any{})
	case errors.Is(err, ErrSubscriptionInactive):
		return response.Error(response.CodeDisabled, "租户订阅不可用", []any{})
	case errors.Is(err, ErrQuotaExceeded):
		return response.Error(response.CodeTooManyRequests, "租户配额已用尽", []any{})
	case errors.Is(err, ErrSSONotConfigured):
		return response.Error(response.CodeUnauthorized, "SSO 未配置或已停用", []any{})
	case errors.Is(err, ErrSSOTokenInvalid):
		return response.Error(response.CodeUnprocessableEntity, "SSO Token 无效", []any{})
	default:
		return response.Error(response.CodeFail, "服务器错误", []any{})
	}
}

// Source: platform_bootstrap_service.go
type PlatformBootstrapService struct{}

func NewPlatformBootstrapService() *PlatformBootstrapService {
	return &PlatformBootstrapService{}
}

func (s *PlatformBootstrapService) EnsureLocalDefaults() error {
	if !s.shouldAutoSeed() || !s.hasPlatformBootstrapTables() {
		return nil
	}
	exists, err := s.platformAdminExists()
	if err != nil {
		return err
	}
	restore := seeders.SetConnection(PlatformConnection())
	defer restore()
	if !exists {
		return s.seedPlatformAccessDefaults()
	}
	return s.syncPlatformAccessDefaults()
}

func (s *PlatformBootstrapService) platformAdminExists() (bool, error) {
	count, err := OrmForConnection(PlatformConnection()).
		Query().
		Table("platform_user").
		Where("username", "admin").
		Where("user_type", "900").
		Count()
	return count > 0, err
}

func (s *PlatformBootstrapService) hasPlatformBootstrapTables() bool {
	for _, table := range []string{
		"platform_user",
		"platform_role",
		"platform_user_belongs_role",
		"platform_menu",
		"platform_role_belongs_menu",
		"platform_casbin_rule",
	} {
		if !facades.Schema().HasTable(table) {
			return false
		}
	}
	return true
}

func (s *PlatformBootstrapService) seedPlatformAccessDefaults() error {
	for _, item := range []interface{ Run() error }{
		&seeders.PlatformAdminSeeder{},
		&seeders.PlatformMenuSeeder{},
		&seeders.PlatformCasbinSeeder{},
	} {
		if err := item.Run(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PlatformBootstrapService) syncPlatformAccessDefaults() error {
	for _, item := range []interface{ Run() error }{
		&seeders.PlatformMenuSeeder{},
		&seeders.PlatformCasbinSeeder{},
	} {
		if err := item.Run(); err != nil {
			return err
		}
	}
	return nil
}

func (s *PlatformBootstrapService) shouldAutoSeed() bool {
	appEnv := facades.Config().GetString("app.env")
	return env.IsTesting() || appEnv == "local"
}

// Source: platform_passport_service.go
type PlatformPassportService struct {
	ctx context.Context
}

func NewPlatformPassportService() *PlatformPassportService {
	return &PlatformPassportService{}
}

func (s *PlatformPassportService) WithContext(ctx context.Context) *PlatformPassportService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *PlatformPassportService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func (s *PlatformPassportService) Orm() contractsorm.Orm {
	return s.orm()
}

func (s *PlatformPassportService) Login(username, password string, signals ...LoginSignal) (LoginResult, error) {
	signal := firstLoginSignal(signals)
	if err := NewPlatformBootstrapService().EnsureLocalDefaults(); err != nil {
		return LoginResult{}, err
	}
	if err := CheckLoginLockout("platform", username); err != nil {
		return LoginResult{}, err
	}
	if err := CheckLoginRisk("platform", username, signal.IP, signal.UserAgent); err != nil {
		return LoginResult{}, err
	}

	var user models.User
	if err := s.orm().Query().
		Table("platform_user").
		Where("username", username).
		Where("user_type", "900").
		First(&user); err != nil {
		_ = RecordLoginFailure("platform", username)
		_ = RecordLoginRiskFailure("platform", username, signal.IP, signal.UserAgent)
		return LoginResult{}, ErrInvalidCredentials
	}

	if !passwordHashMatches(user.Password, password) {
		_ = RecordLoginFailure("platform", username)
		_ = RecordLoginRiskFailure("platform", username, signal.IP, signal.UserAgent)
		return LoginResult{}, ErrInvalidCredentials
	}
	passwordHistory := PlatformPasswordHistoryService().WithContext(s.ctx)
	if err := passwordHistory.SeedIfMissing(user); err != nil {
		return LoginResult{}, err
	}

	if user.Status == 2 {
		return LoginResult{}, ErrUserDisabled
	}
	if mfa := NewPlatformMFAService().WithContext(s.ctx); mfa.Enabled(user.ID) {
		token, err := mfa.IssueChallenge("platform", user.ID)
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{MFARequired: true, MFAToken: token}, nil
	}
	if result, challenged, err := s.passwordChangeChallengeIfExpired(passwordHistory, user.ID); err != nil || challenged {
		return result, err
	}
	_ = RecordLoginSuccess("platform", username)
	_ = RecordLoginRiskSuccess("platform", username, signal.IP, signal.UserAgent)

	return s.issueLoginTokens(user.ID)
}

func (s *PlatformPassportService) CompleteMFALogin(mfaToken, code string, signals ...LoginSignal) (LoginResult, error) {
	signal := firstLoginSignal(signals)
	mfa := NewPlatformMFAService().WithContext(s.ctx)
	userID, err := mfa.ChallengeUserID("platform", mfaToken)
	if err != nil {
		return LoginResult{}, err
	}
	var user models.User
	if err := s.orm().Query().Table("platform_user").Where("id", userID).First(&user); err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status == 2 {
		return LoginResult{}, ErrUserDisabled
	}
	commitMFA, err := mfa.PrepareVerify(user.ID, code)
	if err != nil {
		_ = RecordLoginFailure("platform", user.Username)
		_ = RecordLoginRiskFailure("platform", user.Username, signal.IP, signal.UserAgent)
		return LoginResult{}, err
	}
	consumedUserID, err := mfa.ConsumeChallenge("platform", mfaToken)
	if err != nil || consumedUserID != user.ID {
		return LoginResult{}, ErrMFATokenInvalid
	}
	if err := commitMFA(); err != nil {
		return LoginResult{}, err
	}
	_ = RecordLoginSuccess("platform", user.Username)
	_ = RecordLoginRiskSuccess("platform", user.Username, signal.IP, signal.UserAgent)
	if result, challenged, err := s.passwordChangeChallengeIfExpired(PlatformPasswordHistoryService().WithContext(s.ctx), user.ID); err != nil || challenged {
		return result, err
	}
	return s.issueLoginTokens(user.ID)
}

func (s *PlatformPassportService) CompletePasswordChange(passwordChangeToken string, input ProfileUpdate, signals ...LoginSignal) (LoginResult, error) {
	signal := firstLoginSignal(signals)
	userID, err := PasswordChangeChallengeUserID("platform", passwordChangeToken)
	if err != nil {
		return LoginResult{}, err
	}
	var user models.User
	if err := s.orm().Query().Table("platform_user").Where("id", userID).First(&user); err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status == 2 {
		return LoginResult{}, ErrUserDisabled
	}
	if input.NewPassword == "" {
		return LoginResult{}, BusinessError{Message: "新密码不能为空"}
	}
	values := map[string]any{"updated_at": time.Now()}
	if err := s.appendPasswordUpdate(user.ID, input, values); err != nil {
		return LoginResult{}, err
	}
	consumedUserID, err := ConsumePasswordChangeChallenge("platform", passwordChangeToken)
	if err != nil || consumedUserID != user.ID {
		return LoginResult{}, ErrPasswordChangeTokenInvalid
	}
	if err := s.updatePasswordWithValues(user.ID, values); err != nil {
		return LoginResult{}, err
	}
	_ = RecordLoginSuccess("platform", user.Username)
	_ = RecordLoginRiskSuccess("platform", user.Username, signal.IP, signal.UserAgent)
	return s.issueLoginTokens(user.ID)
}

func (s *PlatformPassportService) UserFromAuthorization(authorization string) (models.User, error) {
	if tokenBlacklisted(bearerToken(authorization)) {
		return models.User{}, ErrUnauthorized
	}

	userID, err := s.UserIDFromAuthorization(authorization, "access")
	if err != nil {
		return models.User{}, err
	}

	var user models.User
	if err := s.orm().Query().Table("platform_user").Where("id", userID).First(&user); err != nil {
		return models.User{}, ErrUnauthorized
	}

	if user.Status == 2 {
		return models.User{}, ErrUserDisabled
	}

	return user, nil
}

func (s *PlatformPassportService) Refresh(authorization string) (LoginResult, error) {
	if tokenBlacklisted(bearerToken(authorization)) {
		return LoginResult{}, ErrUnauthorized
	}

	userID, err := s.UserIDFromAuthorization(authorization, "refresh")
	if err != nil {
		return LoginResult{}, err
	}

	var user models.User
	if err := s.orm().Query().Table("platform_user").Where("id", userID).First(&user); err != nil {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status == 2 {
		return LoginResult{}, ErrUserDisabled
	}

	accessTTL := AccessTokenTTLSeconds()
	refreshTTL := RefreshTokenTTLSeconds()
	accessToken, err := s.buildToken(user.ID, "access", accessTTL)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := s.buildToken(user.ID, "refresh", refreshTTL)
	if err != nil {
		return LoginResult{}, err
	}

	if err := blacklistToken(bearerToken(authorization), tokenBlacklistTTL(refreshTTL)); err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpireAt:     accessTTL,
	}, nil
}

func (s *PlatformPassportService) Logout(authorization string) error {
	token := bearerToken(authorization)
	if token == "" {
		return ErrUnauthorized
	}

	if _, err := s.UserIDFromAuthorization(authorization, "access"); err != nil {
		return err
	}

	return blacklistToken(token, tokenBlacklistTTL(AccessTokenTTLSeconds()))
}

func (s *PlatformPassportService) UserIDFromAuthorization(authorization, tokenType string) (uint64, error) {
	tokenInfo, err := parseApplicationToken(authorization, jwtTokenRequirements{
		Subject: authservice.PlatformTokenSubject,
		Type:    tokenType,
	})
	if err != nil {
		return 0, err
	}
	return tokenInfo.UserID, nil
}

func (s *PlatformPassportService) FormatUserInfo(user models.User) (UserInfo, error) {
	roles, err := s.UserRoles(user.ID)
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
		Departments:    []DepartmentInfo{},
		Positions:      []PositionInfo{},
		Roles:          roles,
	}, nil
}

func (s *PlatformPassportService) UserRoles(userID uint64) ([]RoleInfo, error) {
	roles := make([]RoleInfo, 0)
	err := s.orm().Query().
		Table("platform_role").
		Select("platform_role.id", "platform_role.code", "platform_role.name").
		Join("JOIN platform_user_belongs_role ubr ON ubr.role_id = platform_role.id").
		Where("ubr.user_id", userID).
		Where("platform_role.status", 1).
		OrderBy("platform_role.sort").
		OrderBy("platform_role.id").
		Scan(&roles)

	return roles, err
}

func (s *PlatformPassportService) IsSuperAdmin(user models.User) (bool, error) {
	if user.ID == 1 {
		return true, nil
	}

	roles, err := s.UserRoles(user.ID)
	if err != nil {
		return false, err
	}

	for _, role := range roles {
		if role.Code == "PlatformSuperAdmin" {
			return true, nil
		}
	}

	return false, nil
}

func (s *PlatformPassportService) UpdateProfile(userID uint64, input ProfileUpdate) error {
	values, err := profileUpdateValues(input)
	if err != nil {
		return err
	}
	if err := s.appendPasswordUpdate(userID, input, values); err != nil {
		return err
	}

	err = s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_user").
			Where("id", userID).
			Update(values)
		if err != nil {
			return err
		}
		if hash, ok := values["password"].(string); ok && hash != "" {
			return PlatformPasswordHistoryService().WithContext(s.ctx).RecordWithQuery(tx, userID, hash)
		}
		return nil
	})

	return err
}

func (s *PlatformPassportService) appendPasswordUpdate(userID uint64, input ProfileUpdate, values map[string]any) error {
	if input.NewPassword == "" {
		return nil
	}
	if input.NewPassword != input.NewPasswordConfirmation {
		return ErrInvalidCredentials
	}
	if err := ValidatePasswordPolicy(input.NewPassword); err != nil {
		return err
	}
	if err := PlatformPasswordHistoryService().WithContext(s.ctx).ValidateReuse(userID, input.NewPassword); err != nil {
		return err
	}
	var user models.User
	if err := s.orm().Query().Table("platform_user").Where("id", userID).First(&user); err != nil {
		return ErrUnauthorized
	}
	if !passwordHashMatches(user.Password, input.OldPassword) {
		return ErrInvalidCredentials
	}
	password, err := makePasswordHash(input.NewPassword)
	if err != nil {
		return err
	}
	values["password"] = password
	return nil
}

func (s *PlatformPassportService) updatePassword(userID uint64, input ProfileUpdate) error {
	values := map[string]any{"updated_at": time.Now()}
	if err := s.appendPasswordUpdate(userID, input, values); err != nil {
		return err
	}
	return s.updatePasswordWithValues(userID, values)
}

func (s *PlatformPassportService) updatePasswordWithValues(userID uint64, values map[string]any) error {
	return s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_user").Where("id", userID).Update(values)
		if err != nil {
			return err
		}
		if hash, ok := values["password"].(string); ok && hash != "" {
			return PlatformPasswordHistoryService().WithContext(s.ctx).RecordWithQuery(tx, userID, hash)
		}
		return nil
	})
}

func (s *PlatformPassportService) buildToken(userID uint64, tokenType string, ttlSeconds int) (string, error) {
	return issueApplicationToken(authservice.PlatformTokenSubject, userID, 0, tokenType, ttlSeconds)
}

func (s *PlatformPassportService) issueLoginTokens(userID uint64) (LoginResult, error) {
	accessTTL := AccessTokenTTLSeconds()
	refreshTTL := RefreshTokenTTLSeconds()
	accessToken, err := s.buildToken(userID, "access", accessTTL)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := s.buildToken(userID, "refresh", refreshTTL)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: accessToken, RefreshToken: refreshToken, ExpireAt: accessTTL}, nil
}

func (s *PlatformPassportService) passwordChangeChallengeIfExpired(passwordHistory *PasswordHistoryService, userID uint64) (LoginResult, bool, error) {
	if err := passwordHistory.CheckMaxAge(userID); err != nil {
		if errors.Is(err, ErrBusinessRule) {
			token, challengeErr := IssuePasswordChangeChallenge("platform", userID)
			if challengeErr != nil {
				return LoginResult{}, false, challengeErr
			}
			return LoginResult{PasswordChangeRequired: true, PasswordChangeToken: token}, true, nil
		}
		return LoginResult{}, false, err
	}
	return LoginResult{}, false, nil
}
