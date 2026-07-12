package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607050001CreateUserMFATable struct{}

func (r *M202607050001CreateUserMFATable) Signature() string {
	return "202607050001_create_user_mfa_table"
}

func (r *M202607050001CreateUserMFATable) Up() error {
	return createUserMFATable("user_mfa")
}

func (r *M202607050001CreateUserMFATable) Down() error {
	return facades.Schema().DropIfExists("user_mfa")
}

type M202607050002CreatePlatformUserMFATable struct{}

func (r *M202607050002CreatePlatformUserMFATable) Signature() string {
	return "202607050002_create_platform_user_mfa_table"
}

func (r *M202607050002CreatePlatformUserMFATable) Up() error {
	return createUserMFATable("platform_user_mfa")
}

func (r *M202607050002CreatePlatformUserMFATable) Down() error {
	return facades.Schema().DropIfExists("platform_user_mfa")
}

func createUserMFATable(name string) error {
	if facades.Schema().HasTable(name) {
		return nil
	}
	return facades.Schema().Create(name, func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("user_id")
		table.Text("secret").Default("")
		table.Boolean("enabled").Default(false)
		table.Jsonb("recovery_codes").Nullable()
		table.Timestamp("confirmed_at").Nullable()
		table.Timestamp("last_used_at").Nullable()
		addTimestamps(table)
		table.Unique("user_id")
		table.Index("enabled")
	})
}
