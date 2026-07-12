package modulecatalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"goravel/app/facades"
)

type persistedModuleStateRow struct {
	ModuleID       string     `gorm:"column:module_id"`
	Name           string     `gorm:"column:name"`
	Status         string     `gorm:"column:status"`
	Enabled        bool       `gorm:"column:enabled"`
	Owner          string     `gorm:"column:owner"`
	TargetVersion  string     `gorm:"column:target_version"`
	LastAction     string     `gorm:"column:last_action"`
	LastRunKey     string     `gorm:"column:last_run_key"`
	LastError      string     `gorm:"column:last_error"`
	InstalledAt    *time.Time `gorm:"column:installed_at"`
	UpgradedAt     *time.Time `gorm:"column:upgraded_at"`
	DisabledAt     *time.Time `gorm:"column:disabled_at"`
	LastRunAt      *time.Time `gorm:"column:last_run_at"`
	DisabledReason string     `gorm:"column:disabled_reason"`
}

func persistedModuleStates(ctx context.Context) (map[string]*PersistedModuleState, error) {
	rows := make([]persistedModuleStateRow, 0)
	if err := facades.Orm().WithContext(contextOrBackground(ctx)).Query().Table("module_state").Get(&rows); err != nil {
		if isUndefinedModuleStateTable(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("module state query failed: %w", err)
	}
	states := make(map[string]*PersistedModuleState, len(rows))
	for _, row := range rows {
		states[row.ModuleID] = persistedState(row)
	}
	return states, nil
}

func isUndefinedModuleStateTable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}

func persistedState(row persistedModuleStateRow) *PersistedModuleState {
	return &PersistedModuleState{
		Name: row.Name, Status: row.Status, Enabled: row.Enabled, Owner: row.Owner,
		TargetVersion: row.TargetVersion, LastAction: row.LastAction, LastRunKey: row.LastRunKey,
		LastError: row.LastError, InstalledAt: row.InstalledAt, UpgradedAt: row.UpgradedAt,
		DisabledAt: row.DisabledAt, LastRunAt: row.LastRunAt, DisabledReason: row.DisabledReason,
	}
}
