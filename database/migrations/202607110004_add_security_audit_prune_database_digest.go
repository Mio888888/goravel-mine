package migrations

import "github.com/goravel/framework/contracts/database/schema"

type M202607110004AddSecurityAuditPruneDatabaseDigest struct{}

func (r *M202607110004AddSecurityAuditPruneDatabaseDigest) Signature() string {
	return "202607110004_add_security_audit_prune_database_digest"
}

func (r *M202607110004AddSecurityAuditPruneDatabaseDigest) Up() error {
	dbSchema := securityAuditPruneSchema()
	if !dbSchema.HasTable("security_audit_prune_target") || dbSchema.HasColumn("security_audit_prune_target", "database_digest") {
		return nil
	}
	return dbSchema.Table("security_audit_prune_target", func(table schema.Blueprint) {
		table.String("database_digest", 80).Default("").After("tenant_code")
	})
}

func (r *M202607110004AddSecurityAuditPruneDatabaseDigest) Down() error {
	return nil
}
