package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607030001CreateStorageConfigTable struct{}

func (r *M202607030001CreateStorageConfigTable) Signature() string {
	return "202607030001_create_storage_config_table"
}

func (r *M202607030001CreateStorageConfigTable) Up() error {
	if facades.Schema().HasTable("storage_config") {
		return nil
	}

	if err := facades.Schema().Create("storage_config", func(table schema.Blueprint) {
		table.ID()
		table.String("name", 100)
		table.String("provider", 30)
		table.String("driver", 30)
		table.String("bucket", 100).Nullable()
		table.String("endpoint", 255).Nullable()
		table.String("region", 80).Nullable()
		table.String("access_key", 255).Nullable()
		table.String("secret_key", 255).Nullable()
		table.Timestamp("secret_key_rotated_at").Nullable()
		table.String("base_url", 255).Nullable()
		table.String("path_prefix", 120).Default("uploads")
		table.Boolean("is_default").Default(false)
		table.TinyInteger("status").Default(1)
		table.Jsonb("options").Nullable()
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Index("provider")
		table.Index("status")
	}); err != nil {
		return err
	}

	return sql("CREATE UNIQUE INDEX IF NOT EXISTS storage_config_default_unique ON storage_config (is_default) WHERE is_default = true AND status = 1")
}

func (r *M202607030001CreateStorageConfigTable) Down() error {
	if err := sql("DROP INDEX IF EXISTS storage_config_default_unique"); err != nil {
		return err
	}

	return facades.Schema().DropIfExists("storage_config")
}
