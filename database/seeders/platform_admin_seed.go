package seeders

type PlatformAdminSeeder struct{}

func (s *PlatformAdminSeeder) Signature() string {
	return "platform_admin_seed"
}

func (s *PlatformAdminSeeder) Run() error {
	if err := exec(`
		INSERT INTO platform_user (
			id, username, password, user_type, nickname, email, phone, signed, dashboard,
			status, login_ip, login_time, backend_setting, created_by, updated_by,
			created_at, updated_at, remark
		)
		VALUES (
			1, 'admin', '$2a$10$/mc6xDxW3q3aJfzZBVBXT.a9GEWkm5p2griG8xDcNjKJL9OhLlToe',
			'900', '平台管理员', 'platform@adminmine.com', '16858888988', '平台全局管理',
			'platform:tenant', 1, '127.0.0.1', CURRENT_TIMESTAMP, '{}'::jsonb,
			0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ''
		)
		ON CONFLICT (id) DO UPDATE SET
			username = EXCLUDED.username,
			password = EXCLUDED.password,
			user_type = EXCLUDED.user_type,
			nickname = EXCLUDED.nickname,
			email = EXCLUDED.email,
			phone = EXCLUDED.phone,
			signed = EXCLUDED.signed,
			dashboard = EXCLUDED.dashboard,
			status = EXCLUDED.status,
			updated_at = CURRENT_TIMESTAMP
	`); err != nil {
		return err
	}

	if err := exec(`
		INSERT INTO platform_role (id, name, code, status, sort, created_by, updated_by, created_at, updated_at, remark)
		VALUES (1, '平台超级管理员', 'PlatformSuperAdmin', 1, 0, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			code = EXCLUDED.code,
			status = EXCLUDED.status,
			sort = EXCLUDED.sort,
			updated_at = CURRENT_TIMESTAMP
	`); err != nil {
		return err
	}

	if err := exec(`
		INSERT INTO platform_user_belongs_role (user_id, role_id, created_at, updated_at)
		VALUES (1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, role_id) DO NOTHING
	`); err != nil {
		return err
	}
	if err := syncSequence("platform_user", "id"); err != nil {
		return err
	}

	return syncSequence("platform_role", "id")
}
