package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607020001CreateTenantPermissionAuditTable struct{}

func (r *M202607020001CreateTenantPermissionAuditTable) Signature() string {
	return "202607020001_create_tenant_permission_audit_table"
}

func (r *M202607020001CreateTenantPermissionAuditTable) Up() error {
	if facades.Schema().HasTable("tenant_permission_audit") {
		return nil
	}
	return facades.Schema().Create("tenant_permission_audit", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("tenant_id")
		table.String("tenant_code", 64)
		table.String("operation", 40)
		table.String("source", 40)
		table.Jsonb("before_snapshot").Nullable()
		table.Jsonb("after_snapshot").Nullable()
		table.Jsonb("diff").Nullable()
		table.UnsignedBigInteger("operator_id").Default(0)
		table.String("operator_name", 100).Default("")
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Index("tenant_id")
		table.Index("tenant_code")
		table.Index("operation")
		table.Index("source")
		table.Index("created_at")
	})
}

func (r *M202607020001CreateTenantPermissionAuditTable) Down() error {
	return facades.Schema().DropIfExists("tenant_permission_audit")
}
