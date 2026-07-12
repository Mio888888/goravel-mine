package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	frameworkerrors "github.com/goravel/framework/errors"
	"golang.org/x/crypto/bcrypt"

	"goravel/app/facades"
	"goravel/app/models"
)

const (
	BillingStatusActive     = "active"
	BillingStatusTrialing   = "trialing"
	BillingStatusPastDue    = "past_due"
	BillingStatusCanceled   = "canceled"
	BillingStatusExpired    = "expired"
	externalUserType        = "100"
	externalUserDefaultPass = "__sso_managed__"
)

var (
	ErrSubscriptionInactive = errors.New("tenant subscription is inactive")
	ErrQuotaExceeded        = errors.New("tenant quota exceeded")
	ErrSSONotConfigured     = errors.New("sso provider is not configured")
	ErrSSOTokenInvalid      = errors.New("sso token is invalid")
)

type TenantRuntimeService struct {
	ctx context.Context
}

type TenantPublicConfig struct {
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	Plan         string         `json:"plan"`
	CustomDomain *string        `json:"custom_domain"`
	Branding     models.JSONMap `json:"branding"`
	Features     models.JSONMap `json:"features"`
}

type TenantUsageReport struct {
	ID      uint64         `json:"id"`
	Code    string         `json:"code"`
	Name    string         `json:"name"`
	Plan    string         `json:"plan"`
	Billing models.JSONMap `json:"billing"`
	Quotas  models.JSONMap `json:"quotas"`
	Usage   models.JSONMap `json:"usage"`
}

type TenantQuotaSnapshot struct {
	Users     int64 `json:"users"`
	Roles     int64 `json:"roles"`
	StorageMB int64 `json:"storage_mb"`
}

type SSOLoginPayload struct {
	Provider      string `json:"provider"`
	Scene         string `json:"scene"`
	TransactionID string `json:"transaction_id"`
	State         string `json:"state"`
	IDToken       string `json:"id_token"`
	Code          string `json:"code"`
	CodeVerifier  string `json:"code_verifier"`
	Nonce         string `json:"nonce"`
	RedirectURI   string `json:"redirect_uri"`
	SAMLResponse  string `json:"saml_response"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	Subject       string `json:"subject"`
}

type ssoClaims struct {
	Subject string
	Email   string
	Name    string
	Issuer  string
	Raw     map[string]any
}

func NewTenantRuntimeService() *TenantRuntimeService {
	return &TenantRuntimeService{}
}

func (s *TenantRuntimeService) WithContext(ctx context.Context) *TenantRuntimeService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *TenantRuntimeService) PublicConfig(tenant Tenant, scene ...string) TenantPublicConfig {
	features := s.EffectiveFeatures(tenant)
	features["sso"] = s.publicSSOFeatures(tenant, scene...)
	return TenantPublicConfig{
		Code:         tenant.Code,
		Name:         tenant.Name,
		Plan:         tenant.Plan,
		CustomDomain: tenant.CustomDomain,
		Branding:     mapOrEmpty(tenant.Branding),
		Features:     publicFeatures(features),
	}
}

func (s *TenantRuntimeService) Usage(tenant Tenant) (TenantUsageReport, error) {
	usage, err := s.QuotaSnapshot(tenant)
	if err != nil {
		return TenantUsageReport{}, err
	}

	return TenantUsageReport{
		ID: tenant.ID, Code: tenant.Code, Name: tenant.Name, Plan: tenant.Plan,
		Billing: s.EffectiveBilling(tenant),
		Quotas:  s.EffectiveQuotas(tenant),
		Usage: models.JSONMap{
			"users":      usage.Users,
			"roles":      usage.Roles,
			"storage_mb": usage.StorageMB,
		},
	}, nil
}

func (s *TenantRuntimeService) QuotaSnapshot(tenant Tenant) (TenantQuotaSnapshot, error) {
	connection := RegisterTenantConnection(tenant)
	query := OrmForConnectionWithContext(s.ctx, connection).Query()
	users, err := query.Table(`"user"`).Where("user_type", externalUserType).Count()
	if err != nil {
		return TenantQuotaSnapshot{}, err
	}
	roles, err := OrmForConnectionWithContext(s.ctx, connection).Query().Table("role").Count()
	if err != nil {
		return TenantQuotaSnapshot{}, err
	}
	var storageBytes int64
	err = OrmForConnectionWithContext(s.ctx, connection).Query().
		Raw("SELECT COALESCE(SUM(size_byte), 0) FROM attachment").
		Scan(&storageBytes)
	if err != nil {
		return TenantQuotaSnapshot{}, err
	}

	return TenantQuotaSnapshot{
		Users:     users,
		Roles:     roles,
		StorageMB: bytesToMB(storageBytes),
	}, nil
}

func (s *TenantRuntimeService) EnsureSubscription(tenant Tenant) error {
	billing := s.EffectiveBilling(tenant)
	status := strings.ToLower(strings.TrimSpace(jsonString(billing, "subscription_status")))
	if status == "" {
		status = BillingStatusActive
	}
	if status != BillingStatusActive && status != BillingStatusTrialing {
		return ErrSubscriptionInactive
	}
	if expiresAt := jsonString(billing, "expires_at"); expiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return BusinessError{Message: "租户订阅到期时间格式错误"}
		}
		if time.Now().After(parsed) {
			return ErrSubscriptionInactive
		}
	}
	return nil
}

func (s *TenantRuntimeService) AllowRequest(tenant Tenant) error {
	limit := jsonInt64(s.EffectiveQuotas(tenant), "api_rate_per_minute")
	return NewTenantRateLimiter().Allow(tenant, limit)
}

func (s *TenantRuntimeService) EnsureResourceQuota(tenant Tenant, resource string, add int64) error {
	limit := jsonInt64(s.EffectiveQuotas(tenant), resourceQuotaKey(resource))
	if limit <= 0 {
		return nil
	}

	usage, err := s.QuotaSnapshot(tenant)
	if err != nil {
		return err
	}
	current := resourceUsage(usage, resource)
	if current+add > limit {
		return ErrQuotaExceeded
	}
	return nil
}

func (s *TenantRuntimeService) SSOLogin(tenant Tenant, payload SSOLoginPayload, ip, browser, os string) (LoginResult, error) {
	if strings.TrimSpace(payload.Code) != "" {
		return LoginResult{}, ErrSSOAuthorizationTransactionInvalid
	}
	provider, err := NewSSOProviderServiceForTenant(tenant).WithContext(s.ctx).EnabledProviderForScene(payload.Provider, payload.Scene)
	if err != nil {
		return LoginResult{}, err
	}
	connection := RegisterTenantConnection(tenant)
	audit := NewSSOAuditServiceForConnection(connection).WithContext(s.ctx)
	claims, err := verifySSOClaims(provider, payload)
	if err != nil {
		_ = audit.Log(ssoLogInput{
			ProviderID: provider.ID, Status: ssoLogStatusFailure,
			FailureReason: ssoFailureMessage(err), IP: ip, UserAgent: browser,
		})
		return LoginResult{}, err
	}
	return s.completeSSOLogin(tenant, provider, claims, audit, ip, browser, os)
}

func (s *TenantRuntimeService) completeSSOLogin(
	tenant Tenant,
	provider SSOProvider,
	claims ssoClaims,
	audit *SSOAuditService,
	ip, browser, os string,
) (LoginResult, error) {
	connection := RegisterTenantConnection(tenant)
	passport := NewPassportServiceForTenant(tenant).WithContext(s.ctx)
	passport.connection = connection

	user, err := s.findOrCreateSSOUser(tenant, provider, claims, audit)
	if err != nil {
		_ = audit.Log(ssoLogInput{
			ProviderID: provider.ID, SSOUserID: claims.Subject, SSOEmail: claims.Email,
			Status: ssoLogStatusFailure, FailureReason: ssoFailureMessage(err), IP: ip, UserAgent: browser,
		})
		return LoginResult{}, err
	}
	if user.Status == 2 {
		_ = audit.Log(ssoLogInput{
			UserID: user.ID, ProviderID: provider.ID, SSOUserID: claims.Subject, SSOEmail: claims.Email,
			Status: ssoLogStatusFailure, FailureReason: ssoFailureMessage(ErrUserDisabled), IP: ip, UserAgent: browser,
		})
		return LoginResult{}, ErrUserDisabled
	}
	if err := applySSOMappings(s.ctx, connection, user.ID, provider, claims); err != nil {
		_ = audit.Log(ssoLogInput{
			UserID: user.ID, ProviderID: provider.ID, SSOUserID: claims.Subject, SSOEmail: claims.Email,
			Status: ssoLogStatusFailure, FailureReason: ssoFailureMessage(err), IP: ip, UserAgent: browser,
		})
		return LoginResult{}, err
	}
	InvalidateCurrentUserInfoForConnection(connection, user.ID)

	accessToken, err := passport.buildToken(user.ID, tenant.ID, "access", AccessTokenTTLSeconds())
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := passport.buildToken(user.ID, tenant.ID, "refresh", RefreshTokenTTLSeconds())
	if err != nil {
		return LoginResult{}, err
	}
	binding, err := audit.UpsertBinding(ssoBindingInput{
		UserID: user.ID, ProviderID: provider.ID, SSOUserID: claims.Subject,
		SSOEmail: strings.TrimSpace(claims.Email), SSOUsername: ssoClaimPreferredUsername(claims),
		SSOAvatar: ssoClaimAvatar(claims),
	})
	if err != nil {
		_ = audit.Log(ssoLogInput{
			UserID: user.ID, ProviderID: provider.ID, SSOUserID: claims.Subject, SSOEmail: claims.Email,
			Status: ssoLogStatusFailure, FailureReason: ssoFailureMessage(err), IP: ip, UserAgent: browser,
		})
		return LoginResult{}, err
	}
	if err := audit.Log(ssoLogInput{
		UserID: user.ID, ProviderID: provider.ID, BindingID: binding.ID,
		SSOUserID: claims.Subject, SSOEmail: claims.Email, Status: ssoLogStatusSuccess,
		IP: ip, UserAgent: browser,
	}); err != nil {
		return LoginResult{}, err
	}
	_ = passport.writeLoginLog(user.Username, ip, browser, os, 1, "SSO 登录成功")

	return LoginResult{AccessToken: accessToken, RefreshToken: refreshToken, ExpireAt: AccessTokenTTLSeconds()}, nil
}

func (s *TenantRuntimeService) StartSSOAuthorization(tenant Tenant, providerName, scene string) (SSOAuthorizationResult, error) {
	provider, err := NewSSOProviderServiceForTenant(tenant).WithContext(s.ctx).EnabledProviderForScene(providerName, scene)
	if err != nil {
		return SSOAuthorizationResult{}, err
	}
	if provider.Type != "oidc" && provider.Type != "oauth2" {
		return SSOAuthorizationResult{}, ErrSSOTokenInvalid
	}
	return NewSSOAuthorizationTransactionService().Create(tenant, provider)
}

func (s *TenantRuntimeService) CompleteSSOAuthorization(
	tenant Tenant,
	transactionID, code, state, ip, browser, os string,
) (LoginResult, error) {
	if strings.TrimSpace(code) == "" {
		return LoginResult{}, ErrSSOTokenInvalid
	}
	transactionService := NewSSOAuthorizationTransactionService()
	var result LoginResult
	_, err := transactionService.VerifyAndConsumeCallback(tenant, transactionID, state, func(transaction SSOAuthorizationTransaction) error {
		provider, claims, audit, err := s.verifiedSSOAuthorization(tenant, transactionService, transaction, code, ip, browser)
		if err != nil {
			return err
		}
		result, err = s.completeSSOLogin(tenant, provider, claims, audit, ip, browser, os)
		return err
	})
	if err != nil {
		return LoginResult{}, err
	}
	transactionService.ForgetVerified(transactionID)
	return result, nil
}

func (s *TenantRuntimeService) verifiedSSOAuthorization(
	tenant Tenant,
	transactionService *SSOAuthorizationTransactionService,
	transaction SSOAuthorizationTransaction,
	code, ip, browser string,
) (SSOProvider, ssoClaims, *SSOAuditService, error) {
	if verified, ok := transactionService.LoadVerified(tenant, transaction); ok {
		provider, err := NewSSOProviderServiceForTenant(tenant).WithContext(s.ctx).EnabledProviderForScene(verified.Provider, verified.Scene)
		audit := NewSSOAuditServiceForConnection(RegisterTenantConnection(tenant)).WithContext(s.ctx)
		if err != nil || provider.ID != verified.ProviderID {
			return SSOProvider{}, ssoClaims{}, audit, ErrSSOAuthorizationTransactionInvalid
		}
		return provider, verified.Claims, audit, nil
	}
	provider, claims, audit, err := s.verifySSOAuthorizationCallback(tenant, transaction, code, ip, browser)
	if err != nil {
		return SSOProvider{}, ssoClaims{}, audit, err
	}
	err = transactionService.StoreVerified(transaction, ssoVerifiedAuthorization{
		TenantCode: tenant.Code, ProviderID: provider.ID, Provider: provider.Name, Scene: provider.Scene, Claims: claims,
	})
	return provider, claims, audit, err
}

func (s *TenantRuntimeService) verifySSOAuthorizationCallback(
	tenant Tenant,
	transaction SSOAuthorizationTransaction,
	code, ip, browser string,
) (SSOProvider, ssoClaims, *SSOAuditService, error) {
	connection := RegisterTenantConnection(tenant)
	audit := NewSSOAuditServiceForConnection(connection).WithContext(s.ctx)
	provider, err := NewSSOProviderServiceForTenant(tenant).WithContext(s.ctx).
		EnabledProviderForScene(transaction.Provider, transaction.Scene)
	if err != nil {
		return SSOProvider{}, ssoClaims{}, audit, err
	}
	if strings.TrimSpace(provider.RedirectURI) == "" || provider.RedirectURI != transaction.RedirectURI {
		return SSOProvider{}, ssoClaims{}, audit, ErrSSOAuthorizationTransactionInvalid
	}
	claims, err := verifySSOClaims(provider, SSOLoginPayload{
		Code:         code,
		CodeVerifier: transaction.CodeVerifier,
		Nonce:        transaction.Nonce,
		RedirectURI:  transaction.RedirectURI,
	})
	if err != nil {
		_ = audit.Log(ssoLogInput{
			ProviderID: provider.ID, Status: ssoLogStatusFailure,
			FailureReason: ssoFailureMessage(err), IP: ip, UserAgent: browser,
		})
		return SSOProvider{}, ssoClaims{}, audit, err
	}
	return provider, claims, audit, nil
}

func (s *TenantRuntimeService) findOrCreateSSOUser(
	tenant Tenant,
	provider SSOProvider,
	claims ssoClaims,
	audit *SSOAuditService,
) (models.User, error) {
	connection := RegisterTenantConnection(tenant)
	if audit != nil {
		user, err := audit.BoundUser(provider.ID, claims.Subject)
		if err == nil && user.ID != 0 {
			return user, nil
		}
		if err != nil && !errors.Is(err, frameworkerrors.OrmRecordNotFound) {
			return models.User{}, err
		}
	}
	query := OrmForConnectionWithContext(s.ctx, connection).Query()
	username := strings.TrimSpace(claims.Subject)
	var user models.User
	if err := query.Table(`"user"`).Where("username", username).First(&user); err == nil && user.ID != 0 {
		return user, nil
	}
	if !provider.AutoCreate {
		return models.User{}, ErrUnauthorized
	}

	username, err := s.ssoAutoCreateUsername(connection, provider, claims)
	if err != nil {
		return models.User{}, err
	}
	password, err := randomTenantPassword()
	if err != nil {
		return models.User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return models.User{}, err
	}
	nickname := strings.TrimSpace(claims.Name)
	if nickname == "" {
		nickname = username
	}
	user = models.User{
		Username: username, Password: string(hash), UserType: externalUserType,
		Nickname: nickname, Email: strings.TrimSpace(claims.Email), Status: 1,
		Dashboard: "dashboard:workbench", BackendSetting: nil,
	}
	if err := query.Create(&user); err != nil {
		return models.User{}, err
	}
	if _, err := OrmForConnectionWithContext(s.ctx, connection).Query().Exec(`UPDATE "user" SET backend_setting = '{}'::jsonb WHERE id = ?`, user.ID); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (s *TenantRuntimeService) ssoAutoCreateUsername(connection string, provider SSOProvider, claims ssoClaims) (string, error) {
	subject := strings.TrimSpace(claims.Subject)
	if subject != "" && len(subject) <= 20 {
		if available, err := s.ssoUsernameAvailable(connection, subject); err != nil || available {
			return subject, err
		}
	}

	base := normalizeSSOUsername(ssoUsernameSeed(claims))
	if base == "" {
		base = "sso_user"
	}
	if len(base) > 20 {
		base = strings.Trim(base[:20], "_")
	}
	if base == "" {
		base = "sso_user"
	}
	if available, err := s.ssoUsernameAvailable(connection, base); err != nil || available {
		return base, err
	}

	digest := ssoUsernameHash(provider, claims)
	for attempt := 0; attempt < 100; attempt++ {
		suffix := "_" + digest
		if attempt > 0 {
			suffix = fmt.Sprintf("_%s_%d", digest[:6], attempt+1)
		}
		limit := 20 - len(suffix)
		if limit < 1 {
			limit = 1
		}
		prefix := base
		if len(prefix) > limit {
			prefix = strings.Trim(prefix[:limit], "_")
		}
		if prefix == "" {
			prefix = "s"
		}
		candidate := prefix + suffix
		if available, err := s.ssoUsernameAvailable(connection, candidate); err != nil || available {
			return candidate, err
		}
	}

	return "", BusinessError{Message: "SSO 用户名生成失败"}
}

func ssoUsernameSeed(claims ssoClaims) string {
	for _, key := range []string{"preferred_username", "username"} {
		if value := strings.TrimSpace(jsonString(claims.Raw, key)); value != "" {
			return value
		}
	}
	if email := strings.TrimSpace(claims.Email); email != "" {
		if local, _, ok := strings.Cut(email, "@"); ok {
			return local
		}
		return email
	}
	if name := strings.TrimSpace(claims.Name); name != "" {
		return name
	}
	return claims.Subject
}

func normalizeSSOUsername(value string) string {
	var out strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && out.Len() > 0 {
			out.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(out.String(), "_")
}

func ssoUsernameHash(provider SSOProvider, claims ssoClaims) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", provider.ID, provider.Name, claims.Subject)))
	return hex.EncodeToString(sum[:])[:8]
}

func (s *TenantRuntimeService) ssoUsernameAvailable(connection, username string) (bool, error) {
	count, err := OrmForConnectionWithContext(s.ctx, connection).
		Query().
		Table(`"user"`).
		Where("username", username).
		Count()
	return count == 0, err
}

func EffectiveTenantBilling(tenant Tenant) models.JSONMap {
	return effectiveTenantBillingWithContext(context.Background(), tenant)
}

func ssoClaimPreferredUsername(claims ssoClaims) string {
	for _, key := range []string{"preferred_username", "username", "name"} {
		if value := strings.TrimSpace(jsonString(claims.Raw, key)); value != "" {
			return value
		}
	}
	return claims.Subject
}

func ssoClaimAvatar(claims ssoClaims) string {
	for _, key := range []string{"picture", "avatar"} {
		if value := strings.TrimSpace(jsonString(claims.Raw, key)); value != "" {
			return value
		}
	}
	return ""
}

func EffectiveTenantQuotas(tenant Tenant) models.JSONMap {
	return effectiveTenantQuotasWithContext(context.Background(), tenant)
}

func EffectiveTenantFeatures(tenant Tenant) models.JSONMap {
	return effectiveTenantFeaturesWithContext(context.Background(), tenant)
}

func (s *TenantRuntimeService) EffectiveBilling(tenant Tenant) models.JSONMap {
	return effectiveTenantBillingWithContext(s.ctx, tenant)
}

func (s *TenantRuntimeService) EffectiveQuotas(tenant Tenant) models.JSONMap {
	return effectiveTenantQuotasWithContext(s.ctx, tenant)
}

func (s *TenantRuntimeService) EffectiveFeatures(tenant Tenant) models.JSONMap {
	return effectiveTenantFeaturesWithContext(s.ctx, tenant)
}

func effectiveTenantBillingWithContext(ctx context.Context, tenant Tenant) models.JSONMap {
	plan := tenantPlanForRuntime(ctx, tenant.Plan)
	return mergeJSONMaps(plan.Billing, tenant.Billing)
}

func effectiveTenantQuotasWithContext(ctx context.Context, tenant Tenant) models.JSONMap {
	quotas := baseTenantQuotasWithContext(ctx, tenant)
	policy, ok := tenantGovernanceRuntimePolicy(ctx, tenant.ID)
	if !ok {
		return quotas
	}
	quotas = mergeJSONMaps(quotas, policy.Quotas)
	if policy.RateLimit.PerMinute > 0 {
		quotas["api_rate_per_minute"] = policy.RateLimit.PerMinute
	}
	return quotas
}

func baseTenantQuotasWithContext(ctx context.Context, tenant Tenant) models.JSONMap {
	plan := tenantPlanForRuntime(ctx, tenant.Plan)
	return mergeJSONMaps(plan.Quotas, tenant.Quotas)
}

func effectiveTenantFeaturesWithContext(ctx context.Context, tenant Tenant) models.JSONMap {
	features := baseTenantFeaturesWithContext(ctx, tenant)
	policy, ok := tenantGovernanceRuntimePolicy(ctx, tenant.ID)
	if !ok || len(policy.Modules) == 0 {
		return features
	}
	features["modules"] = mergeModuleFlags(features["modules"], policy.Modules)
	return features
}

func baseTenantFeaturesWithContext(ctx context.Context, tenant Tenant) models.JSONMap {
	plan := tenantPlanForRuntime(ctx, tenant.Plan)
	return mergeJSONMaps(plan.Features, tenant.Features)
}

func tenantGovernanceRuntimePolicy(ctx context.Context, tenantID uint64) (TenantGovernancePolicy, bool) {
	if tenantID == 0 {
		return TenantGovernancePolicy{}, false
	}
	policy, ok, err := NewTenantGovernanceService().WithContext(ctx).loadPolicy(tenantID)
	if err != nil || !ok {
		return TenantGovernancePolicy{}, false
	}
	return policy, true
}

func tenantPlanForRuntime(ctx context.Context, code string) TenantPlan {
	plan, err := NewTenantPlanService().WithContext(ctx).ActiveByCode(code)
	if err != nil {
		return TenantPlan{}
	}
	return plan
}

func mergeJSONMaps(base, override models.JSONMap) models.JSONMap {
	merged := models.JSONMap{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		nestedBase, baseOK := asJSONMap(merged[key])
		nestedOverride, overrideOK := asJSONMap(value)
		if baseOK && overrideOK {
			merged[key] = mergeJSONMaps(nestedBase, nestedOverride)
			continue
		}
		merged[key] = value
	}
	return merged
}

func mergeModuleFlags(base any, override map[string]bool) map[string]any {
	merged := map[string]any{}
	switch typed := base.(type) {
	case map[string]any:
		for key, value := range typed {
			merged[key] = value
		}
	case models.JSONMap:
		for key, value := range typed {
			merged[key] = value
		}
	case map[string]bool:
		for key, value := range typed {
			merged[key] = value
		}
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func publicFeatures(features models.JSONMap) models.JSONMap {
	out := models.JSONMap{}
	sso, ok := jsonObject(features, "sso")
	if !ok {
		out["sso"] = map[string]any{"password_login": true, "providers": []any{}}
		return out
	}
	providers := make([]any, 0)
	if rawProviders, ok := sso["providers"].([]any); ok {
		for _, item := range rawProviders {
			raw, ok := item.(map[string]any)
			if !ok || !jsonBool(raw, "enabled", true) {
				continue
			}
			providers = append(providers, map[string]any{
				"name":                   jsonString(raw, "name"),
				"display_name":           jsonString(raw, "display_name"),
				"scene":                  jsonString(raw, "scene"),
				"type":                   jsonString(raw, "type"),
				"issuer":                 jsonString(raw, "issuer"),
				"discovery_url":          jsonString(raw, "discovery_url"),
				"authorization_endpoint": jsonString(raw, "authorization_endpoint"),
				"client_id":              jsonString(raw, "client_id"),
				"scope":                  jsonString(raw, "scope"),
				"redirect_uri":           jsonString(raw, "redirect_uri"),
				"enable_pkce":            jsonBool(raw, "enable_pkce", true),
				"enable_nonce":           jsonBool(raw, "enable_nonce", true),
				"saml_entrypoint":        jsonString(raw, "saml_entrypoint"),
				"saml_entity_id":         jsonString(raw, "saml_entity_id"),
				"icon":                   jsonString(raw, "icon"),
				"button_color":           jsonString(raw, "button_color"),
				"enabled":                true,
			})
		}
	}
	out["sso"] = map[string]any{
		"password_login": jsonBool(sso, "password_login", true),
		"providers":      providers,
	}
	return out
}

func (s *TenantRuntimeService) publicSSOFeatures(tenant Tenant, scene ...string) models.JSONMap {
	selectedScene := DefaultSSOScene
	if len(scene) > 0 {
		selectedScene = scene[0]
	}
	providers, err := NewSSOProviderServiceForTenant(tenant).WithContext(s.ctx).PublicProviders(selectedScene)
	if err != nil {
		providers = []PublicSSOProvider{}
	}
	rawProviders := make([]any, 0, len(providers))
	for _, provider := range providers {
		rawProviders = append(rawProviders, map[string]any{
			"name":                   provider.Name,
			"display_name":           provider.DisplayName,
			"scene":                  provider.Scene,
			"type":                   provider.Type,
			"issuer":                 provider.Issuer,
			"discovery_url":          provider.DiscoveryURL,
			"authorization_endpoint": provider.AuthorizationEndpoint,
			"client_id":              provider.ClientID,
			"scope":                  provider.Scope,
			"redirect_uri":           provider.RedirectURI,
			"enable_pkce":            provider.EnablePKCE,
			"enable_nonce":           provider.EnableNonce,
			"saml_entrypoint":        provider.SAMLEntrypoint,
			"saml_entity_id":         provider.SAMLEntityID,
			"icon":                   provider.Icon,
			"button_color":           provider.ButtonColor,
			"enabled":                provider.Enabled,
		})
	}
	return models.JSONMap{"password_login": true, "providers": rawProviders}
}

func jsonObject(source models.JSONMap, key string) (map[string]any, bool) {
	value, ok := source[key]
	if !ok {
		return nil, false
	}
	nested, ok := asJSONMap(value)
	if !ok {
		return nil, false
	}
	return map[string]any(nested), true
}

func asJSONMap(value any) (models.JSONMap, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return models.JSONMap(typed), true
	case models.JSONMap:
		return typed, true
	default:
		return nil, false
	}
}

func TenantHostCode(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if value, _, err := net.SplitHostPort(host); err == nil {
		return strings.ToLower(value)
	}
	return strings.ToLower(host)
}

func lastForwardedHost(value string) string {
	parts := strings.Split(value, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if host := strings.TrimSpace(parts[i]); host != "" {
			return host
		}
	}
	return ""
}

func TrustedForwardedHost(header func(string, ...string) string, remoteAddr string) string {
	name := strings.TrimSpace(facades.Config().GetString("tenant.trusted_forwarded_host_header"))
	if name == "" || !trustedForwardedHostProxy(remoteAddr) {
		return ""
	}
	return lastForwardedHost(header(name, ""))
}

func trustedForwardedHostProxy(remoteAddr string) bool {
	remoteIP := remoteAddrIP(remoteAddr)
	if remoteIP == nil {
		return false
	}
	for _, candidate := range strings.Split(facades.Config().GetString("tenant.trusted_forwarded_host_proxies"), ",") {
		if trustedProxyContains(strings.TrimSpace(candidate), remoteIP) {
			return true
		}
	}
	return false
}

func trustedProxyContains(candidate string, remoteIP net.IP) bool {
	if candidate == "" {
		return false
	}
	if _, network, err := net.ParseCIDR(candidate); err == nil {
		return network.Contains(remoteIP)
	}
	return net.ParseIP(candidate).Equal(remoteIP)
}

func remoteAddrIP(remoteAddr string) net.IP {
	value := strings.TrimSpace(remoteAddr)
	if value == "" {
		return nil
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	return net.ParseIP(strings.Trim(value, "[]"))
}

func resourceQuotaKey(resource string) string {
	switch resource {
	case "users":
		return "max_users"
	case "roles":
		return "max_roles"
	case "storage":
		return "max_storage_mb"
	default:
		return resource
	}
}

func resourceUsage(usage TenantQuotaSnapshot, resource string) int64 {
	switch resource {
	case "users":
		return usage.Users
	case "roles":
		return usage.Roles
	case "storage":
		return usage.StorageMB
	default:
		return 0
	}
}

func bytesToMB(bytes int64) int64 {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 1024*1024 - 1) / (1024 * 1024)
}

func audienceMatches(value any, expected string) bool {
	switch aud := value.(type) {
	case string:
		return aud == expected
	case []any:
		for _, item := range aud {
			if fmt.Sprint(item) == expected {
				return true
			}
		}
	}
	return false
}

func jsonString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func jsonBool(values map[string]any, key string, fallback bool) bool {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	enabled, ok := value.(bool)
	if !ok {
		return fallback
	}
	return enabled
}

func jsonInt64(values map[string]any, key string) int64 {
	value, ok := values[key]
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		var parsed int64
		_, _ = fmt.Sscan(v, &parsed)
		return parsed
	default:
		return 0
	}
}
