package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	frameworkerrors "github.com/goravel/framework/errors"

	"goravel/app/models"
)

var (
	ErrTenantGovernanceRunInvalidTransition = errors.New("tenant governance run transition is invalid")
	ErrTenantGovernanceEvidenceExpired      = errors.New("tenant governance evidence is expired")
	ErrTenantGovernanceEvidenceExists       = errors.New("tenant governance evidence already exists")
)

type TenantGovernanceRunCreate struct {
	TenantID       uint64
	TenantCode     string
	Kind           string
	IdempotencyKey string
	PolicyVersion  string
}

type TenantGovernanceEvidenceInput struct {
	URI           string
	ObjectVersion string
	SHA256        string
	VerifiedAt    time.Time
	ExpiresAt     time.Time
	Metadata      map[string]any
}

type TenantGovernanceRunRepository struct {
	ctx context.Context
	now func() time.Time
}

func NewTenantGovernanceRunRepository() *TenantGovernanceRunRepository {
	return &TenantGovernanceRunRepository{now: time.Now}
}

func (r *TenantGovernanceRunRepository) WithContext(ctx context.Context) *TenantGovernanceRunRepository {
	clone := *r
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (r *TenantGovernanceRunRepository) CreateOrGetRun(input TenantGovernanceRunCreate) (models.TenantGovernanceRun, bool, error) {
	input.TenantCode = strings.TrimSpace(input.TenantCode)
	input.Kind = strings.TrimSpace(input.Kind)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.TenantID == 0 || input.TenantCode == "" || !tenantGovernanceRunKindValid(input.Kind) || input.IdempotencyKey == "" {
		return models.TenantGovernanceRun{}, false, ErrTenantGovernanceRunInvalidTransition
	}
	query := r.query()
	var existing models.TenantGovernanceRun
	err := query.Table(existing.TableName()).Where("tenant_id", input.TenantID).Where("kind", input.Kind).
		Where("idempotency_key", input.IdempotencyKey).First(&existing)
	if err == nil && existing.ID != 0 {
		return existing, false, nil
	}
	if err != nil && !errors.Is(err, frameworkerrors.OrmRecordNotFound) {
		return models.TenantGovernanceRun{}, false, err
	}
	now := r.now().UTC()
	run := models.TenantGovernanceRun{
		TenantID: input.TenantID, TenantCode: input.TenantCode, Kind: input.Kind,
		IdempotencyKey: input.IdempotencyKey, PolicyVersion: input.PolicyVersion,
		Status:     models.TenantGovernanceRunStatusPending,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}
	if err := query.Table(run.TableName()).Create(&run); err != nil {
		if loadErr := query.Table(run.TableName()).Where("tenant_id", input.TenantID).Where("kind", input.Kind).
			Where("idempotency_key", input.IdempotencyKey).First(&existing); loadErr == nil && existing.ID != 0 {
			return existing, false, nil
		}
		return models.TenantGovernanceRun{}, false, err
	}
	return run, true, nil
}

func (r *TenantGovernanceRunRepository) Transition(runID uint64, from, to, message string) error {
	if runID == 0 || !tenantGovernanceTransitionAllowed(from, to) {
		return ErrTenantGovernanceRunInvalidTransition
	}
	now := r.now().UTC()
	values := map[string]any{"status": to, "updated_at": now}
	if to == models.TenantGovernanceRunStatusRunning {
		values["started_at"] = now
	}
	if tenantGovernanceRunTerminal(to) {
		values["finished_at"] = now
	}
	if strings.TrimSpace(message) != "" {
		values["error"] = strings.TrimSpace(message)
	}
	result, err := r.query().Table((models.TenantGovernanceRun{}).TableName()).Where("id", runID).Where("status", from).Update(values)
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrTenantGovernanceRunInvalidTransition
	}
	return nil
}

func (r *TenantGovernanceRunRepository) AwaitRetentionEvidence(runID uint64, planID string) error {
	planID = strings.TrimSpace(planID)
	if runID == 0 || planID == "" {
		return ErrTenantGovernanceRunInvalidTransition
	}
	now := r.now().UTC()
	result, err := r.query().Table((models.TenantGovernanceRun{}).TableName()).Where("id", runID).
		Where("kind", models.TenantGovernanceRunKindRetention).Where("status", models.TenantGovernanceRunStatusRunning).
		Update(map[string]any{"plan_id": planID, "status": models.TenantGovernanceRunStatusAwaitingEvidence, "error": "", "updated_at": now})
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrTenantGovernanceRunInvalidTransition
	}
	return nil
}

func (r *TenantGovernanceRunRepository) FinishRetentionPlan(planID, status, message string) error {
	planID = strings.TrimSpace(planID)
	if planID == "" || (status != models.TenantGovernanceRunStatusCompleted && status != models.TenantGovernanceRunStatusFailed) {
		return ErrTenantGovernanceRunInvalidTransition
	}
	var run models.TenantGovernanceRun
	err := r.query().Table(run.TableName()).Where("kind", models.TenantGovernanceRunKindRetention).
		Where("plan_id", planID).First(&run)
	if errors.Is(err, frameworkerrors.OrmRecordNotFound) || run.ID == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	return r.Transition(run.ID, models.TenantGovernanceRunStatusAwaitingEvidence, status, message)
}

func (r *TenantGovernanceRunRepository) AttachEvidence(run models.TenantGovernanceRun, input TenantGovernanceEvidenceInput) (models.TenantGovernanceEvidence, error) {
	if run.ID == 0 || run.TenantID == 0 || strings.TrimSpace(input.URI) == "" || strings.TrimSpace(input.ObjectVersion) == "" ||
		!isSHA256(input.SHA256) || input.VerifiedAt.IsZero() || !input.ExpiresAt.After(input.VerifiedAt) {
		return models.TenantGovernanceEvidence{}, ErrImmutableEvidenceInvalid
	}
	metadata, err := json.Marshal(input.Metadata)
	if err != nil {
		return models.TenantGovernanceEvidence{}, err
	}
	evidence := models.TenantGovernanceEvidence{
		RunID: run.ID, TenantID: run.TenantID, Kind: run.Kind, URI: input.URI,
		ObjectVersion: input.ObjectVersion, SHA256: input.SHA256,
		VerifiedAt: input.VerifiedAt.UTC(), ExpiresAt: input.ExpiresAt.UTC(), Metadata: string(metadata),
		Timestamps: models.Timestamps{CreatedAt: r.now().UTC(), UpdatedAt: r.now().UTC()},
	}
	err = r.orm().Transaction(func(query contractsorm.Query) error {
		var count int64
		var countErr error
		count, countErr = query.Table(evidence.TableName()).Where("run_id", run.ID).Count()
		if countErr != nil {
			return countErr
		}
		if count != 0 {
			return ErrTenantGovernanceEvidenceExists
		}
		if err := query.Table(evidence.TableName()).Create(&evidence); err != nil {
			return err
		}
		result, err := query.Table(run.TableName()).Where("id", run.ID).Where("status", models.TenantGovernanceRunStatusRunning).
			Update(map[string]any{"status": models.TenantGovernanceRunStatusArtifactWritten, "updated_at": r.now().UTC()})
		if err != nil {
			return err
		}
		if result.RowsAffected != 1 {
			return ErrTenantGovernanceRunInvalidTransition
		}
		return nil
	})
	return evidence, err
}

func (r *TenantGovernanceRunRepository) CurrentEvidence(tenantID uint64, kind string) (models.TenantGovernanceEvidence, error) {
	var evidence models.TenantGovernanceEvidence
	err := r.query().Table(evidence.TableName()).Select("tenant_governance_evidence.*").
		Join("JOIN tenant_governance_run ON tenant_governance_run.id = tenant_governance_evidence.run_id").
		Where("tenant_governance_evidence.tenant_id", tenantID).Where("tenant_governance_evidence.kind", kind).
		Where("tenant_governance_run.status", models.TenantGovernanceRunStatusCompleted).
		WhereNull("tenant_governance_evidence.stale_at").Where("tenant_governance_evidence.expires_at > ?", r.now().UTC()).
		OrderByDesc("tenant_governance_evidence.verified_at").First(&evidence)
	if errors.Is(err, frameworkerrors.OrmRecordNotFound) || evidence.ID == 0 {
		return models.TenantGovernanceEvidence{}, ErrTenantGovernanceEvidenceExpired
	}
	if err != nil {
		return models.TenantGovernanceEvidence{}, err
	}
	return evidence, nil
}

func (r *TenantGovernanceRunRepository) MarkStale(now time.Time) (int64, error) {
	now = now.UTC()
	result, err := r.query().Table((models.TenantGovernanceEvidence{}).TableName()).WhereNull("stale_at").Where("expires_at <= ?", now).
		Update(map[string]any{"stale_at": now, "updated_at": now})
	if err != nil {
		return 0, err
	}
	runIDs, err := r.expiredRunIDs(now)
	if err != nil {
		return 0, err
	}
	if len(runIDs) > 0 {
		if _, err := r.query().Table((models.TenantGovernanceRun{}).TableName()).Where("status", models.TenantGovernanceRunStatusCompleted).
			WhereIn("id", runIDs).Update(map[string]any{"status": models.TenantGovernanceRunStatusStale, "updated_at": now}); err != nil {
			return 0, err
		}
	}
	return result.RowsAffected, nil
}

func (r *TenantGovernanceRunRepository) expiredRunIDs(now time.Time) ([]any, error) {
	rows := make([]models.TenantGovernanceEvidence, 0)
	if err := r.query().Table((models.TenantGovernanceEvidence{}).TableName()).Select("run_id").Where("expires_at <= ?", now).Get(&rows); err != nil {
		return nil, err
	}
	ids := make([]any, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RunID)
	}
	return ids, nil
}

func (r *TenantGovernanceRunRepository) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(r.ctx, PlatformConnection())
}

func (r *TenantGovernanceRunRepository) query() contractsorm.Query { return r.orm().Query() }

func tenantGovernanceRunKindValid(kind string) bool {
	return kind == models.TenantGovernanceRunKindRetention || kind == models.TenantGovernanceRunKindExport || kind == models.TenantGovernanceRunKindIsolationVerify
}

func tenantGovernanceTransitionAllowed(from, to string) bool {
	allowed := map[string]map[string]bool{
		models.TenantGovernanceRunStatusPending:          {models.TenantGovernanceRunStatusRunning: true, models.TenantGovernanceRunStatusFailed: true},
		models.TenantGovernanceRunStatusRunning:          {models.TenantGovernanceRunStatusAwaitingEvidence: true, models.TenantGovernanceRunStatusArtifactWritten: true, models.TenantGovernanceRunStatusCompleted: true, models.TenantGovernanceRunStatusFailed: true},
		models.TenantGovernanceRunStatusAwaitingEvidence: {models.TenantGovernanceRunStatusCompleted: true, models.TenantGovernanceRunStatusFailed: true},
		models.TenantGovernanceRunStatusArtifactWritten:  {models.TenantGovernanceRunStatusCompleted: true, models.TenantGovernanceRunStatusFailed: true},
		models.TenantGovernanceRunStatusCompleted:        {models.TenantGovernanceRunStatusStale: true},
	}
	return allowed[from][to]
}

func tenantGovernanceRunTerminal(status string) bool {
	return status == models.TenantGovernanceRunStatusCompleted || status == models.TenantGovernanceRunStatusFailed || status == models.TenantGovernanceRunStatusStale
}
