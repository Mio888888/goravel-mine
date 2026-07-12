package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290000CreateTenantTable struct{}

func (r *M202606290000CreateTenantTable) Signature() string {
	return "202606290000_create_tenant_table"
}

func (r *M202606290000CreateTenantTable) Up() error {
	if facades.Schema().HasTable("tenant") {
		return nil
	}

	return facades.Schema().Create("tenant", func(table schema.Blueprint) {
		table.ID()
		table.String("code", 64)
		table.String("name", 100)
		table.TinyInteger("status").Default(1)
		table.String("plan", 30).Default("standard")
		table.String("db_host", 255).Default("")
		table.Integer("db_port").Default(5432)
		table.String("db_database", 128)
		table.String("db_username", 128).Default("")
		table.String("db_password", 255).Default("")
		table.String("db_schema", 64).Default("public")
		table.String("custom_domain", 255).Nullable()
		table.Jsonb("branding").Nullable()
		table.Jsonb("billing").Nullable()
		table.Jsonb("quotas").Nullable()
		table.Jsonb("features").Nullable()
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("code")
		table.Unique("custom_domain")
		table.Index("status")
	})
}

func (r *M202606290000CreateTenantTable) Down() error {
	return facades.Schema().DropIfExists("tenant")
}
