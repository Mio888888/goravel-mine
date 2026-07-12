package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607110001AddSensitiveBindingToEnterpriseSecurityApproval struct{}

func (r *M202607110001AddSensitiveBindingToEnterpriseSecurityApproval) Signature() string {
	return "202607110001_add_sensitive_binding_to_enterprise_security_approval"
}

func (r *M202607110001AddSensitiveBindingToEnterpriseSecurityApproval) Up() error {
	dbSchema := sensitiveApprovalSchema()
	if !dbSchema.HasTable("enterprise_security_approval") {
		return nil
	}
	return dbSchema.Sql(`
		ALTER TABLE enterprise_security_approval
			ADD COLUMN IF NOT EXISTS tenant_id BIGINT NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS policy_key VARCHAR(120) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS binding_digest VARCHAR(64) NOT NULL DEFAULT '';

		CREATE INDEX IF NOT EXISTS enterprise_security_approval_policy_key_index
			ON enterprise_security_approval (policy_key);
		CREATE INDEX IF NOT EXISTS enterprise_security_approval_tenant_id_index
			ON enterprise_security_approval (tenant_id);
		CREATE INDEX IF NOT EXISTS enterprise_security_approval_binding_digest_index
			ON enterprise_security_approval (binding_digest);
	`)
}

func (r *M202607110001AddSensitiveBindingToEnterpriseSecurityApproval) Down() error {
	dbSchema := sensitiveApprovalSchema()
	if !dbSchema.HasTable("enterprise_security_approval") {
		return nil
	}
	return dbSchema.Sql(`
		DROP INDEX IF EXISTS enterprise_security_approval_binding_digest_index;
		DROP INDEX IF EXISTS enterprise_security_approval_tenant_id_index;
		DROP INDEX IF EXISTS enterprise_security_approval_policy_key_index;
		ALTER TABLE enterprise_security_approval
			DROP COLUMN IF EXISTS binding_digest,
			DROP COLUMN IF EXISTS policy_key,
			DROP COLUMN IF EXISTS tenant_id;
	`)
}

func sensitiveApprovalSchema() schema.Schema {
	connection := facades.Config().GetString("tenant.platform_connection")
	if connection == "" {
		connection = facades.Config().GetString("database.default")
	}
	return facades.Schema().Connection(connection)
}
