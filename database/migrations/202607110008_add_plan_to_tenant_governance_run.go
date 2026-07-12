package migrations

import "goravel/app/facades"

type M202607110008AddPlanToTenantGovernanceRun struct{}

func (r *M202607110008AddPlanToTenantGovernanceRun) Signature() string {
	return "202607110008_add_plan_to_tenant_governance_run"
}

func (r *M202607110008AddPlanToTenantGovernanceRun) Up() error {
	dbSchema := facades.Schema().Connection(platformMigrationConnection())
	if !dbSchema.HasColumn("tenant_governance_run", "plan_id") {
		if err := dbSchema.Sql("ALTER TABLE tenant_governance_run ADD COLUMN plan_id VARCHAR(64) NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	return dbSchema.Sql("CREATE INDEX IF NOT EXISTS tenant_governance_run_plan_id_index ON tenant_governance_run (plan_id)")
}

func (r *M202607110008AddPlanToTenantGovernanceRun) Down() error {
	dbSchema := facades.Schema().Connection(platformMigrationConnection())
	if !dbSchema.HasColumn("tenant_governance_run", "plan_id") {
		return nil
	}
	return dbSchema.Sql("ALTER TABLE tenant_governance_run DROP COLUMN plan_id")
}
