package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"
	"github.com/stretchr/testify/require"
)

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
