package seeders

import "goravel/app/facades"

type TenantSeeder struct{}

func (s *TenantSeeder) Signature() string {
	return "tenant_seed"
}

func (s *TenantSeeder) Run() error {
	if err := exec(`
		INSERT INTO tenant (
			id, code, name, status, plan, db_host, db_port, db_database,
			db_username, db_password, db_schema, billing, quotas, features, branding,
			created_at, updated_at, remark
		)
		VALUES (
			1, ?, '默认租户', 1, 'standard',
			?, ?, ?, ?, ?, ?,
			'{"subscription_status":"active"}'::jsonb,
			'{"api_rate_per_minute":0,"max_users":0,"max_roles":0,"max_storage_mb":0}'::jsonb,
			'{}'::jsonb,
			'{}'::jsonb,
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ''
		)
		ON CONFLICT (id) DO UPDATE SET
			code = EXCLUDED.code,
			name = EXCLUDED.name,
			status = EXCLUDED.status,
			db_host = EXCLUDED.db_host,
			db_port = EXCLUDED.db_port,
			db_database = EXCLUDED.db_database,
			db_username = EXCLUDED.db_username,
			db_password = EXCLUDED.db_password,
			db_schema = EXCLUDED.db_schema,
			billing = COALESCE(tenant.billing, EXCLUDED.billing),
			quotas = COALESCE(tenant.quotas, EXCLUDED.quotas),
			features = COALESCE(tenant.features, EXCLUDED.features),
			branding = COALESCE(tenant.branding, EXCLUDED.branding),
			updated_at = CURRENT_TIMESTAMP
	`, facades.Config().GetString("tenant.default", "default"),
		facades.Config().GetString("database.connections.postgres.host"),
		facades.Config().GetInt("database.connections.postgres.port"),
		facades.Config().GetString("database.connections.postgres.database"),
		facades.Config().GetString("database.connections.postgres.username"),
		facades.Config().GetString("database.connections.postgres.password"),
		facades.Config().GetString("database.connections.postgres.schema", "public")); err != nil {
		return err
	}

	return syncSequence("tenant", "id")
}
