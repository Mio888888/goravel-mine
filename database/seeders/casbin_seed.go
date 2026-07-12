package seeders

type CasbinSeeder struct{}

func (s *CasbinSeeder) Signature() string {
	return "casbin_seed"
}

func (s *CasbinSeeder) Run() error {
	if err := exec(`
		INSERT INTO casbin_rule (ptype, v0, v1, created_at, updated_at)
		VALUES ('g', 'user:1', 'role:SuperAdmin', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		return err
	}

	if err := exec(`
		INSERT INTO role_belongs_menu (role_id, menu_id, created_at, updated_at)
		SELECT 1, id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM menu
		ON CONFLICT (role_id, menu_id) DO NOTHING
	`); err != nil {
		return err
	}

	return exec(`
		INSERT INTO casbin_rule (ptype, v0, v1, v2, created_at, updated_at)
		SELECT 'p', 'role:SuperAdmin', name, '*', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM menu
		WHERE NOT EXISTS (
			SELECT 1 FROM casbin_rule
			WHERE ptype = 'p' AND v0 = 'role:SuperAdmin' AND v1 = menu.name AND v2 = '*'
		)
	`)
}
