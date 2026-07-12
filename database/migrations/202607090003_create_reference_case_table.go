package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607090003CreateReferenceCaseTable struct{}

func (r *M202607090003CreateReferenceCaseTable) Signature() string {
	return "202607090003_create_reference_case_table"
}

func (r *M202607090003CreateReferenceCaseTable) Up() error {
	if facades.Schema().HasTable("reference_case") {
		return nil
	}

	return facades.Schema().Create("reference_case", func(table schema.Blueprint) {
		table.ID()
		table.String("code", 80)
		table.String("title", 160)
		table.TinyInteger("status").Default(1)
		table.String("version", 40).Default("1.0.0")
		table.Jsonb("payload").Nullable()
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("code")
		table.Index("status")
		table.Index("version")
	})
}

func (r *M202607090003CreateReferenceCaseTable) Down() error {
	return facades.Schema().DropIfExists("reference_case")
}
