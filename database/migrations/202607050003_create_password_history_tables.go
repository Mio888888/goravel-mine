package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607050003CreateUserPasswordHistoryTable struct{}

func (r *M202607050003CreateUserPasswordHistoryTable) Signature() string {
	return "202607050003_create_user_password_history_table"
}

func (r *M202607050003CreateUserPasswordHistoryTable) Up() error {
	return createPasswordHistoryTable("user_password_history")
}

func (r *M202607050003CreateUserPasswordHistoryTable) Down() error {
	return facades.Schema().DropIfExists("user_password_history")
}

type M202607050004CreatePlatformUserPasswordHistoryTable struct{}

func (r *M202607050004CreatePlatformUserPasswordHistoryTable) Signature() string {
	return "202607050004_create_platform_user_password_history_table"
}

func (r *M202607050004CreatePlatformUserPasswordHistoryTable) Up() error {
	return createPasswordHistoryTable("platform_user_password_history")
}

func (r *M202607050004CreatePlatformUserPasswordHistoryTable) Down() error {
	return facades.Schema().DropIfExists("platform_user_password_history")
}

func createPasswordHistoryTable(name string) error {
	if facades.Schema().HasTable(name) {
		return nil
	}
	return facades.Schema().Create(name, func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("user_id")
		table.String("password", 100)
		addTimestamps(table)
		table.Index("user_id")
		table.Index("created_at")
	})
}
