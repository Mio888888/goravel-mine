package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607090002CreateModuleLifecycleStepTable struct{}

func (r *M202607090002CreateModuleLifecycleStepTable) Signature() string {
	return "202607090002_create_module_lifecycle_step_table"
}

func (r *M202607090002CreateModuleLifecycleStepTable) Up() error {
	if facades.Schema().HasTable("module_lifecycle_step") {
		return nil
	}
	return facades.Schema().Create("module_lifecycle_step", func(table schema.Blueprint) {
		table.ID()
		table.String("run_key", 220)
		table.String("module_id", 120)
		table.String("action", 30)
		table.String("step_name", 80)
		table.String("command", 255).Default("")
		table.String("status", 30).Default("running")
		table.LongText("stdout").Nullable()
		table.LongText("stderr").Nullable()
		table.LongText("error").Nullable()
		table.Timestamp("started_at").Nullable()
		table.Timestamp("finished_at").Nullable()
		addTimestamps(table)
		table.Index("run_key")
		table.Index("module_id")
		table.Index("action")
		table.Index("status")
		table.Index("started_at")
	})
}

func (r *M202607090002CreateModuleLifecycleStepTable) Down() error {
	return dropTables("module_lifecycle_step")
}
