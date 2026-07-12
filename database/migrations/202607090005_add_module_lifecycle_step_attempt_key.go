package migrations

import "goravel/app/facades"

type M202607090005AddModuleLifecycleStepAttemptKey struct{}

func (r *M202607090005AddModuleLifecycleStepAttemptKey) Signature() string {
	return "202607090005_add_module_lifecycle_step_attempt_key"
}

func (r *M202607090005AddModuleLifecycleStepAttemptKey) Up() error {
	dbSchema := facades.Schema()
	if !dbSchema.HasTable("module_lifecycle_step") {
		return nil
	}
	return dbSchema.Sql(`
		ALTER TABLE module_lifecycle_step
			ADD COLUMN IF NOT EXISTS attempt_key VARCHAR(64);

		ALTER TABLE module_lifecycle_step
			DROP CONSTRAINT IF EXISTS module_lifecycle_step_run_key_step_name_unique;
		DROP INDEX IF EXISTS module_lifecycle_step_run_key_step_name_unique;

		UPDATE module_lifecycle_step
		SET attempt_key = CONCAT('legacy:', id::text)
		WHERE attempt_key IS NULL OR BTRIM(attempt_key) = '';

		ALTER TABLE module_lifecycle_step
			ALTER COLUMN attempt_key TYPE VARCHAR(64),
			ALTER COLUMN attempt_key SET NOT NULL;

		CREATE UNIQUE INDEX IF NOT EXISTS module_lifecycle_step_attempt_key_unique
			ON module_lifecycle_step (attempt_key);
	`)
}

func (r *M202607090005AddModuleLifecycleStepAttemptKey) Down() error {
	dbSchema := facades.Schema()
	if !dbSchema.HasTable("module_lifecycle_step") || !dbSchema.HasColumn("module_lifecycle_step", "attempt_key") {
		return nil
	}
	return dbSchema.Sql(`
		WITH ranked_attempts AS (
			SELECT id, ROW_NUMBER() OVER (
				PARTITION BY run_key, step_name
				ORDER BY id DESC
			) AS attempt_rank
			FROM module_lifecycle_step
		)
		DELETE FROM module_lifecycle_step
		WHERE id IN (
			SELECT id FROM ranked_attempts WHERE attempt_rank > 1
		);

		ALTER TABLE module_lifecycle_step
			DROP CONSTRAINT IF EXISTS module_lifecycle_step_attempt_key_unique;
		DROP INDEX IF EXISTS module_lifecycle_step_attempt_key_unique;
		ALTER TABLE module_lifecycle_step DROP COLUMN attempt_key;
		ALTER TABLE module_lifecycle_step
			ADD CONSTRAINT module_lifecycle_step_run_key_step_name_unique UNIQUE (run_key, step_name);
	`)
}
