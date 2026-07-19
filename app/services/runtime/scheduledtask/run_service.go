package scheduledtask

import (
	"context"
	"fmt"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/models"
)

func (s *ScheduledTaskService) runTask(ctx context.Context, task ScheduledTask, triggerMode string, scheduledAt time.Time, token string) (ScheduledTaskLog, error) {
	start := scheduledTaskNow()
	log := ScheduledTaskLog{
		TaskID: task.ID, TaskName: task.Name, TaskCode: task.Code, RunToken: token,
		TriggerMode: triggerMode, TaskType: task.TaskType, NodeIP: s.nodeIP,
		Status: ScheduledTaskLogStatusRunning, ScheduledAt: &scheduledAt, StartedAt: &start,
		Timestamps: models.Timestamps{CreatedAt: start, UpdatedAt: start},
	}
	if err := s.query().Table("scheduled_task_log").Create(&log); err != nil {
		_ = s.recordTaskRunWriteFailure(task.ID, triggerMode, token, start, err)
		return ScheduledTaskLog{}, err
	}
	scope, scopeErr := s.scheduledTaskTenantScopeFor(task)
	if scopeErr == nil {
		log.Tenants = scope.JSONSlice()
		if err := updateScheduledTaskLogJSON(s.query(), log.ID, task.Payload, log.Tenants); err != nil {
			_ = s.recordTaskRunWriteFailure(task.ID, triggerMode, token, start, err)
			return log, err
		}
	} else {
		if err := updateScheduledTaskLogJSON(s.query(), log.ID, task.Payload, nil); err != nil {
			_ = s.recordTaskRunWriteFailure(task.ID, triggerMode, token, start, err)
			return log, err
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, scheduledTaskTimeout(task))
	defer cancel()

	result := ScheduledTaskExecutionResult{}
	if scopeErr != nil {
		result = taskFailure(scopeErr.Error())
	} else {
		result = safeExecuteScheduledTaskWithScope(runCtx, task, scope)
	}
	result = trimExecutionResult(result, maxLogOutput(task))
	finished := scheduledTaskNow()
	duration := int(finished.Sub(start).Milliseconds())
	if result.Status == "" {
		result.Status = ScheduledTaskLogStatusSuccess
	}
	log.Status = result.Status
	log.FinishedAt = &finished
	log.DurationMS = duration
	log.ExitCode = result.ExitCode
	log.HTTPStatus = result.HTTPStatus
	log.Stdout = result.Stdout
	log.Stderr = result.Stderr
	log.ErrorMessage = result.ErrorMessage
	if task.TaskType == ScheduledTaskTypeGovernance {
		RecordTenantGovernanceEvent(ctx, map[string]any{
			"outcome":     governanceTaskOutcome(result.Status),
			"task_id":     task.ID,
			"task_code":   task.Code,
			"run_id":      log.ID,
			"handler":     jsonString(task.Payload, "handler"),
			"duration_ms": duration,
			"error":       result.ErrorMessage,
		})
	}

	logErr := s.finishScheduledTaskLog(log, finished)
	taskErr := s.finishScheduledTask(task.ID, triggerMode, token, start, finished, log)
	return log, firstError(logErr, taskErr)
}

func governanceTaskOutcome(status string) string {
	if status == ScheduledTaskLogStatusSuccess {
		return "success"
	}
	return "failure"
}

func GovernanceTaskOutcome(status string) string {
	return governanceTaskOutcome(status)
}

func (s *ScheduledTaskService) finishScheduledTaskLog(log ScheduledTaskLog, finished time.Time) error {
	_, err := s.query().Table("scheduled_task_log").Where("id", log.ID).Update(map[string]any{
		"status": log.Status, "finished_at": finished, "duration_ms": log.DurationMS,
		"exit_code": log.ExitCode, "http_status": log.HTTPStatus, "stdout": log.Stdout,
		"stderr": log.Stderr, "error_message": log.ErrorMessage, "updated_at": finished,
	})
	return err
}

func (s *ScheduledTaskService) finishScheduledTask(taskID uint64, triggerMode, token string, start, finished time.Time, log ScheduledTaskLog) error {
	taskUpdates := map[string]any{
		"last_run_at": start, "last_status": log.Status, "last_duration_ms": log.DurationMS,
		"last_message": firstNonEmpty(log.ErrorMessage, log.Stdout), "updated_at": finished,
	}
	taskQuery := s.taskRunQuery(taskID, triggerMode, token)
	if triggerMode == ScheduledTaskTriggerSchedule {
		taskUpdates["locked_until"] = nil
		taskUpdates["lock_owner"] = ""
		taskUpdates["run_token"] = ""
	}
	_, err := taskQuery.Update(taskUpdates)
	return err
}

func (s *ScheduledTaskService) recordTaskRunWriteFailure(taskID uint64, triggerMode, token string, start time.Time, err error) error {
	finished := scheduledTaskNow()
	log := ScheduledTaskLog{
		Status:       ScheduledTaskLogStatusFailed,
		DurationMS:   int(finished.Sub(start).Milliseconds()),
		ErrorMessage: err.Error(),
	}
	return s.finishScheduledTask(taskID, triggerMode, token, start, finished, log)
}

func (s *ScheduledTaskService) taskRunQuery(taskID uint64, triggerMode, token string) contractsorm.Query {
	query := s.query().Table("scheduled_task").Where("id", taskID)
	if triggerMode == ScheduledTaskTriggerSchedule {
		query = query.Where("run_token", token)
	}
	return query
}

func firstError(values ...error) error {
	for _, err := range values {
		if err != nil {
			return err
		}
	}
	return nil
}

func safeExecuteScheduledTask(ctx context.Context, task ScheduledTask) (result ScheduledTaskExecutionResult) {
	scope, err := NewScheduledTaskService().scheduledTaskTenantScopeFor(task)
	if err != nil {
		return taskFailure(err.Error())
	}
	return safeExecuteScheduledTaskWithScope(ctx, task, scope)
}

func safeExecuteScheduledTaskWithScope(ctx context.Context, task ScheduledTask, scope scheduledTaskTenantScope) (result ScheduledTaskExecutionResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = ScheduledTaskExecutionResult{
				Status:       ScheduledTaskLogStatusFailed,
				ErrorMessage: fmt.Sprintf("panic: %v", recovered),
			}
		}
	}()
	return executeScheduledTaskWithScope(ctx, task, scope)
}
