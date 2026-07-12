package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290006CreateUserBelongsRoleTable struct{}

func (r *M202606290006CreateUserBelongsRoleTable) Signature() string {
	return "202606290006_create_user_belongs_role_table"
}

func (r *M202606290006CreateUserBelongsRoleTable) Up() error {
	if facades.Schema().HasTable("user_belongs_role") {
		return nil
	}

	return facades.Schema().Create("user_belongs_role", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("user_id")
		table.UnsignedBigInteger("role_id")
		addTimestamps(table)
		table.Unique("user_id", "role_id")
		table.Index("role_id")
	})
}

func (r *M202606290006CreateUserBelongsRoleTable) Down() error {
	return facades.Schema().DropIfExists("user_belongs_role")
}
