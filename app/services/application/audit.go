package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"goravel/app/facades"
	"goravel/app/models"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Source: audit_prune_executor.go
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

// Source: audit_prune_plan_service.go
const auditPruneDefaultRetentionDays = 180

var (
	ErrAuditPrunePlanNotFound      = errors.New("audit prune plan was not found")
	ErrAuditPruneScopeInvalid      = errors.New("audit prune scope is invalid")
	ErrAuditPruneExecutionRequired = errors.New("audit prune requires explicit --execute")
	ErrAuditPrunePlanNotExecutable = errors.New("audit prune plan cannot be executed")
)

type AuditPrunePlanOptions struct {
	Scope         string
	RetentionDays int
}

type AuditPrunePlan struct {
	RunID         uint64                            `json:"run_id"`
	PlanID        string                            `json:"plan_id"`
	Scope         string                            `json:"scope"`
	RetentionDays int                               `json:"retention_days"`
	Cutoff        time.Time                         `json:"cutoff"`
	TargetDigest  string                            `json:"target_digest"`
	TargetCount   int64                             `json:"target_count"`
	Status        string                            `json:"status"`
	ScopeCounts   map[string]int64                  `json:"scope_counts"`
	TableCounts   map[string]int64                  `json:"table_counts"`
	MinTimestamp  *time.Time                        `json:"min_timestamp,omitempty"`
	MaxTimestamp  *time.Time                        `json:"max_timestamp,omitempty"`
	Targets       []models.SecurityAuditPruneTarget `json:"targets"`
}

func (p AuditPrunePlan) MinTimestampOrCutoff() time.Time {
	if p.MinTimestamp == nil {
		return p.Cutoff
	}
	return *p.MinTimestamp
}

func (p AuditPrunePlan) MaxTimestampOrCutoff() time.Time {
	if p.MaxTimestamp == nil {
		return p.Cutoff
	}
	return *p.MaxTimestamp
}

type auditPruneScope struct {
	Name           string
	Connection     string
	TenantID       uint64
	TenantCode     string
	DatabaseDigest string
	RetentionDays  int
}

type auditPruneCandidate struct {
	ID         uint64    `gorm:"column:id"`
	OccurredAt time.Time `gorm:"column:occurred_at"`
	RecordJSON string    `gorm:"column:record_json"`
}

type AuditPrunePlanService struct {
	ctx       context.Context
	now       func() time.Time
	id        func() (string, error)
	retention func(Tenant, int) (int, error)
}

func NewAuditPrunePlanService() *AuditPrunePlanService {
	return &AuditPrunePlanService{
		now:       time.Now,
		id:        newAuditPrunePlanID,
		retention: tenantAuditRetentionDays,
	}
}

func (s *AuditPrunePlanService) WithContext(ctx context.Context) *AuditPrunePlanService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *AuditPrunePlanService) Create(options AuditPrunePlanOptions) (AuditPrunePlan, error) {
	if s == nil {
		return AuditPrunePlan{}, ErrAuditPruneScopeInvalid
	}
	scopes, scope, err := s.resolveScopes(options)
	if err != nil {
		return AuditPrunePlan{}, err
	}
	planID, err := s.id()
	if err != nil {
		return AuditPrunePlan{}, err
	}
	cutoff := s.now().UTC()
	plan := AuditPrunePlan{PlanID: planID, Scope: scope, Cutoff: cutoff, Status: models.SecurityAuditPruneStatusPlanned, ScopeCounts: map[string]int64{}, TableCounts: map[string]int64{}}
	targets, err := s.collectTargets(scopes, cutoff)
	if err != nil {
		return AuditPrunePlan{}, err
	}
	plan.Targets = targets
	if len(scopes) == 1 {
		plan.RetentionDays = scopes[0].RetentionDays
	} else {
		plan.RetentionDays = normalizedAuditRetentionDays(options.RetentionDays)
	}
	plan.TargetCount = int64(len(targets))
	plan.TargetDigest = auditPruneTargetDigest(targets)
	plan.ScopeCounts, plan.TableCounts, plan.MinTimestamp, plan.MaxTimestamp = auditPruneSummary(targets)

	run := models.SecurityAuditPruneRun{
		PlanID: plan.PlanID, Scope: plan.Scope, RetentionDays: plan.RetentionDays, Cutoff: plan.Cutoff,
		TargetDigest: plan.TargetDigest, TargetCount: plan.TargetCount,
		ScopeCounts: int64MapJSON(plan.ScopeCounts), TableCounts: int64MapJSON(plan.TableCounts),
		MinTimestamp: plan.MinTimestamp, MaxTimestamp: plan.MaxTimestamp,
		Status:     models.SecurityAuditPruneStatusPlanned,
		Timestamps: models.Timestamps{CreatedAt: cutoff, UpdatedAt: cutoff},
	}
	orm := OrmForConnectionWithContext(s.ctx, PlatformConnection())
	if err := orm.Transaction(func(query contractsorm.Query) error {
		if err := query.Table(run.TableName()).Create(&run); err != nil {
			return err
		}
		plan.RunID = run.ID
		for index := range plan.Targets {
			plan.Targets[index].RunID = run.ID
			plan.Targets[index].Timestamps = models.Timestamps{CreatedAt: cutoff, UpdatedAt: cutoff}
			if err := query.Table(plan.Targets[index].TableName()).Create(&plan.Targets[index]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return AuditPrunePlan{}, err
	}
	return plan, nil
}

func (s *AuditPrunePlanService) Load(planID string) (AuditPrunePlan, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return AuditPrunePlan{}, ErrAuditPrunePlanNotFound
	}
	query := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query()
	var run models.SecurityAuditPruneRun
	if err := query.Table(run.TableName()).Where("plan_id", planID).First(&run); err != nil {
		return AuditPrunePlan{}, err
	}
	if run.ID == 0 {
		return AuditPrunePlan{}, ErrAuditPrunePlanNotFound
	}
	targets := make([]models.SecurityAuditPruneTarget, 0)
	if err := query.Table((models.SecurityAuditPruneTarget{}).TableName()).Where("run_id", run.ID).OrderBy("id").Get(&targets); err != nil {
		return AuditPrunePlan{}, err
	}
	return AuditPrunePlan{
		RunID: run.ID, PlanID: run.PlanID, Scope: run.Scope, RetentionDays: run.RetentionDays, Cutoff: run.Cutoff,
		TargetDigest: run.TargetDigest, TargetCount: run.TargetCount, ScopeCounts: parseInt64MapJSON(run.ScopeCounts),
		Status:      run.Status,
		TableCounts: parseInt64MapJSON(run.TableCounts), MinTimestamp: run.MinTimestamp, MaxTimestamp: run.MaxTimestamp,
		Targets: targets,
	}, nil
}

func (s *AuditPrunePlanService) resolveScopes(options AuditPrunePlanOptions) ([]auditPruneScope, string, error) {
	scope := strings.TrimSpace(options.Scope)
	if scope == "" {
		scope = "all"
	}
	retentionDays := normalizedAuditRetentionDays(options.RetentionDays)
	platform := auditPruneScope{Name: "platform", Connection: PlatformConnection(), RetentionDays: retentionDays}
	switch {
	case scope == "platform":
		return []auditPruneScope{platform}, scope, nil
	case scope == "all":
		scopes := []auditPruneScope{platform}
		tenants := make([]Tenant, 0)
		if err := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query().Table("tenant").Get(&tenants); err != nil {
			return nil, "", err
		}
		for _, tenant := range tenants {
			days, err := s.retention(tenant, retentionDays)
			if err != nil {
				return nil, "", err
			}
			RegisterTenantConnection(tenant)
			scopes = append(scopes, auditPruneScope{Name: "tenant:" + tenant.Code, Connection: TenantConnectionName(tenant), TenantID: tenant.ID, TenantCode: tenant.Code, DatabaseDigest: tenantDatabaseDigest(tenant), RetentionDays: days})
		}
		return scopes, scope, nil
	case strings.HasPrefix(scope, "tenant:"):
		code := strings.TrimSpace(strings.TrimPrefix(scope, "tenant:"))
		if code == "" {
			return nil, "", ErrAuditPruneScopeInvalid
		}
		var tenant Tenant
		if err := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query().Table("tenant").Where("code", code).First(&tenant); err != nil {
			return nil, "", err
		}
		if tenant.ID == 0 {
			return nil, "", ErrAuditPruneScopeInvalid
		}
		days, err := s.retention(tenant, retentionDays)
		if err != nil {
			return nil, "", err
		}
		RegisterTenantConnection(tenant)
		return []auditPruneScope{{Name: "tenant:" + tenant.Code, Connection: TenantConnectionName(tenant), TenantID: tenant.ID, TenantCode: tenant.Code, DatabaseDigest: tenantDatabaseDigest(tenant), RetentionDays: days}}, "tenant:" + tenant.Code, nil
	default:
		return nil, "", ErrAuditPruneScopeInvalid
	}
}

func (s *AuditPrunePlanService) collectTargets(scopes []auditPruneScope, plannedAt time.Time) ([]models.SecurityAuditPruneTarget, error) {
	targets := make([]models.SecurityAuditPruneTarget, 0)
	for _, scope := range scopes {
		cutoff := plannedAt.AddDate(0, 0, -scope.RetentionDays)
		retention := NewAuditRetentionService(scope.Connection).WithContext(s.ctx)
		for _, definition := range retention.Targets() {
			rows := make([]auditPruneCandidate, 0)
			err := OrmForConnectionWithContext(s.ctx, scope.Connection).Query().Table(definition.Table).
				Select("id", definition.Column+" AS occurred_at", auditPruneRecordSelect(definition.Table)).
				Where(definition.Column+" < ?", cutoff).
				OrderBy("id").
				Get(&rows)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				record, err := canonicalAuditPruneRecord([]byte(row.RecordJSON))
				if err != nil {
					return nil, err
				}
				targets = append(targets, models.SecurityAuditPruneTarget{
					Scope: scope.Name, Connection: scope.Connection, TenantID: scope.TenantID, TenantCode: scope.TenantCode, DatabaseDigest: scope.DatabaseDigest,
					AuditTable: definition.Table, TimestampColumn: definition.Column, TargetID: row.ID, OccurredAt: row.OccurredAt,
					RecordDigest: digestBytes(record), Record: record,
					Cutoff: cutoff, RetentionDays: scope.RetentionDays, Status: models.SecurityAuditPruneStatusPlanned,
				})
			}
		}
	}
	sortAuditPruneTargets(targets)
	return targets, nil
}

func tenantAuditRetentionDays(tenant Tenant, fallback int) (int, error) {
	governance := NewTenantGovernanceService()
	policy, found, err := governance.loadPolicy(tenant.ID)
	if err != nil {
		return 0, err
	}
	if found && policy.Retention.AuditDays > 0 {
		return policy.Retention.AuditDays, nil
	}
	return normalizedAuditRetentionDays(fallback), nil
}

func normalizedAuditRetentionDays(days int) int {
	if days > 0 {
		return days
	}
	configured := facades.Config().GetInt("security.audit.retention_days", auditPruneDefaultRetentionDays)
	if configured > 0 {
		return configured
	}
	return auditPruneDefaultRetentionDays
}

func newAuditPrunePlanID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func auditPruneTargetDigest(targets []models.SecurityAuditPruneTarget) string {
	items := append([]models.SecurityAuditPruneTarget(nil), targets...)
	sortAuditPruneTargets(items)
	payload := make([]string, 0, len(items))
	for _, target := range items {
		payload = append(payload, strings.Join([]string{
			target.Scope, target.Connection, strconv.FormatUint(target.TenantID, 10), target.TenantCode, target.DatabaseDigest,
			target.AuditTable, target.TimestampColumn, strconv.FormatUint(target.TargetID, 10), target.OccurredAt.UTC().Format(time.RFC3339Nano), target.RecordDigest,
			target.Cutoff.UTC().Format(time.RFC3339Nano), strconv.Itoa(target.RetentionDays),
		}, "\x00"))
	}
	return digestBytes([]byte(strings.Join(payload, "\n")))
}

func tenantDatabaseDigest(tenant Tenant) string {
	payload := strings.Join([]string{
		strings.TrimSpace(tenant.DBHost), strconv.Itoa(tenant.DBPort), strings.TrimSpace(tenant.DBDatabase),
		strings.TrimSpace(tenant.DBUsername), strings.TrimSpace(tenant.DBSchema),
	}, "\x00")
	return digestBytes([]byte(payload))
}

func sortAuditPruneTargets(items []models.SecurityAuditPruneTarget) {
	sort.Slice(items, func(left, right int) bool {
		leftKey := fmt.Sprintf("%s\x00%s\x00%020d", items[left].Scope, items[left].AuditTable, items[left].TargetID)
		rightKey := fmt.Sprintf("%s\x00%s\x00%020d", items[right].Scope, items[right].AuditTable, items[right].TargetID)
		return leftKey < rightKey
	})
}

func auditPruneSummary(targets []models.SecurityAuditPruneTarget) (map[string]int64, map[string]int64, *time.Time, *time.Time) {
	scopes, tables := map[string]int64{}, map[string]int64{}
	var minimum, maximum *time.Time
	for _, target := range targets {
		scopes[target.Scope]++
		tables[target.Scope+":"+target.AuditTable]++
		occurredAt := target.OccurredAt.UTC()
		if minimum == nil || occurredAt.Before(*minimum) {
			value := occurredAt
			minimum = &value
		}
		if maximum == nil || occurredAt.After(*maximum) {
			value := occurredAt
			maximum = &value
		}
	}
	return scopes, tables, minimum, maximum
}

func int64MapJSON(items map[string]int64) string {
	payload, _ := json.Marshal(items)
	return string(payload)
}

func parseInt64MapJSON(value string) map[string]int64 {
	result := map[string]int64{}
	_ = json.Unmarshal([]byte(value), &result)
	return result
}

// Source: audit_prune_proof_verifier.go
var ErrAuditPruneProofInvalid = errors.New("audit prune proof is invalid")

type AuditPruneWORMProof struct {
	PlanID         string    `json:"plan_id"`
	TargetDigest   string    `json:"target_digest"`
	ArchiveURI     string    `json:"archive_uri"`
	ObjectVersion  string    `json:"object_version"`
	ManifestSHA256 string    `json:"manifest_sha256"`
	WindowFrom     time.Time `json:"window_from"`
	WindowTo       time.Time `json:"window_to"`
	ImmutableUntil time.Time `json:"immutable_until"`
	VerifiedAt     time.Time `json:"verified_at"`
}

type AuditPruneArchiveManifest struct {
	PlanID       string                    `json:"plan_id"`
	TargetDigest string                    `json:"target_digest"`
	WindowFrom   time.Time                 `json:"window_from"`
	WindowTo     time.Time                 `json:"window_to"`
	Records      []AuditPruneArchiveRecord `json:"records"`
}

type AuditPruneArchiveRecord struct {
	Scope        string          `json:"scope"`
	Table        string          `json:"table"`
	TargetID     uint64          `json:"target_id"`
	OccurredAt   time.Time       `json:"occurred_at"`
	Record       json.RawMessage `json:"record"`
	RecordDigest string          `json:"record_digest"`
}

type AuditPruneProofVerifier struct {
	immutable *ImmutableEvidenceVerifier
}

func NewAuditPruneProofVerifier() *AuditPruneProofVerifier {
	return &AuditPruneProofVerifier{immutable: NewImmutableEvidenceVerifier()}
}

func (v *AuditPruneProofVerifier) Verify(plan AuditPrunePlan, proof AuditPruneWORMProof) error {
	_, err := v.VerifyAttested(plan, proof)
	return err
}

func (v *AuditPruneProofVerifier) VerifyAttested(plan AuditPrunePlan, proof AuditPruneWORMProof) (ImmutableEvidenceAttestation, error) {
	if v == nil || v.immutable == nil {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	if strings.TrimSpace(proof.PlanID) != plan.PlanID || !sameDigest(proof.TargetDigest, plan.TargetDigest) {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	if proof.WindowFrom.IsZero() || proof.WindowTo.IsZero() || proof.WindowTo.Before(proof.WindowFrom) {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	if proof.WindowFrom.After(plan.MinTimestampOrCutoff()) || proof.WindowTo.Before(plan.MaxTimestampOrCutoff()) {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	attestation, err := v.immutable.Verify(nil, ImmutableEvidence{
		URI: proof.ArchiveURI, ObjectVersion: proof.ObjectVersion, SHA256: proof.ManifestSHA256,
		ImmutableUntil: proof.ImmutableUntil, VerifiedAt: proof.VerifiedAt,
	})
	if err != nil {
		return ImmutableEvidenceAttestation{}, err
	}
	if !sameDigest(proof.ManifestSHA256, digestBytes(attestation.Manifest)) ||
		!attestation.ImmutableUntil.Equal(proof.ImmutableUntil) {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	var manifest AuditPruneArchiveManifest
	if json.Unmarshal(attestation.Manifest, &manifest) != nil || manifest.PlanID != plan.PlanID ||
		!sameDigest(manifest.TargetDigest, plan.TargetDigest) || !manifest.WindowFrom.Equal(proof.WindowFrom) ||
		!manifest.WindowTo.Equal(proof.WindowTo) || !auditPruneArchiveMatchesPlan(plan, manifest.Records) {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	return attestation, nil
}

func ReadAuditPruneProofFile(path string) (AuditPruneWORMProof, error) {
	payload, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return AuditPruneWORMProof{}, err
	}
	var proof AuditPruneWORMProof
	if err := json.Unmarshal(payload, &proof); err != nil {
		return AuditPruneWORMProof{}, err
	}
	return proof, nil
}

func AuditPruneProofDigest(proof AuditPruneWORMProof) string {
	payload, _ := json.Marshal(proof)
	return digestBytes(payload)
}

func auditPruneManifestJSON(plan AuditPrunePlan, proof AuditPruneWORMProof) []byte {
	payload, _ := json.Marshal(AuditPruneArchiveManifestForPlan(plan, proof.WindowFrom, proof.WindowTo))
	return payload
}

func AuditPruneArchiveManifestForPlan(plan AuditPrunePlan, windowFrom, windowTo time.Time) AuditPruneArchiveManifest {
	if windowFrom.IsZero() {
		windowFrom = plan.MinTimestampOrCutoff()
	}
	if windowTo.IsZero() {
		windowTo = plan.MaxTimestampOrCutoff()
	}
	return AuditPruneArchiveManifest{
		PlanID: plan.PlanID, TargetDigest: plan.TargetDigest,
		WindowFrom: windowFrom, WindowTo: windowTo, Records: auditPruneArchiveRecords(plan.Targets),
	}
}

func auditPruneArchiveRecords(targets []models.SecurityAuditPruneTarget) []AuditPruneArchiveRecord {
	records := make([]AuditPruneArchiveRecord, 0, len(targets))
	for _, target := range targets {
		records = append(records, AuditPruneArchiveRecord{
			Scope: target.Scope, Table: target.AuditTable, TargetID: target.TargetID,
			OccurredAt: target.OccurredAt, Record: target.Record, RecordDigest: target.RecordDigest,
		})
	}
	return records
}

func auditPruneArchiveMatchesPlan(plan AuditPrunePlan, records []AuditPruneArchiveRecord) bool {
	if len(records) != len(plan.Targets) {
		return false
	}
	expected := make(map[string]models.SecurityAuditPruneTarget, len(plan.Targets))
	for _, target := range plan.Targets {
		expected[auditPruneArchiveKey(target.Scope, target.AuditTable, target.TargetID)] = target
	}
	for _, record := range records {
		target, ok := expected[auditPruneArchiveKey(record.Scope, record.Table, record.TargetID)]
		digest, err := auditPruneRecordDigest(record.Record)
		if !ok || err != nil || !record.OccurredAt.Equal(target.OccurredAt) ||
			!sameDigest(record.RecordDigest, target.RecordDigest) || !sameDigest(digest, target.RecordDigest) {
			return false
		}
		delete(expected, auditPruneArchiveKey(record.Scope, record.Table, record.TargetID))
	}
	return len(expected) == 0
}

func auditPruneArchiveKey(scope, table string, targetID uint64) string {
	return scope + "\x00" + table + "\x00" + strconv.FormatUint(targetID, 10)
}

func isSHA256(value string) bool {
	value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sameDigest(left, right string) bool {
	return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(left), "sha256:"), strings.TrimPrefix(strings.TrimSpace(right), "sha256:"))
}

// Source: audit_prune_record.go
type auditPruneRecordRow struct {
	ID         uint64 `gorm:"column:id"`
	RecordJSON string `gorm:"column:record_json"`
}

func auditPruneRecordSelect(table string) string {
	return fmt.Sprintf("to_jsonb(%s.*)::text AS record_json", table)
}

func canonicalAuditPruneRecord(payload []byte) ([]byte, error) {
	var record any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&record); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("audit prune record contains trailing JSON")
	}
	return json.Marshal(record)
}

func auditPruneRecordDigest(payload []byte) (string, error) {
	canonical, err := canonicalAuditPruneRecord(payload)
	if err != nil {
		return "", err
	}
	return digestBytes(canonical), nil
}

func loadAuditPruneRecord(query contractsorm.Query, table string, targetID uint64, lock bool) ([]byte, bool, error) {
	query = query.Table(table).Select("id", auditPruneRecordSelect(table)).Where("id", targetID)
	if lock {
		query = query.LockForUpdate()
	}
	rows := make([]auditPruneRecordRow, 0, 1)
	if err := query.Limit(1).Get(&rows); err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	canonical, err := canonicalAuditPruneRecord([]byte(strings.TrimSpace(rows[0].RecordJSON)))
	return canonical, true, err
}

// Source: audit_retention_service.go
type AuditRetentionService struct {
	ctx        context.Context
	connection string
}

type AuditRetentionResult struct {
	Scope string `json:"scope"`
	Table string `json:"table"`
	Rows  int64  `json:"rows"`
}

type AuditRetentionTarget struct {
	Table  string
	Column string
}

type auditRetentionTarget = AuditRetentionTarget

var auditRetentionHasTable = auditRetentionSchemaHasTable

func NewAuditRetentionService(connection string) *AuditRetentionService {
	return &AuditRetentionService{connection: connection}
}

func (s *AuditRetentionService) WithContext(ctx context.Context) *AuditRetentionService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *AuditRetentionService) Targets() []AuditRetentionTarget {
	return availableAuditRetentionTargets(s.connection, defaultAuditRetentionTargets())
}

func (s *AuditRetentionService) Prune(retentionDays int, dryRun bool) ([]AuditRetentionResult, error) {
	if retentionDays <= 0 {
		return nil, BusinessError{Message: "审计留存天数必须大于 0"}
	}
	if !dryRun {
		return nil, ErrAuditPruneExecutionRequired
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	results := make([]AuditRetentionResult, 0)
	for _, target := range s.Targets() {
		query := OrmForConnectionWithContext(s.ctx, s.connection).Query().
			Table(target.Table).
			Where(target.Column+" < ?", cutoff)
		count, err := query.Count()
		if err != nil {
			return nil, err
		}
		results = append(results, AuditRetentionResult{Scope: s.connection, Table: target.Table, Rows: count})
	}
	return results, nil
}

func PruneAllAuditRetention(ctx context.Context, retentionDays int, dryRun bool) ([]AuditRetentionResult, error) {
	if !dryRun {
		return nil, ErrAuditPruneExecutionRequired
	}
	results := make([]AuditRetentionResult, 0)
	platformConnection := PlatformConnection()
	platformResults, err := NewAuditRetentionService(platformConnection).WithContext(ctx).Prune(retentionDays, true)
	if err != nil {
		return nil, err
	}
	results = append(results, namedAuditRetentionResults("platform", platformResults)...)

	tenants := make([]Tenant, 0)
	if err := OrmForConnectionWithContext(ctx, platformConnection).Query().Table("tenant").Get(&tenants); err != nil {
		return nil, err
	}
	for _, tenant := range tenants {
		RegisterTenantConnection(tenant)
		items, err := NewAuditRetentionService(TenantConnectionName(tenant)).WithContext(ctx).Prune(retentionDays, true)
		if err != nil {
			return nil, err
		}
		results = append(results, namedAuditRetentionResults("tenant:"+tenant.Code, items)...)
	}
	return results, nil
}

func namedAuditRetentionResults(scope string, items []AuditRetentionResult) []AuditRetentionResult {
	for index := range items {
		items[index].Scope = scope
	}
	return items
}

func defaultAuditRetentionTargets() []AuditRetentionTarget {
	return []AuditRetentionTarget{
		{Table: "user_login_log", Column: "login_time"},
		{Table: "user_operation_log", Column: "created_at"},
		{Table: "sso_login_log", Column: "login_at"},
		{Table: "tenant_permission_audit", Column: "created_at"},
	}
}

func availableAuditRetentionTargets(connection string, targets []AuditRetentionTarget) []AuditRetentionTarget {
	available := make([]AuditRetentionTarget, 0, len(targets))
	for _, target := range targets {
		if auditRetentionHasTable(connection, target.Table) {
			available = append(available, target)
		}
	}
	return available
}

func auditRetentionSchemaHasTable(connection, table string) bool {
	previous := facades.Schema().GetConnection()
	facades.Schema().SetConnection(connection)
	defer facades.Schema().SetConnection(previous)
	return facades.Schema().HasTable(table)
}

// Source: immutable_evidence_attestor_s3.go
type s3ImmutableEvidenceAttestor struct{}

const defaultImmutableEvidenceMaxBytes int64 = 64 << 20

func NewS3ImmutableEvidenceAttestor() ImmutableEvidenceAttestor {
	return &s3ImmutableEvidenceAttestor{}
}

func (a *s3ImmutableEvidenceAttestor) Attest(ctx context.Context, evidence ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
	storage, objectPath, err := immutableS3Object(evidence.URI)
	if err != nil {
		return ImmutableEvidenceAttestation{}, err
	}
	config, err := NewStorageConfigService().WithContext(ctx).ActiveDefault()
	if err != nil || config.Driver != storageDriverS3Compatible || config.Bucket != storage {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	client := newObjectStorageClient(config)
	maximum := int64(facades.Config().GetInt("security.audit.immutable_evidence_max_bytes", int(defaultImmutableEvidenceMaxBytes)))
	if maximum <= 0 {
		maximum = defaultImmutableEvidenceMaxBytes
	}
	metadata, err := immutableS3Request(ctx, client, http.MethodHead, objectPath, evidence.ObjectVersion)
	if err != nil {
		return ImmutableEvidenceAttestation{}, err
	}
	defer func() { _ = metadata.Body.Close() }()
	if metadata.ContentLength > maximum {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	if !strings.EqualFold(metadata.Header.Get("X-Amz-Object-Lock-Mode"), "COMPLIANCE") {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	immutableUntil, err := time.Parse(time.RFC3339, metadata.Header.Get("X-Amz-Object-Lock-Retain-Until-Date"))
	if err != nil || metadata.Header.Get("X-Amz-Version-Id") != evidence.ObjectVersion {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	object, err := immutableS3Request(ctx, client, http.MethodGet, objectPath, evidence.ObjectVersion)
	if err != nil {
		return ImmutableEvidenceAttestation{}, err
	}
	defer func() { _ = object.Body.Close() }()
	if object.ContentLength > maximum {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	manifest, err := readImmutableEvidenceBody(object.Body, maximum)
	if err != nil {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	return ImmutableEvidenceAttestation{
		Manifest: manifest, ObjectVersion: evidence.ObjectVersion,
		ImmutableUntil: immutableUntil.UTC(), VerifiedAt: time.Now().UTC(),
	}, nil
}

func readImmutableEvidenceBody(body io.Reader, maximum int64) ([]byte, error) {
	if maximum <= 0 {
		return nil, ErrImmutableEvidenceUnattested
	}
	payload, err := io.ReadAll(io.LimitReader(body, maximum+1))
	if err != nil || len(payload) == 0 || int64(len(payload)) > maximum {
		return nil, ErrImmutableEvidenceUnattested
	}
	return payload, nil
}

func immutableS3Object(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "s3" || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return "", "", ErrImmutableEvidenceInvalid
	}
	return parsed.Host, strings.TrimLeft(parsed.Path, "/"), nil
}

func immutableS3Request(ctx context.Context, client *objectStorageClient, method, objectPath, version string) (*http.Response, error) {
	endpoint, err := client.ObjectURL(objectPath)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("versionId", version)
	parsed.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	client.Sign(req, nil)
	response, err := client.HTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer func() { _ = response.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return nil, fmt.Errorf("immutable object request failed: %s %s", response.Status, strings.TrimSpace(string(body)))
	}
	return response, nil
}

// Source: immutable_evidence_verifier.go
var (
	ErrImmutableEvidenceInvalid    = errors.New("immutable evidence is invalid")
	ErrImmutableEvidenceStale      = errors.New("immutable evidence verification is stale")
	ErrImmutableEvidenceUnattested = errors.New("immutable evidence was not attested by trusted storage")
)

type ImmutableEvidence struct {
	URI            string    `json:"uri"`
	ObjectVersion  string    `json:"object_version"`
	SHA256         string    `json:"sha256"`
	ImmutableUntil time.Time `json:"immutable_until"`
	VerifiedAt     time.Time `json:"verified_at"`
}

type ImmutableEvidenceVerifier struct {
	now          func() time.Time
	maxFreshness time.Duration
	attestor     ImmutableEvidenceAttestor
}

type ImmutableEvidenceAttestation struct {
	Manifest       []byte
	ObjectVersion  string
	ImmutableUntil time.Time
	VerifiedAt     time.Time
}

type ImmutableEvidenceAttestor interface {
	Attest(context.Context, ImmutableEvidence) (ImmutableEvidenceAttestation, error)
}

type immutableEvidenceAttestorFunc func(ImmutableEvidence) (ImmutableEvidenceAttestation, error)

func (f immutableEvidenceAttestorFunc) Attest(_ context.Context, evidence ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
	return f(evidence)
}

func NewImmutableEvidenceVerifier() *ImmutableEvidenceVerifier {
	return &ImmutableEvidenceVerifier{now: time.Now, maxFreshness: 24 * time.Hour, attestor: NewS3ImmutableEvidenceAttestor()}
}

func (v *ImmutableEvidenceVerifier) Verify(ctx context.Context, evidence ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
	if v == nil || v.attestor == nil {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	if !immutableURIAllowed(evidence.URI) || strings.TrimSpace(evidence.ObjectVersion) == "" || !isSHA256(evidence.SHA256) {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceInvalid
	}
	attestation, err := v.attestor.Attest(contextOrBackground(ctx), evidence)
	if err != nil {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	now := v.now().UTC()
	if attestation.ObjectVersion != evidence.ObjectVersion || attestation.ImmutableUntil.IsZero() || !attestation.ImmutableUntil.After(now) {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceInvalid
	}
	if attestation.VerifiedAt.IsZero() || attestation.VerifiedAt.After(now.Add(5*time.Minute)) || now.Sub(attestation.VerifiedAt) > v.maxFreshness {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceStale
	}
	return attestation, nil
}

func immutableURIAllowed(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(value, "s3://") || strings.HasPrefix(value, "gs://") || strings.HasPrefix(value, "azblob://") || strings.HasPrefix(value, "worm://")
}
