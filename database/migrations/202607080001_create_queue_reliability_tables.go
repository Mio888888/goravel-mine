package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607080001CreateQueueReliabilityTables struct{}

func (r *M202607080001CreateQueueReliabilityTables) Signature() string {
	return "202607080001_create_queue_reliability_tables"
}

func (r *M202607080001CreateQueueReliabilityTables) Up() error {
	if err := createQueueOutboxTable(); err != nil {
		return err
	}
	if err := createQueueIdempotencyTable(); err != nil {
		return err
	}
	return createQueueTaskLockTable()
}

func (r *M202607080001CreateQueueReliabilityTables) Down() error {
	return dropTables("queue_task_lock", "queue_idempotency", "queue_outbox")
}

func createQueueOutboxTable() error {
	if facades.Schema().HasTable("queue_outbox") {
		return nil
	}
	return facades.Schema().Create("queue_outbox", func(table schema.Blueprint) {
		table.ID()
		table.String("topic", 120)
		table.String("connection", 40).Default("redis")
		table.String("queue", 120).Default("default")
		table.Jsonb("payload").Nullable()
		table.String("status", 30).Default("pending")
		table.Integer("attempts").Default(0)
		table.Timestamp("available_at").Nullable()
		table.Timestamp("locked_until").Nullable()
		table.String("lock_owner", 100).Default("")
		table.String("claim_token", 64).Default("")
		table.LongText("last_error").Nullable()
		addTimestamps(table)
		table.Index("status", "available_at")
		table.Index("topic")
		table.Index("lock_owner")
		table.Index("claim_token")
	})
}

func createQueueIdempotencyTable() error {
	if facades.Schema().HasTable("queue_idempotency") {
		return nil
	}
	return facades.Schema().Create("queue_idempotency", func(table schema.Blueprint) {
		table.ID()
		table.String("key", 180)
		table.String("status", 30).Default("success")
		table.LongText("result").Nullable()
		table.LongText("last_error").Nullable()
		table.Timestamp("locked_until").Nullable()
		table.String("claim_token", 64).Default("")
		addTimestamps(table)
		table.Unique("key")
		table.Index("status")
		table.Index("locked_until")
		table.Index("claim_token")
	})
}

func createQueueTaskLockTable() error {
	if facades.Schema().HasTable("queue_task_lock") {
		return nil
	}
	return facades.Schema().Create("queue_task_lock", func(table schema.Blueprint) {
		table.ID()
		table.String("key", 180)
		table.String("owner", 100)
		table.Timestamp("expires_at")
		addTimestamps(table)
		table.Unique("key")
		table.Index("owner")
		table.Index("expires_at")
	})
}
