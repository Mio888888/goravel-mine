package seeders

type AdminSeeder struct{}

func (s *AdminSeeder) Signature() string {
	return "admin_seed"
}

func (s *AdminSeeder) Run() error {
	if err := exec(`
		INSERT INTO "user" (
			id, username, password, user_type, nickname, email, phone, signed, dashboard,
			status, login_ip, login_time, backend_setting, created_by, updated_by,
			created_at, updated_at, remark
		)
		VALUES (
			1, 'admin', '$2a$10$/mc6xDxW3q3aJfzZBVBXT.a9GEWkm5p2griG8xDcNjKJL9OhLlToe',
			'100', '创始人', 'admin@adminmine.com', '16858888988', '广阔天地，大有所为',
			'dashboard:workbench', 1, '127.0.0.1', CURRENT_TIMESTAMP, '{}'::jsonb,
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
		INSERT INTO role (id, name, code, status, sort, created_by, updated_by, created_at, updated_at, remark)
		VALUES (1, '超级管理员', 'SuperAdmin', 1, 0, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
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
		INSERT INTO user_belongs_role (user_id, role_id, created_at, updated_at)
		VALUES (1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, role_id) DO NOTHING
	`); err != nil {
		return err
	}
	if err := syncSequence(`"user"`, "id"); err != nil {
		return err
	}

	return syncSequence("role", "id")
}
