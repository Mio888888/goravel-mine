package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSensitiveOperationGuardRejectsInvalidPlansWithoutConsumingEvidence(t *testing.T) {
	for _, testCase := range []struct {
		name string
		plan SensitiveOperationPlan
	}{
		{name: "unknown policy", plan: SensitiveOperationPlan{PolicyKey: "missing.policy", ActorID: 10}},
		{name: "missing actor", plan: SensitiveOperationPlan{PolicyKey: "module.lifecycle.execute"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			guard := newSensitiveOperationGuardForTest()
			err := guard.Validate(context.Background(), testCase.plan, SensitiveOperationEvidence{})
			require.ErrorIs(t, err, ErrSensitiveOperationPolicy)
		})
	}
}

func TestSensitiveOperationGuardRejectsInvalidEvidence(t *testing.T) {
	useEnterpriseSecurityCache(t)
	ResetEnterpriseSecurityControlForTest()
	security := NewEnterpriseSecurityControlService()
	guard := newSensitiveOperationGuardForTest()
	plan, err := guard.Prepare(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPrepareInput{
		Resource: "module-lifecycle:alpha:upgrade",
	})
	require.NoError(t, err)

	validToken, err := security.IssueReAuthToken(ReAuthTokenClaims{
		UserID: 10, Operation: plan.Scope, Resource: plan.Resource, ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.NoError(t, security.RegisterPermissionApproval("approval-valid", PermissionApprovalRequest{
		RequesterID: 10, ApproverID: 20, PolicyKey: plan.PolicyKey, BindingDigest: plan.BindingDigest,
		Scope: plan.Scope, Resource: plan.Resource, Status: "approved", ExpiresAt: time.Now().Add(time.Minute),
	}))

	for _, testCase := range []struct {
		name     string
		evidence SensitiveOperationEvidence
		prepare  func() error
		expected error
	}{
		{name: "missing re-auth", evidence: SensitiveOperationEvidence{ApprovalID: "approval-valid"}, expected: ErrReAuthRequired},
		{name: "missing approval", evidence: SensitiveOperationEvidence{ReAuthToken: validToken}, expected: ErrApprovalRequired},
		{name: "wrong digest", evidence: SensitiveOperationEvidence{ReAuthToken: validToken, ApprovalID: "approval-wrong-digest"}, prepare: func() error {
			return security.RegisterPermissionApproval("approval-wrong-digest", PermissionApprovalRequest{
				RequesterID: 10, ApproverID: 20, PolicyKey: plan.PolicyKey, BindingDigest: "invalid",
				Scope: plan.Scope, Resource: plan.Resource, Status: "approved", ExpiresAt: time.Now().Add(time.Minute),
			})
		}, expected: ErrApprovalRequired},
		{name: "self approval", evidence: SensitiveOperationEvidence{ReAuthToken: validToken, ApprovalID: "approval-self"}, prepare: func() error {
			security.mu.Lock()
			security.approvals["approval-self"] = PermissionApprovalRequest{
				RequesterID: 10, ApproverID: 10, PolicyKey: plan.PolicyKey, BindingDigest: plan.BindingDigest,
				Scope: plan.Scope, Resource: plan.Resource, Status: "approved", ExpiresAt: time.Now().Add(time.Minute),
			}
			security.mu.Unlock()
			return nil
		}, expected: ErrApprovalSelfApproved},
		{name: "expired approval", evidence: SensitiveOperationEvidence{ReAuthToken: validToken, ApprovalID: "approval-expired"}, prepare: func() error {
			security.mu.Lock()
			security.approvals["approval-expired"] = PermissionApprovalRequest{
				RequesterID: 10, ApproverID: 20, PolicyKey: plan.PolicyKey, BindingDigest: plan.BindingDigest,
				Scope: plan.Scope, Resource: plan.Resource, Status: "approved", ExpiresAt: time.Now().Add(-time.Minute),
			}
			security.mu.Unlock()
			return nil
		}, expected: ErrApprovalRequired},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.prepare != nil {
				require.NoError(t, testCase.prepare())
			}
			err := guard.Validate(context.Background(), plan, testCase.evidence)
			require.ErrorIs(t, err, testCase.expected)
		})
	}
}

func TestSensitiveOperationGuardBindsApprovalToCanonicalPlan(t *testing.T) {
	useEnterpriseSecurityCache(t)
	ResetEnterpriseSecurityControlForTest()
	security := NewEnterpriseSecurityControlService()
	guard := newSensitiveOperationGuardForTest()
	plan, err := guard.Prepare(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPrepareInput{
		Resource: "module-lifecycle:alpha:upgrade",
		Before:   []string{"alpha@1.0.0"},
		After:    []string{"alpha@1.1.0"},
	})
	require.NoError(t, err)

	reauthToken, err := security.IssueReAuthToken(ReAuthTokenClaims{
		UserID: 10, Operation: plan.Scope, Resource: plan.Resource, ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.NoError(t, security.RegisterPermissionApproval("approval-bound", PermissionApprovalRequest{
		RequesterID:   10,
		ApproverID:    20,
		Scope:         plan.Scope,
		Resource:      plan.Resource,
		PolicyKey:     plan.PolicyKey,
		BindingDigest: plan.BindingDigest,
		Before:        plan.Before,
		After:         plan.After,
		Status:        "approved",
		ExpiresAt:     time.Now().Add(time.Minute),
	}))

	mutated := false
	err = guard.Execute(context.Background(), plan, SensitiveOperationEvidence{
		ReAuthToken: reauthToken,
		ApprovalID:  "approval-bound",
	}, func() error {
		mutated = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, mutated)

	reauthToken, err = security.IssueReAuthToken(ReAuthTokenClaims{
		UserID: 10, Operation: plan.Scope, Resource: plan.Resource, ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	err = guard.Execute(context.Background(), plan, SensitiveOperationEvidence{
		ReAuthToken: reauthToken,
		ApprovalID:  "approval-bound",
	}, func() error { return nil })
	require.ErrorIs(t, err, ErrApprovalRequired)
}

func TestSensitiveOperationGuardConsumesEvidenceExactlyOnceUnderConcurrency(t *testing.T) {
	useEnterpriseSecurityCache(t)
	ResetEnterpriseSecurityControlForTest()
	security := NewEnterpriseSecurityControlService()
	guard := newSensitiveOperationGuardForTest()
	plan, err := guard.Prepare(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPrepareInput{
		Resource: "module-lifecycle:alpha:upgrade",
		Before:   []string{"alpha@1.0.0"},
		After:    []string{"alpha@1.1.0"},
	})
	require.NoError(t, err)
	reauthToken, err := security.IssueReAuthToken(ReAuthTokenClaims{
		UserID: 10, Operation: plan.Scope, Resource: plan.Resource, ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.NoError(t, security.RegisterPermissionApproval("approval-guard-cas", PermissionApprovalRequest{
		RequesterID:   10,
		ApproverID:    20,
		PolicyKey:     plan.PolicyKey,
		BindingDigest: plan.BindingDigest,
		Scope:         plan.Scope,
		Resource:      plan.Resource,
		Status:        "approved",
		ExpiresAt:     time.Now().Add(time.Minute),
	}))

	const workers = 12
	start := make(chan struct{})
	errs := make([]error, workers)
	var group sync.WaitGroup
	for index := range workers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			errs[index] = guard.Execute(context.Background(), plan, SensitiveOperationEvidence{
				ReAuthToken: reauthToken,
				ApprovalID:  "approval-guard-cas",
			}, func() error { return nil })
		}(index)
	}
	close(start)
	group.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		require.True(t, errors.Is(err, ErrReAuthRequired) || errors.Is(err, ErrApprovalRequired))
	}
	require.Equal(t, 1, successes)
}

func TestSensitiveOperationGuardDoesNotConsumeEvidenceWhenValidationFails(t *testing.T) {
	useEnterpriseSecurityCache(t)
	ResetEnterpriseSecurityControlForTest()
	security := NewEnterpriseSecurityControlService()
	guard := newSensitiveOperationGuardForTest()
	plan, err := guard.Prepare(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPrepareInput{
		Resource: "module-lifecycle:alpha:upgrade",
	})
	require.NoError(t, err)

	reauthToken, err := security.IssueReAuthToken(ReAuthTokenClaims{
		UserID: 10, Operation: plan.Scope, Resource: plan.Resource, ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.NoError(t, security.RegisterPermissionApproval("approval-wrong-resource", PermissionApprovalRequest{
		RequesterID:   10,
		ApproverID:    20,
		Scope:         plan.Scope,
		Resource:      "module-lifecycle:beta:upgrade",
		PolicyKey:     plan.PolicyKey,
		BindingDigest: plan.BindingDigest,
		Status:        "approved",
		ExpiresAt:     time.Now().Add(time.Minute),
	}))

	err = guard.Validate(context.Background(), plan, SensitiveOperationEvidence{
		ReAuthToken: reauthToken,
		ApprovalID:  "approval-wrong-resource",
	})
	require.ErrorIs(t, err, ErrApprovalRequired)

	require.NoError(t, security.RequireSensitiveOperation(SensitiveOperationRequest{
		UserID: 10, Operation: plan.Scope, Resource: plan.Resource, ReAuthToken: reauthToken,
	}))
}

func TestSensitiveOperationGuardConsumesEvidenceWhenMutationFails(t *testing.T) {
	useEnterpriseSecurityCache(t)
	ResetEnterpriseSecurityControlForTest()
	security := NewEnterpriseSecurityControlService()
	guard := newSensitiveOperationGuardForTest()
	plan, err := guard.Prepare(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPrepareInput{
		Resource: "module-lifecycle:alpha:upgrade",
	})
	require.NoError(t, err)

	reauthToken, err := security.IssueReAuthToken(ReAuthTokenClaims{
		UserID: 10, Operation: plan.Scope, Resource: plan.Resource, ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.NoError(t, security.RegisterPermissionApproval("approval-failure", PermissionApprovalRequest{
		RequesterID:   10,
		ApproverID:    20,
		Scope:         plan.Scope,
		Resource:      plan.Resource,
		PolicyKey:     plan.PolicyKey,
		BindingDigest: plan.BindingDigest,
		Status:        "approved",
		ExpiresAt:     time.Now().Add(time.Minute),
	}))

	err = guard.Execute(context.Background(), plan, SensitiveOperationEvidence{
		ReAuthToken: reauthToken,
		ApprovalID:  "approval-failure",
	}, func() error { return errors.New("mutation failed") })
	require.EqualError(t, err, "mutation failed")
	require.ErrorIs(t, security.RequireSensitiveOperation(SensitiveOperationRequest{
		UserID: 10, Operation: plan.Scope, Resource: plan.Resource, ReAuthToken: reauthToken,
	}), ErrReAuthRequired)
	require.ErrorIs(t, security.RequireRegisteredPermissionApproval(context.Background(), "approval-failure", 10, plan.Scope, plan.Resource), ErrApprovalRequired)
}

func TestSensitiveOperationPlanUsesStableCanonicalDigest(t *testing.T) {
	registry := NewSensitiveOperationPolicyRegistry()
	first, err := registry.Prepare(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPrepareInput{
		Resource: "module-lifecycle:alpha:upgrade",
		Before:   []string{"b", "a", "a"},
		After:    []string{"c", "b"},
	})
	require.NoError(t, err)
	second, err := registry.Prepare(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPrepareInput{
		Resource: "module-lifecycle:alpha:upgrade",
		Before:   []string{"a", "b"},
		After:    []string{"b", "c"},
	})
	require.NoError(t, err)
	require.Equal(t, first.Before, second.Before)
	require.Equal(t, first.After, second.After)
	require.Equal(t, first.BindingDigest, second.BindingDigest)
	require.Equal(t, []SensitiveOperationPolicyContract{
		{PolicyKey: "audit.prune.execute", Scope: "audit.prune.execute", Permission: "platform:security:control", Action: "DELETE", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "mfa.disable", Scope: "mfa.disable", Permission: "platform:security:mfa", TenantPermission: "security:mfa", Action: "POST", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "module.admission.approve", Scope: "module.admission.approve", Permission: "platform:moduleLifecycle:execute", Action: "POST", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "module.lifecycle.execute", Scope: "module.lifecycle.execute", Permission: "platform:moduleLifecycle:execute", Action: "POST", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "module.lifecycle.release-lock", Scope: "module.lifecycle.release-lock", Permission: "platform:moduleLifecycle:execute", Action: "POST", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "module.replacement.emergency-remove", Scope: "module.replacement.emergency-remove", Permission: "platform:moduleLifecycle:execute", Action: "DELETE", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "role.permissions.sync", Scope: "role.permissions.sync", Permission: "platform:role:setMenu", TenantPermission: "permission:role:setMenu", Action: "PUT", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "sso.provider.secret.change", Scope: "sso.provider.secret.change", TenantPermission: "security:ssoProvider:update", Action: "PUT", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "storage.secret.change", Scope: "storage.secret.change", Permission: "platform:storageConfig:update", Action: "PUT", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "tenant.data.delete", Scope: "tenant.data.delete", Permission: "platform:tenant:destroy", Action: "DELETE", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "tenant.data.export", Scope: "tenant.data.export", Permission: "platform:tenant:export", Action: "POST", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "tenant.governance.change", Scope: "tenant.governance.change", Permission: "platform:tenant:governance", Action: "PUT", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "tenant.permissions.sync", Scope: "tenant.permissions.sync", Permission: "platform:tenant:permissions", Action: "PUT", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "tenant.plan.change", Scope: "tenant.plan.change", Permission: "platform:tenant:updatePlan", Action: "PUT", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "tenant.status.change", Scope: "tenant.status.change", Permission: "platform:tenant:suspend", Action: "PUT", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "user.password.reset", Scope: "user.password.reset", Permission: "platform:user:password", TenantPermission: "permission:user:password", Action: "PUT", RequiresReAuth: true, RequiresApproval: true},
		{PolicyKey: "user.roles.sync", Scope: "user.roles.sync", Permission: "platform:user:setRole", TenantPermission: "permission:user:setRole", Action: "PUT", RequiresReAuth: true, RequiresApproval: true},
	}, registry.Export())
}

func TestSensitiveOperationPolicyRegistryCoversEnterpriseSensitiveMutations(t *testing.T) {
	registry := NewSensitiveOperationPolicyRegistry()
	for _, policyKey := range []string{
		"audit.prune.execute",
		"mfa.disable",
		"module.lifecycle.execute",
		"module.lifecycle.release-lock",
		"role.permissions.sync",
		"sso.provider.secret.change",
		"storage.secret.change",
		"tenant.data.delete",
		"tenant.data.export",
		"tenant.governance.change",
		"tenant.permissions.sync",
		"tenant.plan.change",
		"tenant.status.change",
		"user.password.reset",
		"user.roles.sync",
	} {
		t.Run(policyKey, func(t *testing.T) {
			policy, ok := registry.Policy(policyKey)
			require.True(t, ok)
			require.True(t, policy.RequiresReAuth)
			require.True(t, policy.RequiresApproval)
		})
	}
}

func TestTenantExportSelectorRejectsArbitraryDatasetAndDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	parsed, err := parseTenantExportSelector(tenantExportResource(9, "users", "jsonl", digest))
	require.NoError(t, err)
	require.Equal(t, uint64(9), parsed.TenantID)
	require.Equal(t, digest, parsed.FilterDigest)

	_, err = parseTenantExportSelector(tenantExportResource(9, "secrets", "jsonl", digest))
	require.ErrorIs(t, err, ErrSensitiveOperationPolicy)
	_, err = parseTenantExportSelector(tenantExportResource(9, "users", "sql", digest))
	require.ErrorIs(t, err, ErrSensitiveOperationPolicy)
	_, err = parseTenantExportSelector(tenantExportResource(9, "users", "jsonl", "sha256:bad"))
	require.ErrorIs(t, err, ErrSensitiveOperationPolicy)
}

func TestSensitiveOperationBindingSeparatesTenantDomains(t *testing.T) {
	registry := NewSensitiveOperationPolicyRegistry()
	first, err := registry.Prepare(context.Background(), "user.password.reset", 7, 11, SensitiveOperationPrepareInput{
		Resource: "user:42:password",
		Before:   []string{"password:configured"},
		After:    []string{"password:reset"},
	})
	require.NoError(t, err)
	second, err := registry.Prepare(context.Background(), "user.password.reset", 7, 12, SensitiveOperationPrepareInput{
		Resource: "user:42:password",
		Before:   []string{"password:configured"},
		After:    []string{"password:reset"},
	})
	require.NoError(t, err)
	require.NotEqual(t, first.BindingDigest, second.BindingDigest)
}

func TestSensitiveOperationPolicyResolvesResourceSpecificPermission(t *testing.T) {
	registry := NewSensitiveOperationPolicyRegistry()
	for _, testCase := range []struct {
		policyKey string
		tenantID  uint64
		resource  string
		want      string
	}{
		{policyKey: "tenant.status.change", resource: "tenant:4:status:suspended", want: "platform:tenant:suspend"},
		{policyKey: "tenant.status.change", resource: "tenant:4:status:active", want: "platform:tenant:resume"},
		{policyKey: "tenant.status.change", resource: "tenant:4:status:archived", want: "platform:tenant:archive"},
		{policyKey: "sso.provider.secret.change", tenantID: 3, resource: "sso-provider:create", want: "security:ssoProvider:save"},
		{policyKey: "sso.provider.secret.change", tenantID: 3, resource: "sso-provider:update:9", want: "security:ssoProvider:update"},
		{policyKey: "sso.provider.secret.change", tenantID: 3, resource: "sso-provider:delete:9", want: "security:ssoProvider:delete"},
		{policyKey: "storage.secret.change", resource: "storage-config:create", want: "platform:storageConfig:save"},
		{policyKey: "storage.secret.change", resource: "storage-config:update:9", want: "platform:storageConfig:update"},
		{policyKey: "storage.secret.change", resource: "storage-config:delete:9", want: "platform:storageConfig:delete"},
	} {
		t.Run(testCase.resource, func(t *testing.T) {
			permission, action, err := registry.PermissionFor(testCase.policyKey, testCase.tenantID, testCase.resource)
			require.NoError(t, err)
			require.Equal(t, testCase.want, permission)
			require.NotEmpty(t, action)
		})
	}
}

func TestSensitiveOperationGuardUsesCanonicalPlanProvider(t *testing.T) {
	registry := NewSensitiveOperationPolicyRegistry()
	expected, err := registry.Prepare(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPrepareInput{
		Resource: "module-lifecycle:alpha:upgrade",
		Before:   []string{"server-before"},
		After:    []string{"server-after"},
	})
	require.NoError(t, err)

	guard := newSensitiveOperationGuardForTest()
	guard.planProvider = sensitiveOperationPlanProviderFunc(func(
		_ context.Context,
		policyKey string,
		actorID uint64,
		tenantID uint64,
		selector SensitiveOperationPlanSelector,
	) (SensitiveOperationPlan, error) {
		require.Equal(t, "module.lifecycle.execute", policyKey)
		require.Equal(t, uint64(10), actorID)
		require.Equal(t, uint64(0), tenantID)
		require.Equal(t, "module-lifecycle:alpha:upgrade", selector.Resource)
		return expected, nil
	})

	plan, err := guard.PrepareCanonical(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPlanSelector{
		Resource: "module-lifecycle:alpha:upgrade",
	})

	require.NoError(t, err)
	require.Equal(t, expected, plan)
}

func TestAuditPruneResourceSelectorPreservesPrefixedDigest(t *testing.T) {
	planID, digest, err := parseAuditPruneResourceSelector("audit-prune:plan-1:sha256:" + strings.Repeat("a", 64))

	require.NoError(t, err)
	require.Equal(t, "plan-1", planID)
	require.Equal(t, "sha256:"+strings.Repeat("a", 64), digest)
}

func TestSensitiveOperationPoliciesMatchRoutePermissions(t *testing.T) {
	registry := NewSensitiveOperationPolicyRegistry()
	for _, testCase := range []struct {
		policyKey string
		method    string
		path      string
	}{
		{policyKey: "module.lifecycle.execute", method: "POST", path: "/admin/platform/module-lifecycle/execute"},
		{policyKey: "module.lifecycle.release-lock", method: "POST", path: "/admin/platform/module-lifecycle/locks/release-stale"},
		{policyKey: "tenant.data.delete", method: "DELETE", path: "/admin/platform/tenant"},
	} {
		t.Run(testCase.policyKey, func(t *testing.T) {
			policy, ok := registry.Policy(testCase.policyKey)
			require.True(t, ok)
			require.Contains(t, PlatformPermissionsForRoute(testCase.method, testCase.path), policy.Permission)
		})
	}
}

func newSensitiveOperationGuardForTest() *SensitiveOperationGuard {
	guard := NewSensitiveOperationGuard(NewSensitiveOperationPolicyRegistry())
	guard.audit = func(context.Context, SensitiveOperationPlan, string, string) {}
	return guard
}
