package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/models"
)

const auditPruneBatchSize = 100
const auditPruneLeaseTimeout = 5 * time.Minute

var (
	ErrAuditPruneEvidenceRequired = errors.New("audit prune requires sensitive evidence")
	ErrAuditPrunePartialRun       = errors.New("audit prune plan is partially executed and cannot continue")
	ErrAuditPruneTargetChanged    = errors.New("audit prune target no longer matches the persisted plan")
)

type AuditPruneEvidence struct {
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

type AuditPruneExecutionResult struct {
	PlanID    string `json:"plan_id"`
	Status    string `json:"status"`
	Completed int64  `json:"completed"`
	NoOp      int64  `json:"no_op"`
	Failed    int64  `json:"failed"`
}

type AuditPruneExecutor struct {
	ctx    context.Context
	now    func() time.Time
	plans  *AuditPrunePlanService
	proofs *AuditPruneProofVerifier
}

func NewAuditPruneExecutor() *AuditPruneExecutor {
	return &AuditPruneExecutor{
		now:    time.Now,
		plans:  NewAuditPrunePlanService(),
		proofs: NewAuditPruneProofVerifier(),
	}
}

func (e *AuditPruneExecutor) WithContext(ctx context.Context) *AuditPruneExecutor {
	clone := *e
	clone.ctx = contextOrBackground(ctx)
	clone.plans = clone.plans.WithContext(ctx)
	return &clone
}

func AuditPruneResource(plan AuditPrunePlan) string {
	return "audit-prune:" + plan.PlanID + ":" + plan.TargetDigest
}

func (e *AuditPruneExecutor) Execute(planID string, proof AuditPruneWORMProof, evidence AuditPruneEvidence) (AuditPruneExecutionResult, error) {
	if e == nil || e.plans == nil || e.proofs == nil {
		return AuditPruneExecutionResult{}, ErrAuditPrunePlanNotExecutable
	}
	if strings.TrimSpace(evidence.ReAuthToken) == "" || strings.TrimSpace(evidence.ApprovalID) == "" {
		return AuditPruneExecutionResult{}, ErrAuditPruneEvidenceRequired
	}
	if err := e.recoverAbandonedRun(planID); err != nil {
		return AuditPruneExecutionResult{}, err
	}
	plan, err := e.plans.Load(planID)
	if err != nil {
		return AuditPruneExecutionResult{}, err
	}
	if err := validateAuditPrunePlanForExecution(plan); err != nil {
		return AuditPruneExecutionResult{}, err
	}
	attestation, err := e.proofs.VerifyAttested(plan, proof)
	if err != nil {
		return AuditPruneExecutionResult{}, err
	}
	proof.ImmutableUntil = attestation.ImmutableUntil
	proof.VerifiedAt = attestation.VerifiedAt
	result := AuditPruneExecutionResult{PlanID: plan.PlanID}
	execute := func() error {
		var executeErr error
		result, executeErr = e.executeVerified(plan, proof)
		return executeErr
	}
	err = e.executeSensitive(contextOrBackground(e.ctx), plan, evidence, execute)
	if err != nil {
		if result.Status != "" {
			_ = NewTenantGovernanceRunRepository().WithContext(e.ctx).FinishRetentionPlan(plan.PlanID, models.TenantGovernanceRunStatusFailed, err.Error())
		}
		return result, err
	}
	if err := NewTenantGovernanceRunRepository().WithContext(e.ctx).FinishRetentionPlan(plan.PlanID, models.TenantGovernanceRunStatusCompleted, ""); err != nil {
		return result, err
	}
	return result, nil
}

func (e *AuditPruneExecutor) executeVerified(plan AuditPrunePlan, proof AuditPruneWORMProof) (AuditPruneExecutionResult, error) {
	query := OrmForConnectionWithContext(e.ctx, PlatformConnection()).Query()
	executionID, err := newAuditPrunePlanID()
	if err != nil {
		return AuditPruneExecutionResult{PlanID: plan.PlanID}, err
	}
	now := e.now().UTC()
	claimed, err := query.Table((models.SecurityAuditPruneRun{}).TableName()).
		Where("id", plan.RunID).
		Where("status", models.SecurityAuditPruneStatusPlanned).
		Update(map[string]any{
			"status":            models.SecurityAuditPruneStatusRunning,
			"execution_id":      executionID,
			"heartbeat_at":      now,
			"archive_uri":       proof.ArchiveURI,
			"object_version":    proof.ObjectVersion,
			"manifest_sha256":   proof.ManifestSHA256,
			"immutable_until":   proof.ImmutableUntil,
			"proof_verified_at": proof.VerifiedAt,
			"started_at":        now,
			"updated_at":        now,
		})
	if err != nil {
		return AuditPruneExecutionResult{PlanID: plan.PlanID}, err
	}
	if claimed.RowsAffected != 1 {
		return AuditPruneExecutionResult{PlanID: plan.PlanID}, ErrAuditPrunePartialRun
	}

	result := AuditPruneExecutionResult{PlanID: plan.PlanID}
	for start := 0; start < len(plan.Targets); start += auditPruneBatchSize {
		end := start + auditPruneBatchSize
		if end > len(plan.Targets) {
			end = len(plan.Targets)
		}
		for _, target := range plan.Targets[start:end] {
			status, err := e.executeTarget(plan.RunID, executionID, target)
			switch status {
			case models.SecurityAuditPruneTargetStatusCompleted:
				result.Completed++
			case models.SecurityAuditPruneTargetStatusNoOp:
				result.NoOp++
			case models.SecurityAuditPruneTargetStatusFailed:
				result.Failed++
			}
			if err != nil {
				continue
			}
		}
	}
	result.Status = auditPruneRunStatus(result, plan.TargetCount)
	finishedAt := e.now().UTC()
	updateErr := updateAuditPruneRun(query, plan.RunID, executionID, map[string]any{
		"status":       result.Status,
		"heartbeat_at": finishedAt,
		"finished_at":  finishedAt,
		"error":        auditPruneResultError(result),
		"updated_at":   finishedAt,
	})
	if updateErr != nil {
		return result, updateErr
	}
	if result.Status == models.SecurityAuditPruneStatusFailed || result.Status == models.SecurityAuditPruneStatusPartiallyExecuted {
		return result, fmt.Errorf("audit prune %s", result.Status)
	}
	return result, nil
}

func (e *AuditPruneExecutor) executeTarget(runID uint64, executionID string, target models.SecurityAuditPruneTarget) (string, error) {
	if err := e.registerAndVerifyTargetConnection(target); err != nil {
		return models.SecurityAuditPruneTargetStatusFailed, err
	}
	now := e.now().UTC()
	if err := e.heartbeatRun(runID, executionID, now); err != nil {
		return models.SecurityAuditPruneTargetStatusFailed, err
	}
	claimed, err := OrmForConnectionWithContext(e.ctx, PlatformConnection()).Query().
		Table((models.SecurityAuditPruneTarget{}).TableName()).
		Where("id", target.ID).
		Where("status", models.SecurityAuditPruneStatusPlanned).
		Update(map[string]any{"status": models.SecurityAuditPruneStatusRunning, "updated_at": now})
	if err != nil {
		return models.SecurityAuditPruneTargetStatusFailed, err
	}
	if claimed.RowsAffected != 1 {
		return models.SecurityAuditPruneTargetStatusFailed, ErrAuditPrunePartialRun
	}
	status, err := deleteExactAuditPruneTarget(e.ctx, target)
	values := map[string]any{"status": status, "processed_at": now, "updated_at": now}
	if err != nil {
		values["status"] = models.SecurityAuditPruneTargetStatusFailed
		values["error"] = err.Error()
		status = models.SecurityAuditPruneTargetStatusFailed
	}
	_, updateErr := OrmForConnectionWithContext(e.ctx, PlatformConnection()).Query().
		Table((models.SecurityAuditPruneTarget{}).TableName()).
		Where("id", target.ID).
		Where("status", models.SecurityAuditPruneStatusRunning).
		Update(values)
	if updateErr != nil {
		return models.SecurityAuditPruneTargetStatusFailed, updateErr
	}
	return status, err
}

func (e *AuditPruneExecutor) heartbeatRun(runID uint64, executionID string, now time.Time) error {
	result, err := OrmForConnectionWithContext(e.ctx, PlatformConnection()).Query().
		Table((models.SecurityAuditPruneRun{}).TableName()).
		Where("id", runID).
		Where("status", models.SecurityAuditPruneStatusRunning).
		Where("execution_id", executionID).
		Update(map[string]any{"heartbeat_at": now, "updated_at": now})
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrAuditPrunePartialRun
	}
	return nil
}

func (e *AuditPruneExecutor) recoverAbandonedRun(planID string) error {
	now := e.now().UTC()
	_, err := OrmForConnectionWithContext(e.ctx, PlatformConnection()).Query().
		Table((models.SecurityAuditPruneRun{}).TableName()).
		Where("plan_id", strings.TrimSpace(planID)).
		Where("status", models.SecurityAuditPruneStatusRunning).
		Where("heartbeat_at < ?", now.Add(-auditPruneLeaseTimeout)).
		Update(map[string]any{
			"status": models.SecurityAuditPruneStatusPartiallyExecuted, "finished_at": now,
			"error": "execution lease expired; manual evidence review required", "updated_at": now,
		})
	return err
}

func (e *AuditPruneExecutor) registerAndVerifyTargetConnection(target models.SecurityAuditPruneTarget) error {
	if target.TenantID == 0 {
		if target.Connection != PlatformConnection() || target.DatabaseDigest != "" || target.TenantCode != "" {
			return ErrAuditPruneTargetChanged
		}
		return nil
	}
	var tenant Tenant
	err := OrmForConnectionWithContext(e.ctx, PlatformConnection()).Query().Table("tenant").Where("id", target.TenantID).First(&tenant)
	if err != nil || tenant.ID == 0 || tenant.Code != target.TenantCode || TenantConnectionName(tenant) != target.Connection ||
		target.DatabaseDigest == "" || !sameDigest(tenantDatabaseDigest(tenant), target.DatabaseDigest) {
		return ErrAuditPruneTargetChanged
	}
	RegisterTenantConnection(tenant)
	return nil
}

func deleteExactAuditPruneTarget(ctx context.Context, target models.SecurityAuditPruneTarget) (string, error) {
	status := models.SecurityAuditPruneTargetStatusNoOp
	err := OrmForConnectionWithContext(ctx, target.Connection).Transaction(func(query contractsorm.Query) error {
		record, found, err := loadAuditPruneRecord(query, target.AuditTable, target.TargetID, true)
		if err != nil || !found {
			return err
		}
		digest, err := auditPruneRecordDigest(record)
		if err != nil {
			return err
		}
		if !sameDigest(digest, target.RecordDigest) {
			return ErrAuditPruneTargetChanged
		}
		result, err := query.Table(target.AuditTable).
			Where("id", target.TargetID).
			Where(target.TimestampColumn+" < ?", target.Cutoff).
			Where(target.TimestampColumn+" = ?", target.OccurredAt).
			Delete()
		if err != nil {
			return err
		}
		if result.RowsAffected == 1 {
			status = models.SecurityAuditPruneTargetStatusCompleted
		}
		return nil
	})
	if err != nil {
		return models.SecurityAuditPruneTargetStatusFailed, err
	}
	return status, nil
}

func validateAuditPrunePlanForExecution(plan AuditPrunePlan) error {
	if strings.TrimSpace(plan.PlanID) == "" || !isSHA256(plan.TargetDigest) || plan.Status != models.SecurityAuditPruneStatusPlanned {
		return ErrAuditPrunePlanNotExecutable
	}
	if plan.TargetDigest != auditPruneTargetDigest(plan.Targets) || plan.TargetCount != int64(len(plan.Targets)) {
		return ErrAuditPruneTargetChanged
	}
	for _, target := range plan.Targets {
		if !isSHA256(target.RecordDigest) {
			return ErrAuditPruneTargetChanged
		}
		if target.Status == models.SecurityAuditPruneTargetStatusCompleted || target.Status == models.SecurityAuditPruneTargetStatusNoOp || target.Status == models.SecurityAuditPruneTargetStatusFailed {
			return ErrAuditPrunePartialRun
		}
		if target.Status != models.SecurityAuditPruneStatusPlanned {
			return ErrAuditPrunePlanNotExecutable
		}
	}
	return nil
}

func auditPruneRunStatus(result AuditPruneExecutionResult, targetCount int64) string {
	if targetCount == 0 || result.NoOp == targetCount {
		return models.SecurityAuditPruneStatusNoOp
	}
	if result.Failed > 0 && result.Completed+result.NoOp > 0 {
		return models.SecurityAuditPruneStatusPartiallyExecuted
	}
	if result.Failed > 0 {
		return models.SecurityAuditPruneStatusFailed
	}
	return models.SecurityAuditPruneStatusCompleted
}

func auditPruneResultError(result AuditPruneExecutionResult) string {
	if result.Failed == 0 {
		return ""
	}
	payload, _ := json.Marshal(map[string]int64{"completed": result.Completed, "no_op": result.NoOp, "failed": result.Failed})
	return string(payload)
}

func (e *AuditPruneExecutor) executeSensitive(ctx context.Context, prunePlan AuditPrunePlan, evidence AuditPruneEvidence, mutate func() error) error {
	security := NewEnterpriseSecurityControlService()
	approval, ok := security.memoryApproval(evidence.ApprovalID)
	if !ok {
		var err error
		approval, ok, err = security.loadPermissionApproval(ctx, evidence.ApprovalID)
		if err != nil {
			return err
		}
	}
	if !ok || approval.RequesterID == 0 || approval.TenantID != 0 {
		return ErrApprovalRequired
	}
	guard := NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(ctx, "audit.prune.execute", approval.RequesterID, 0, SensitiveOperationPlanSelector{
		Resource: AuditPruneResource(prunePlan),
	})
	if err != nil {
		return err
	}
	return guard.Execute(ctx, plan, SensitiveOperationEvidence{
		ReAuthToken: evidence.ReAuthToken,
		ApprovalID:  evidence.ApprovalID,
	}, mutate)
}

func updateAuditPruneRun(query contractsorm.Query, runID uint64, executionID string, values map[string]any) error {
	result, err := query.Table((models.SecurityAuditPruneRun{}).TableName()).Where("id", runID).
		Where("status", models.SecurityAuditPruneStatusRunning).Where("execution_id", executionID).Update(values)
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrAuditPrunePartialRun
	}
	return nil
}
