package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290002CreateUserTable struct{}

func (r *M202606290002CreateUserTable) Signature() string {
	return "202606290002_create_user_table"
}

func (r *M202606290002CreateUserTable) Up() error {
	if facades.Schema().HasTable("user") {
		return nil
	}

	return facades.Schema().Create("user", func(table schema.Blueprint) {
		table.ID()
		table.String("username", 20).Default("")
		table.String("password", 100)
		table.String("user_type", 3).Default("100")
		table.String("nickname", 30).Default("")
		table.String("phone", 11).Default("")
		table.String("email", 50).Default("")
		table.String("avatar", 255).Default("")
		table.String("signed", 255).Default("")
		table.String("dashboard", 100).Default("")
		table.TinyInteger("status").Default(1)
		table.String("login_ip", 45).Default("127.0.0.1")
		table.Timestamp("login_time").UseCurrent()
		table.Jsonb("backend_setting").Nullable()
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("username")
	})
}

func (r *M202606290002CreateUserTable) Down() error {
	return facades.Schema().DropIfExists("user")
}
