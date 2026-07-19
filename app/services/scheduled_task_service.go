package services

import (
	"context"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
)

const (
	ScheduledTaskStatusEnabled  int8 = 1
	ScheduledTaskStatusDisabled int8 = 2

	ScheduledTaskTypeURL        = "url"
	ScheduledTaskTypeScript     = "script"
	ScheduledTaskTypeMethod     = "method"
	ScheduledTaskTypeBackup     = "backup"
	ScheduledTaskTypeGovernance = "governance"

	ScheduledTaskLogStatusRunning = "running"
	ScheduledTaskLogStatusSuccess = "success"
	ScheduledTaskLogStatusFailed  = "failed"
	ScheduledTaskLogStatusSkipped = "skipped"

	ScheduledTaskTriggerSchedule = "schedule"
	ScheduledTaskTriggerManual   = "manual"
)

var scheduledTaskNow = time.Now

type ScheduledTask = models.ScheduledTask
type ScheduledTaskLog = models.ScheduledTaskLog

type ScheduledTaskPayload struct {
	Name           string           `json:"name"`
	Code           string           `json:"code"`
	Description    string           `json:"description"`
	CronExpression string           `json:"cron_expression"`
	Timezone       string           `json:"timezone"`
	TaskType       string           `json:"task_type"`
	Payload        models.JSONMap   `json:"payload"`
	TimeoutSeconds int              `json:"timeout_seconds"`
	AllowOverlap   bool             `json:"allow_overlap"`
	MaxLogOutput   int              `json:"max_log_output"`
	TargetIPs      models.JSONSlice `json:"target_ips"`
	TenantIDs      models.JSONSlice `json:"tenant_ids"`
	RunOnOneServer bool             `json:"run_on_one_server"`
	Status         int8             `json:"status"`
	Remark         string           `json:"remark"`
}

type ScheduledTaskService struct {
	ctx    context.Context
	nodeIP string
}

func NewScheduledTaskService() *ScheduledTaskService {
	return &ScheduledTaskService{nodeIP: SchedulerNodeIP()}
}

func NewScheduledTaskServiceForNode(nodeIP string) *ScheduledTaskService {
	return &ScheduledTaskService{nodeIP: strings.TrimSpace(nodeIP)}
}

func (s *ScheduledTaskService) WithContext(ctx context.Context) *ScheduledTaskService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *ScheduledTaskService) List(filters map[string]string, page, pageSize int) (request.PageResult[ScheduledTask], error) {
	query := s.query().Table("scheduled_task")
	query = query.Scopes(scopes.Contains("name", filters["name"]))
	query = query.Scopes(scopes.Contains("code", filters["code"]))
	query = query.Scopes(scopes.Equal("task_type", filters["task_type"]))
	query = query.Scopes(scopes.EqualIfPresent("status", filters["status"]))
	return request.Paginate[ScheduledTask](query.OrderByDesc("id"), page, pageSize)
}

func (s *ScheduledTaskService) Logs(filters map[string]string, page, pageSize int) (request.PageResult[ScheduledTaskLog], error) {
	query := s.query().Table("scheduled_task_log")
	query = query.Scopes(scopes.Equal("task_code", filters["task_code"]))
	query = query.Scopes(scopes.Equal("status", filters["status"]))
	query = query.Scopes(scopes.Equal("trigger_mode", filters["trigger_mode"]))
	query = query.Scopes(scopes.EqualIfPresent("task_id", filters["task_id"]))
	return request.Paginate[ScheduledTaskLog](query.OrderByDesc("id"), page, pageSize)
}

func (s *ScheduledTaskService) Detail(id uint64) (ScheduledTask, error) {
	return s.find(id)
}

func (s *ScheduledTaskService) Create(input ScheduledTaskPayload, operatorID uint64) (ScheduledTask, error) {
	task, err := input.ScheduledTask()
	if err != nil {
		return ScheduledTask{}, err
	}
	task.AuditColumns = models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID}
	if err := s.validateScheduledTask(task); err != nil {
		return ScheduledTask{}, err
	}
	nextRunAt, err := taskNextRun(task, scheduledTaskNow())
	if err != nil {
		return ScheduledTask{}, err
	}
	task.NextRunAt = &nextRunAt
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		row := scheduledTaskScalar(task)
		if err := tx.Table("scheduled_task").Create(&row); err != nil {
			return err
		}
		task.ID = row.ID
		return updateScheduledTaskJSON(tx, row.ID, task.Payload, task.TargetIPs, task.TenantIDs)
	}); err != nil {
		return ScheduledTask{}, err
	}
	return task, nil
}

func (s *ScheduledTaskService) Update(id uint64, input ScheduledTaskPayload, operatorID uint64) (ScheduledTask, error) {
	if _, err := s.find(id); err != nil {
		return ScheduledTask{}, err
	}
	task, err := input.ScheduledTask()
	if err != nil {
		return ScheduledTask{}, err
	}
	task.ID = id
	task.UpdatedBy = operatorID
	if err := s.validateScheduledTask(task); err != nil {
		return ScheduledTask{}, err
	}
	nextRunAt, err := taskNextRun(task, scheduledTaskNow())
	if err != nil {
		return ScheduledTask{}, err
	}
	task.NextRunAt = &nextRunAt
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("scheduled_task").Where("id", id).Update(map[string]any{
			"name": task.Name, "code": task.Code, "description": task.Description,
			"cron_expression": task.CronExpression, "timezone": task.Timezone, "next_run_at": nextRunAt,
			"task_type": task.TaskType, "timeout_seconds": task.TimeoutSeconds,
			"allow_overlap": task.AllowOverlap, "max_log_output": task.MaxLogOutput,
			"run_on_one_server": task.RunOnOneServer,
			"status":            task.Status, "updated_by": operatorID, "updated_at": scheduledTaskNow(),
			"remark": task.Remark,
		})
		if err != nil {
			return err
		}
		return updateScheduledTaskJSON(tx, id, task.Payload, task.TargetIPs, task.TenantIDs)
	}); err != nil {
		return ScheduledTask{}, err
	}
	return task, nil
}

func (s *ScheduledTaskService) Delete(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	return s.orm().Transaction(func(tx contractsorm.Query) error {
		if _, err := tx.Table("scheduled_task_log").WhereIn("task_id", uint64Any(ids)).Delete(); err != nil {
			return err
		}
		_, err := tx.Table("scheduled_task").WhereIn("id", uint64Any(ids)).Delete()
		return err
	})
}

func (s *ScheduledTaskService) Enable(id uint64, operatorID uint64) (ScheduledTask, error) {
	return s.setStatus(id, ScheduledTaskStatusEnabled, operatorID)
}

func (s *ScheduledTaskService) Disable(id uint64, operatorID uint64) (ScheduledTask, error) {
	return s.setStatus(id, ScheduledTaskStatusDisabled, operatorID)
}

func (s *ScheduledTaskService) ManualRun(ctx context.Context, id uint64) (ScheduledTaskLog, error) {
	task, err := s.find(id)
	if err != nil {
		return ScheduledTaskLog{}, err
	}
	if !ScheduledTaskTargetsNode(stringSliceFromJSON(task.TargetIPs), s.nodeIP) {
		return ScheduledTaskLog{}, BusinessError{Message: "当前节点不在任务指定 IP 范围"}
	}
	return s.runTask(ctx, task, ScheduledTaskTriggerManual, scheduledTaskNow(), randomRunToken())
}

func (s *ScheduledTaskService) DueTasks(now time.Time, limit int) ([]ScheduledTask, error) {
	if limit < 1 {
		limit = 20
	}
	return s.dueTasksAfter(now, time.Time{}, 0, limit)
}

func (s *ScheduledTaskService) dueTasksAfter(now time.Time, afterRunAt time.Time, afterID uint64, limit int) ([]ScheduledTask, error) {
	rows := make([]ScheduledTask, 0)
	query := s.query().Table("scheduled_task").
		Where("status", ScheduledTaskStatusEnabled).
		Where("next_run_at <= ?", now).
		Where("(locked_until IS NULL OR locked_until <= ?)", now)
	if !afterRunAt.IsZero() {
		query = query.Where("(next_run_at > ? OR (next_run_at = ? AND id > ?))", afterRunAt, afterRunAt, afterID)
	}
	err := query.
		OrderBy("next_run_at").
		OrderBy("id").
		Limit(limit).
		Get(&rows)
	return rows, err
}

func (s *ScheduledTaskService) RunDue(ctx context.Context, now time.Time) error {
	const batchSize = 50
	var lastRunAt time.Time
	var lastID uint64
	for {
		tasks, err := s.dueTasksAfter(now, lastRunAt, lastID, batchSize)
		if err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}
		for _, task := range tasks {
			if task.NextRunAt != nil {
				lastRunAt = *task.NextRunAt
			}
			lastID = task.ID
			if !ScheduledTaskTargetsNode(stringSliceFromJSON(task.TargetIPs), s.nodeIP) {
				continue
			}
			claimed, token, scheduledAt, err := s.Claim(task, now)
			if err != nil || !claimed {
				continue
			}
			go func(task ScheduledTask, scheduledAt time.Time, token string) {
				_, _ = s.runTask(ctx, task, ScheduledTaskTriggerSchedule, scheduledAt, token)
			}(task, scheduledAt, token)
		}
		if len(tasks) < batchSize {
			return nil
		}
	}
}

func (s *ScheduledTaskService) Claim(task ScheduledTask, now time.Time) (bool, string, time.Time, error) {
	next, err := taskNextRun(task, now)
	if err != nil {
		return false, "", time.Time{}, err
	}
	token := randomRunToken()
	timeout := scheduledTaskTimeout(task)
	if task.AllowOverlap {
		timeout = 5 * time.Second
	}
	lockUntil := now.Add(timeout + 5*time.Second)
	scheduledAt := now
	if task.NextRunAt != nil {
		scheduledAt = *task.NextRunAt
	}
	result, err := s.query().Table("scheduled_task").
		Where("id", task.ID).
		Where("status", ScheduledTaskStatusEnabled).
		Where("next_run_at <= ?", now).
		Where("(locked_until IS NULL OR locked_until <= ?)", now).
		Update(map[string]any{
			"next_run_at": next, "locked_until": lockUntil, "lock_owner": s.nodeIP,
			"run_token": token, "updated_at": now,
		})
	if err != nil {
		return false, "", time.Time{}, err
	}
	return result.RowsAffected == 1, token, scheduledAt, nil
}
