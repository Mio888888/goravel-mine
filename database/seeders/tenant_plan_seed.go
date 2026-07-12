package seeders

import (
	"encoding/json"
	"sort"
	"strings"
)

type TenantPlanSeeder struct{}

func (s *TenantPlanSeeder) Signature() string {
	return "tenant_plan_seed"
}

func (s *TenantPlanSeeder) Run() error {
	features, err := tenantPlanFullPermissionsFeatureJSON()
	if err != nil {
		return err
	}

	if err := exec(`
		INSERT INTO tenant_plan (
			id, code, name, status, sort, billing, quotas, features,
			created_at, updated_at, remark
		)
		VALUES
			(
				1, 'standard', '标准版', 1, 10,
				'{"subscription_status":"active","currency":"CNY"}'::jsonb,
				'{"api_rate_per_minute":600,"max_users":0,"max_roles":0,"max_storage_mb":0}'::jsonb,
				?::jsonb,
				CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ''
			),
			(
				2, 'enterprise', '企业版', 1, 20,
				'{"subscription_status":"active","currency":"CNY"}'::jsonb,
				'{"api_rate_per_minute":1200,"max_users":200,"max_roles":50,"max_storage_mb":102400}'::jsonb,
				?::jsonb,
				CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ''
			),
			(
				3, 'vip', '旗舰版', 1, 30,
				'{"subscription_status":"active","currency":"CNY"}'::jsonb,
				'{"api_rate_per_minute":3000,"max_users":0,"max_roles":0,"max_storage_mb":0}'::jsonb,
				?::jsonb,
				CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ''
			)
		ON CONFLICT (code) DO UPDATE SET
			name = EXCLUDED.name,
			status = EXCLUDED.status,
			sort = EXCLUDED.sort,
			billing = EXCLUDED.billing,
			quotas = EXCLUDED.quotas,
			features = EXCLUDED.features,
			remark = EXCLUDED.remark,
			updated_at = CURRENT_TIMESTAMP
	`, features, features, features); err != nil {
		return err
	}

	return syncSequence("tenant_plan", "id")
}

func tenantPlanFullPermissionsFeatureJSON() (string, error) {
	payload := map[string]any{
		"permissions": map[string]any{
			"allowed": tenantPlanFullPermissionNames(),
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func tenantPlanFullPermissionNames() []string {
	seen := make(map[string]struct{})
	for _, seed := range TenantMenuCatalogSeeds() {
		name := strings.TrimSpace(seed.Name)
		if name == "" || strings.HasPrefix(name, "platform:") {
			continue
		}
		seen[name] = struct{}{}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
