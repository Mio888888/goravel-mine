package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290008CreateAttachmentTable struct{}

func (r *M202606290008CreateAttachmentTable) Signature() string {
	return "202606290008_create_attachment_table"
}

func (r *M202606290008CreateAttachmentTable) Up() error {
	if facades.Schema().HasTable("attachment") {
		return nil
	}

	if err := facades.Schema().Create("attachment", func(table schema.Blueprint) {
		table.ID()
		table.String("storage_mode", 20).Default("local")
		table.UnsignedBigInteger("storage_config_id").Default(0)
		table.String("origin_name", 255).Nullable()
		table.String("object_name", 50).Nullable()
		table.String("hash", 64).Nullable()
		table.String("mime_type", 255).Nullable()
		table.String("storage_path", 255).Nullable()
		table.String("suffix", 20).Nullable()
		table.BigInteger("size_byte").Nullable()
		table.String("size_info", 50).Nullable()
		table.String("url", 512).Nullable()
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Index("storage_config_id")
		table.Index("storage_path")
	}); err != nil {
		return err
	}

	return sql("CREATE UNIQUE INDEX IF NOT EXISTS attachment_hash_storage_unique ON attachment (hash, storage_mode, storage_config_id, storage_path) WHERE hash IS NOT NULL")
}

func (r *M202606290008CreateAttachmentTable) Down() error {
	if err := sql("DROP INDEX IF EXISTS attachment_hash_storage_unique"); err != nil {
		return err
	}
	if err := sql("DROP INDEX IF EXISTS attachment_hash_unique"); err != nil {
		return err
	}

	return facades.Schema().DropIfExists("attachment")
}
