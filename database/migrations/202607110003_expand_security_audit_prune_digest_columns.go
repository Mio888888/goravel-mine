package migrations

import "github.com/goravel/framework/contracts/database/schema"

type M202607110003ExpandSecurityAuditPruneDigestColumns struct{}

func (r *M202607110003ExpandSecurityAuditPruneDigestColumns) Signature() string {
	return "202607110003_expand_security_audit_prune_digest_columns"
}

func (r *M202607110003ExpandSecurityAuditPruneDigestColumns) Up() error {
	dbSchema := securityAuditPruneSchema()
	if !dbSchema.HasTable("security_audit_prune_run") {
		return nil
	}

	if err := dbSchema.Table("security_audit_prune_run", func(table schema.Blueprint) {
		table.String("target_digest", 80).Change()
		table.String("manifest_sha256", 80).Default("").Change()
	}); err != nil {
		return err
	}
	if !dbSchema.HasTable("security_audit_prune_target") || dbSchema.HasColumn("security_audit_prune_target", "record_digest") {
		return nil
	}
	return dbSchema.Table("security_audit_prune_target", func(table schema.Blueprint) {
		table.String("record_digest", 80).Default("")
	})
}

func (r *M202607110003ExpandSecurityAuditPruneDigestColumns) Down() error {
	return nil
}
