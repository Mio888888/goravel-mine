package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290009CreateUserLoginLogTable struct{}

func (r *M202606290009CreateUserLoginLogTable) Signature() string {
	return "202606290009_create_user_login_log_table"
}

func (r *M202606290009CreateUserLoginLogTable) Up() error {
	if facades.Schema().HasTable("user_login_log") {
		return nil
	}

	return facades.Schema().Create("user_login_log", func(table schema.Blueprint) {
		table.ID()
		table.String("username", 20)
		table.String("ip", 45).Nullable()
		table.String("os", 255).Nullable()
		table.String("browser", 255).Nullable()
		table.SmallInteger("status").Default(1)
		table.String("message", 50).Nullable()
		table.DateTime("login_time")
		table.String("remark", 255).Nullable()
		table.Index("username")
	})
}

func (r *M202606290009CreateUserLoginLogTable) Down() error {
	return facades.Schema().DropIfExists("user_login_log")
}
