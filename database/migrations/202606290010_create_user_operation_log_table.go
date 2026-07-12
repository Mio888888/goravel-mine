package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290010CreateUserOperationLogTable struct{}

func (r *M202606290010CreateUserOperationLogTable) Signature() string {
	return "202606290010_create_user_operation_log_table"
}

func (r *M202606290010CreateUserOperationLogTable) Up() error {
	if facades.Schema().HasTable("user_operation_log") {
		return nil
	}

	return facades.Schema().Create("user_operation_log", func(table schema.Blueprint) {
		table.ID()
		table.String("username", 20)
		table.String("method", 20)
		table.String("router", 500)
		table.String("service_name", 30)
		table.String("ip", 45).Nullable()
		addTimestamps(table)
		table.String("remark", 255).Nullable()
		table.Index("username")
	})
}

func (r *M202606290010CreateUserOperationLogTable) Down() error {
	return facades.Schema().DropIfExists("user_operation_log")
}
