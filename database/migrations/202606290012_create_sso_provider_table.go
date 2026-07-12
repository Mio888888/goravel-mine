package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290012CreateSSOProviderTable struct{}

func (r *M202606290012CreateSSOProviderTable) Signature() string {
	return "202606290012_create_sso_provider_table"
}

func (r *M202606290012CreateSSOProviderTable) Up() error {
	if facades.Schema().HasTable("sso_provider") {
		return nil
	}

	return facades.Schema().Create("sso_provider", func(table schema.Blueprint) {
		table.ID()
		table.String("name", 64)
		table.String("display_name", 100).Default("")
		table.String("scene", 50).Default("admin")
		table.String("type", 20).Default("oidc")
		table.Boolean("enabled").Default(true)
		table.String("issuer", 500).Default("")
		table.String("audience", 255).Default("")
		table.String("jwt_secret", 500).Default("")
		table.Timestamp("jwt_secret_rotated_at").Nullable()
		table.String("discovery_url", 500).Default("")
		table.String("authorization_endpoint", 500).Default("")
		table.String("token_endpoint", 500).Default("")
		table.String("userinfo_endpoint", 500).Default("")
		table.String("jwks_uri", 500).Default("")
		table.Text("jwks_json")
		table.String("client_id", 255).Default("")
		table.String("client_secret", 500).Default("")
		table.Timestamp("client_secret_rotated_at").Nullable()
		table.String("scope", 500).Default("openid profile email")
		table.String("redirect_uri", 500).Default("")
		table.Boolean("enable_pkce").Default(true)
		table.Boolean("enable_nonce").Default(true)
		table.Boolean("auto_create").Default(false)
		table.String("icon", 255).Default("")
		table.String("button_color", 20).Default("")
		table.Integer("display_order").Default(0)
		table.String("saml_entrypoint", 500).Default("")
		table.String("saml_entity_id", 255).Default("")
		table.String("saml_certificate", 4000).Default("")
		table.Jsonb("role_mapping").Nullable()
		table.Jsonb("data_permission_mapping").Nullable()
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("name", "scene")
		table.Index("scene")
		table.Index("enabled")
		table.Index("display_order")
	})
}

func (r *M202606290012CreateSSOProviderTable) Down() error {
	return facades.Schema().DropIfExists("sso_provider")
}
