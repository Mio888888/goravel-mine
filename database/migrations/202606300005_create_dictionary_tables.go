package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606300005CreateDictionaryTables struct{}

func (r *M202606300005CreateDictionaryTables) Signature() string {
	return "202606300005_create_dictionary_tables"
}

func (r *M202606300005CreateDictionaryTables) Up() error {
	if err := createDictTypeTable(); err != nil {
		return err
	}
	return createDictItemTable()
}

func (r *M202606300005CreateDictionaryTables) Down() error {
	return dropTables("dict_item", "dict_type")
}

func createDictTypeTable() error {
	if facades.Schema().HasTable("dict_type") {
		return nil
	}
	return facades.Schema().Create("dict_type", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("source_id").Default(0)
		table.String("source_code", 64).Default("")
		table.String("code", 64)
		table.String("name", 100)
		table.TinyInteger("status").Default(1)
		table.Integer("sort").Default(0)
		table.Integer("version").Default(1)
		table.Boolean("is_system").Default(true)
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("code")
		table.Index("source_id")
		table.Index("source_code")
		table.Index("status")
		table.Index("sort")
	})
}

func createDictItemTable() error {
	if facades.Schema().HasTable("dict_item") {
		return nil
	}
	return facades.Schema().Create("dict_item", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("type_id")
		table.UnsignedBigInteger("source_id").Default(0)
		table.String("source_code", 100).Default("")
		table.String("type_code", 64)
		table.String("label", 100)
		table.String("value", 100)
		table.String("i18n", 150).Default("")
		table.String("color", 30).Default("")
		table.TinyInteger("status").Default(1)
		table.Integer("sort").Default(0)
		table.Integer("version").Default(1)
		table.Boolean("is_system").Default(true)
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("type_code", "value")
		table.Index("type_id")
		table.Index("source_id")
		table.Index("source_code")
		table.Index("type_code")
		table.Index("status")
		table.Index("sort")
	})
}
