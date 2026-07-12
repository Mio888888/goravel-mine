package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

func addAuditColumns(table schema.Blueprint) {
	table.UnsignedBigInteger("created_by").Default(0)
	table.UnsignedBigInteger("updated_by").Default(0)
}

func addTimestamps(table schema.Blueprint) {
	table.Timestamp("created_at").Nullable()
	table.Timestamp("updated_at").Nullable()
}

func addSoftDeletes(table schema.Blueprint) {
	table.Timestamp("deleted_at").Nullable()
}

func dropTables(names ...string) error {
	for _, name := range names {
		if err := facades.Schema().DropIfExists(name); err != nil {
			return err
		}
	}

	return nil
}

func sql(statement string) error {
	return facades.Schema().Sql(statement)
}
