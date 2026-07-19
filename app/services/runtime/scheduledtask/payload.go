package scheduledtask

import (
	"strings"
	"time"

	"goravel/app/models"
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
	taskType := strings.TrimSpace(p.TaskType)
	handlerKey := strings.TrimSpace(p.HandlerKey)
	if handlerKey == "" {
		handlerKey = strings.TrimSpace(jsonString(p.Payload, "handler"))
	}
	if taskType == "" {
		taskType = ScheduledTaskTypeHandler
	}
	parameters := cloneJSONMap(p.Parameters)
	if parameters == nil {
		parameters = cloneJSONMap(p.Payload)
		delete(parameters, "handler")
	}
	definition, definitionFound := scheduledTaskHandlerDefinition(handlerKey)
	timeout := p.TimeoutSeconds
	if timeout < 1 && definitionFound {
		timeout = definition.DefaultTimeout
	}
	if timeout < 1 {
		timeout = 60
	}
	concurrencyPolicy := strings.ToUpper(strings.TrimSpace(p.ConcurrencyPolicy))
	if concurrencyPolicy == "" {
		if p.AllowOverlap {
			concurrencyPolicy = ScheduledTaskConcurrencyAllow
		} else {
			concurrencyPolicy = ScheduledTaskConcurrencyForbid
		}
	}
	misfirePolicy := strings.ToUpper(strings.TrimSpace(p.MisfirePolicy))
	if misfirePolicy == "" {
		misfirePolicy = ScheduledTaskMisfireSchedulerDefault
	}
	scope := strings.ToUpper(strings.TrimSpace(p.Scope))
	if scope == "" {
		if definitionFound && definition.TenantCapability == ScheduledTaskTenantGlobalOnly {
			scope = ScheduledTaskScopeGlobal
		} else {
			scope = ScheduledTaskScopePerTenant
		}
	}
	retryPolicy := cloneJSONMap(p.RetryPolicy)
	if retryPolicy == nil {
		retryPolicy = models.JSONMap{
			"max_attempts":          1,
			"initial_delay_seconds": 1,
			"max_delay_seconds":     30,
		}
	}
	maxOutput := p.MaxLogOutput
	if maxOutput < 1 {
		maxOutput = 4000
	}
	return ScheduledTask{
		Name: strings.TrimSpace(p.Name), Code: strings.TrimSpace(p.Code),
		Description: strings.TrimSpace(p.Description), CronExpression: strings.TrimSpace(p.CronExpression),
		Timezone: timezone, TaskType: taskType, Payload: cloneJSONMap(p.Payload),
		HandlerKey: handlerKey, Parameters: parameters,
		TimeoutSeconds: timeout, AllowOverlap: concurrencyPolicy == ScheduledTaskConcurrencyAllow,
		ConcurrencyPolicy: concurrencyPolicy, MisfirePolicy: misfirePolicy,
		RetryPolicy: retryPolicy, Scope: scope, MaxLogOutput: maxOutput,
		TargetIPs: normalizeTargetIPs(p.TargetIPs), TenantIDs: normalizeTenantIDs(p.TenantIDs),
		RunOnOneServer: true, Status: status, RuntimeState: ScheduledTaskRuntimeRegistered,
		Version: max(p.Version, 1),
		Remark:  strings.TrimSpace(p.Remark),
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
	switch task.ConcurrencyPolicy {
	case ScheduledTaskConcurrencyAllow, ScheduledTaskConcurrencyForbid:
	case ScheduledTaskConcurrencyReplace:
		definition, ok := scheduledTaskHandlerDefinition(task.HandlerKey)
		if !ok || !definition.SupportsCancellation {
			return BusinessError{Message: "任务处理器不支持 REPLACE 并发策略"}
		}
	default:
		return BusinessError{Message: "任务并发策略无效"}
	}
	switch task.MisfirePolicy {
	case ScheduledTaskMisfireIgnore, ScheduledTaskMisfireFireOnceNow, ScheduledTaskMisfireSchedulerDefault:
	default:
		return BusinessError{Message: "任务错过触发策略无效"}
	}
	if task.Scope != ScheduledTaskScopeGlobal && task.Scope != ScheduledTaskScopePerTenant {
		return BusinessError{Message: "任务作用域无效"}
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
