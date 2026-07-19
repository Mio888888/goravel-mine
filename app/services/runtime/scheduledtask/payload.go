package scheduledtask

import (
	"strings"
	"time"
)

func (p ScheduledTaskPayload) ScheduledTask() (ScheduledTask, error) {
	status := p.Status
	if status == 0 {
		status = ScheduledTaskStatusEnabled
	}
	timezone := strings.TrimSpace(p.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	timeout := p.TimeoutSeconds
	if timeout < 1 {
		timeout = 60
	}
	maxOutput := p.MaxLogOutput
	if maxOutput < 1 {
		maxOutput = 4000
	}
	return ScheduledTask{
		Name: strings.TrimSpace(p.Name), Code: strings.TrimSpace(p.Code),
		Description: strings.TrimSpace(p.Description), CronExpression: strings.TrimSpace(p.CronExpression),
		Timezone: timezone, TaskType: strings.TrimSpace(p.TaskType), Payload: p.Payload,
		TimeoutSeconds: timeout, AllowOverlap: p.AllowOverlap, MaxLogOutput: maxOutput,
		TargetIPs: normalizeTargetIPs(p.TargetIPs), TenantIDs: normalizeTenantIDs(p.TenantIDs),
		RunOnOneServer: true, Status: status,
		Remark: strings.TrimSpace(p.Remark),
	}, nil
}

func (s *ScheduledTaskService) validateScheduledTask(task ScheduledTask) error {
	if task.Name == "" || task.Code == "" {
		return BusinessError{Message: "任务名称和编码不能为空"}
	}
	if task.Status != ScheduledTaskStatusEnabled && task.Status != ScheduledTaskStatusDisabled {
		return BusinessError{Message: "任务状态无效"}
	}
	if task.CronExpression == "" {
		return BusinessError{Message: "Cron 表达式不能为空"}
	}
	if _, err := time.LoadLocation(task.Timezone); err != nil {
		return BusinessError{Message: "任务时区无效"}
	}
	if _, err := taskNextRun(task, scheduledTaskNow()); err != nil {
		return err
	}
	if err := s.validateScheduledTaskTenantIDs(task.TenantIDs); err != nil {
		return err
	}
	return validateScheduledTaskPayload(task)
}

func taskNextRun(task ScheduledTask, after time.Time) (time.Time, error) {
	location, err := time.LoadLocation(task.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	return NextScheduledRun(task.CronExpression, location, after)
}
