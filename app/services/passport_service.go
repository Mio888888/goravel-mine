package services

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/http/response"
	"goravel/app/models"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user disabled")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrJWTSecretMissing   = errors.New("jwt secret is not configured")
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
	secret, err := JWTSecret()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":  "user",
		"uid":  userID,
		"tid":  tenantID,
		"type": tokenType,
		"jti":  rand.Text(),
		"iat":  now.Unix(),
	}
	if ttlSeconds > 0 {
		claims["exp"] = now.Add(time.Duration(ttlSeconds) * time.Second).Unix()
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
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

func JWTSecret() (string, error) {
	secret := facades.Config().GetString("jwt.secret")
	if secret != "" {
		return secret, nil
	}

	appKey := facades.Config().GetString("app.key")
	if appKey != "" {
		return appKey, nil
	}

	if facades.Config().GetString("app.env") == "local" {
		return "local-development-jwt-secret", nil
	}

	return "", ErrJWTSecretMissing
}

func AccessTokenTTLSeconds() int {
	return jwtTTLSeconds("jwt.ttl", 60)
}

func RefreshTokenTTLSeconds() int {
	return jwtTTLSeconds("jwt.refresh_ttl", 20160)
}

func jwtTTLSeconds(configKey string, defaultMinutes int) int {
	minutes := facades.Config().GetInt(configKey, defaultMinutes)
	if minutes <= 0 {
		return 0
	}
	return minutes * 60
}

func tokenBlacklistTTL(ttlSeconds int) time.Duration {
	if ttlSeconds <= 0 {
		return 0
	}
	return time.Duration(ttlSeconds) * time.Second
}

func bearerToken(authorization string) string {
	token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	if token == authorization {
		return ""
	}

	return token
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
