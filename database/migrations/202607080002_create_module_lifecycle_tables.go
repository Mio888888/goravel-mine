package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607080002CreateModuleLifecycleTables struct{}

func (r *M202607080002CreateModuleLifecycleTables) Signature() string {
	return "202607080002_create_module_lifecycle_tables"
}

func (r *M202607080002CreateModuleLifecycleTables) Up() error {
	if err := createModuleStateTable(); err != nil {
		return err
	}
	if err := createModuleLifecycleRunTable(); err != nil {
		return err
	}
	return createModuleLifecycleLockTable()
}

func (r *M202607080002CreateModuleLifecycleTables) Down() error {
	return dropTables("module_lifecycle_lock", "module_lifecycle_run", "module_state")
}

func createModuleStateTable() error {
	if facades.Schema().HasTable("module_state") {
		return nil
	}
	return facades.Schema().Create("module_state", func(table schema.Blueprint) {
		table.ID()
		table.String("module_id", 120)
		table.String("name", 160).Default("")
		table.String("version", 60).Default("")
		table.String("target_version", 60).Default("")
		table.String("status", 30).Default("new")
		table.Boolean("enabled").Default(true)
		table.String("owner", 120).Default("")
		table.String("disabled_reason", 255).Default("")
		table.String("last_action", 30).Default("")
		table.String("last_run_key", 220).Default("")
		table.LongText("last_error").Nullable()
		table.Jsonb("metadata").Nullable()
		table.Timestamp("installed_at").Nullable()
		table.Timestamp("upgraded_at").Nullable()
		table.Timestamp("disabled_at").Nullable()
		table.Timestamp("last_run_at").Nullable()
		addTimestamps(table)
		table.Unique("module_id")
		table.Index("status")
		table.Index("enabled")
		table.Index("owner")
		table.Index("last_action")
	})
}

func createModuleLifecycleRunTable() error {
	if facades.Schema().HasTable("module_lifecycle_run") {
		return nil
	}
	return facades.Schema().Create("module_lifecycle_run", func(table schema.Blueprint) {
		table.ID()
		table.String("idempotency_key", 220)
		table.String("module_id", 120)
		table.String("action", 30)
		table.String("from_version", 60).Default("")
		table.String("to_version", 60).Default("")
		table.String("status", 30).Default("running")
		table.Boolean("dry_run").Default(false)
		table.String("owner", 120).Default("")
		table.String("reason", 255).Default("")
		table.String("command", 255).Default("")
		table.LongText("error").Nullable()
		table.Jsonb("plan").Nullable()
		table.Timestamp("started_at").Nullable()
		table.Timestamp("finished_at").Nullable()
		addTimestamps(table)
		table.Unique("idempotency_key")
		table.Index("module_id")
		table.Index("action")
		table.Index("status")
		table.Index("owner")
	})
}

func createModuleLifecycleLockTable() error {
	if facades.Schema().HasTable("module_lifecycle_lock") {
		return nil
	}
	return facades.Schema().Create("module_lifecycle_lock", func(table schema.Blueprint) {
		table.ID()
		table.String("key", 180)
		table.String("owner", 120)
		table.String("run_key", 220).Default("")
		table.Timestamp("expires_at")
		addTimestamps(table)
		table.Unique("key")
		table.Index("owner")
		table.Index("run_key")
		table.Index("expires_at")
	})
}
