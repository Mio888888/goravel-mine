package models

import "time"

const (
	TenantGovernanceRunKindRetention       = "retention"
	TenantGovernanceRunKindExport          = "export"
	TenantGovernanceRunKindIsolationVerify = "isolation_verify"

	TenantGovernanceRunStatusPending          = "pending"
	TenantGovernanceRunStatusRunning          = "running"
	TenantGovernanceRunStatusAwaitingEvidence = "awaiting_evidence"
	TenantGovernanceRunStatusArtifactWritten  = "artifact_written"
	TenantGovernanceRunStatusCompleted        = "completed"
	TenantGovernanceRunStatusFailed           = "failed"
	TenantGovernanceRunStatusStale            = "stale"
)

type TenantGovernanceRun struct {
	ID             uint64     `gorm:"column:id;primaryKey" json:"id"`
	TenantID       uint64     `gorm:"column:tenant_id" json:"tenant_id"`
	TenantCode     string     `gorm:"column:tenant_code" json:"tenant_code"`
	Kind           string     `gorm:"column:kind" json:"kind"`
	IdempotencyKey string     `gorm:"column:idempotency_key" json:"idempotency_key"`
	PolicyVersion  string     `gorm:"column:policy_version" json:"policy_version"`
	PlanID         string     `gorm:"column:plan_id" json:"plan_id,omitempty"`
	Status         string     `gorm:"column:status" json:"status"`
	StartedAt      *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	FinishedAt     *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	Error          string     `gorm:"column:error" json:"error,omitempty"`
	Timestamps
}

func (TenantGovernanceRun) TableName() string { return "tenant_governance_run" }
