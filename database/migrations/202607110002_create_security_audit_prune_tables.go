package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607110002CreateSecurityAuditPruneTables struct{}

func (r *M202607110002CreateSecurityAuditPruneTables) Signature() string {
	return "202607110002_create_security_audit_prune_tables"
}

func (r *M202607110002CreateSecurityAuditPruneTables) Up() error {
	if err := createSecurityAuditPruneRunTable(); err != nil {
		return err
	}
	return createSecurityAuditPruneTargetTable()
}

func (r *M202607110002CreateSecurityAuditPruneTables) Down() error {
	dbSchema := securityAuditPruneSchema()
	if err := dbSchema.DropIfExists("security_audit_prune_target"); err != nil {
		return err
	}
	return dbSchema.DropIfExists("security_audit_prune_run")
}

func createSecurityAuditPruneRunTable() error {
	dbSchema := securityAuditPruneSchema()
	if dbSchema.HasTable("security_audit_prune_run") {
		return nil
	}
	return dbSchema.Create("security_audit_prune_run", func(table schema.Blueprint) {
		table.ID()
		table.String("plan_id", 64)
		table.String("scope", 120)
		table.Integer("retention_days")
		table.Timestamp("cutoff")
		table.String("target_digest", 80)
		table.BigInteger("target_count").Default(0)
		table.Jsonb("scope_counts").Nullable()
		table.Jsonb("table_counts").Nullable()
		table.Timestamp("min_timestamp").Nullable()
		table.Timestamp("max_timestamp").Nullable()
		table.String("status", 30).Default("planned")
		table.String("execution_id", 64).Default("")
		table.Timestamp("heartbeat_at").Nullable()
		table.String("archive_uri", 2048).Default("")
		table.String("object_version", 512).Default("")
		table.String("manifest_sha256", 80).Default("")
		table.Timestamp("immutable_until").Nullable()
		table.Timestamp("proof_verified_at").Nullable()
		table.Timestamp("started_at").Nullable()
		table.Timestamp("finished_at").Nullable()
		table.LongText("error").Nullable()
		addTimestamps(table)
		table.Unique("plan_id")
		table.Index("status")
		table.Index("scope")
		table.Index("target_digest")
	})
}

func createSecurityAuditPruneTargetTable() error {
	dbSchema := securityAuditPruneSchema()
	if dbSchema.HasTable("security_audit_prune_target") {
		return nil
	}
	return dbSchema.Create("security_audit_prune_target", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("run_id")
		table.String("scope", 120)
		table.String("connection", 120)
		table.UnsignedBigInteger("tenant_id").Default(0)
		table.String("tenant_code", 64).Default("")
		table.String("database_digest", 80).Default("")
		table.String("table_name", 120)
		table.String("timestamp_column", 120)
		table.UnsignedBigInteger("target_id")
		table.Timestamp("occurred_at")
		table.String("record_digest", 80)
		table.Timestamp("cutoff")
		table.Integer("retention_days")
		table.String("status", 30).Default("planned")
		table.Timestamp("processed_at").Nullable()
		table.LongText("error").Nullable()
		addTimestamps(table)
		table.Unique("run_id", "scope", "table_name", "target_id")
		table.Index("run_id")
		table.Index("status")
		table.Index("tenant_id")
		table.Index("table_name")
	})
}

func securityAuditPruneSchema() schema.Schema {
	connection := facades.Config().GetString("tenant.platform_connection")
	if connection == "" {
		connection = facades.Config().GetString("database.default")
	}
	return facades.Schema().Connection(connection)
}
