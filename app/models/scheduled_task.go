package models

import "time"

type ScheduledTask struct {
	ID                uint64     `gorm:"column:id;primaryKey" json:"id"`
	Name              string     `gorm:"column:name" json:"name"`
	Code              string     `gorm:"column:code" json:"code"`
	Description       string     `gorm:"column:description" json:"description"`
	CronExpression    string     `gorm:"column:cron_expression" json:"cron_expression"`
	Timezone          string     `gorm:"column:timezone" json:"timezone"`
	NextRunAt         *time.Time `gorm:"column:next_run_at" json:"next_run_at"`
	TaskType          string     `gorm:"column:task_type" json:"task_type"`
	Payload           JSONMap    `gorm:"column:payload;type:jsonb" json:"payload"`
	HandlerKey        string     `gorm:"column:handler_key" json:"handler_key"`
	Parameters        JSONMap    `gorm:"column:parameters;type:jsonb" json:"parameters"`
	TimeoutSeconds    int        `gorm:"column:timeout_seconds" json:"timeout_seconds"`
	AllowOverlap      bool       `gorm:"column:allow_overlap" json:"allow_overlap"`
	ConcurrencyPolicy string     `gorm:"column:concurrency_policy" json:"concurrency_policy"`
	MisfirePolicy     string     `gorm:"column:misfire_policy" json:"misfire_policy"`
	RetryPolicy       JSONMap    `gorm:"column:retry_policy;type:jsonb" json:"retry_policy"`
	Scope             string     `gorm:"column:scope" json:"scope"`
	MaxLogOutput      int        `gorm:"column:max_log_output" json:"max_log_output"`
	TargetIPs         JSONSlice  `gorm:"column:target_ips;type:jsonb" json:"target_ips"`
	TenantIDs         JSONSlice  `gorm:"column:tenant_ids;type:jsonb" json:"tenant_ids"`
	RunOnOneServer    bool       `gorm:"column:run_on_one_server" json:"run_on_one_server"`
	Status            int8       `gorm:"column:status" json:"status"`
	LastRunAt         *time.Time `gorm:"column:last_run_at" json:"last_run_at"`
	LastStatus        string     `gorm:"column:last_status" json:"last_status"`
	LastDurationMS    int        `gorm:"column:last_duration_ms" json:"last_duration_ms"`
	LastMessage       string     `gorm:"column:last_message" json:"last_message"`
	LockedUntil       *time.Time `gorm:"column:locked_until" json:"locked_until"`
	LockOwner         string     `gorm:"column:lock_owner" json:"lock_owner"`
	RunToken          string     `gorm:"column:run_token" json:"run_token"`
	RuntimeState      string     `gorm:"column:runtime_state" json:"runtime_state"`
	Version           int        `gorm:"column:version" json:"version"`
	AuditColumns
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (ScheduledTask) TableName() string {
	return "scheduled_task"
}

type ScheduledTaskLog struct {
	ID                 uint64     `gorm:"column:id;primaryKey" json:"id"`
	TaskID             uint64     `gorm:"column:task_id" json:"task_id"`
	TaskName           string     `gorm:"column:task_name" json:"task_name"`
	TaskCode           string     `gorm:"column:task_code" json:"task_code"`
	RunToken           string     `gorm:"column:run_token" json:"run_token"`
	TriggerMode        string     `gorm:"column:trigger_mode" json:"trigger_mode"`
	TaskType           string     `gorm:"column:task_type" json:"task_type"`
	LogicalExecutionID string     `gorm:"column:logical_execution_id" json:"logical_execution_id"`
	IdempotencyKey     string     `gorm:"column:idempotency_key" json:"idempotency_key"`
	Attempt            int        `gorm:"column:attempt" json:"attempt"`
	CorrelationID      string     `gorm:"column:correlation_id" json:"correlation_id"`
	NodeIP             string     `gorm:"column:node_ip" json:"node_ip"`
	Status             string     `gorm:"column:status" json:"status"`
	ScheduledAt        *time.Time `gorm:"column:scheduled_at" json:"scheduled_at"`
	StartedAt          *time.Time `gorm:"column:started_at" json:"started_at"`
	FinishedAt         *time.Time `gorm:"column:finished_at" json:"finished_at"`
	DurationMS         int        `gorm:"column:duration_ms" json:"duration_ms"`
	ExitCode           *int       `gorm:"column:exit_code" json:"exit_code"`
	HTTPStatus         *int       `gorm:"column:http_status" json:"http_status"`
	Stdout             string     `gorm:"column:stdout" json:"stdout"`
	Stderr             string     `gorm:"column:stderr" json:"stderr"`
	ErrorMessage       string     `gorm:"column:error_message" json:"error_message"`
	Payload            JSONMap    `gorm:"column:payload;type:jsonb" json:"payload"`
	Tenants            JSONSlice  `gorm:"column:tenants;type:jsonb" json:"tenants"`
	Timestamps
}

func (ScheduledTaskLog) TableName() string {
	return "scheduled_task_log"
}
