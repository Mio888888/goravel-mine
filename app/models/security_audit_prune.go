package models

import (
	"encoding/json"
	"time"
)

const (
	SecurityAuditPruneStatusPlanned           = "planned"
	SecurityAuditPruneStatusRunning           = "running"
	SecurityAuditPruneStatusCompleted         = "completed"
	SecurityAuditPruneStatusNoOp              = "no_op"
	SecurityAuditPruneStatusPartiallyExecuted = "partially_executed"
	SecurityAuditPruneStatusFailed            = "failed"
	SecurityAuditPruneTargetStatusCompleted   = "completed"
	SecurityAuditPruneTargetStatusNoOp        = "no_op"
	SecurityAuditPruneTargetStatusFailed      = "failed"
)

type SecurityAuditPruneRun struct {
	ID              uint64     `gorm:"column:id;primaryKey" json:"id"`
	PlanID          string     `gorm:"column:plan_id" json:"plan_id"`
	Scope           string     `gorm:"column:scope" json:"scope"`
	RetentionDays   int        `gorm:"column:retention_days" json:"retention_days"`
	Cutoff          time.Time  `gorm:"column:cutoff" json:"cutoff"`
	TargetDigest    string     `gorm:"column:target_digest" json:"target_digest"`
	TargetCount     int64      `gorm:"column:target_count" json:"target_count"`
	ScopeCounts     string     `gorm:"column:scope_counts;type:jsonb" json:"-"`
	TableCounts     string     `gorm:"column:table_counts;type:jsonb" json:"-"`
	MinTimestamp    *time.Time `gorm:"column:min_timestamp" json:"min_timestamp,omitempty"`
	MaxTimestamp    *time.Time `gorm:"column:max_timestamp" json:"max_timestamp,omitempty"`
	Status          string     `gorm:"column:status" json:"status"`
	ExecutionID     string     `gorm:"column:execution_id" json:"execution_id,omitempty"`
	HeartbeatAt     *time.Time `gorm:"column:heartbeat_at" json:"heartbeat_at,omitempty"`
	ArchiveURI      string     `gorm:"column:archive_uri" json:"archive_uri,omitempty"`
	ObjectVersion   string     `gorm:"column:object_version" json:"object_version,omitempty"`
	ManifestSHA256  string     `gorm:"column:manifest_sha256" json:"manifest_sha256,omitempty"`
	ImmutableUntil  *time.Time `gorm:"column:immutable_until" json:"immutable_until,omitempty"`
	ProofVerifiedAt *time.Time `gorm:"column:proof_verified_at" json:"proof_verified_at,omitempty"`
	StartedAt       *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	FinishedAt      *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	Error           string     `gorm:"column:error" json:"error,omitempty"`
	Timestamps
}

func (SecurityAuditPruneRun) TableName() string {
	return "security_audit_prune_run"
}

type SecurityAuditPruneTarget struct {
	ID              uint64          `gorm:"column:id;primaryKey" json:"id"`
	RunID           uint64          `gorm:"column:run_id" json:"run_id"`
	Scope           string          `gorm:"column:scope" json:"scope"`
	Connection      string          `gorm:"column:connection" json:"connection"`
	TenantID        uint64          `gorm:"column:tenant_id" json:"tenant_id,omitempty"`
	TenantCode      string          `gorm:"column:tenant_code" json:"tenant_code,omitempty"`
	DatabaseDigest  string          `gorm:"column:database_digest" json:"database_digest,omitempty"`
	AuditTable      string          `gorm:"column:table_name" json:"table_name"`
	TimestampColumn string          `gorm:"column:timestamp_column" json:"timestamp_column"`
	TargetID        uint64          `gorm:"column:target_id" json:"target_id"`
	OccurredAt      time.Time       `gorm:"column:occurred_at" json:"occurred_at"`
	RecordDigest    string          `gorm:"column:record_digest" json:"record_digest"`
	Record          json.RawMessage `gorm:"-" json:"record,omitempty"`
	Cutoff          time.Time       `gorm:"column:cutoff" json:"cutoff"`
	RetentionDays   int             `gorm:"column:retention_days" json:"retention_days"`
	Status          string          `gorm:"column:status" json:"status"`
	ProcessedAt     *time.Time      `gorm:"column:processed_at" json:"processed_at,omitempty"`
	Error           string          `gorm:"column:error" json:"error,omitempty"`
	Timestamps
}

func (SecurityAuditPruneTarget) TableName() string {
	return "security_audit_prune_target"
}
