package modulecatalog

import (
	"context"
	"time"

	"goravel/app/facades"
	"goravel/app/http/request"
)

type adminLockRecord struct {
	ID        uint64     `gorm:"column:id"`
	Key       string     `gorm:"column:key"`
	Owner     string     `gorm:"column:owner"`
	RunKey    string     `gorm:"column:run_key"`
	ExpiresAt *time.Time `gorm:"column:expires_at"`
	CreatedAt *time.Time `gorm:"column:created_at"`
	UpdatedAt *time.Time `gorm:"column:updated_at"`
}

type adminLockQuery struct {
	ctx context.Context
}

func newAdminLockQuery(ctx context.Context) adminLockQuery {
	return adminLockQuery{ctx: contextOrBackground(ctx)}
}

func (q adminLockQuery) locks() (request.PageResult[AdminLockRow], error) {
	records := make([]adminLockRecord, 0)
	err := facades.Orm().WithContext(q.ctx).
		Query().
		Table("module_lifecycle_lock").
		OrderByDesc("expires_at").
		Get(&records)
	rows := mapSlice(records, adminLockDTO)
	return request.PageResult[AdminLockRow]{List: rows, Total: int64(len(rows))}, err
}

func adminLockDTO(record adminLockRecord) AdminLockRow {
	return AdminLockRow{
		ID: record.ID, Key: record.Key, Owner: record.Owner, RunKey: record.RunKey,
		ExpiresAt: record.ExpiresAt, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}
