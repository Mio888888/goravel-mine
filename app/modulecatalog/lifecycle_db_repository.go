package modulecatalog

import (
	"context"
	"strings"
	"time"
)

type lifecycleRunRow struct {
	IdempotencyKey string `gorm:"column:idempotency_key"`
	Status         string `gorm:"column:status"`
}

func (s *DBLifecycleStore) SuccessfulRunExists(ctx context.Context, key string) (bool, error) {
	ctx = contextOrBackground(ctx)
	var row lifecycleRunRow
	err := s.orm(ctx).Query().Table("module_lifecycle_run").Where("idempotency_key", key).First(&row)
	if err != nil {
		return false, err
	}
	if row.IdempotencyKey == "" {
		return false, nil
	}
	return lifecycleRunBlocksAutomaticRetry(row.Status), nil
}

func lifecycleRunBlocksAutomaticRetry(status string) bool {
	return status == LifecycleStatusSucceeded || status == LifecycleStatusReconciliationRequired
}

func (s *DBLifecycleStore) BeginRun(ctx context.Context, run LifecycleRunRecord) error {
	ctx = contextOrBackground(ctx)
	now := firstLifecycleTime(run.started, s.now())
	query := s.orm(ctx).Query()
	result, err := query.Table("module_lifecycle_run").
		Where("idempotency_key", run.IdempotencyKey).
		Where("status <> ?", LifecycleStatusSucceeded).
		Update(map[string]any{
			"status":      LifecycleStatusRunning,
			"owner":       run.Owner,
			"reason":      run.Reason,
			"command":     run.Command,
			"plan":        nullableJSON(run.Plan),
			"error":       nil,
			"started_at":  now,
			"finished_at": nil,
			"updated_at":  now,
		})
	if err != nil {
		return err
	}
	if result.RowsAffected == 1 {
		return nil
	}
	return query.Table("module_lifecycle_run").Create(map[string]any{
		"idempotency_key": run.IdempotencyKey,
		"module_id":       run.ModuleID,
		"action":          run.Action,
		"from_version":    run.FromVersion,
		"to_version":      run.ToVersion,
		"status":          LifecycleStatusRunning,
		"dry_run":         run.DryRun,
		"owner":           run.Owner,
		"reason":          run.Reason,
		"command":         run.Command,
		"plan":            nullableJSON(run.Plan),
		"started_at":      now,
		"created_at":      now,
		"updated_at":      now,
	})
}

func (s *DBLifecycleStore) CompleteRun(ctx context.Context, completion LifecycleRunCompletion) error {
	ctx = contextOrBackground(ctx)
	now := firstLifecycleTime(completion.FinishedAt, s.now())
	_, err := s.orm(ctx).Query().
		Table("module_lifecycle_run").
		Where("idempotency_key", completion.IdempotencyKey).
		Update(map[string]any{
			"status":      completion.Status,
			"error":       nullableString(completion.Error),
			"finished_at": now,
			"updated_at":  now,
		})
	return err
}

func (s *DBLifecycleStore) RecordStep(ctx context.Context, step LifecycleStepRecord) error {
	ctx = contextOrBackground(ctx)
	now := firstLifecycleTime(step.Finished, step.Started, s.now())
	values := map[string]any{
		"attempt_key": step.AttemptKey,
		"module_id":   step.ModuleID,
		"action":      step.Action,
		"status":      step.Status,
		"stdout":      nullableString(step.Stdout),
		"stderr":      nullableString(step.Stderr),
		"error":       nullableString(step.Error),
		"updated_at":  now,
	}
	if !step.Started.IsZero() {
		values["started_at"] = step.Started
	}
	if !step.Finished.IsZero() {
		values["finished_at"] = step.Finished
	}
	result, err := s.orm(ctx).Query().Table("module_lifecycle_step").Where("attempt_key", step.AttemptKey).Update(values)
	if err != nil {
		return err
	}
	if result.RowsAffected == 1 {
		return nil
	}
	values["run_key"] = step.RunKey
	values["step_name"] = step.StepName
	values["command"] = step.Command
	values["created_at"] = now
	if _, ok := values["started_at"]; !ok {
		values["started_at"] = now
	}
	return s.orm(ctx).Query().Table("module_lifecycle_step").Create(values)
}

func (s *DBLifecycleStore) UpsertState(ctx context.Context, state LifecycleStateRecord) error {
	ctx = contextOrBackground(ctx)
	now := firstLifecycleTime(state.updatedAt, s.now())
	query := s.orm(ctx).Query()
	values := lifecycleStateValues(state, now)
	result, err := query.Table("module_state").Where("module_id", state.ModuleID).Update(values)
	if err != nil {
		return err
	}
	if result.RowsAffected == 1 {
		return nil
	}
	values["created_at"] = now
	return query.Table("module_state").Create(values)
}

func lifecycleStateValues(state LifecycleStateRecord, now time.Time) map[string]any {
	values := map[string]any{
		"module_id":       state.ModuleID,
		"name":            state.Name,
		"version":         state.Version,
		"target_version":  state.TargetVersion,
		"status":          state.Status,
		"enabled":         state.Enabled,
		"owner":           state.Owner,
		"disabled_reason": state.DisabledReason,
		"last_action":     state.LastAction,
		"last_run_key":    state.LastRunKey,
		"last_error":      nullableString(state.LastError),
		"metadata":        nullableJSON(state.Metadata),
		"last_run_at":     now,
		"updated_at":      now,
	}
	switch state.LastAction {
	case LifecycleActionInstall:
		if state.Status == "installed" {
			values["installed_at"] = now
		}
	case LifecycleActionUpgrade:
		if state.Status == "upgraded" {
			values["upgraded_at"] = now
		}
	case LifecycleActionUninstall:
		if state.Status == "uninstalled" {
			values["disabled_at"] = now
		}
	}
	return values
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableJSON(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func firstLifecycleTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}
