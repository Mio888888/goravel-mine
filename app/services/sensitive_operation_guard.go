package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"goravel/app/facades"
	"goravel/app/models"
)

type SensitiveOperationEvidence struct {
	ReAuthToken string `json:"reauth_token"`
	ApprovalID  string `json:"approval_id"`
}

type SensitiveOperationGuard struct {
	registry     *SensitiveOperationPolicyRegistry
	security     *EnterpriseSecurityControlService
	planProvider SensitiveOperationPlanProvider
	audit        func(context.Context, SensitiveOperationPlan, string, string)
}

type SensitiveOperationPlanSelector struct {
	Resource string
}

type SensitiveOperationPlanProvider interface {
	Prepare(context.Context, string, uint64, uint64, SensitiveOperationPlanSelector) (SensitiveOperationPlan, error)
}

type sensitiveOperationPlanProviderFunc func(
	context.Context,
	string,
	uint64,
	uint64,
	SensitiveOperationPlanSelector,
) (SensitiveOperationPlan, error)

func (f sensitiveOperationPlanProviderFunc) Prepare(
	ctx context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	selector SensitiveOperationPlanSelector,
) (SensitiveOperationPlan, error) {
	return f(ctx, policyKey, actorID, tenantID, selector)
}

func NewSensitiveOperationGuard(registry *SensitiveOperationPolicyRegistry) *SensitiveOperationGuard {
	if registry == nil {
		registry = NewSensitiveOperationPolicyRegistry()
	}
	security := NewEnterpriseSecurityControlService()
	return &SensitiveOperationGuard{
		registry:     registry,
		security:     security,
		planProvider: security.canonicalPlanProvider(),
		audit:        recordSensitiveOperationAudit,
	}
}

func (g *SensitiveOperationGuard) Prepare(
	ctx context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	input SensitiveOperationPrepareInput,
) (SensitiveOperationPlan, error) {
	return g.registry.Prepare(ctx, policyKey, actorID, tenantID, input)
}

func (g *SensitiveOperationGuard) PrepareCanonical(
	ctx context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	selector SensitiveOperationPlanSelector,
) (SensitiveOperationPlan, error) {
	if g == nil || g.planProvider == nil {
		return SensitiveOperationPlan{}, ErrSensitiveOperationPolicy
	}
	return g.planProvider.Prepare(ctx, policyKey, actorID, tenantID, selector)
}

func (g *SensitiveOperationGuard) Validate(ctx context.Context, plan SensitiveOperationPlan, evidence SensitiveOperationEvidence) error {
	policy, err := g.validPolicy(plan)
	if err != nil {
		return err
	}
	request := SensitiveOperationRequest{
		UserID: plan.ActorID, TenantID: plan.TenantID, Operation: plan.Scope, Resource: plan.Resource, ReAuthToken: evidence.ReAuthToken,
	}
	if policy.RequiresApproval {
		approvalGate := func() error {
			return g.security.ValidateRegisteredPermissionApprovalBinding(ctx, evidence.ApprovalID, plan)
		}
		if policy.RequiresReAuth {
			return g.security.validateSensitiveOperation(request, approvalGate)
		}
		return approvalGate()
	}
	if policy.RequiresReAuth {
		return g.security.validateSensitiveOperation(request, nil)
	}
	return nil
}

func (g *SensitiveOperationGuard) Execute(
	ctx context.Context,
	plan SensitiveOperationPlan,
	evidence SensitiveOperationEvidence,
	mutate func() error,
) error {
	policy, err := g.validPolicy(plan)
	if err != nil {
		return err
	}
	request := SensitiveOperationRequest{
		UserID: plan.ActorID, TenantID: plan.TenantID, Operation: plan.Scope, Resource: plan.Resource, ReAuthToken: evidence.ReAuthToken,
	}
	consume := func() error {
		if policy.RequiresApproval {
			return g.security.ConsumeRegisteredPermissionApprovalBinding(ctx, evidence.ApprovalID, plan)
		}
		return nil
	}
	if policy.RequiresReAuth {
		err = g.security.ExecuteSensitiveOperationNoRestore(request, consume, mutate)
	} else {
		err = consume()
		if err == nil {
			err = mutate()
		}
	}
	if err != nil {
		g.recordAudit(ctx, plan, evidence.ApprovalID, "operation_failed")
		return err
	}
	g.recordAudit(ctx, plan, evidence.ApprovalID, "success")
	return nil
}

type sensitiveOperationPlanResolver struct {
	registry *SensitiveOperationPolicyRegistry
}

type sensitiveModuleStateSnapshot struct {
	ModuleID      string `gorm:"column:module_id"`
	Version       string `gorm:"column:version"`
	TargetVersion string `gorm:"column:target_version"`
	Status        string `gorm:"column:status"`
	Enabled       bool   `gorm:"column:enabled"`
	LastAction    string `gorm:"column:last_action"`
}

type sensitiveLifecycleLockSnapshot struct {
	Key       string    `gorm:"column:key"`
	Owner     string    `gorm:"column:owner"`
	RunKey    string    `gorm:"column:run_key"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
}

type sensitiveMFASnapshot struct {
	UserID        uint64           `gorm:"column:user_id" json:"user_id"`
	Enabled       bool             `gorm:"column:enabled" json:"enabled"`
	ConfirmedAt   time.Time        `gorm:"column:confirmed_at" json:"confirmed_at,omitempty"`
	RecoveryCodes models.JSONSlice `gorm:"column:recovery_codes;type:jsonb" json:"-"`
}

func NewSensitiveOperationPlanResolver(registry *SensitiveOperationPolicyRegistry) SensitiveOperationPlanProvider {
	if registry == nil {
		registry = NewSensitiveOperationPolicyRegistry()
	}
	return sensitiveOperationPlanResolver{registry: registry}
}

func (r sensitiveOperationPlanResolver) Prepare(
	ctx context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	selector SensitiveOperationPlanSelector,
) (SensitiveOperationPlan, error) {
	policy, ok := r.registry.Policy(policyKey)
	if !ok {
		return SensitiveOperationPlan{}, ErrSensitiveOperationPolicy
	}
	resource, before, after, err := r.resolve(ctx, policy.PolicyKey, actorID, tenantID, selector.Resource)
	if err != nil {
		return SensitiveOperationPlan{}, err
	}
	return r.registry.Prepare(ctx, policy.PolicyKey, actorID, tenantID, SensitiveOperationPrepareInput{
		Resource: resource,
		Before:   before,
		After:    after,
	})
}

func (r sensitiveOperationPlanResolver) resolve(
	ctx context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	selector string,
) (string, []string, []string, error) {
	switch policyKey {
	case "audit.prune.execute":
		return resolveAuditPruneSensitivePlan(ctx, selector)
	case "module.lifecycle.execute":
		return resolveLifecycleOperationPlan(ctx, selector)
	case "module.lifecycle.release-lock":
		return resolveLifecycleLockReleasePlan(ctx, selector)
	case "tenant.data.delete":
		return resolveTenantDeletionPlan(ctx, selector)
	case "tenant.data.export":
		return resolveTenantExportPlan(ctx, tenantID, selector)
	case "mfa.disable":
		return resolveMFADisablePlan(ctx, actorID, tenantID, selector)
	case "user.password.reset", "user.roles.sync", "role.permissions.sync":
		return resolveRBACSensitivePlan(ctx, policyKey, tenantID, selector)
	case "sso.provider.secret.change", "storage.secret.change":
		return resolveSecretSensitivePlan(ctx, policyKey, tenantID, selector)
	case "tenant.permissions.sync", "tenant.plan.change", "tenant.governance.change", "tenant.status.change":
		return resolveTenantSensitivePlan(ctx, policyKey, selector)
	default:
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
}

func resolveAuditPruneSensitivePlan(ctx context.Context, selector string) (string, []string, []string, error) {
	planID, _, err := parseAuditPruneResourceSelector(selector)
	if err != nil {
		return "", nil, nil, err
	}
	plan, err := NewAuditPrunePlanService().WithContext(ctx).Load(planID)
	if err != nil {
		return "", nil, nil, err
	}
	resource := AuditPruneResource(plan)
	if resource != strings.TrimSpace(selector) {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	before, err := sensitiveSnapshot(struct {
		PlanID       string `json:"plan_id"`
		Scope        string `json:"scope"`
		TargetDigest string `json:"target_digest"`
		TargetCount  int64  `json:"target_count"`
	}{plan.PlanID, plan.Scope, plan.TargetDigest, plan.TargetCount})
	if err != nil {
		return "", nil, nil, err
	}
	return resource, []string{before}, []string{"audit-prune:executed"}, nil
}

func parseAuditPruneResourceSelector(selector string) (string, string, error) {
	const prefix = "audit-prune:"
	value := strings.TrimSpace(selector)
	parts := strings.SplitN(strings.TrimPrefix(value, prefix), ":", 2)
	if !strings.HasPrefix(value, prefix) || len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || !isSHA256(parts[1]) {
		return "", "", ErrSensitiveOperationPolicy
	}
	return parts[0], parts[1], nil
}

func resolveMFADisablePlan(ctx context.Context, actorID, tenantID uint64, selector string) (string, []string, []string, error) {
	want := fmt.Sprintf("mfa:user:%d:disable", actorID)
	if actorID == 0 || strings.TrimSpace(selector) != want {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	connection, table := PlatformConnection(), "platform_user_mfa"
	if tenantID != 0 {
		var tenant Tenant
		if err := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).Query().Table("tenant").Where("id", tenantID).First(&tenant); err != nil {
			return "", nil, nil, err
		}
		connection, table = TenantConnectionName(tenant), "user_mfa"
	}
	var row sensitiveMFASnapshot
	if err := OrmForConnectionWithContext(contextOrBackground(ctx), connection).Query().Table(table).Where("user_id", actorID).First(&row); err != nil {
		return "", nil, nil, err
	}
	recoveryCodes := jsonSliceStrings(row.RecoveryCodes)
	before, err := sensitiveSnapshot(struct {
		UserID        uint64    `json:"user_id"`
		Enabled       bool      `json:"enabled"`
		ConfirmedAt   time.Time `json:"confirmed_at,omitempty"`
		RecoveryCount int       `json:"recovery_count"`
	}{row.UserID, row.Enabled, row.ConfirmedAt, len(recoveryCodes)})
	if err != nil {
		return "", nil, nil, err
	}
	return want, []string{before}, []string{"mfa:disabled"}, nil
}

func resolveLifecycleOperationPlan(ctx context.Context, selector string) (string, []string, []string, error) {
	moduleID, action, ok := lifecycleOperationSelector(selector)
	if !ok {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	orm := facades.Orm()
	if orm == nil {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	rows := make([]sensitiveModuleStateSnapshot, 0)
	query := orm.WithContext(contextOrBackground(ctx)).Query().Table("module_state").OrderBy("module_id")
	if moduleID != "all" {
		query = query.Where("module_id", moduleID)
	}
	if err := query.Get(&rows); err != nil {
		return "", nil, nil, err
	}
	if moduleID != "all" && len(rows) > 1 {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	before, err := sensitiveSnapshots(rows)
	if err != nil {
		return "", nil, nil, err
	}
	if moduleID != "all" && len(rows) == 0 {
		before = []string{"module-state:" + moduleID + ":absent"}
	}
	return "module-lifecycle:" + moduleID + ":" + action, before, []string{"lifecycle-action:" + action}, nil
}

func resolveLifecycleLockReleasePlan(ctx context.Context, selector string) (string, []string, []string, error) {
	key, ok := lifecycleLockReleaseSelector(selector)
	if !ok {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	orm := facades.Orm()
	if orm == nil {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	rows := make([]sensitiveLifecycleLockSnapshot, 0)
	query := orm.WithContext(contextOrBackground(ctx)).Query().Table("module_lifecycle_lock").
		Where("expires_at < ?", time.Now()).OrderBy("key").OrderBy("owner").OrderBy("run_key")
	if key != "all" {
		query = query.Where("key", key)
	}
	if err := query.Get(&rows); err != nil {
		return "", nil, nil, err
	}
	before, err := sensitiveSnapshots(rows)
	if err != nil {
		return "", nil, nil, err
	}
	return "module-lifecycle:stale-locks:" + key, before, append([]string(nil), before...), nil
}

func resolveTenantDeletionPlan(ctx context.Context, selector string) (string, []string, []string, error) {
	ids, mode, ok := tenantDeletionSelector(selector)
	if !ok {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	tenants := make([]Tenant, 0, len(ids))
	if err := OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).Query().
		Table("tenant").WhereIn("id", uint64Any(ids)).OrderBy("id").Get(&tenants); err != nil {
		return "", nil, nil, err
	}
	if len(tenants) != len(ids) {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	before := make([]string, 0, len(tenants))
	for _, tenant := range tenants {
		policy, err := NewTenantGovernanceService().WithContext(ctx).Policy(tenant)
		if err != nil {
			return "", nil, nil, err
		}
		snapshot, err := sensitiveSnapshot(struct {
			ID               uint64 `json:"id"`
			Code             string `json:"code"`
			Status           int8   `json:"status"`
			Plan             string `json:"plan"`
			Database         string `json:"database"`
			DeletionEnabled  bool   `json:"deletion_enabled"`
			ApprovalRequired bool   `json:"approval_required"`
		}{
			ID: tenant.ID, Code: tenant.Code, Status: tenant.Status, Plan: tenant.Plan, Database: tenant.DBDatabase,
			DeletionEnabled: policy.DataDeletion.Enabled, ApprovalRequired: policy.DataDeletion.RequiresApproval,
		})
		if err != nil {
			return "", nil, nil, err
		}
		before = append(before, snapshot)
	}
	resource := TenantDataActionResource("delete", ids, mode)
	return resource, before, []string{"tenant-deletion:" + mode}, nil
}

func lifecycleOperationSelector(value string) (string, string, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 3 || parts[0] != "module-lifecycle" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	action := strings.TrimSpace(parts[2])
	switch action {
	case "install", "upgrade", "rollback", "uninstall":
		return strings.TrimSpace(parts[1]), action, true
	default:
		return "", "", false
	}
}

func lifecycleLockReleaseSelector(value string) (string, bool) {
	const prefix = "module-lifecycle:stale-locks:"
	key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), prefix))
	if !strings.HasPrefix(strings.TrimSpace(value), prefix) || key == "" {
		return "", false
	}
	return key, true
}

func tenantDeletionSelector(value string) ([]uint64, string, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 4 || parts[0] != "tenant-data" || parts[1] != "delete" {
		return nil, "", false
	}
	mode := strings.TrimSpace(parts[3])
	if mode != "metadata" && mode != "database" {
		return nil, "", false
	}
	seen := map[uint64]struct{}{}
	ids := make([]uint64, 0)
	for _, rawID := range strings.Split(parts[2], ",") {
		id, err := strconv.ParseUint(strings.TrimSpace(rawID), 10, 64)
		if err != nil || id == 0 {
			return nil, "", false
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, "", false
	}
	return ids, mode, true
}

func sensitiveSnapshots[T any](values []T) ([]string, error) {
	snapshots := make([]string, 0, len(values))
	for _, value := range values {
		snapshot, err := sensitiveSnapshot(value)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func sensitiveSnapshot(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (g *SensitiveOperationGuard) validPolicy(plan SensitiveOperationPlan) (SensitiveOperationPolicy, error) {
	policy, ok := g.registry.Policy(plan.PolicyKey)
	if !ok || plan.ActorID == 0 || policy.Scope != plan.Scope || plan.Resource == "" {
		return SensitiveOperationPolicy{}, ErrSensitiveOperationPolicy
	}
	digest, err := sensitiveOperationBindingDigest(plan.PolicyKey, plan.Scope, plan.Resource, plan.TenantID, plan.Before, plan.After)
	if err != nil || plan.BindingDigest == "" || plan.BindingDigest != digest {
		return SensitiveOperationPolicy{}, ErrSensitiveOperationPolicy
	}
	return policy, nil
}

func recordSensitiveOperationAudit(ctx context.Context, plan SensitiveOperationPlan, approvalID, outcome string) {
	RecordAuditEvent(ctx, AuditEvent{
		Action: "sensitive_operation", Outcome: outcome, Actor: fmt.Sprintf("user:%d", plan.ActorID),
		Fields: map[string]any{
			"policy_key": plan.PolicyKey, "approval_id": approvalID, "binding_digest": plan.BindingDigest,
		},
	})
}

func (g *SensitiveOperationGuard) recordAudit(ctx context.Context, plan SensitiveOperationPlan, approvalID, outcome string) {
	if g.audit != nil {
		g.audit(ctx, plan, approvalID, outcome)
	}
}
