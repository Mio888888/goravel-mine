package services

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
)

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
