package unit

import (
	"testing"

	"github.com/goravel/framework/contracts/database/schema"
	"github.com/stretchr/testify/require"

	"goravel/app/facades"
	"goravel/app/services"
)

func TestTenantConnectionNameIsStableAndSafe(t *testing.T) {
	tenant := services.Tenant{ID: 42, Code: "Acme-Corp_01"}

	require.Equal(t, "tenant_42_acme_corp_01", services.TenantConnectionName(tenant))
}

func TestTenantConnectionNameCannotCollideAfterCodeNormalization(t *testing.T) {
	first := services.Tenant{ID: 41, Code: "Acme Corp"}
	second := services.Tenant{ID: 42, Code: "Acme-Corp"}

	require.NotEqual(t, services.TenantConnectionName(first), services.TenantConnectionName(second))
}

func TestTenantDatabaseConfigUsesTenantCredentials(t *testing.T) {
	tenant := services.Tenant{
		DBHost:     "10.0.0.2",
		DBPort:     15432,
		DBDatabase: "tenant_acme",
		DBUsername: "tenant_user",
		DBPassword: "secret",
		DBSchema:   "custom",
	}

	config := services.TenantDatabaseConfig(tenant)

	require.Equal(t, "10.0.0.2", config["host"])
	require.Equal(t, 15432, config["port"])
	require.Equal(t, "tenant_acme", config["database"])
	require.Equal(t, "tenant_user", config["username"])
	require.Equal(t, "secret", config["password"])
	require.Equal(t, "custom", config["schema"])
	require.Equal(t, "disable", config["sslmode"])
	require.False(t, config["singular"].(bool))
}

func TestTenantDatabaseConfigDefaultsSchemaAndPort(t *testing.T) {
	tenant := services.Tenant{DBHost: "127.0.0.1", DBDatabase: "tenant_default"}

	config := services.TenantDatabaseConfig(tenant)

	require.Equal(t, 5432, config["port"])
	require.Equal(t, "public", config["schema"])
}

func TestTenantBusinessMigrationsExcludePlatformTenantTable(t *testing.T) {
	signatures := make([]string, 0)
	for _, migration := range services.TenantBusinessMigrations() {
		signatures = append(signatures, migration.Signature())
	}

	require.NotContains(t, signatures, "202606290000_create_tenant_table")
	require.Contains(t, signatures, "202606290002_create_user_table")
	require.Contains(t, signatures, "202606290003_create_role_table")
	require.Contains(t, signatures, "202606290010_create_user_operation_log_table")
	require.Contains(t, signatures, "202606300003_create_sso_user_binding_table")
	require.Contains(t, signatures, "202606300004_create_sso_login_log_table")
}

func TestTenantBusinessMigrationsIncludeModuleProvider(t *testing.T) {
	services.SetTenantModuleMigrationsProvider(func() []schema.Migration {
		return []schema.Migration{tenantModuleMigration{signature: "202607090010_create_alpha_item_table"}}
	})
	t.Cleanup(func() {
		services.SetTenantModuleMigrationsProvider(nil)
	})

	signatures := make([]string, 0)
	for _, migration := range services.TenantBusinessMigrations() {
		signatures = append(signatures, migration.Signature())
	}

	require.Contains(t, signatures, "202607090010_create_alpha_item_table")
}

type tenantModuleMigration struct {
	signature string
}

func (m tenantModuleMigration) Signature() string {
	return m.signature
}

func (m tenantModuleMigration) Up() error {
	return nil
}

func (m tenantModuleMigration) Down() error {
	return nil
}

func TestTenantCreatePayloadBuildsTenantWithDatabaseSettings(t *testing.T) {
	payload := services.TenantPayload{
		Code:       "vip",
		Name:       "VIP 客户",
		Plan:       "enterprise",
		DBHost:     "10.0.0.8",
		DBPort:     15432,
		DBDatabase: "vip_db",
		DBUsername: "vip_user",
		DBPassword: "vip_secret",
		DBSchema:   "tenant",
	}

	tenant := payload.Tenant()

	require.Equal(t, "vip", tenant.Code)
	require.Equal(t, "VIP 客户", tenant.Name)
	require.Equal(t, services.TenantStatusActive, tenant.Status)
	require.Equal(t, "enterprise", tenant.Plan)
	require.Equal(t, "10.0.0.8", tenant.DBHost)
	require.Equal(t, 15432, tenant.DBPort)
	require.Equal(t, "vip_db", tenant.DBDatabase)
	require.Equal(t, "vip_user", tenant.DBUsername)
	require.Equal(t, "vip_secret", tenant.DBPassword)
	require.Equal(t, "tenant", tenant.DBSchema)
}

func TestTenantCreatePayloadDefaultsDatabaseSettingsFromBasicInfo(t *testing.T) {
	payload := services.TenantPayload{
		Code: "Acme Corp",
		Name: "Acme 租户",
	}

	tenant := payload.Tenant()

	require.Equal(t, "Acme Corp", tenant.Code)
	require.Equal(t, "Acme 租户", tenant.Name)
	require.Equal(t, services.TenantStatusActive, tenant.Status)
	require.Equal(t, "standard", tenant.Plan)
	require.Equal(t, 5432, tenant.DBPort)
	require.Equal(t, "tenant_acme_corp", tenant.DBDatabase)
	require.Equal(t, "public", tenant.DBSchema)
}

func TestTrustedForwardedHostRequiresTrustedProxy(t *testing.T) {
	originalHeader := facades.Config().GetString("tenant.trusted_forwarded_host_header")
	originalProxies := facades.Config().GetString("tenant.trusted_forwarded_host_proxies")
	t.Cleanup(func() {
		facades.Config().Add("tenant.trusted_forwarded_host_header", originalHeader)
		facades.Config().Add("tenant.trusted_forwarded_host_proxies", originalProxies)
	})
	facades.Config().Add("tenant.trusted_forwarded_host_header", "X-Forwarded-Host")
	facades.Config().Add("tenant.trusted_forwarded_host_proxies", "127.0.0.1,10.0.0.0/24")
	header := func(name string, defaults ...string) string {
		require.Equal(t, "X-Forwarded-Host", name)
		return "client.example.test, tenant.example.test:2888"
	}

	require.Equal(t, "tenant.example.test:2888", services.TrustedForwardedHost(header, "127.0.0.1:53800"))
	require.Equal(t, "tenant.example.test:2888", services.TrustedForwardedHost(header, "10.0.0.42:53800"))
	require.Empty(t, services.TrustedForwardedHost(header, "203.0.113.10:53800"))
}

func TestPostgresProvisionPlanQuotesIdentifiersAndLiterals(t *testing.T) {
	tenant := services.Tenant{
		DBDatabase: `tenant"db`,
		DBUsername: `tenant"user`,
		DBPassword: `pa'ss`,
		DBSchema:   `tenant"schema`,
	}

	plan, err := services.NewPostgresProvisionPlan(tenant)

	require.NoError(t, err)
	require.Equal(t, []string{
		"DO $$\nBEGIN\n\tIF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tenant\"user') THEN\n\t\tCREATE ROLE \"tenant\"\"user\" LOGIN PASSWORD 'pa''ss';\n\tELSE\n\t\tALTER ROLE \"tenant\"\"user\" WITH LOGIN PASSWORD 'pa''ss';\n\tEND IF;\nEND $$",
		"CREATE DATABASE \"tenant\"\"db\" OWNER \"tenant\"\"user\"",
		"REVOKE CONNECT ON DATABASE \"tenant\"\"db\" FROM PUBLIC",
		"GRANT ALL PRIVILEGES ON DATABASE \"tenant\"\"db\" TO \"tenant\"\"user\"",
	}, plan.PlatformStatements)
	require.Equal(t, []string{
		"CREATE SCHEMA IF NOT EXISTS \"tenant\"\"schema\" AUTHORIZATION \"tenant\"\"user\"",
		"GRANT USAGE, CREATE ON SCHEMA \"tenant\"\"schema\" TO \"tenant\"\"user\"",
		"GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA \"tenant\"\"schema\" TO \"tenant\"\"user\"",
		"GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA \"tenant\"\"schema\" TO \"tenant\"\"user\"",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA \"tenant\"\"schema\" GRANT ALL PRIVILEGES ON TABLES TO \"tenant\"\"user\"",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA \"tenant\"\"schema\" GRANT ALL PRIVILEGES ON SEQUENCES TO \"tenant\"\"user\"",
	}, plan.TenantStatements)
}

func TestPostgresProvisionDefaultsFillMissingCredentials(t *testing.T) {
	tenant := services.Tenant{Code: "Acme Corp", DBDatabase: "tenant_acme"}

	prepared, err := services.ApplyPostgresProvisionDefaults(tenant)

	require.NoError(t, err)
	require.Equal(t, "tenant_acme_corp", prepared.DBUsername)
	require.NotEmpty(t, prepared.DBPassword)
	require.Len(t, prepared.DBPassword, 32)
}
