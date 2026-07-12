package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607050005AddSecretRotationMetadata struct{}

func (r *M202607050005AddSecretRotationMetadata) Signature() string {
	return "202607050005_add_secret_rotation_metadata"
}

func (r *M202607050005AddSecretRotationMetadata) Up() error {
	if err := addRotationColumn("sso_provider", "jwt_secret_rotated_at"); err != nil {
		return err
	}
	if err := addRotationColumn("sso_provider", "client_secret_rotated_at"); err != nil {
		return err
	}
	return addRotationColumn("storage_config", "secret_key_rotated_at")
}

func (r *M202607050005AddSecretRotationMetadata) Down() error {
	if err := dropRotationColumn("storage_config", "secret_key_rotated_at"); err != nil {
		return err
	}
	if err := dropRotationColumn("sso_provider", "client_secret_rotated_at"); err != nil {
		return err
	}
	return dropRotationColumn("sso_provider", "jwt_secret_rotated_at")
}

func addRotationColumn(tableName, column string) error {
	if !facades.Schema().HasTable(tableName) || facades.Schema().HasColumn(tableName, column) {
		return nil
	}
	return facades.Schema().Table(tableName, func(table schema.Blueprint) {
		table.Timestamp(column).Nullable()
	})
}

func dropRotationColumn(tableName, column string) error {
	if !facades.Schema().HasTable(tableName) || !facades.Schema().HasColumn(tableName, column) {
		return nil
	}
	return facades.Schema().Table(tableName, func(table schema.Blueprint) {
		table.DropColumn(column)
	})
}
