package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606300003CreateSSOUserBindingTable struct{}

func (r *M202606300003CreateSSOUserBindingTable) Signature() string {
	return "202606300003_create_sso_user_binding_table"
}

func (r *M202606300003CreateSSOUserBindingTable) Up() error {
	if facades.Schema().HasTable("sso_user_binding") {
		return nil
	}

	return facades.Schema().Create("sso_user_binding", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("user_id")
		table.UnsignedBigInteger("provider_id")
		table.String("sso_user_id", 255)
		table.String("sso_email", 255).Nullable()
		table.String("sso_username", 255).Nullable()
		table.String("sso_avatar", 500).Nullable()
		table.Text("access_token").Nullable()
		table.Text("refresh_token").Nullable()
		table.Timestamp("token_expires_at").Nullable()
		table.Timestamp("first_login_at").Nullable()
		table.Timestamp("last_login_at").Nullable()
		table.Integer("login_count").Default(0)
		addTimestamps(table)
		table.Unique("provider_id", "sso_user_id")
		table.Index("user_id")
		table.Index("provider_id")
		table.Index("sso_email")
	})
}

func (r *M202606300003CreateSSOUserBindingTable) Down() error {
	return facades.Schema().DropIfExists("sso_user_binding")
}
