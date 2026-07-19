package bootstrap

import (
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"

	"goravel/app/moduleboot"
	"goravel/database/migrations"
)

func Migrations() []schema.Migration {
	items := []schema.Migration{
		&migrations.M20210101000001CreateJobsTable{},
		&migrations.M202606290000CreateTenantTable{},
		&migrations.M202606290001CreateCasbinRuleTable{},
		&migrations.M202606290002CreateUserTable{},
		&migrations.M202606290003CreateRoleTable{},
		&migrations.M202606290004CreateMenuTable{},
		&migrations.M202606290005CreateRoleBelongsMenuTable{},
		&migrations.M202606290006CreateUserBelongsRoleTable{},
		&migrations.M202606290007CreateDepartmentTables{},
		&migrations.M202606290008CreateAttachmentTable{},
		&migrations.M202606290009CreateUserLoginLogTable{},
		&migrations.M202606290010CreateUserOperationLogTable{},
		&migrations.M202606290012CreateSSOProviderTable{},
		&migrations.M202606290011CreatePlatformRBACTables{},
		&migrations.M202606300001AddBillingToTenantTable{},
		&migrations.M202606300002CreateTenantPlanTable{},
		&migrations.M202606300003CreateSSOUserBindingTable{},
		&migrations.M202606300004CreateSSOLoginLogTable{},
		&migrations.M202606300005CreateDictionaryTables{},
		&migrations.M202606300006CreatePlatformDictionaryTables{},
		&migrations.M202607020001CreateTenantPermissionAuditTable{},
		&migrations.M202607030001CreateStorageConfigTable{},
		&migrations.M202607030002AddStorageConfigIDToAttachmentTable{},
		&migrations.M202607050001CreateUserMFATable{},
		&migrations.M202607050002CreatePlatformUserMFATable{},
		&migrations.M202607050003CreateUserPasswordHistoryTable{},
		&migrations.M202607050004CreatePlatformUserPasswordHistoryTable{},
		&migrations.M202607050005AddSecretRotationMetadata{},
		&migrations.M202607080001CreateQueueReliabilityTables{},
		&migrations.M202607080002CreateModuleLifecycleTables{},
		&migrations.M202607090001CreateTenantGovernanceTable{},
		&migrations.M202607090002CreateModuleLifecycleStepTable{},
		&migrations.M202607090004CreateEnterpriseSecurityApprovalTable{},
		&migrations.M202607090005AddModuleLifecycleStepAttemptKey{},
		&migrations.M202607110001AddSensitiveBindingToEnterpriseSecurityApproval{},
		&migrations.M202607110002CreateSecurityAuditPruneTables{},
		&migrations.M202607110003ExpandSecurityAuditPruneDigestColumns{},
		&migrations.M202607110004AddSecurityAuditPruneDatabaseDigest{},
		&migrations.M202607110005AddSecurityAuditPruneExecutionLease{},
		&migrations.M202607110006ValidateSecurityAuditPruneSchema{},
		&migrations.M202607110007CreateTenantGovernanceExecutionTables{},
		&migrations.M202607110008AddPlanToTenantGovernanceRun{},
		&migrations.M202607110009EnforceTenantDatabaseIsolation{},
		&migrations.M202607190001DropQueueTaskLockTable{},
		&migrations.M202607190003ExtendQueueMessageOutbox{},
	}

	return moduleboot.Modules().MergeMigrationsAfter(items, "202607030002_add_storage_config_id_to_attachment_table")
}

func ModuleSeeders() []seeder.Seeder {
	return moduleboot.Modules().Seeders()
}
