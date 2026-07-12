package seeders

type DepartmentSeeder struct{}

func (s *DepartmentSeeder) Signature() string {
	return "department_seed"
}

func (s *DepartmentSeeder) Run() error {
	if err := exec(`
		INSERT INTO department (id, name, parent_id, created_at, updated_at)
		VALUES (1, '默认部门', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			parent_id = EXCLUDED.parent_id,
			updated_at = CURRENT_TIMESTAMP
	`); err != nil {
		return err
	}

	if err := exec(`
		INSERT INTO position (id, name, dept_id, created_at, updated_at)
		VALUES (1, '默认岗位', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			dept_id = EXCLUDED.dept_id,
			updated_at = CURRENT_TIMESTAMP
	`); err != nil {
		return err
	}
	if err := syncSequence("department", "id"); err != nil {
		return err
	}

	return syncSequence("position", "id")
}
