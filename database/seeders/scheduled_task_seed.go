package seeders

import (
	"fmt"
	"time"

	"goravel/app/facades"
	"goravel/app/support/cronexpr"
)

type ScheduledTaskSeeder struct{}

func (s *ScheduledTaskSeeder) Signature() string { return "scheduled_task_governance_seed" }

func (s *ScheduledTaskSeeder) Run() error {
	connection := scheduledTaskPlatformConnection()
	if !facades.Schema().Connection(connection).HasTable("scheduled_task") {
		return nil
	}
	tasks := governanceScheduledTasks()
	for _, task := range tasks {
		if err := insertGovernanceScheduledTask(connection, task); err != nil {
			return err
		}
	}
	return nil
}

func insertGovernanceScheduledTask(connection string, task governanceScheduledTask) error {
	now := time.Now().UTC()
	nextRunAt, err := nextGovernanceScheduledTaskRun(task.Cron, now)
	if err != nil {
		return fmt.Errorf("calculate next run for %s: %w", task.Code, err)
	}
	_, err = facades.Orm().Connection(connection).Query().Exec(`
		INSERT INTO scheduled_task (
			name, code, description, cron_expression, timezone, task_type, payload,
			timeout_seconds, allow_overlap, max_log_output, target_ips, tenant_ids,
			run_on_one_server, status, next_run_at, created_by, updated_by, created_at, updated_at, remark
		)
		VALUES (
			?, ?, ?, ?, ?, ?, ?::jsonb,
			?, false, ?, '[]'::jsonb, '[]'::jsonb,
			true, ?, ?, 0, 0, ?, ?, ?
		)
			ON CONFLICT (code) DO NOTHING
	`, task.Name, task.Code, task.Description, task.Cron, "UTC", "governance", `{"handler":"`+task.Handler+`"}`, 3600, 65536, task.Status,
		nextRunAt, now, now, task.Remark)
	return err
}

type governanceScheduledTask struct {
	Name, Code, Description, Cron, Handler, Remark string
	Status                                         int8
}

func governanceScheduledTasks() []governanceScheduledTask {
	return []governanceScheduledTask{
		{Name: "租户审计留存计划", Code: "tenant_retention_governance", Description: "生成逐租户 retention plan；真实删除仍需 WORM proof 与一次性审批", Cron: "0 0 2 * * *", Handler: "scheduler.tenant_retention", Status: 2, Remark: "disabled by default"},
		{Name: "租户隔离证明验证", Code: "tenant_isolation_verification", Description: "逐租户验证数据库身份与负向访问，并归档不可变隔离证据", Cron: "0 0 3 * * *", Handler: "scheduler.tenant_isolation_verify", Status: 1, Remark: "enabled governance control"},
	}
}

func nextGovernanceScheduledTaskRun(expression string, after time.Time) (time.Time, error) {
	schedule, err := cronexpr.ParseWithSeconds(expression)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(after.UTC()), nil
}

func scheduledTaskPlatformConnection() string {
	connection := facades.Config().GetString("tenant.platform_connection")
	if connection == "" {
		return facades.Config().GetString("database.default")
	}
	return connection
}
