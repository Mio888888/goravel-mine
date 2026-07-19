package migrations

import (
	"fmt"
	"time"

	"goravel/app/facades"
	"goravel/app/support/cronexpr"
)

type M202607110010UpsertTenantGovernanceTasks struct{}

const tenantGovernanceTaskConflictUpdate = `
	name = EXCLUDED.name,
	description = EXCLUDED.description,
	task_type = EXCLUDED.task_type,
	payload = EXCLUDED.payload,
	updated_at = EXCLUDED.updated_at`

func (r *M202607110010UpsertTenantGovernanceTasks) Signature() string {
	return "202607110010_upsert_tenant_governance_tasks"
}

func (r *M202607110010UpsertTenantGovernanceTasks) Connection() string {
	return scheduledTaskMigrationConnection()
}

func (r *M202607110010UpsertTenantGovernanceTasks) Up() error {
	if err := rejectScheduledTaskConnectionMoveWithData(); err != nil {
		return err
	}
	if !facades.Schema().HasTable("scheduled_task") {
		if err := createScheduledTaskTable(); err != nil {
			return fmt.Errorf("create platform scheduled_task table: %w", err)
		}
	}
	if !facades.Schema().HasTable("scheduled_task_log") {
		if err := createScheduledTaskLogTable(); err != nil {
			return fmt.Errorf("create platform scheduled_task_log table: %w", err)
		}
	}
	now := time.Now().UTC()
	tasks := tenantGovernanceTaskSeeds()
	for _, task := range tasks {
		nextRunAt, err := nextTenantGovernanceTaskRun(task.Cron, now)
		if err != nil {
			return fmt.Errorf("calculate next run for %s: %w", task.Code, err)
		}
		_, err = facades.Orm().Query().Exec(`
				INSERT INTO scheduled_task (
				name, code, description, cron_expression, timezone, task_type, payload,
				timeout_seconds, allow_overlap, max_log_output, target_ips, tenant_ids,
				run_on_one_server, status, next_run_at, created_by, updated_by, created_at, updated_at, remark
			) VALUES (?, ?, ?, ?, 'UTC', 'governance', ?::jsonb, 3600, false, 65536,
				'[]'::jsonb, '[]'::jsonb, true, ?, ?, 0, 0, ?, ?, ?)
				ON CONFLICT (code) DO UPDATE SET `+tenantGovernanceTaskConflictUpdate, task.Name, task.Code, task.Description, task.Cron, task.Payload, task.Status,
			nextRunAt, now, now, task.Remark)
		if err != nil {
			return err
		}
	}
	return nil
}

func rejectScheduledTaskConnectionMoveWithData() error {
	platform := scheduledTaskMigrationConnection()
	defaultConnection := facades.Config().GetString("database.default")
	if platform == "" || defaultConnection == "" || platform == defaultConnection {
		return nil
	}
	defaultSchema := facades.Schema().Connection(defaultConnection)
	for _, table := range []string{"scheduled_task", "scheduled_task_log"} {
		if !defaultSchema.HasTable(table) {
			continue
		}
		count, err := facades.Orm().Connection(defaultConnection).Query().Table(table).Count()
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("cannot move %s to tenant.platform_connection while default connection contains %d rows; migrate scheduled-task data before upgrade", table, count)
		}
	}
	return nil
}

func scheduledTaskMigrationConnection() string {
	connection := facades.Config().GetString("tenant.platform_connection")
	if connection == "" {
		return facades.Config().GetString("database.default")
	}
	return connection
}

func (r *M202607110010UpsertTenantGovernanceTasks) Down() error {
	return nil
}

type tenantGovernanceTaskSeed struct {
	Name, Code, Description, Cron, Payload, Remark string
	Status                                         int8
}

func tenantGovernanceTaskSeeds() []tenantGovernanceTaskSeed {
	return []tenantGovernanceTaskSeed{
		{Name: "租户审计留存计划", Code: "tenant_retention_governance", Description: "生成逐租户 retention plan；真实删除仍需 WORM proof 与一次性审批", Cron: "0 0 2 * * *", Payload: `{"handler":"scheduler.tenant_retention"}`, Status: 2, Remark: "disabled by default"},
		{Name: "租户隔离证明验证", Code: "tenant_isolation_verification", Description: "逐租户验证数据库身份与负向访问，并归档不可变隔离证据", Cron: "0 0 3 * * *", Payload: `{"handler":"scheduler.tenant_isolation_verify"}`, Status: 1, Remark: "enabled governance control"},
	}
}

func nextTenantGovernanceTaskRun(expression string, after time.Time) (time.Time, error) {
	schedule, err := cronexpr.ParseWithSeconds(expression)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(after.UTC()), nil
}
