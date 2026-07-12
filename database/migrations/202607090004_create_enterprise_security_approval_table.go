package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"
)

type M202607090004CreateEnterpriseSecurityApprovalTable struct{}

func (r *M202607090004CreateEnterpriseSecurityApprovalTable) Signature() string {
	return "202607090004_create_enterprise_security_approval_table"
}

func (r *M202607090004CreateEnterpriseSecurityApprovalTable) Up() error {
	if sensitiveApprovalSchema().HasTable("enterprise_security_approval") {
		return addEnterpriseSecurityApprovalColumns()
	}
	return sensitiveApprovalSchema().Create("enterprise_security_approval", func(table schema.Blueprint) {
		table.ID()
		table.String("approval_id", 120)
		table.UnsignedBigInteger("requester_id").Default(0)
		table.UnsignedBigInteger("approver_id").Default(0)
		table.UnsignedBigInteger("tenant_id").Default(0)
		table.String("scope", 120)
		table.String("resource", 220).Default("")
		table.String("status", 30).Default("pending")
		table.String("reason", 255).Default("")
		table.Jsonb("before_snapshot").Nullable()
		table.Jsonb("after_snapshot").Nullable()
		table.Timestamp("used_at").Nullable()
		table.Timestamp("expires_at").Nullable()
		addTimestamps(table)
		table.Unique("approval_id")
		table.Index("requester_id")
		table.Index("approver_id")
		table.Index("tenant_id")
		table.Index("scope")
		table.Index("resource")
		table.Index("status")
		table.Index("used_at")
		table.Index("expires_at")
	})
}

func addEnterpriseSecurityApprovalColumns() error {
	dbSchema := sensitiveApprovalSchema()
	if err := backfillEnterpriseSecurityApprovalID(dbSchema); err != nil {
		return err
	}
	columns := []struct {
		name  string
		apply func(schema.Blueprint)
	}{
		{"requester_id", func(table schema.Blueprint) { table.UnsignedBigInteger("requester_id").Default(0) }},
		{"approver_id", func(table schema.Blueprint) { table.UnsignedBigInteger("approver_id").Default(0) }},
		{"tenant_id", func(table schema.Blueprint) { table.UnsignedBigInteger("tenant_id").Default(0) }},
		{"scope", func(table schema.Blueprint) { table.String("scope", 120).Default("") }},
		{"resource", func(table schema.Blueprint) { table.String("resource", 220).Default("") }},
		{"status", func(table schema.Blueprint) { table.String("status", 30).Default("pending") }},
		{"reason", func(table schema.Blueprint) { table.String("reason", 255).Default("") }},
		{"before_snapshot", func(table schema.Blueprint) { table.Jsonb("before_snapshot").Nullable() }},
		{"after_snapshot", func(table schema.Blueprint) { table.Jsonb("after_snapshot").Nullable() }},
		{"used_at", func(table schema.Blueprint) { table.Timestamp("used_at").Nullable() }},
		{"expires_at", func(table schema.Blueprint) { table.Timestamp("expires_at").Nullable() }},
		{"created_at", func(table schema.Blueprint) { table.Timestamp("created_at").Nullable() }},
		{"updated_at", func(table schema.Blueprint) { table.Timestamp("updated_at").Nullable() }},
	}
	for _, column := range columns {
		if dbSchema.HasColumn("enterprise_security_approval", column.name) {
			continue
		}
		if err := dbSchema.Table("enterprise_security_approval", column.apply); err != nil {
			return err
		}
	}
	indexes := []struct {
		name  string
		apply func(schema.Blueprint)
	}{
		{"enterprise_security_approval_approval_id_unique", func(table schema.Blueprint) { table.Unique("approval_id") }},
		{"enterprise_security_approval_requester_id_index", func(table schema.Blueprint) { table.Index("requester_id") }},
		{"enterprise_security_approval_approver_id_index", func(table schema.Blueprint) { table.Index("approver_id") }},
		{"enterprise_security_approval_tenant_id_index", func(table schema.Blueprint) { table.Index("tenant_id") }},
		{"enterprise_security_approval_scope_index", func(table schema.Blueprint) { table.Index("scope") }},
		{"enterprise_security_approval_resource_index", func(table schema.Blueprint) { table.Index("resource") }},
		{"enterprise_security_approval_status_index", func(table schema.Blueprint) { table.Index("status") }},
		{"enterprise_security_approval_used_at_index", func(table schema.Blueprint) { table.Index("used_at") }},
		{"enterprise_security_approval_expires_at_index", func(table schema.Blueprint) { table.Index("expires_at") }},
	}
	for _, index := range indexes {
		if dbSchema.HasIndex("enterprise_security_approval", index.name) {
			continue
		}
		if err := dbSchema.Table("enterprise_security_approval", index.apply); err != nil {
			return err
		}
	}
	return nil
}

func backfillEnterpriseSecurityApprovalID(dbSchema schema.Schema) error {
	if !dbSchema.HasColumn("enterprise_security_approval", "approval_id") {
		return dbSchema.Sql(`
			ALTER TABLE enterprise_security_approval ADD COLUMN approval_id VARCHAR(120);
			UPDATE enterprise_security_approval SET approval_id = CONCAT('legacy:', id::text);
			ALTER TABLE enterprise_security_approval ALTER COLUMN approval_id SET NOT NULL;
		`)
	}
	return dbSchema.Sql(`
		UPDATE enterprise_security_approval
		SET approval_id = CONCAT('legacy:', id::text)
		WHERE approval_id IS NULL OR BTRIM(approval_id) = '';
		ALTER TABLE enterprise_security_approval ALTER COLUMN approval_id DROP DEFAULT;
		ALTER TABLE enterprise_security_approval ALTER COLUMN approval_id SET NOT NULL;
	`)
}

func (r *M202607090004CreateEnterpriseSecurityApprovalTable) Down() error {
	return sensitiveApprovalSchema().DropIfExists("enterprise_security_approval")
}
