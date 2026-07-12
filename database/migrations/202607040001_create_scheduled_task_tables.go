package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607040001CreateScheduledTaskTables struct{}

func (r *M202607040001CreateScheduledTaskTables) Signature() string {
	return "202607040001_create_scheduled_task_tables"
}

func (r *M202607040001CreateScheduledTaskTables) Connection() string {
	return scheduledTaskMigrationConnection()
}

func (r *M202607040001CreateScheduledTaskTables) Up() error {
	if err := createScheduledTaskTable(); err != nil {
		return err
	}
	return createScheduledTaskLogTable()
}

func (r *M202607040001CreateScheduledTaskTables) Down() error {
	return dropTables("scheduled_task_log", "scheduled_task")
}

func createScheduledTaskTable() error {
	if facades.Schema().HasTable("scheduled_task") {
		return nil
	}

	return facades.Schema().Create("scheduled_task", func(table schema.Blueprint) {
		table.ID()
		table.String("name", 100)
		table.String("code", 120)
		table.String("description", 255).Default("")
		table.String("cron_expression", 120)
		table.String("timezone", 80).Default("UTC")
		table.Timestamp("next_run_at").Nullable()
		table.String("task_type", 30)
		table.Jsonb("payload").Nullable()
		table.Integer("timeout_seconds").Default(60)
		table.Boolean("allow_overlap").Default(false)
		table.Integer("max_log_output").Default(4000)
		table.Jsonb("target_ips").Nullable()
		table.Jsonb("tenant_ids").Nullable()
		table.Boolean("run_on_one_server").Default(true)
		table.TinyInteger("status").Default(1)
		table.Timestamp("last_run_at").Nullable()
		table.String("last_status", 30).Default("")
		table.Integer("last_duration_ms").Default(0)
		table.String("last_message", 255).Default("")
		table.Timestamp("locked_until").Nullable()
		table.String("lock_owner", 100).Default("")
		table.String("run_token", 80).Default("")
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("code")
		table.Index("status", "next_run_at")
		table.Index("task_type")
		table.Index("lock_owner")
	})
}

func createScheduledTaskLogTable() error {
	if facades.Schema().HasTable("scheduled_task_log") {
		return nil
	}

	return facades.Schema().Create("scheduled_task_log", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("task_id")
		table.String("task_name", 100)
		table.String("task_code", 120)
		table.String("run_token", 80)
		table.String("trigger_mode", 20)
		table.String("task_type", 30)
		table.String("node_ip", 100)
		table.String("status", 30)
		table.Timestamp("scheduled_at").Nullable()
		table.Timestamp("started_at").Nullable()
		table.Timestamp("finished_at").Nullable()
		table.Integer("duration_ms").Default(0)
		table.Integer("exit_code").Nullable()
		table.Integer("http_status").Nullable()
		table.LongText("stdout").Nullable()
		table.LongText("stderr").Nullable()
		table.LongText("error_message").Nullable()
		table.Jsonb("payload").Nullable()
		table.Jsonb("tenants").Nullable()
		addTimestamps(table)
		table.Index("task_id")
		table.Index("task_code")
		table.Index("status")
		table.Index("started_at")
	})
}
