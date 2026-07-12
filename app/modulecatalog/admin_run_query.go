package modulecatalog

import (
	"context"
	"time"

	"goravel/app/http/request"
)

type adminRunRecord struct {
	ID             uint64     `gorm:"column:id"`
	IdempotencyKey string     `gorm:"column:idempotency_key"`
	ModuleID       string     `gorm:"column:module_id"`
	Action         string     `gorm:"column:action"`
	FromVersion    string     `gorm:"column:from_version"`
	ToVersion      string     `gorm:"column:to_version"`
	Status         string     `gorm:"column:status"`
	DryRun         bool       `gorm:"column:dry_run"`
	Owner          string     `gorm:"column:owner"`
	Reason         string     `gorm:"column:reason"`
	Command        string     `gorm:"column:command"`
	Error          string     `gorm:"column:error"`
	StartedAt      *time.Time `gorm:"column:started_at"`
	FinishedAt     *time.Time `gorm:"column:finished_at"`
	CreatedAt      *time.Time `gorm:"column:created_at"`
}

type adminRunQuery struct {
	ctx context.Context
}

func newAdminRunQuery(ctx context.Context) adminRunQuery {
	return adminRunQuery{ctx: contextOrBackground(ctx)}
}

func (q adminRunQuery) runs(pageRequest adminPageRequest) (request.PageResult[AdminRunRow], error) {
	return adminListQuery[adminRunRecord, AdminRunRow]{
		spec: adminListSpec{
			table: "module_lifecycle_run", orderBy: "id",
			filters: []adminEqualColumn{
				{filter: "run_key", column: "idempotency_key"},
				{filter: "module_id", column: "module_id"},
				{filter: "action", column: "action"},
				{filter: "status", column: "status"},
				{filter: "owner", column: "owner"},
			},
		},
		mapper: adminRunDTO,
	}.page(q.ctx, pageRequest)
}

func adminRunDTO(record adminRunRecord) AdminRunRow {
	return AdminRunRow{
		ID: record.ID, IdempotencyKey: record.IdempotencyKey, ModuleID: record.ModuleID,
		Action: record.Action, FromVersion: record.FromVersion, ToVersion: record.ToVersion,
		Status: record.Status, DryRun: record.DryRun, Owner: record.Owner, Reason: record.Reason,
		Command: record.Command, Error: record.Error, StartedAt: record.StartedAt,
		FinishedAt: record.FinishedAt, CreatedAt: record.CreatedAt,
	}
}
