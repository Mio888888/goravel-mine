package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type tenantSensitiveSelector struct {
	TenantID uint64          `json:"tenant_id"`
	Desired  json.RawMessage `json:"desired"`
}

func tenantSensitiveResource(kind string, tenantID uint64, desired any) (string, error) {
	if tenantID == 0 {
		return "", ErrSensitiveOperationPolicy
	}
	raw, err := json.Marshal(desired)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(tenantSensitiveSelector{TenantID: tenantID, Desired: raw})
	if err != nil {
		return "", err
	}
	return "tenant-change:" + kind + ":" + base64.RawURLEncoding.EncodeToString(payload), nil
}

func parseTenantSensitiveResource(resource, kind string, desired any) (uint64, string, error) {
	prefix := "tenant-change:" + kind + ":"
	if !strings.HasPrefix(resource, prefix) {
		return 0, "", ErrSensitiveOperationPolicy
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(resource, prefix))
	if err != nil {
		return 0, "", ErrSensitiveOperationPolicy
	}
	var selector tenantSensitiveSelector
	if json.Unmarshal(payload, &selector) != nil || selector.TenantID == 0 || json.Unmarshal(selector.Desired, desired) != nil {
		return 0, "", ErrSensitiveOperationPolicy
	}
	canonicalDesired, err := json.Marshal(desired)
	if err != nil {
		return 0, "", err
	}
	canonicalPayload, err := json.Marshal(tenantSensitiveSelector{TenantID: selector.TenantID, Desired: canonicalDesired})
	if err != nil {
		return 0, "", err
	}
	digest := sha256.Sum256(canonicalPayload)
	return selector.TenantID, fmt.Sprintf("tenant:%d:%s:%s", selector.TenantID, kind, hex.EncodeToString(digest[:])), nil
}

func resolveTenantSensitivePlan(ctx context.Context, policyKey, selector string) (string, []string, []string, error) {
	service := NewTenantService().WithContext(ctx)
	switch policyKey {
	case "tenant.permissions.sync":
		var desired TenantPermissionPayload
		id, resource, err := parseTenantSensitiveResource(selector, "permissions", &desired)
		if err != nil {
			return "", nil, nil, err
		}
		tenant, err := service.FindByID(id)
		if err != nil {
			return "", nil, nil, err
		}
		return tenantSensitiveSnapshots(resource, TenantEffectivePermissionPayload(tenant), normalizePermissionPayload(desired))
	case "tenant.plan.change":
		var desired TenantPlanUpdatePayload
		id, resource, err := parseTenantSensitiveResource(selector, "plan", &desired)
		if err != nil {
			return "", nil, nil, err
		}
		tenant, err := service.FindByID(id)
		if err != nil {
			return "", nil, nil, err
		}
		plan, err := NewTenantPlanService().WithContext(ctx).ActiveByCode(strings.TrimSpace(desired.Plan))
		if err != nil || plan.ID == 0 {
			return "", nil, nil, ErrSensitiveOperationPolicy
		}
		afterPermissions, _ := tenantPermissionPayloadFromFeatures(SnapshotFeaturesForPlan(plan.Features, desired.Features))
		return tenantSensitiveSnapshots(resource,
			map[string]any{"plan": tenant.Plan, "permissions": TenantEffectivePermissionPayload(tenant).Allowed},
			map[string]any{"plan": plan.Code, "permissions": normalizePermissionPayload(afterPermissions).Allowed},
		)
	case "tenant.governance.change":
		var patch TenantGovernancePatch
		id, resource, err := parseTenantSensitiveResource(selector, "governance", &patch)
		if err != nil {
			return "", nil, nil, err
		}
		tenant, err := service.FindByID(id)
		if err != nil {
			return "", nil, nil, err
		}
		governance := NewTenantGovernanceService().WithContext(ctx)
		before, err := governance.Policy(tenant)
		if err != nil {
			return "", nil, nil, err
		}
		after := before
		applyTenantGovernancePatch(&after, patch)
		normalizeTenantGovernancePolicy(&after)
		return tenantSensitiveSnapshots(resource, before, after)
	case "tenant.status.change":
		var status int8
		id, _, err := parseTenantSensitiveResource(selector, "status", &status)
		if err != nil || !validTenantStatus(status) {
			return "", nil, nil, ErrSensitiveOperationPolicy
		}
		tenant, err := service.FindByID(id)
		if err != nil {
			return "", nil, nil, err
		}
		resource := "tenant:" + strconv.FormatUint(id, 10) + ":status:" + tenantStatusName(status)
		return tenantSensitiveSnapshots(resource, tenant.Status, status)
	default:
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
}

func tenantSensitiveSnapshots(resource string, before, after any) (string, []string, []string, error) {
	beforeSnapshot, err := sensitiveSnapshot(before)
	if err != nil {
		return "", nil, nil, err
	}
	afterSnapshot, err := sensitiveSnapshot(after)
	if err != nil {
		return "", nil, nil, err
	}
	return resource, []string{beforeSnapshot}, []string{afterSnapshot}, nil
}

func tenantStatusName(status int8) string {
	switch status {
	case TenantStatusActive:
		return "active"
	case TenantStatusSuspended:
		return "suspended"
	case TenantStatusArchived:
		return "archived"
	default:
		return "invalid"
	}
}

func executeTenantSensitive(ctx context.Context, policyKey string, actorID uint64, selector string, evidence SensitiveOperationEvidence, mutate func() error) error {
	guard := NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(ctx, policyKey, actorID, 0, SensitiveOperationPlanSelector{Resource: selector})
	if err != nil {
		return err
	}
	return guard.Execute(ctx, plan, evidence, mutate)
}

func (s *TenantService) UpdatePermissionsSensitive(id uint64, payload TenantPermissionPayload, operator TenantPermissionOperator, evidence SensitiveOperationEvidence) (TenantPermissionPayload, error) {
	selector, err := tenantSensitiveResource("permissions", id, normalizePermissionPayload(payload))
	if err != nil {
		return TenantPermissionPayload{}, err
	}
	var result TenantPermissionPayload
	err = executeTenantSensitive(s.ctx, "tenant.permissions.sync", operator.ID, selector, evidence, func() error {
		var mutationErr error
		result, mutationErr = s.UpdatePermissions(id, payload, operator)
		return mutationErr
	})
	return result, err
}

func (s *TenantService) UpdatePlanSensitive(id uint64, input TenantPlanUpdatePayload, operator TenantPermissionOperator, evidence SensitiveOperationEvidence) (Tenant, error) {
	selector, err := tenantSensitiveResource("plan", id, input)
	if err != nil {
		return Tenant{}, err
	}
	var result Tenant
	err = executeTenantSensitive(s.ctx, "tenant.plan.change", operator.ID, selector, evidence, func() error {
		var mutationErr error
		result, mutationErr = s.UpdatePlan(id, input, operator)
		return mutationErr
	})
	return result, err
}

func (s *TenantService) UpdateStatusSensitive(actorID, id uint64, status int8, evidence SensitiveOperationEvidence) error {
	selector, err := tenantSensitiveResource("status", id, status)
	if err != nil {
		return err
	}
	return executeTenantSensitive(s.ctx, "tenant.status.change", actorID, selector, evidence, func() error {
		return s.UpdateStatus(id, status)
	})
}

func (s *TenantGovernanceService) PatchPolicySensitive(actorID uint64, tenant Tenant, patch TenantGovernancePatch, evidence SensitiveOperationEvidence) (TenantGovernancePolicy, error) {
	if !s.hasGovernanceTable() {
		return TenantGovernancePolicy{}, ErrSensitiveOperationPolicy
	}
	selector, err := tenantSensitiveResource("governance", tenant.ID, patch)
	if err != nil {
		return TenantGovernancePolicy{}, err
	}
	var result TenantGovernancePolicy
	err = executeTenantSensitive(s.ctx, "tenant.governance.change", actorID, selector, evidence, func() error {
		var mutationErr error
		result, mutationErr = s.PatchPolicy(tenant, patch)
		return mutationErr
	})
	return result, err
}
