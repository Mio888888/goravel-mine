package seeders

type PlatformCasbinSeeder struct{}

func (s *PlatformCasbinSeeder) Signature() string {
	return "platform_casbin_seed"
}

func (s *PlatformCasbinSeeder) Run() error {
	if err := exec(`
		INSERT INTO platform_casbin_rule (ptype, v0, v1, created_at, updated_at)
		SELECT 'g', 'user:1', 'role:PlatformSuperAdmin', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		WHERE NOT EXISTS (
			SELECT 1 FROM platform_casbin_rule
			WHERE ptype = 'g' AND v0 = 'user:1' AND v1 = 'role:PlatformSuperAdmin'
		)
	`); err != nil {
		return err
	}

	if err := exec(`
		INSERT INTO platform_role_belongs_menu (role_id, menu_id, created_at, updated_at)
		SELECT 1, id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM platform_menu
		ON CONFLICT (role_id, menu_id) DO NOTHING
	`); err != nil {
		return err
	}

	return exec(`
		INSERT INTO platform_casbin_rule (ptype, v0, v1, v2, created_at, updated_at)
		SELECT 'p', 'role:PlatformSuperAdmin', name, '*', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM platform_menu
		WHERE NOT EXISTS (
			SELECT 1 FROM platform_casbin_rule
			WHERE ptype = 'p' AND v0 = 'role:PlatformSuperAdmin' AND v1 = platform_menu.name AND v2 = '*'
		)
	`)
}
