package migrations

import (
	"strings"

	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202607190003ExtendQueueMessageOutbox struct{}

func (r *M202607190003ExtendQueueMessageOutbox) Signature() string {
	return "202607190003_extend_queue_message_outbox"
}

func (r *M202607190003ExtendQueueMessageOutbox) Up() error {
	for _, connection := range queueMessageOutboxMigrationConnections() {
		if err := extendQueueMessageOutboxSchema(facades.Schema().Connection(connection)); err != nil {
			return err
		}
	}
	return nil
}

func extendQueueMessageOutboxSchema(dbSchema schema.Schema) error {
	if err := createQueueOutboxTableOn(dbSchema); err != nil {
		return err
	}
	if err := createQueueIdempotencyTableOn(dbSchema); err != nil {
		return err
	}
	if dbSchema.HasTable("queue_outbox") {
		columns := []struct {
			name  string
			apply func(schema.Blueprint)
		}{
			{"message_id", func(table schema.Blueprint) { table.String("message_id", 100).Default("") }},
			{"message_type", func(table schema.Blueprint) { table.String("message_type", 160).Default("") }},
			{"schema_version", func(table schema.Blueprint) { table.Integer("schema_version").Default(1) }},
			{"route_id", func(table schema.Blueprint) { table.UnsignedBigInteger("route_id").Default(0) }},
			{"adapter_id", func(table schema.Blueprint) { table.UnsignedBigInteger("adapter_id").Default(0) }},
			{"envelope", func(table schema.Blueprint) { table.Jsonb("envelope").Nullable() }},
			{"correlation_id", func(table schema.Blueprint) { table.String("correlation_id", 100).Default("") }},
			{"tenant_id", func(table schema.Blueprint) { table.String("tenant_id", 100).Default("") }},
			{"published_at", func(table schema.Blueprint) { table.Timestamp("published_at").Nullable() }},
			{"publish_receipt", func(table schema.Blueprint) { table.String("publish_receipt", 180).Default("") }},
		}
		for _, column := range columns {
			if dbSchema.HasColumn("queue_outbox", column.name) {
				continue
			}
			if err := dbSchema.Table("queue_outbox", column.apply); err != nil {
				return err
			}
		}
		if err := dbSchema.Sql(`
			CREATE INDEX IF NOT EXISTS queue_outbox_message_id_index ON queue_outbox (message_id);
			CREATE INDEX IF NOT EXISTS queue_outbox_message_type_index ON queue_outbox (message_type);
			CREATE INDEX IF NOT EXISTS queue_outbox_route_id_index ON queue_outbox (route_id);
		`); err != nil {
			return err
		}
	}
	if dbSchema.HasTable("message_dead_letter") && !dbSchema.HasColumn("message_dead_letter", "envelope_encrypted") {
		if err := dbSchema.Table("message_dead_letter", func(table schema.Blueprint) {
			table.LongText("envelope_encrypted").Nullable()
		}); err != nil {
			return err
		}
	}
	if !dbSchema.HasTable("queue_idempotency") {
		return nil
	}
	columns := []struct {
		name  string
		apply func(schema.Blueprint)
	}{
		{"consumer_key", func(table schema.Blueprint) { table.String("consumer_key", 160).Default("") }},
		{"idempotency_key", func(table schema.Blueprint) { table.String("idempotency_key", 180).Default("") }},
		{"message_id", func(table schema.Blueprint) { table.String("message_id", 100).Default("") }},
	}
	for _, column := range columns {
		if dbSchema.HasColumn("queue_idempotency", column.name) {
			continue
		}
		if err := dbSchema.Table("queue_idempotency", column.apply); err != nil {
			return err
		}
	}
	return dbSchema.Sql(`
		CREATE INDEX IF NOT EXISTS queue_idempotency_consumer_key_index ON queue_idempotency (consumer_key);
		CREATE INDEX IF NOT EXISTS queue_idempotency_message_id_index ON queue_idempotency (message_id);
		CREATE UNIQUE INDEX IF NOT EXISTS queue_idempotency_consumer_key_idempotency_key_unique
			ON queue_idempotency (consumer_key, idempotency_key)
			WHERE consumer_key <> '' AND idempotency_key <> '';
	`)
}

func queueMessageOutboxMigrationConnections() []string {
	defaultConnection := strings.TrimSpace(facades.Config().GetString("database.default"))
	platformConnection := strings.TrimSpace(platformMigrationConnection())
	return uniqueQueueMessageOutboxMigrationConnections(defaultConnection, platformConnection)
}

func uniqueQueueMessageOutboxMigrationConnections(defaultConnection, platformConnection string) []string {
	defaultConnection = strings.TrimSpace(defaultConnection)
	platformConnection = strings.TrimSpace(platformConnection)
	if platformConnection == "" || platformConnection == defaultConnection {
		return []string{defaultConnection}
	}
	return []string{defaultConnection, platformConnection}
}

func (r *M202607190003ExtendQueueMessageOutbox) Down() error {
	return nil
}
