package modulecatalog

import (
	"context"
	"time"

	"goravel/app/http/request"
)

type adminStepRecord struct {
	ID         uint64     `gorm:"column:id"`
	AttemptKey string     `gorm:"column:attempt_key"`
	RunKey     string     `gorm:"column:run_key"`
	ModuleID   string     `gorm:"column:module_id"`
	Action     string     `gorm:"column:action"`
	StepName   string     `gorm:"column:step_name"`
	Command    string     `gorm:"column:command"`
	Status     string     `gorm:"column:status"`
	Stdout     string     `gorm:"column:stdout"`
	Stderr     string     `gorm:"column:stderr"`
	Error      string     `gorm:"column:error"`
	StartedAt  *time.Time `gorm:"column:started_at"`
	FinishedAt *time.Time `gorm:"column:finished_at"`
	CreatedAt  *time.Time `gorm:"column:created_at"`
}

type adminStepQuery struct {
	ctx context.Context
}

func newAdminStepQuery(ctx context.Context) adminStepQuery {
	return adminStepQuery{ctx: contextOrBackground(ctx)}
}

func (q adminStepQuery) steps(pageRequest adminPageRequest) (request.PageResult[AdminStepRow], error) {
	return adminListQuery[adminStepRecord, AdminStepRow]{
		spec: adminListSpec{
			table: "module_lifecycle_step", orderBy: "id",
			filters: []adminEqualColumn{
				{filter: "run_key", column: "run_key"},
				{filter: "module_id", column: "module_id"},
				{filter: "action", column: "action"},
				{filter: "status", column: "status"},
			},
		},
		mapper: adminStepDTO,
	}.page(q.ctx, pageRequest)
}

func adminStepDTO(record adminStepRecord) AdminStepRow {
	return AdminStepRow{
		ID: record.ID, AttemptKey: record.AttemptKey, RunKey: record.RunKey,
		ModuleID: record.ModuleID, Action: record.Action, StepName: record.StepName,
		Command: record.Command, Status: record.Status, Stdout: record.Stdout,
		Stderr: record.Stderr, Error: record.Error, StartedAt: record.StartedAt,
		FinishedAt: record.FinishedAt, CreatedAt: record.CreatedAt,
	}
}
