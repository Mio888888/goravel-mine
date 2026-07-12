package migrations

import "fmt"

type M202607110006ValidateSecurityAuditPruneSchema struct{}

func (r *M202607110006ValidateSecurityAuditPruneSchema) Signature() string {
	return "202607110006_validate_security_audit_prune_schema"
}

func (r *M202607110006ValidateSecurityAuditPruneSchema) Up() error {
	dbSchema := securityAuditPruneSchema()
	checks := map[string][]string{
		"security_audit_prune_run": {
			"plan_id", "scope", "retention_days", "cutoff", "target_digest", "target_count",
			"scope_counts", "table_counts", "status", "execution_id", "heartbeat_at", "manifest_sha256",
		},
		"security_audit_prune_target": {
			"run_id", "scope", "connection", "tenant_id", "tenant_code", "database_digest",
			"table_name", "timestamp_column", "target_id", "occurred_at", "record_digest", "cutoff", "status",
		},
	}
	for table, columns := range checks {
		if !dbSchema.HasTable(table) || !dbSchema.HasColumns(table, columns) {
			return fmt.Errorf("security audit prune schema is incomplete: %s", table)
		}
	}
	return nil
}

func (r *M202607110006ValidateSecurityAuditPruneSchema) Down() error {
	return nil
}
