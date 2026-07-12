package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"goravel/app/models"
)

type TenantRetentionResult struct {
	TenantID    uint64 `json:"tenant_id"`
	TenantCode  string `json:"tenant_code"`
	RunID       uint64 `json:"run_id"`
	PlanID      string `json:"plan_id,omitempty"`
	AuditDays   int    `json:"audit_days"`
	TargetCount int64  `json:"target_count"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

type TenantRetentionService struct {
	ctx     context.Context
	now     func() time.Time
	tenants func(context.Context) ([]Tenant, error)
	plans   func(context.Context, Tenant, int) (AuditPrunePlan, error)
	runs    *TenantGovernanceRunRepository
}

func NewTenantRetentionService() *TenantRetentionService {
	return &TenantRetentionService{
		now: time.Now, tenants: activeRetentionTenants, plans: createTenantRetentionPlan,
		runs: NewTenantGovernanceRunRepository(),
	}
}

func (s *TenantRetentionService) WithContext(ctx context.Context) *TenantRetentionService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	clone.runs = clone.runs.WithContext(ctx)
	return &clone
}

func (s *TenantRetentionService) Run() ([]TenantRetentionResult, error) {
	tenants, err := s.tenants(contextOrBackground(s.ctx))
	if err != nil {
		return nil, err
	}
	results := make([]TenantRetentionResult, 0, len(tenants))
	for _, tenant := range tenants {
		results = append(results, s.runTenant(tenant))
	}
	return results, nil
}

func (s *TenantRetentionService) runTenant(tenant Tenant) TenantRetentionResult {
	policy, err := NewTenantGovernanceService().WithContext(s.ctx).Policy(tenant)
	if err != nil {
		return TenantRetentionResult{TenantID: tenant.ID, TenantCode: tenant.Code, Status: models.TenantGovernanceRunStatusFailed, Error: err.Error()}
	}
	idempotencyKey := fmt.Sprintf("%d:%s:%s:retention", tenant.ID, tenantGovernancePolicyVersion(policy), s.now().UTC().Format("2006-01-02"))
	run, created, err := s.runs.CreateOrGetRun(TenantGovernanceRunCreate{
		TenantID: tenant.ID, TenantCode: tenant.Code, Kind: models.TenantGovernanceRunKindRetention,
		IdempotencyKey: idempotencyKey, PolicyVersion: tenantGovernancePolicyVersion(policy),
	})
	result := TenantRetentionResult{TenantID: tenant.ID, TenantCode: tenant.Code, RunID: run.ID, PlanID: run.PlanID, AuditDays: policy.Retention.AuditDays, Status: run.Status}
	if err != nil || !created {
		if err != nil {
			result.Error = err.Error()
		}
		return result
	}
	if err := s.runs.Transition(run.ID, models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning, ""); err != nil {
		result.Status, result.Error = models.TenantGovernanceRunStatusFailed, err.Error()
		return result
	}
	plan, err := s.plans(contextOrBackground(s.ctx), tenant, policy.Retention.AuditDays)
	if err != nil {
		_ = s.runs.Transition(run.ID, models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusFailed, err.Error())
		result.Status, result.Error = models.TenantGovernanceRunStatusFailed, err.Error()
		return result
	}
	result.PlanID, result.TargetCount = plan.PlanID, plan.TargetCount
	if err := s.runs.AwaitRetentionEvidence(run.ID, plan.PlanID); err != nil {
		result.Status, result.Error = models.TenantGovernanceRunStatusFailed, err.Error()
		return result
	}
	result.Status = models.TenantGovernanceRunStatusAwaitingEvidence
	return result
}

func activeRetentionTenants(ctx context.Context) ([]Tenant, error) {
	tenants := make([]Tenant, 0)
	err := OrmForConnectionWithContext(ctx, PlatformConnection()).Query().Table("tenant").Where("status", TenantStatusActive).OrderBy("id").Get(&tenants)
	return tenants, err
}

func createTenantRetentionPlan(ctx context.Context, tenant Tenant, auditDays int) (AuditPrunePlan, error) {
	RegisterTenantConnection(tenant)
	return NewAuditPrunePlanService().WithContext(ctx).Create(AuditPrunePlanOptions{Scope: "tenant:" + tenant.Code, RetentionDays: auditDays})
}

func tenantGovernancePolicyVersion(policy TenantGovernancePolicy) string {
	payload, _ := json.Marshal(struct {
		Retention TenantRetentionPolicy `json:"retention"`
	}{policy.Retention})
	return digestBytes(payload)
}

func tenantRetentionScheduledTaskHandler(ctx context.Context, _ models.JSONMap) ScheduledTaskExecutionResult {
	results, err := NewTenantRetentionService().WithContext(ctx).Run()
	payload, _ := json.Marshal(results)
	if err != nil {
		return taskFailure(err.Error())
	}
	for _, result := range results {
		if result.Status == models.TenantGovernanceRunStatusFailed {
			return ScheduledTaskExecutionResult{Status: ScheduledTaskLogStatusFailed, Stdout: string(payload), ErrorMessage: result.Error}
		}
	}
	return ScheduledTaskExecutionResult{Status: ScheduledTaskLogStatusSuccess, Stdout: string(payload)}
}
