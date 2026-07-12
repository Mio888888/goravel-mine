package modulecatalog

import (
	"context"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
)

func (s *AdminService) ReleaseStaleLocks(payload AdminLockReleasePayload) (AdminLockReleaseResult, error) {
	ctx := contextOrBackground(s.ctx)
	payload.Key = strings.TrimSpace(payload.Key)
	gate := newAdminLockReleaseSecurityGate(payload)
	if !payload.DryRun {
		if err := gate.preflight(ctx); err != nil {
			return AdminLockReleaseResult{}, err
		}
	}
	rows, err := findStaleLocks(ctx, payload.Key)
	if err != nil {
		return AdminLockReleaseResult{}, err
	}
	if payload.DryRun || len(rows) == 0 {
		return AdminLockReleaseResult{DryRun: payload.DryRun, Released: rows}, nil
	}
	if s.afterStaleLockRead != nil {
		if err := s.afterStaleLockRead(ctx, rows); err != nil {
			return AdminLockReleaseResult{}, err
		}
	}
	released, err := deleteObservedStaleLocks(ctx, rows, func(mutate func() error) error { return gate.execute(ctx, mutate) })
	return AdminLockReleaseResult{DryRun: false, Released: released}, err
}

func findStaleLocks(ctx context.Context, key string) ([]AdminLockRow, error) {
	query := facades.Orm().WithContext(ctx).
		Query().
		Table("module_lifecycle_lock").
		Where("expires_at < ?", time.Now())
	if key != "" {
		query = query.Where("key", key)
	}
	records := make([]adminLockRecord, 0)
	err := query.OrderBy("expires_at").Get(&records)
	return mapSlice(records, adminLockDTO), err
}

func (s *AdminService) SetAfterStaleLockReadForTest(hook func(context.Context, []AdminLockRow) error) {
	s.afterStaleLockRead = hook
}

func deleteObservedStaleLocks(ctx context.Context, rows []AdminLockRow, executeGate func(func() error) error) ([]AdminLockRow, error) {
	released := make([]AdminLockRow, 0, len(rows))
	err := facades.Orm().WithContext(ctx).Transaction(func(tx contractsorm.Query) error {
		observed, err := lockObservedStaleRows(tx, rows)
		if err != nil || len(observed) == 0 {
			return err
		}
		mutate := func() error {
			for _, row := range observed {
				result, err := tx.Table("module_lifecycle_lock").
					Where("key", row.Key).
					Where("owner", row.Owner).
					Where("run_key", row.RunKey).
					Where("expires_at", *row.ExpiresAt).
					Delete()
				if err != nil {
					return err
				}
				if result.RowsAffected == 1 {
					released = append(released, row)
				}
			}
			return nil
		}
		if executeGate == nil {
			return mutate()
		}
		return executeGate(mutate)
	})
	return released, err
}

func lockObservedStaleRows(tx contractsorm.Query, rows []AdminLockRow) ([]AdminLockRow, error) {
	observed := make([]AdminLockRow, 0, len(rows))
	now := time.Now()
	for _, row := range rows {
		if row.ExpiresAt == nil {
			continue
		}
		matches := make([]adminLockRecord, 0, 1)
		err := tx.Table("module_lifecycle_lock").
			LockForUpdate().
			Where("key", row.Key).
			Where("owner", row.Owner).
			Where("run_key", row.RunKey).
			Where("expires_at", *row.ExpiresAt).
			Where("expires_at < ?", now).
			Limit(1).
			Get(&matches)
		if err != nil {
			return nil, err
		}
		if len(matches) == 1 {
			observed = append(observed, adminLockDTO(matches[0]))
		}
	}
	return observed, nil
}

func staleLockReleaseResource(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "all"
	}
	return "module-lifecycle:stale-locks:" + key
}
