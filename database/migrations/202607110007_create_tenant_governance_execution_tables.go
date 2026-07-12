package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607110007CreateTenantGovernanceExecutionTables struct{}

func (r *M202607110007CreateTenantGovernanceExecutionTables) Signature() string {
	return "202607110007_create_tenant_governance_execution_tables"
}

func (r *M202607110007CreateTenantGovernanceExecutionTables) Up() error {
	dbSchema := facades.Schema().Connection(platformMigrationConnection())
	if !dbSchema.HasTable("tenant_governance_run") {
		if err := dbSchema.Create("tenant_governance_run", func(table schema.Blueprint) {
			table.ID()
			table.UnsignedBigInteger("tenant_id")
			table.String("tenant_code", 64)
			table.String("kind", 40)
			table.String("idempotency_key", 255)
			table.String("policy_version", 80).Default("")
			table.String("plan_id", 64).Default("")
			table.String("status", 30).Default("pending")
			table.Timestamp("started_at").Nullable()
			table.Timestamp("finished_at").Nullable()
			table.LongText("error").Nullable()
			addTimestamps(table)
			table.Unique("tenant_id", "kind", "idempotency_key")
			table.Index("status")
			table.Index("tenant_id", "kind")
			table.Index("plan_id")
		}); err != nil {
			return err
		}
	}
	if dbSchema.HasTable("tenant_governance_evidence") {
		return nil
	}
	return dbSchema.Create("tenant_governance_evidence", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("run_id")
		table.UnsignedBigInteger("tenant_id")
		table.String("kind", 40)
		table.String("uri", 2048)
		table.String("object_version", 512)
		table.String("sha256", 80)
		table.Timestamp("verified_at")
		table.Timestamp("expires_at")
		table.Jsonb("metadata").Nullable()
		table.Timestamp("stale_at").Nullable()
		addTimestamps(table)
		table.Unique("run_id")
		table.Index("tenant_id", "kind")
		table.Index("expires_at")
		table.Index("stale_at")
	})
}

func (r *M202607110007CreateTenantGovernanceExecutionTables) Down() error {
	dbSchema := facades.Schema().Connection(platformMigrationConnection())
	if err := dbSchema.DropIfExists("tenant_governance_evidence"); err != nil {
		return err
	}
	return dbSchema.DropIfExists("tenant_governance_run")
}

func platformMigrationConnection() string {
	connection := facades.Config().GetString("tenant.platform_connection")
	if connection == "" {
		return facades.Config().GetString("database.default")
	}
	return connection
}
