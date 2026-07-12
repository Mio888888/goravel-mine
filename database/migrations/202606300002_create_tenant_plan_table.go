package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606300002CreateTenantPlanTable struct{}

func (r *M202606300002CreateTenantPlanTable) Signature() string {
	return "202606300002_create_tenant_plan_table"
}

func (r *M202606300002CreateTenantPlanTable) Up() error {
	if facades.Schema().HasTable("tenant_plan") {
		return nil
	}

	return facades.Schema().Create("tenant_plan", func(table schema.Blueprint) {
		table.ID()
		table.String("code", 30)
		table.String("name", 100)
		table.TinyInteger("status").Default(1)
		table.Integer("sort").Default(0)
		table.Jsonb("billing").Nullable()
		table.Jsonb("quotas").Nullable()
		table.Jsonb("features").Nullable()
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("code")
		table.Index("status")
	})
}

func (r *M202606300002CreateTenantPlanTable) Down() error {
	return facades.Schema().DropIfExists("tenant_plan")
}
