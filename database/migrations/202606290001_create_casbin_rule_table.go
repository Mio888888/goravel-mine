package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290001CreateCasbinRuleTable struct{}

func (r *M202606290001CreateCasbinRuleTable) Signature() string {
	return "202606290001_create_casbin_rule_table"
}

func (r *M202606290001CreateCasbinRuleTable) Up() error {
	if facades.Schema().HasTable("casbin_rule") {
		return nil
	}

	return facades.Schema().Create("casbin_rule", func(table schema.Blueprint) {
		table.ID()
		table.String("ptype").Nullable()
		table.String("v0").Nullable()
		table.String("v1").Nullable()
		table.String("v2").Nullable()
		table.String("v3").Nullable()
		table.String("v4").Nullable()
		table.String("v5").Nullable()
		addTimestamps(table)
		table.Index("ptype", "v0")
	})
}

func (r *M202606290001CreateCasbinRuleTable) Down() error {
	return facades.Schema().DropIfExists("casbin_rule")
}
