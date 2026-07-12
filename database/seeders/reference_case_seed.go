package seeders

type ReferenceCaseSeeder struct{}

func (s *ReferenceCaseSeeder) Signature() string {
	return "reference_case_seed"
}

func (s *ReferenceCaseSeeder) Run() error {
	return exec(`
		INSERT INTO reference_case (
			code, title, status, version, payload, created_at, updated_at, remark
		)
		VALUES (
			'golden-case', 'Golden Reference Case', 1, '1.0.0',
			'{"scenario":"baseline","upgrade":"reference-case:upgrade","rollback":"reference-case:rollback"}'::jsonb,
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'golden reference module baseline'
		)
		ON CONFLICT (code) DO UPDATE SET
			title = EXCLUDED.title,
			status = EXCLUDED.status,
			payload = EXCLUDED.payload,
			remark = EXCLUDED.remark,
			updated_at = CURRENT_TIMESTAMP
	`)
}
