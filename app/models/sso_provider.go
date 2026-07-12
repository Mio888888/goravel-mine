package models

import "time"

type SSOProvider struct {
	ID                    uint64    `gorm:"column:id;primaryKey" json:"id"`
	Name                  string    `gorm:"column:name" json:"name"`
	DisplayName           string    `gorm:"column:display_name" json:"display_name"`
	Scene                 string    `gorm:"column:scene" json:"scene"`
	Type                  string    `gorm:"column:type" json:"type"`
	Enabled               bool      `gorm:"column:enabled" json:"enabled"`
	Issuer                string    `gorm:"column:issuer" json:"issuer"`
	Audience              string    `gorm:"column:audience" json:"audience"`
	JWTSecret             string    `gorm:"column:jwt_secret" json:"-"`
	JWTSecretRotatedAt    time.Time `gorm:"column:jwt_secret_rotated_at" json:"jwt_secret_rotated_at"`
	DiscoveryURL          string    `gorm:"column:discovery_url" json:"discovery_url"`
	AuthorizationEndpoint string    `gorm:"column:authorization_endpoint" json:"authorization_endpoint"`
	TokenEndpoint         string    `gorm:"column:token_endpoint" json:"token_endpoint"`
	UserinfoEndpoint      string    `gorm:"column:userinfo_endpoint" json:"userinfo_endpoint"`
	JWKSURI               string    `gorm:"column:jwks_uri" json:"jwks_uri"`
	JWKSJSON              string    `gorm:"column:jwks_json" json:"jwks_json"`
	ClientID              string    `gorm:"column:client_id" json:"client_id"`
	ClientSecret          string    `gorm:"column:client_secret" json:"-"`
	ClientSecretRotatedAt time.Time `gorm:"column:client_secret_rotated_at" json:"client_secret_rotated_at"`
	Scope                 string    `gorm:"column:scope" json:"scope"`
	RedirectURI           string    `gorm:"column:redirect_uri" json:"redirect_uri"`
	EnablePKCE            bool      `gorm:"column:enable_pkce" json:"enable_pkce"`
	EnableNonce           bool      `gorm:"column:enable_nonce" json:"enable_nonce"`
	AutoCreate            bool      `gorm:"column:auto_create" json:"auto_create"`
	Icon                  string    `gorm:"column:icon" json:"icon"`
	ButtonColor           string    `gorm:"column:button_color" json:"button_color"`
	DisplayOrder          int       `gorm:"column:display_order" json:"display_order"`
	SAMLEntrypoint        string    `gorm:"column:saml_entrypoint" json:"saml_entrypoint"`
	SAMLEntityID          string    `gorm:"column:saml_entity_id" json:"saml_entity_id"`
	SAMLCertificate       string    `gorm:"column:saml_certificate" json:"saml_certificate"`
	RoleMapping           JSONMap   `gorm:"column:role_mapping;type:jsonb" json:"role_mapping"`
	DataPermissionMapping JSONMap   `gorm:"column:data_permission_mapping;type:jsonb" json:"data_permission_mapping"`
	AuditColumns
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (SSOProvider) TableName() string {
	return "sso_provider"
}
