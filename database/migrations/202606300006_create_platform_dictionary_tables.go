package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606300006CreatePlatformDictionaryTables struct{}

func (r *M202606300006CreatePlatformDictionaryTables) Signature() string {
	return "202606300006_create_platform_dictionary_tables"
}

func (r *M202606300006CreatePlatformDictionaryTables) Up() error {
	if err := createPlatformDictTypeTable(); err != nil {
		return err
	}
	return createPlatformDictItemTable()
}

func (r *M202606300006CreatePlatformDictionaryTables) Down() error {
	return dropTables("platform_dict_item", "platform_dict_type")
}

func createPlatformDictTypeTable() error {
	if facades.Schema().HasTable("platform_dict_type") {
		return nil
	}
	return facades.Schema().Create("platform_dict_type", func(table schema.Blueprint) {
		table.ID()
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
		table.Index("status")
		table.Index("sort")
	})
}

func createPlatformDictItemTable() error {
	if facades.Schema().HasTable("platform_dict_item") {
		return nil
	}
	return facades.Schema().Create("platform_dict_item", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("type_id")
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
		table.Index("type_code")
		table.Index("status")
		table.Index("sort")
	})
}
