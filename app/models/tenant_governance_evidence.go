package models

import "time"

type TenantGovernanceEvidence struct {
	ID            uint64     `gorm:"column:id;primaryKey" json:"id"`
	RunID         uint64     `gorm:"column:run_id" json:"run_id"`
	TenantID      uint64     `gorm:"column:tenant_id" json:"tenant_id"`
	Kind          string     `gorm:"column:kind" json:"kind"`
	URI           string     `gorm:"column:uri" json:"uri"`
	ObjectVersion string     `gorm:"column:object_version" json:"object_version"`
	SHA256        string     `gorm:"column:sha256" json:"sha256"`
	VerifiedAt    time.Time  `gorm:"column:verified_at" json:"verified_at"`
	ExpiresAt     time.Time  `gorm:"column:expires_at" json:"expires_at"`
	Metadata      string     `gorm:"column:metadata;type:jsonb" json:"-"`
	StaleAt       *time.Time `gorm:"column:stale_at" json:"stale_at,omitempty"`
	Timestamps
}

func (TenantGovernanceEvidence) TableName() string { return "tenant_governance_evidence" }
