package application

import (
	"context"
	"errors"
	contractscache "github.com/goravel/framework/contracts/cache"
	"github.com/stretchr/testify/require"
	"goravel/app/models"
	"strings"
	"sync"
	"testing"
	"time"
)

// Source: digest_test.go
func TestSHA256HelpersKeepExpectedFormats(t *testing.T) {
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	require.Equal(t, emptySHA256, sha256Hex(nil))
	require.Equal(t, "sha256:"+emptySHA256, digestBytes(nil))
	require.Len(t, sha256Hex([]byte("goravel")), 64)
	require.True(t, strings.HasPrefix(digestBytes([]byte("goravel")), "sha256:"))
}

// Source: enterprise_security_control_service_test.go
func TestEnterpriseSecurityControlRequiresReAuthForSensitiveOperation(t *testing.T) {
	useEnterpriseSecurityCache(t)
	service := NewEnterpriseSecurityControlService()

	err := service.RequireSensitiveOperation(SensitiveOperationRequest{
		UserID:    10,
		TenantID:  20,
		Operation: "tenant.destroy",
		Resource:  "tenant:20",
	})

	if err == nil || err.Error() != "sensitive operation requires valid re-auth token" {
		t.Fatalf("RequireSensitiveOperation() error = %v", err)
	}
}

func TestEnterpriseSecurityControlAcceptsBoundReAuthToken(t *testing.T) {
	useEnterpriseSecurityCache(t)
	service := NewEnterpriseSecurityControlService()
	token, err := service.IssueReAuthToken(ReAuthTokenClaims{
		UserID:    10,
		TenantID:  20,
		Operation: "tenant.destroy",
		Resource:  "tenant:20",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("IssueReAuthToken() error = %v", err)
	}

	err = service.RequireSensitiveOperation(SensitiveOperationRequest{
		UserID:      10,
		TenantID:    20,
		Operation:   "tenant.destroy",
		Resource:    "tenant:20",
		ReAuthToken: token,
	})

	if err != nil {
		t.Fatalf("RequireSensitiveOperation() error = %v", err)
	}
}

func TestEnterpriseSecurityControlConsumesReAuthToken(t *testing.T) {
	useEnterpriseSecurityCache(t)
	ResetEnterpriseSecurityControlForTest()
	service := NewEnterpriseSecurityControlService()
	token, err := service.IssueReAuthToken(ReAuthTokenClaims{
		UserID:    10,
		Operation: "module.lifecycle.execute",
		Resource:  "module-lifecycle:alpha:upgrade",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("IssueReAuthToken() error = %v", err)
	}
	req := SensitiveOperationRequest{
		UserID:      10,
		Operation:   "module.lifecycle.execute",
		Resource:    "module-lifecycle:alpha:upgrade",
		ReAuthToken: token,
	}

	if err := service.RequireSensitiveOperation(req); err != nil {
		t.Fatalf("RequireSensitiveOperation() first use error = %v", err)
	}
	if err := service.RequireSensitiveOperation(req); err == nil {
		t.Fatal("RequireSensitiveOperation() should reject reused re-auth token")
	}
}

func TestEnterpriseSecurityControlDoesNotConsumeReAuthTokenOnClaimMismatch(t *testing.T) {
	useEnterpriseSecurityCache(t)
	service := NewEnterpriseSecurityControlService()
	token, err := service.IssueReAuthToken(ReAuthTokenClaims{
		UserID:    10,
		Operation: "module.lifecycle.execute",
		Resource:  "module-lifecycle:alpha:upgrade",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)

	err = service.RequireSensitiveOperation(SensitiveOperationRequest{
		UserID:      10,
		Operation:   "module.lifecycle.execute",
		Resource:    "module-lifecycle:beta:upgrade",
		ReAuthToken: token,
	})
	require.ErrorIs(t, err, ErrReAuthRequired)

	err = service.RequireSensitiveOperation(SensitiveOperationRequest{
		UserID:      10,
		Operation:   "module.lifecycle.execute",
		Resource:    "module-lifecycle:alpha:upgrade",
		ReAuthToken: token,
	})
	require.NoError(t, err)
}

func TestEnterpriseSecurityControlSharesReAuthTokensAcrossServiceInstances(t *testing.T) {
	useEnterpriseSecurityCache(t)
	issuer := &EnterpriseSecurityControlService{}
	consumer := &EnterpriseSecurityControlService{}
	token, err := issuer.IssueReAuthToken(ReAuthTokenClaims{
		UserID:    10,
		Operation: "module.lifecycle.execute",
		Resource:  "module-lifecycle:alpha:upgrade",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)

	err = consumer.RequireSensitiveOperation(SensitiveOperationRequest{
		UserID:      10,
		Operation:   "module.lifecycle.execute",
		Resource:    "module-lifecycle:alpha:upgrade",
		ReAuthToken: token,
	})

	require.NoError(t, err)
}

func TestEnterpriseSecurityControlLocksReAuthTokenForItsRemainingLifetime(t *testing.T) {
	cache := &lockTTLTrackingCache{Driver: newTestCache()}
	original := enterpriseSecurityCache
	enterpriseSecurityCache = func() contractscache.Driver { return cache }
	t.Cleanup(func() { enterpriseSecurityCache = original })
	service := NewEnterpriseSecurityControlService()
	token, err := service.IssueReAuthToken(ReAuthTokenClaims{
		UserID:    10,
		Operation: "module.lifecycle.execute",
		Resource:  "module-lifecycle:alpha:upgrade",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)

	err = service.validateSensitiveOperation(SensitiveOperationRequest{
		UserID:      10,
		Operation:   "module.lifecycle.execute",
		Resource:    "module-lifecycle:alpha:upgrade",
		ReAuthToken: token,
	}, nil)

	require.NoError(t, err)
	require.Greater(t, cache.ttl, 30*time.Second)
}

func TestEnterpriseSecurityControlDoesNotExecuteWhenReAuthTokenCannotBeConsumed(t *testing.T) {
	cache := &forgetRejectingCache{Driver: newTestCache()}
	original := enterpriseSecurityCache
	enterpriseSecurityCache = func() contractscache.Driver { return cache }
	t.Cleanup(func() { enterpriseSecurityCache = original })
	service := NewEnterpriseSecurityControlService()
	token, err := service.IssueReAuthToken(ReAuthTokenClaims{
		UserID:    10,
		Operation: "tenant.data.delete",
		Resource:  "tenant-data:delete:database:1",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	executed := false

	err = service.ExecuteSensitiveOperation(SensitiveOperationRequest{
		UserID:      10,
		Operation:   "tenant.data.delete",
		Resource:    "tenant-data:delete:database:1",
		ReAuthToken: token,
	}, func() error {
		executed = true
		return nil
	})

	require.ErrorIs(t, err, ErrReAuthRequired)
	require.False(t, executed)
}

type lockTTLTrackingCache struct {
	contractscache.Driver
	ttl time.Duration
}

type forgetRejectingCache struct {
	contractscache.Driver
}

func (c *forgetRejectingCache) Forget(string) bool {
	return false
}

func (c *lockTTLTrackingCache) Lock(key string, ttl ...time.Duration) contractscache.Lock {
	if len(ttl) > 0 {
		c.ttl = ttl[0]
	}
	return c.Driver.Lock(key, ttl...)
}

func useEnterpriseSecurityCache(t *testing.T) {
	t.Helper()
	cache := newTestCache()
	original := enterpriseSecurityCache
	enterpriseSecurityCache = func() contractscache.Driver { return cache }
	t.Cleanup(func() {
		enterpriseSecurityCache = original
	})
}

func TestEnterpriseSecurityControlRequiresApprovalForPermissionChange(t *testing.T) {
	service := NewEnterpriseSecurityControlService()

	err := service.RequirePermissionApproval(PermissionApprovalRequest{
		RequesterID: 10,
		Scope:       "tenant.permissions",
		Before:      []string{"a:list"},
		After:       []string{"a:list", "a:delete"},
	})

	if err == nil || err.Error() != "permission change requires approved approval record" {
		t.Fatalf("RequirePermissionApproval() error = %v", err)
	}
}

func TestEnterpriseSecurityControlRejectsRequesterSelfApproval(t *testing.T) {
	service := NewEnterpriseSecurityControlService()

	err := service.RequirePermissionApproval(PermissionApprovalRequest{
		RequesterID: 10,
		ApproverID:  10,
		Status:      "approved",
		Scope:       "tenant.permissions",
		Before:      []string{"a:list"},
		After:       []string{"a:list", "a:delete"},
	})

	if err == nil || err.Error() != "permission change approver must differ from requester" {
		t.Fatalf("RequirePermissionApproval() error = %v", err)
	}
}

func TestEnterpriseSecurityControlRemovesConsumedMemoryApproval(t *testing.T) {
	ResetEnterpriseSecurityControlForTest()
	service := NewEnterpriseSecurityControlService()
	require.NoError(t, service.RegisterPermissionApproval("approval-memory", PermissionApprovalRequest{
		RequesterID: 10,
		ApproverID:  20,
		Scope:       "module.lifecycle.execute",
		Resource:    "module-lifecycle:alpha:upgrade",
		Status:      "approved",
		ExpiresAt:   time.Now().Add(time.Minute),
	}))

	require.NoError(t, service.RequireRegisteredPermissionApproval(
		t.Context(),
		"approval-memory",
		10,
		"module.lifecycle.execute",
		"module-lifecycle:alpha:upgrade",
	))

	service.mu.Lock()
	_, retained := service.approvals["approval-memory"]
	service.mu.Unlock()
	require.False(t, retained)
	require.ErrorIs(t, service.RequireRegisteredPermissionApproval(
		t.Context(),
		"approval-memory",
		10,
		"module.lifecycle.execute",
		"module-lifecycle:alpha:upgrade",
	), ErrApprovalRequired)
}

func TestEnterpriseSecurityControlRejectsBindingDigestConstructionError(t *testing.T) {
	registry := NewSensitiveOperationPolicyRegistry()
	registry.bindingDigest = func(string, string, string, uint64, []string, []string) (string, error) {
		return "", errors.New("digest unavailable")
	}
	service := &EnterpriseSecurityControlService{
		policyRegistry: registry,
		planProvider: sensitiveOperationPlanProviderFunc(func(ctx context.Context, policyKey string, actorID uint64, tenantID uint64, selector SensitiveOperationPlanSelector) (SensitiveOperationPlan, error) {
			return registry.Prepare(ctx, policyKey, actorID, tenantID, SensitiveOperationPrepareInput{Resource: selector.Resource})
		}),
	}

	_, err := service.CreatePlatformApproval(context.Background(), PlatformApprovalCreateRequest{
		RequesterID: 10,
		PolicyKey:   "module.lifecycle.execute",
		Resource:    "module-lifecycle:alpha:upgrade",
		Reason:      "verify digest failure closes approval creation",
	})

	require.ErrorIs(t, err, ErrSensitiveOperationBinding)
}

func TestEnterpriseSecurityControlRejectsBindingAwareApprovalWithoutBindingColumns(t *testing.T) {
	original := enterpriseSecurityHasApprovalBindingColumns
	enterpriseSecurityHasApprovalBindingColumns = func() bool { return false }
	t.Cleanup(func() { enterpriseSecurityHasApprovalBindingColumns = original })

	service := NewEnterpriseSecurityControlService()
	_, err := service.CreatePlatformApproval(context.Background(), PlatformApprovalCreateRequest{
		RequesterID: 10,
		PolicyKey:   "module.lifecycle.execute",
		Resource:    "module-lifecycle:alpha:upgrade",
		Reason:      "reject unpersistable binding-aware approval",
	})

	require.ErrorIs(t, err, ErrApprovalRequired)
}

func TestEnterpriseSecurityControlRejectsLegacyApprovalForBindingAwarePolicy(t *testing.T) {
	ResetEnterpriseSecurityControlForTest()
	service := NewEnterpriseSecurityControlService()
	plan, err := NewSensitiveOperationPolicyRegistry().Prepare(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPrepareInput{
		Resource: "module-lifecycle:alpha:upgrade",
	})
	require.NoError(t, err)

	service.mu.Lock()
	service.approvals["approval-legacy"] = PermissionApprovalRequest{
		RequesterID: 10,
		ApproverID:  20,
		Scope:       plan.Scope,
		Resource:    plan.Resource,
		Status:      "approved",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	service.mu.Unlock()

	err = service.ConsumeRegisteredPermissionApprovalBinding(context.Background(), "approval-legacy", plan)
	require.ErrorIs(t, err, ErrApprovalRequired)

	service.mu.Lock()
	_, retained := service.approvals["approval-legacy"]
	service.mu.Unlock()
	require.True(t, retained)
}

func TestEnterpriseSecurityControlRejectsApprovalFromAnotherTenantDomain(t *testing.T) {
	ResetEnterpriseSecurityControlForTest()
	service := NewEnterpriseSecurityControlService()
	plan, err := NewSensitiveOperationPolicyRegistry().Prepare(context.Background(), "user.password.reset", 10, 21, SensitiveOperationPrepareInput{
		Resource: "user:42:password",
		Before:   []string{"password:configured"},
		After:    []string{"password:reset"},
	})
	require.NoError(t, err)
	require.NoError(t, service.RegisterPermissionApproval("approval-other-tenant", PermissionApprovalRequest{
		RequesterID: 10, ApproverID: 20, TenantID: 22,
		PolicyKey: plan.PolicyKey, BindingDigest: plan.BindingDigest, Scope: plan.Scope, Resource: plan.Resource,
		Status: "approved", ExpiresAt: time.Now().Add(time.Minute),
	}))

	err = service.ConsumeRegisteredPermissionApprovalBinding(context.Background(), "approval-other-tenant", plan)
	require.ErrorIs(t, err, ErrApprovalRequired)
}

func TestEnterpriseSecurityControlConsumesBoundApprovalWithSingleUseCAS(t *testing.T) {
	ResetEnterpriseSecurityControlForTest()
	service := NewEnterpriseSecurityControlService()
	plan, err := NewSensitiveOperationPolicyRegistry().Prepare(context.Background(), "module.lifecycle.execute", 10, 0, SensitiveOperationPrepareInput{
		Resource: "module-lifecycle:alpha:upgrade",
	})
	require.NoError(t, err)
	require.NoError(t, service.RegisterPermissionApproval("approval-cas", PermissionApprovalRequest{
		RequesterID:   10,
		ApproverID:    20,
		PolicyKey:     plan.PolicyKey,
		BindingDigest: plan.BindingDigest,
		Scope:         plan.Scope,
		Resource:      plan.Resource,
		Status:        "approved",
		ExpiresAt:     time.Now().Add(time.Minute),
	}))

	const workers = 16
	start := make(chan struct{})
	errs := make([]error, workers)
	var group sync.WaitGroup
	for index := range workers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			errs[index] = service.ConsumeRegisteredPermissionApprovalBinding(context.Background(), "approval-cas", plan)
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
		require.ErrorIs(t, err, ErrApprovalRequired)
	}
	require.Equal(t, 1, successes)
}

func TestEnterpriseSecurityControlRequiresWORMProofBeforeAuditPrune(t *testing.T) {
	service := NewEnterpriseSecurityControlService()

	err := service.RequireAuditPruneProof(AuditPruneProof{})

	if err == nil || err.Error() != "audit prune requires WORM archive proof" {
		t.Fatalf("RequireAuditPruneProof() error = %v", err)
	}
}

func TestEnterpriseSecurityControlValidatesCSPNonceOrHash(t *testing.T) {
	service := NewEnterpriseSecurityControlService()

	if err := service.ValidateCSP("script-src 'self' 'unsafe-inline'"); err == nil {
		t.Fatal("ValidateCSP() should reject unsafe-inline")
	}
	if err := service.ValidateCSP("script-src 'self'; style-src 'self' 'nonce-style'"); err == nil {
		t.Fatal("ValidateCSP() should reject policies without script-src nonce or hash")
	}
	if err := service.ValidateCSP("script-src 'self' 'nonce-abc123'"); err != nil {
		t.Fatalf("ValidateCSP(nonce) error = %v", err)
	}
	if err := service.ValidateCSP("script-src 'self' 'sha256-abc123'"); err != nil {
		t.Fatalf("ValidateCSP(hash) error = %v", err)
	}
}

func TestEnterpriseSecurityControlRejectsEmptyCSPNonceOrHash(t *testing.T) {
	service := NewEnterpriseSecurityControlService()

	for _, policy := range []string{
		"script-src 'self' 'nonce-'",
		"script-src 'self' 'sha256-'",
		"script-src 'self' 'sha384-'",
		"script-src 'self' 'sha512-'",
	} {
		if err := service.ValidateCSP(policy); err == nil {
			t.Fatalf("ValidateCSP(%q) should reject empty nonce/hash", policy)
		}
	}
}

// Source: security_redaction_test.go
func TestRedactSensitiveDataMasksConfiguredFieldsRecursively(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.sensitive_data.fields": []string{"password", "token", "client_secret"},
	})
	defer restore()

	input := map[string]any{
		"username": "alice",
		"password": "secret",
		"profile": map[string]any{
			"token": "jwt",
			"email": "a@example.com",
		},
		"providers": []any{
			map[string]any{"client_secret": "hidden"},
		},
	}

	output := RedactSensitiveData(input).(map[string]any)

	require.Equal(t, "alice", output["username"])
	require.Equal(t, RedactedValue, output["password"])
	profile := output["profile"].(map[string]any)
	require.Equal(t, RedactedValue, profile["token"])
	require.Equal(t, "a@example.com", profile["email"])
	providers := output["providers"].([]any)
	require.Equal(t, RedactedValue, providers[0].(map[string]any)["client_secret"])
}

// Source: sensitive_operation_guard_test.go
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

// Source: sensitive_operation_rbac_test.go
func TestRBACSensitiveSelectorsBindCanonicalDesiredRoles(t *testing.T) {
	selector, err := rbacUserRolesSelector(42, []string{" Auditor ", "Ops", "Auditor"})
	require.NoError(t, err)

	resource, userID, roles, err := parseRBACUserRolesSelector(selector)
	require.NoError(t, err)
	require.Equal(t, uint64(42), userID)
	require.Equal(t, []string{"Auditor", "Ops"}, roles)
	require.NotEqual(t, "rbac:user:42:roles", resource)
	require.Contains(t, resource, "rbac:user:42:roles:")
}

func TestRBACSensitiveSelectorsBindCanonicalDesiredPermissions(t *testing.T) {
	selector, err := rbacRolePermissionsSelector(7, []string{" permission:user:index ", "permission:role:index", "permission:user:index"})
	require.NoError(t, err)

	resource, roleID, permissions, err := parseRBACRolePermissionsSelector(selector)
	require.NoError(t, err)
	require.Equal(t, uint64(7), roleID)
	require.Equal(t, []string{"permission:role:index", "permission:user:index"}, permissions)
	require.NotEqual(t, "rbac:role:7:permissions", resource)
	require.Contains(t, resource, "rbac:role:7:permissions:")
}

func TestRBACPasswordSnapshotNeverContainsPasswordHash(t *testing.T) {
	before, after, err := rbacPasswordSnapshots(42, true)
	require.NoError(t, err)
	require.NotContains(t, before, "password_hash")
	require.NotContains(t, before, "$2")
	require.NotContains(t, after, "password_hash")
	require.NotContains(t, after, "$2")
}

func TestPermissionAdminUpdateUserRejectsPasswordMutation(t *testing.T) {
	err := NewPermissionAdminService().UpdateUser(42, UserPayload{Password: "new-password"}, 7)

	require.ErrorIs(t, err, ErrSensitiveOperationPolicy)
}

func TestPlatformPermissionAdminUpdateUserRejectsPasswordMutation(t *testing.T) {
	err := NewPlatformPermissionAdminService().UpdateUser(42, UserPayload{Password: "new-password"}, 7)

	require.ErrorIs(t, err, ErrSensitiveOperationPolicy)
}

// Source: sensitive_operation_secrets_test.go
func TestResolveSecretSensitivePlanKeepsSecretValuesOutOfBindingSnapshots(t *testing.T) {
	resource, before, after, err := resolveSecretSensitivePlan(
		context.Background(),
		"storage.secret.change",
		0,
		"storage-config:create",
	)

	require.NoError(t, err)
	require.Equal(t, "storage-config:create", resource)
	require.JSONEq(t, `{"resource_type":"storage_config","state":"absent"}`, before[0])
	require.JSONEq(t, `{"resource_type":"storage_config","rotation_intent":"set","secret_presence":"changed"}`, after[0])
	for _, snapshot := range append(before, after...) {
		require.NotContains(t, snapshot, "top-secret")
		require.NotContains(t, snapshot, "options")
		require.NotContains(t, snapshot, "sha256")
	}
}

func TestResolveSecretSensitivePlanRejectsNonCanonicalTarget(t *testing.T) {
	_, _, _, err := resolveSecretSensitivePlan(
		context.Background(),
		"storage.secret.change",
		0,
		"storage-config:create:top-secret",
	)

	require.ErrorIs(t, err, ErrSensitiveOperationPolicy)
}

func TestSSOPayloadChangesProtectedConfiguration(t *testing.T) {
	enabled, autoCreate := true, false
	existing := SSOProvider{
		Type:         "oidc",
		Audience:     "mine-admin",
		DiscoveryURL: "https://idp.example/.well-known/openid-configuration",
		JWKSURI:      "https://idp.example/jwks",
		ClientID:     "mine-client",
		Scope:        "openid profile email",
		ClientSecret: "existing-secret",
		Enabled:      true, EnablePKCE: true, EnableNonce: true, AutoCreate: false,
		RoleMapping:           models.JSONMap{"claim": "groups", "mapping": models.JSONMap{"ops": []any{"Admin"}}},
		DataPermissionMapping: models.JSONMap{"claim": "department", "default": "self"},
	}

	tests := []struct {
		name    string
		input   SSOProviderPayload
		changes bool
	}{
		{name: "discovery endpoint", input: SSOProviderPayload{DiscoveryURL: "https://other.example/discovery", JWKSURI: existing.JWKSURI}, changes: true},
		{name: "jwks endpoint", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: "https://other.example/jwks"}, changes: true},
		{name: "client secret", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, ClientSecret: "rotated"}, changes: true},
		{name: "audience", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.Audience = "other" }), changes: true},
		{name: "client id", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.ClientID = "other" }), changes: true},
		{name: "scope", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.Scope = "openid" }), changes: true},
		{name: "disable pkce", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.EnablePKCE = boolPointer(false) }), changes: true},
		{name: "disable nonce", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.EnableNonce = boolPointer(false) }), changes: true},
		{name: "role mapping", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, Enabled: &enabled, AutoCreate: &autoCreate, RoleMapping: models.JSONMap{"default": []any{"SuperAdmin"}}, DataPermissionMapping: existing.DataPermissionMapping}, changes: true},
		{name: "data permission mapping", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, Enabled: &enabled, AutoCreate: &autoCreate, RoleMapping: existing.RoleMapping, DataPermissionMapping: models.JSONMap{"default": "all"}}, changes: true},
		{name: "auto create", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, Enabled: &enabled, AutoCreate: boolPointer(true), RoleMapping: existing.RoleMapping, DataPermissionMapping: existing.DataPermissionMapping}, changes: true},
		{name: "disable provider", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, Enabled: boolPointer(false), AutoCreate: &autoCreate, RoleMapping: existing.RoleMapping, DataPermissionMapping: existing.DataPermissionMapping}, changes: true},
		{name: "empty secret preserves existing", input: ssoSensitiveTestPayload(existing, func(*SSOProviderPayload) {}), changes: false},
		{name: "display metadata only", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.DisplayName = "Renamed" }), changes: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.changes, ssoPayloadChangesProtectedConfiguration(test.input, existing))
		})
	}
}

func boolPointer(value bool) *bool { return &value }

func ssoSensitiveTestPayload(existing SSOProvider, change func(*SSOProviderPayload)) SSOProviderPayload {
	input := SSOProviderPayload{
		Type: existing.Type, Enabled: boolPointer(existing.Enabled), Audience: existing.Audience,
		DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, ClientID: existing.ClientID,
		Scope: existing.Scope, EnablePKCE: boolPointer(existing.EnablePKCE), EnableNonce: boolPointer(existing.EnableNonce),
		AutoCreate: boolPointer(existing.AutoCreate), RoleMapping: existing.RoleMapping, DataPermissionMapping: existing.DataPermissionMapping,
	}
	change(&input)
	return input
}

func TestStoragePayloadChangesProtectedConfiguration(t *testing.T) {
	existing := StorageConfig{
		Provider: "minio", Driver: storageDriverS3Compatible,
		Bucket: "uploads", Endpoint: "https://storage.example", Region: "us-east-1",
		AccessKey: "access", SecretKey: "secret", BaseURL: "https://cdn.example",
		PathPrefix: "tenant", IsDefault: false, Status: StorageConfigStatusEnabled,
		Options: models.JSONMap{"force_path_style": true},
	}

	tests := []struct {
		name    string
		input   StorageConfigPayload
		changes bool
	}{
		{name: "endpoint", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.Endpoint = "https://other.example" }), changes: true},
		{name: "bucket", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.Bucket = "archive" }), changes: true},
		{name: "region", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.Region = "eu-west-1" }), changes: true},
		{name: "access key", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.AccessKey = "other" }), changes: true},
		{name: "base url", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.BaseURL = "https://other.example" }), changes: true},
		{name: "path prefix", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.PathPrefix = "archive" }), changes: true},
		{name: "default backend", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.IsDefault = true }), changes: true},
		{name: "status", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.Status = 2 }), changes: true},
		{name: "options", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.Options = models.JSONMap{"force_path_style": false} }), changes: true},
		{name: "empty secret preserves existing", input: storageSensitiveTestPayload(existing, func(*StorageConfigPayload) {}), changes: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.changes, storagePayloadChangesProtectedConfiguration(test.input, existing))
		})
	}
}

func storageSensitiveTestPayload(existing StorageConfig, change func(*StorageConfigPayload)) StorageConfigPayload {
	input := StorageConfigPayload{
		Provider: existing.Provider, Driver: existing.Driver, Bucket: existing.Bucket,
		Endpoint: existing.Endpoint, Region: existing.Region, AccessKey: existing.AccessKey,
		BaseURL: existing.BaseURL, PathPrefix: existing.PathPrefix, IsDefault: existing.IsDefault,
		Status: existing.Status, Options: existing.Options,
	}
	change(&input)
	return input
}
