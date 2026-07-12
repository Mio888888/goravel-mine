package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606300004CreateSSOLoginLogTable struct{}

func (r *M202606300004CreateSSOLoginLogTable) Signature() string {
	return "202606300004_create_sso_login_log_table"
}

func (r *M202606300004CreateSSOLoginLogTable) Up() error {
	if facades.Schema().HasTable("sso_login_log") {
		return nil
	}

	return facades.Schema().Create("sso_login_log", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("user_id").Nullable()
		table.UnsignedBigInteger("provider_id")
		table.UnsignedBigInteger("binding_id").Nullable()
		table.String("sso_user_id", 255).Nullable()
		table.String("sso_email", 255).Nullable()
		table.SmallInteger("status").Default(1)
		table.Text("failure_reason").Nullable()
		table.String("ip", 45).Nullable()
		table.String("user_agent", 500).Nullable()
		table.String("device_type", 50).Nullable()
		table.Timestamp("login_at").Nullable()
		table.Index("user_id")
		table.Index("provider_id")
		table.Index("status")
		table.Index("login_at")
	})
}

func (r *M202606300004CreateSSOLoginLogTable) Down() error {
	return facades.Schema().DropIfExists("sso_login_log")
}
