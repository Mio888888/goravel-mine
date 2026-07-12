package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290003CreateRoleTable struct{}

func (r *M202606290003CreateRoleTable) Signature() string {
	return "202606290003_create_role_table"
}

func (r *M202606290003CreateRoleTable) Up() error {
	if facades.Schema().HasTable("role") {
		return nil
	}

	return facades.Schema().Create("role", func(table schema.Blueprint) {
		table.ID()
		table.String("name", 30)
		table.String("code", 100)
		table.TinyInteger("status").Default(1)
		table.SmallInteger("sort").Default(0)
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("code")
	})
}

func (r *M202606290003CreateRoleTable) Down() error {
	return facades.Schema().DropIfExists("role")
}
