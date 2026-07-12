package services

import (
	"context"
	"errors"
	"time"

	"goravel/app/models"
)

type TenantGovernanceObservability struct {
	EvidenceExpired    int64
	VerificationFailed int64
	OldestRunAge       time.Duration
}

func TenantGovernanceObservabilitySnapshot(ctx context.Context, now time.Time) (TenantGovernanceObservability, error) {
	database := Orm()
	if database == nil {
		return TenantGovernanceObservability{}, errors.New("tenant governance observability requires ORM binding")
	}
	query := database.WithContext(contextOrBackground(ctx)).Connection(PlatformConnection()).Query()
	expired, err := query.Table((models.TenantGovernanceEvidence{}).TableName()).Where("expires_at <= ?", now.UTC()).Count()
	if err != nil {
		return TenantGovernanceObservability{}, err
	}
	failed, err := query.Table((models.TenantGovernanceRun{}).TableName()).Where("kind", models.TenantGovernanceRunKindIsolationVerify).
		Where("status", models.TenantGovernanceRunStatusFailed).Count()
	if err != nil {
		return TenantGovernanceObservability{}, err
	}
	var oldest *time.Time
	err = query.Table((models.TenantGovernanceRun{}).TableName()).WhereIn("status", []any{
		models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusAwaitingEvidence,
	}).OrderBy("created_at").Limit(1).Pluck("created_at", &oldest)
	age := time.Duration(0)
	if err == nil && oldest != nil {
		age = now.UTC().Sub(oldest.UTC())
	}
	return TenantGovernanceObservability{EvidenceExpired: expired, VerificationFailed: failed, OldestRunAge: age}, err
}
