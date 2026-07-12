package migrations

import "github.com/goravel/framework/contracts/database/schema"

type M202607110005AddSecurityAuditPruneExecutionLease struct{}

func (r *M202607110005AddSecurityAuditPruneExecutionLease) Signature() string {
	return "202607110005_add_security_audit_prune_execution_lease"
}

func (r *M202607110005AddSecurityAuditPruneExecutionLease) Up() error {
	dbSchema := securityAuditPruneSchema()
	if !dbSchema.HasTable("security_audit_prune_run") {
		return nil
	}
	return dbSchema.Table("security_audit_prune_run", func(table schema.Blueprint) {
		if !dbSchema.HasColumn("security_audit_prune_run", "execution_id") {
			table.String("execution_id", 64).Default("")
		}
		if !dbSchema.HasColumn("security_audit_prune_run", "heartbeat_at") {
			table.Timestamp("heartbeat_at").Nullable()
		}
	})
}

func (r *M202607110005AddSecurityAuditPruneExecutionLease) Down() error {
	return nil
}
