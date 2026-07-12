package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290004CreateMenuTable struct{}

func (r *M202606290004CreateMenuTable) Signature() string {
	return "202606290004_create_menu_table"
}

func (r *M202606290004CreateMenuTable) Up() error {
	if facades.Schema().HasTable("menu") {
		return nil
	}

	return facades.Schema().Create("menu", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("parent_id").Default(0)
		table.String("name", 50).Default("")
		table.Jsonb("meta").Nullable()
		table.String("path", 60).Default("")
		table.String("component", 150).Default("")
		table.String("redirect", 100).Default("")
		table.TinyInteger("status").Default(1)
		table.SmallInteger("sort").Default(0)
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 60).Default("")
		table.Unique("name")
		table.Index("parent_id")
	})
}

func (r *M202606290004CreateMenuTable) Down() error {
	return facades.Schema().DropIfExists("menu")
}
