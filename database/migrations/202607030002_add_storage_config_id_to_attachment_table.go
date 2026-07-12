package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607030002AddStorageConfigIDToAttachmentTable struct{}

func (r *M202607030002AddStorageConfigIDToAttachmentTable) Signature() string {
	return "202607030002_add_storage_config_id_to_attachment_table"
}

func (r *M202607030002AddStorageConfigIDToAttachmentTable) Up() error {
	if !facades.Schema().HasTable("attachment") {
		return nil
	}
	if !facades.Schema().HasColumn("attachment", "storage_config_id") {
		if err := facades.Schema().Table("attachment", func(table schema.Blueprint) {
			table.UnsignedBigInteger("storage_config_id").Default(0)
		}); err != nil {
			return err
		}
	}
	if err := sql("ALTER TABLE attachment ALTER COLUMN storage_path TYPE varchar(255)"); err != nil {
		return err
	}
	if err := sql("ALTER TABLE attachment ALTER COLUMN url TYPE varchar(512)"); err != nil {
		return err
	}
	if err := sql("DROP INDEX IF EXISTS attachment_hash_unique"); err != nil {
		return err
	}
	if err := sql("CREATE UNIQUE INDEX IF NOT EXISTS attachment_hash_storage_unique ON attachment (hash, storage_mode, storage_config_id, storage_path) WHERE hash IS NOT NULL"); err != nil {
		return err
	}
	return sql("CREATE INDEX IF NOT EXISTS attachment_storage_config_id_index ON attachment (storage_config_id)")
}

func (r *M202607030002AddStorageConfigIDToAttachmentTable) Down() error {
	if !facades.Schema().HasTable("attachment") {
		return nil
	}
	if err := sql("DROP INDEX IF EXISTS attachment_storage_config_id_index"); err != nil {
		return err
	}
	if err := sql("DROP INDEX IF EXISTS attachment_hash_storage_unique"); err != nil {
		return err
	}
	if err := sql(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM attachment
				WHERE hash IS NOT NULL
				GROUP BY hash
				HAVING COUNT(*) > 1
			) THEN
				CREATE UNIQUE INDEX IF NOT EXISTS attachment_hash_unique ON attachment (hash) WHERE hash IS NOT NULL;
			END IF;
		END $$;
	`); err != nil {
		return err
	}
	if !facades.Schema().HasColumn("attachment", "storage_config_id") {
		return nil
	}
	return facades.Schema().Table("attachment", func(table schema.Blueprint) {
		table.DropColumn("storage_config_id")
	})
}
