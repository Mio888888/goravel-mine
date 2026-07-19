package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607190002CreateMiddlewarePlatformTables struct{}

func (r *M202607190002CreateMiddlewarePlatformTables) Signature() string {
	return "202607190002_create_middleware_platform_tables"
}

func (r *M202607190002CreateMiddlewarePlatformTables) Connection() string {
	return platformMigrationConnection()
}

func (r *M202607190002CreateMiddlewarePlatformTables) Up() error {
	dbSchema := facades.Schema().Connection(platformMigrationConnection())
	if err := extendScheduledTaskPlatformSchema(dbSchema); err != nil {
		return err
	}
	if err := createMiddlewareAdapterTable(dbSchema); err != nil {
		return err
	}
	if err := createMessageRouteTable(dbSchema); err != nil {
		return err
	}
	if err := createMessageDeliveryTable(dbSchema); err != nil {
		return err
	}
	if err := createMessageDeadLetterTable(dbSchema); err != nil {
		return err
	}
	if err := createProtectionRuleSetTable(dbSchema); err != nil {
		return err
	}
	return createProtectionRuleVersionTable(dbSchema)
}

func (r *M202607190002CreateMiddlewarePlatformTables) Down() error {
	dbSchema := facades.Schema().Connection(platformMigrationConnection())
	return dbSchema.Sql(`
		DROP TABLE IF EXISTS protection_rule_version CASCADE;
		DROP TABLE IF EXISTS protection_rule_set CASCADE;
		DROP TABLE IF EXISTS message_dead_letter CASCADE;
		DROP TABLE IF EXISTS message_delivery CASCADE;
		DROP TABLE IF EXISTS message_route CASCADE;
		DROP TABLE IF EXISTS middleware_adapter CASCADE;
	`)
}

func extendScheduledTaskPlatformSchema(dbSchema schema.Schema) error {
	if dbSchema.HasTable("scheduled_task") {
		columns := []struct {
			name  string
			apply func(schema.Blueprint)
		}{
			{"handler_key", func(table schema.Blueprint) { table.String("handler_key", 160).Default("") }},
			{"parameters", func(table schema.Blueprint) { table.Jsonb("parameters").Nullable() }},
			{"concurrency_policy", func(table schema.Blueprint) { table.String("concurrency_policy", 20).Default("FORBID") }},
			{"misfire_policy", func(table schema.Blueprint) { table.String("misfire_policy", 30).Default("SCHEDULER_DEFAULT") }},
			{"retry_policy", func(table schema.Blueprint) { table.Jsonb("retry_policy").Nullable() }},
			{"scope", func(table schema.Blueprint) { table.String("scope", 20).Default("GLOBAL") }},
			{"runtime_state", func(table schema.Blueprint) { table.String("runtime_state", 30).Default("REGISTERED") }},
			{"version", func(table schema.Blueprint) { table.Integer("version").Default(1) }},
		}
		for _, column := range columns {
			if dbSchema.HasColumn("scheduled_task", column.name) {
				continue
			}
			if err := dbSchema.Table("scheduled_task", column.apply); err != nil {
				return err
			}
		}
		if err := dbSchema.Sql(`
			UPDATE scheduled_task
			SET handler_key = COALESCE(NULLIF(handler_key, ''), payload->>'handler', ''),
				parameters = COALESCE(parameters, payload, '{}'::jsonb),
				concurrency_policy = CASE WHEN allow_overlap THEN 'ALLOW' ELSE 'FORBID' END,
				scope = CASE
					WHEN jsonb_typeof(tenant_ids) = 'array' AND jsonb_array_length(tenant_ids) > 0 THEN 'PER_TENANT'
					ELSE 'GLOBAL'
				END,
				runtime_state = CASE
					WHEN task_type IN ('method', 'governance', 'backup') THEN 'REGISTERED'
					ELSE 'LEGACY_UNSAFE'
				END,
				version = GREATEST(COALESCE(version, 1), 1)
		`); err != nil {
			return err
		}
		for _, index := range []struct {
			name  string
			apply func(schema.Blueprint)
		}{
			{"scheduled_task_handler_key_index", func(table schema.Blueprint) { table.Index("handler_key") }},
			{"scheduled_task_runtime_state_index", func(table schema.Blueprint) { table.Index("runtime_state") }},
		} {
			if dbSchema.HasIndex("scheduled_task", index.name) {
				continue
			}
			if err := dbSchema.Table("scheduled_task", index.apply); err != nil {
				return err
			}
		}
	}

	if !dbSchema.HasTable("scheduled_task_log") {
		return nil
	}
	columns := []struct {
		name  string
		apply func(schema.Blueprint)
	}{
		{"logical_execution_id", func(table schema.Blueprint) { table.String("logical_execution_id", 100).Default("") }},
		{"idempotency_key", func(table schema.Blueprint) { table.String("idempotency_key", 180).Default("") }},
		{"attempt", func(table schema.Blueprint) { table.Integer("attempt").Default(1) }},
		{"correlation_id", func(table schema.Blueprint) { table.String("correlation_id", 100).Default("") }},
	}
	for _, column := range columns {
		if dbSchema.HasColumn("scheduled_task_log", column.name) {
			continue
		}
		if err := dbSchema.Table("scheduled_task_log", column.apply); err != nil {
			return err
		}
	}
	if err := dbSchema.Sql(`
		UPDATE scheduled_task_log
		SET logical_execution_id = CASE
				WHEN logical_execution_id = '' THEN CONCAT('legacy:', id::text)
				ELSE logical_execution_id
			END,
			attempt = GREATEST(COALESCE(attempt, 1), 1)
	`); err != nil {
		return err
	}
	return dbSchema.Sql(`
		CREATE INDEX IF NOT EXISTS scheduled_task_log_logical_execution_id_index
			ON scheduled_task_log (logical_execution_id);
		CREATE UNIQUE INDEX IF NOT EXISTS scheduled_task_log_manual_idempotency_unique
			ON scheduled_task_log (task_id, idempotency_key)
			WHERE idempotency_key <> '';
	`)
}

func createMiddlewareAdapterTable(dbSchema schema.Schema) error {
	if dbSchema.HasTable("middleware_adapter") {
		return nil
	}
	return dbSchema.Create("middleware_adapter", func(table schema.Blueprint) {
		table.ID()
		table.String("adapter_key", 120)
		table.String("name", 120)
		table.String("adapter_type", 40)
		table.String("connection", 80).Default("")
		table.Jsonb("capabilities").Nullable()
		table.LongText("config_encrypted").Nullable()
		table.String("config_fingerprint", 80).Default("")
		table.Boolean("enabled").Default(true)
		table.String("health_status", 20).Default("UNKNOWN")
		table.Timestamp("last_checked_at").Nullable()
		table.Integer("version").Default(1)
		addAuditColumns(table)
		addTimestamps(table)
		table.Unique("adapter_key")
		table.Unique("name")
		table.Index("adapter_type")
		table.Index("enabled")
		table.Index("health_status")
	})
}

func createMessageRouteTable(dbSchema schema.Schema) error {
	if dbSchema.HasTable("message_route") {
		return nil
	}
	return dbSchema.Create("message_route", func(table schema.Blueprint) {
		table.ID()
		table.String("name", 120)
		table.String("message_type", 160)
		table.UnsignedBigInteger("adapter_id")
		table.String("destination", 160)
		table.String("consumption_mode", 20).Default("CLUSTER")
		table.String("consumer_group", 160).Default("")
		table.Integer("concurrency").Default(1)
		table.Boolean("ordering_enabled").Default(false)
		table.Jsonb("retry_policy").Nullable()
		table.Jsonb("dead_letter_policy").Nullable()
		table.String("status", 20).Default("DRAFT")
		table.Boolean("enabled").Default(true)
		table.Integer("version").Default(1)
		table.Timestamp("published_at").Nullable()
		addAuditColumns(table)
		addTimestamps(table)
		table.Unique("name")
		table.Index("message_type")
		table.Index("adapter_id")
		table.Index("status")
		table.Index("enabled")
	})
}

func createMessageDeliveryTable(dbSchema schema.Schema) error {
	if dbSchema.HasTable("message_delivery") {
		return nil
	}
	return dbSchema.Create("message_delivery", func(table schema.Blueprint) {
		table.ID()
		table.String("message_id", 100)
		table.String("message_type", 160)
		table.String("consumer_key", 160)
		table.UnsignedBigInteger("route_id").Default(0)
		table.UnsignedBigInteger("adapter_id").Default(0)
		table.String("status", 30)
		table.Integer("attempt").Default(1)
		table.Timestamp("received_at").Nullable()
		table.Timestamp("finished_at").Nullable()
		table.Integer("duration_ms").Default(0)
		table.String("correlation_id", 100).Default("")
		table.String("external_position", 180).Default("")
		table.LongText("error_summary").Nullable()
		addTimestamps(table)
		table.Index("message_id")
		table.Index("message_type")
		table.Index("consumer_key")
		table.Index("route_id")
		table.Index("status")
		table.Index("created_at")
	})
}

func createMessageDeadLetterTable(dbSchema schema.Schema) error {
	if dbSchema.HasTable("message_dead_letter") {
		return nil
	}
	return dbSchema.Create("message_dead_letter", func(table schema.Blueprint) {
		table.ID()
		table.String("message_id", 100)
		table.String("message_type", 160)
		table.String("consumer_key", 160)
		table.UnsignedBigInteger("route_id").Default(0)
		table.UnsignedBigInteger("adapter_id").Default(0)
		table.Jsonb("envelope").Nullable()
		table.LongText("envelope_encrypted").Nullable()
		table.String("failure_class", 30)
		table.LongText("error_summary").Nullable()
		table.Timestamp("first_failed_at").Nullable()
		table.Timestamp("last_failed_at").Nullable()
		table.Integer("replay_count").Default(0)
		table.String("resolution_status", 30).Default("OPEN")
		table.UnsignedBigInteger("resolved_by").Default(0)
		table.Timestamp("resolved_at").Nullable()
		addTimestamps(table)
		table.Index("message_id")
		table.Index("message_type")
		table.Index("consumer_key")
		table.Index("failure_class")
		table.Index("resolution_status")
		table.Index("created_at")
	})
}

func createProtectionRuleSetTable(dbSchema schema.Schema) error {
	if dbSchema.HasTable("protection_rule_set") {
		return nil
	}
	return dbSchema.Create("protection_rule_set", func(table schema.Blueprint) {
		table.ID()
		table.String("name", 120)
		table.String("scope", 20)
		table.String("resource_pattern", 200)
		table.Jsonb("rules").Nullable()
		table.String("status", 20).Default("DRAFT")
		table.Boolean("enabled").Default(true)
		table.Integer("version").Default(1)
		table.Integer("published_version").Default(0)
		table.Timestamp("published_at").Nullable()
		addAuditColumns(table)
		addTimestamps(table)
		table.Unique("name")
		table.Index("scope")
		table.Index("resource_pattern")
		table.Index("status")
		table.Index("enabled")
	})
}

func createProtectionRuleVersionTable(dbSchema schema.Schema) error {
	if dbSchema.HasTable("protection_rule_version") {
		return nil
	}
	if err := dbSchema.Create("protection_rule_version", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("rule_set_id")
		table.Integer("version")
		table.String("name", 120)
		table.String("scope", 20)
		table.String("resource_pattern", 200)
		table.Jsonb("rules").Nullable()
		table.Boolean("enabled").Default(true)
		table.UnsignedBigInteger("published_by").Default(0)
		table.Timestamp("published_at")
		addTimestamps(table)
		table.Index("rule_set_id")
		table.Index("published_at")
	}); err != nil {
		return err
	}
	return dbSchema.Sql(`
		CREATE UNIQUE INDEX IF NOT EXISTS protection_rule_version_rule_set_version_unique
			ON protection_rule_version (rule_set_id, version);
	`)
}
