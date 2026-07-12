package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290005CreateRoleBelongsMenuTable struct{}

func (r *M202606290005CreateRoleBelongsMenuTable) Signature() string {
	return "202606290005_create_role_belongs_menu_table"
}

func (r *M202606290005CreateRoleBelongsMenuTable) Up() error {
	if facades.Schema().HasTable("role_belongs_menu") {
		return nil
	}

	return facades.Schema().Create("role_belongs_menu", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("role_id")
		table.UnsignedBigInteger("menu_id")
		addTimestamps(table)
		table.Unique("role_id", "menu_id")
		table.Index("menu_id")
	})
}

func (r *M202606290005CreateRoleBelongsMenuTable) Down() error {
	return facades.Schema().DropIfExists("role_belongs_menu")
}
