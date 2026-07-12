package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606300001AddBillingToTenantTable struct{}

func (r *M202606300001AddBillingToTenantTable) Signature() string {
	return "202606300001_add_billing_to_tenant_table"
}

func (r *M202606300001AddBillingToTenantTable) Up() error {
	if !facades.Schema().HasTable("tenant") || facades.Schema().HasColumn("tenant", "billing") {
		return nil
	}

	return facades.Schema().Table("tenant", func(table schema.Blueprint) {
		table.Jsonb("billing").Nullable()
	})
}

func (r *M202606300001AddBillingToTenantTable) Down() error {
	if !facades.Schema().HasTable("tenant") || !facades.Schema().HasColumn("tenant", "billing") {
		return nil
	}

	return facades.Schema().Table("tenant", func(table schema.Blueprint) {
		table.DropColumn("billing")
	})
}
