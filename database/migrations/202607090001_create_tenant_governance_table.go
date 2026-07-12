package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607090001CreateTenantGovernanceTable struct{}

func (r *M202607090001CreateTenantGovernanceTable) Signature() string {
	return "202607090001_create_tenant_governance_table"
}

func (r *M202607090001CreateTenantGovernanceTable) Up() error {
	if facades.Schema().HasTable("tenant_governance") {
		return nil
	}
	return facades.Schema().Create("tenant_governance", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("tenant_id")
		table.String("tenant_code", 64)
		table.Jsonb("modules").Nullable()
		table.Jsonb("quotas").Nullable()
		table.Jsonb("rate_limit").Nullable()
		table.Jsonb("retention").Nullable()
		table.Jsonb("data_export").Nullable()
		table.Jsonb("data_deletion").Nullable()
		table.Jsonb("isolation_proof").Nullable()
		addTimestamps(table)
		table.Unique("tenant_id")
		table.Index("tenant_code")
	})
}

func (r *M202607090001CreateTenantGovernanceTable) Down() error {
	return facades.Schema().DropIfExists("tenant_governance")
}
