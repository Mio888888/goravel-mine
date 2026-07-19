package migrations

import "goravel/app/facades"

type M202607190001DropQueueTaskLockTable struct{}

func (r *M202607190001DropQueueTaskLockTable) Signature() string {
	return "202607190001_drop_queue_task_lock_table"
}

func (r *M202607190001DropQueueTaskLockTable) Up() error {
	return facades.Schema().DropIfExists("queue_task_lock")
}

func (r *M202607190001DropQueueTaskLockTable) Down() error {
	return createQueueTaskLockTable()
}
