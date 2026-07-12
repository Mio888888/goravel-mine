package modulecatalog

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
	"github.com/stretchr/testify/require"

	"goravel/app/modules"
	"goravel/app/services"
)

type lifecycleStubModule struct {
	id       string
	metadata modules.Metadata
}

type lifecycleReplacementStubModule struct {
	lifecycleStubModule
	replacement modules.ReplacementPlan
}

func TestModuleLifecycleDryRunOrdersInstallAndRollback(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "beta", metadata: modules.Metadata{
			Name:         "Beta",
			Version:      "1.0.0",
			Dependencies: []modules.Dependency{modules.RequiredDependency("alpha")},
		}},
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{Name: "Alpha", Version: "1.0.0"}},
	})
	service := NewLifecycleService(registry)

	install, err := service.Execute(context.Background(), LifecycleActionInstall, LifecycleOptions{})
	require.NoError(t, err)
	require.True(t, install.DryRun)
	require.Equal(t, []string{"alpha", "beta"}, lifecycleIDs(install.Items))

	rollback, err := service.Execute(context.Background(), LifecycleActionRollback, LifecycleOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{"beta", "alpha"}, lifecycleIDs(rollback.Items))
}

func TestModuleLifecycleValidatesSemanticVersionConstraints(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{Name: "Alpha", Version: "1.4.0"}},
		lifecycleStubModule{id: "beta", metadata: modules.Metadata{
			Name:    "Beta",
			Version: "1.0.0",
			Dependencies: []modules.Dependency{{
				ID:                "alpha",
				VersionConstraint: ">=1.5.0",
				Required:          true,
			}},
		}},
	})
	service := NewLifecycleService(registry)

	_, err := service.Execute(context.Background(), LifecycleActionInstall, LifecycleOptions{})

	require.ErrorContains(t, err, "module beta requires alpha >=1.5.0, got 1.4.0")
}

func TestModuleLifecycleRejectsMissingRequiredDependency(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "beta", metadata: modules.Metadata{
			Name:    "Beta",
			Version: "1.0.0",
			Dependencies: []modules.Dependency{{
				ID:       "alpha",
				Required: true,
			}},
		}},
	})
	service := NewLifecycleService(registry)

	_, err := service.Execute(context.Background(), LifecycleActionInstall, LifecycleOptions{})

	require.ErrorContains(t, err, "module beta requires missing dependency: alpha")
}

func TestModuleLifecycleRejectsPrereleaseVersions(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{Name: "Alpha", Version: "1.0.0-rc.1"}},
		lifecycleStubModule{id: "beta", metadata: modules.Metadata{
			Name:    "Beta",
			Version: "1.0.0",
			Dependencies: []modules.Dependency{{
				ID:                "alpha",
				VersionConstraint: ">=1.0.0",
				Required:          true,
			}},
		}},
	})
	service := NewLifecycleService(registry)

	_, err := service.Execute(context.Background(), LifecycleActionInstall, LifecycleOptions{})

	require.ErrorContains(t, err, "prerelease")
}

func TestModuleLifecycleAllowsBuildMetadataVersions(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{Name: "Alpha", Version: "1.0.0+build.1"}},
		lifecycleStubModule{id: "beta", metadata: modules.Metadata{
			Name:    "Beta",
			Version: "1.0.0",
			Dependencies: []modules.Dependency{{
				ID:                "alpha",
				VersionConstraint: "=1.0.0",
				Required:          true,
			}},
		}},
	})
	service := NewLifecycleService(registry)

	result, err := service.Execute(context.Background(), LifecycleActionInstall, LifecycleOptions{})

	require.NoError(t, err)
	require.True(t, result.DryRun)
}

func TestModuleLifecycleDryRunDoesNotCallRunner(t *testing.T) {
	called := false
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{
			Name: "Alpha",
			Lifecycle: modules.Lifecycle{
				Install: "module:manifest:check",
			},
		}},
	})
	service := NewLifecycleService(registry)
	service.SetRunnerForTest(func(context.Context, string) error {
		called = true
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionInstall, LifecycleOptions{})

	require.NoError(t, err)
	require.True(t, result.DryRun)
	require.False(t, called)
	require.Equal(t, LifecycleStatusPlanned, result.Items[0].Status)
	require.Equal(t, "module:manifest:check", result.Items[0].Command)
}

func TestModuleLifecycleExecuteNormalizesGoRunArtisanCommands(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{
			Name:    "Alpha",
			Version: "1.0.0",
			Lifecycle: modules.Lifecycle{
				Upgrade:          "go run . artisan tenant:migrate && go run . artisan db:seed",
				DestructiveCheck: "go run . artisan module:manifest:check",
			},
		}},
	})
	service := NewLifecycleService(registry)
	store := NewMemoryLifecycleStore()
	bindLifecyclePersistence(service, store)
	var commands []string
	service.SetRunnerForTest(func(ctx context.Context, command string) error {
		commands = append(commands, command)
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "release",
	})

	require.NoError(t, err)
	require.Equal(t, LifecycleStatusSucceeded, result.Items[0].Status)
	require.Equal(t, []string{"module:manifest:check", "tenant:migrate", "db:seed"}, commands)
	require.Len(t, store.steps, 3)
	require.Equal(t, LifecycleStatusSucceeded, store.steps[0].Status)
}

func TestLifecycleLimitedBufferCapsOutputWhilePreservingPrefix(t *testing.T) {
	buffer := newLimitedLifecycleBuffer(5)

	n, err := buffer.Write([]byte("abcdef"))

	require.NoError(t, err)
	require.Equal(t, 6, n)
	require.Equal(t, "abcde", buffer.String())
}

func TestLifecycleOutputTruncationPreservesValidUTF8(t *testing.T) {
	value := strings.Repeat("a", maxLifecycleOutput-1) + "界"

	result := truncateLifecycleOutput(value)

	require.True(t, utf8.ValidString(result))
	require.LessOrEqual(t, len(result), maxLifecycleOutput)
}

func TestModuleLifecycleExecuteRequiresOwnerAndReason(t *testing.T) {
	for _, tt := range []struct {
		name   string
		opts   LifecycleOptions
		errMsg string
	}{
		{
			name:   "missing owner",
			opts:   LifecycleOptions{Execute: true, Reason: "release"},
			errMsg: "owner",
		},
		{
			name:   "missing reason",
			opts:   LifecycleOptions{Execute: true, Owner: "unit-test"},
			errMsg: "reason",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, store := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "migrate"})
			called := false
			service.SetRunnerForTest(func(context.Context, string) error {
				called = true
				return nil
			})

			result, err := service.Execute(context.Background(), LifecycleActionUpgrade, tt.opts)

			require.ErrorContains(t, err, "module lifecycle execute requires owner and reason")
			require.ErrorContains(t, err, tt.errMsg)
			require.Empty(t, result.Items)
			require.False(t, called)
			require.Empty(t, store.runs)
			require.Empty(t, store.states)
		})
	}
}

func TestModuleLifecycleDryRunShowsDisabledModulesAsSkipped(t *testing.T) {
	t.Setenv("MODULE_DISABLED", "alpha")
	service, _ := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "migrate"})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{})

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.True(t, result.DryRun)
	require.Equal(t, LifecycleStatusSkipped, result.Items[0].Status)
	require.True(t, result.Items[0].Skipped)
	require.Contains(t, result.Items[0].Error, "disabled by MODULE_DISABLED")
}

func TestModuleLifecycleExecuteSkipsDisabledModules(t *testing.T) {
	t.Setenv("MODULE_DISABLED", "alpha")
	service, store := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "migrate"})
	called := false
	service.SetRunnerForTest(func(context.Context, string) error {
		called = true
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "skip disabled module",
	})

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, LifecycleStatusSkipped, result.Items[0].Status)
	require.True(t, result.Items[0].Skipped)
	require.Contains(t, result.Items[0].Error, "disabled by MODULE_DISABLED")
	require.False(t, called)
	require.Empty(t, store.runs)
}

func TestModuleLifecycleRejectsInvalidAction(t *testing.T) {
	service := NewLifecycleService(modules.NewRegistry(nil))

	_, err := service.Execute(context.Background(), "restart", LifecycleOptions{})

	require.ErrorContains(t, err, "unsupported lifecycle action: restart")
}

func TestModuleLifecycleExecuteStopsOnRunnerError(t *testing.T) {
	service, _ := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "migrate"})
	service.SetRunnerForTest(func(ctx context.Context, command string) error {
		return errors.New("boom")
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify runner error handling",
	})

	require.ErrorContains(t, err, "boom")
	require.Len(t, result.Items, 1)
	require.Equal(t, LifecycleStatusFailed, result.Items[0].Status)
	require.Contains(t, result.Items[0].Error, "boom")
}

func TestModuleLifecycleRetryKeepsPreviousStepAttempts(t *testing.T) {
	service, store := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "migrate"})
	attempt := 0
	service.SetRunnerForTest(func(context.Context, string) error {
		attempt++
		if attempt == 1 {
			return errors.New("first attempt failed")
		}
		return nil
	})
	opts := LifecycleOptions{Execute: true, Owner: "unit-test", Reason: "retry audit"}

	_, err := service.Execute(context.Background(), LifecycleActionUpgrade, opts)
	require.ErrorContains(t, err, "first attempt failed")
	_, err = service.Execute(context.Background(), LifecycleActionUpgrade, opts)
	require.NoError(t, err)

	require.Len(t, store.steps, 3)
	require.Equal(t, LifecycleStatusFailed, store.steps[0].Status)
	require.Equal(t, LifecycleStatusSucceeded, store.steps[1].Status)
	require.Equal(t, LifecycleStatusSucceeded, store.steps[2].Status)
}

func TestModuleLifecycleExecuteStopsOnSuccessfulRunReadError(t *testing.T) {
	service, store := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "migrate"})
	service.repository = &failingSuccessfulRunStore{
		MemoryLifecycleStore: store,
		err:                  errors.New("read failed"),
	}
	service.lockManager = store
	called := false
	service.SetRunnerForTest(func(context.Context, string) error {
		called = true
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify run read error handling",
	})

	require.ErrorContains(t, err, "read failed")
	require.Len(t, result.Items, 1)
	require.Equal(t, LifecycleStatusFailed, result.Items[0].Status)
	require.False(t, called)
}

func TestModuleLifecycleDoesNotKeepSucceededRunWhenStateWriteFails(t *testing.T) {
	service, store := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "migrate"})
	service.repository = &failingUpsertStateStore{
		MemoryLifecycleStore: store,
		err:                  errors.New("state write failed"),
	}
	service.lockManager = store
	service.SetRunnerForTest(func(context.Context, string) error {
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify state write failure handling",
	})

	require.ErrorContains(t, err, "state write failed")
	require.Len(t, result.Items, 1)
	require.Equal(t, LifecycleStatusFailed, result.Items[0].Status)
	require.Equal(t, LifecycleStatusFailed, store.runs[result.Items[0].IdempotencyKey].Status)
}

func TestLifecycleStateValuesDoNotWriteSuccessTimestampsForFailedRuns(t *testing.T) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	for _, action := range []string{LifecycleActionInstall, LifecycleActionUpgrade, LifecycleActionUninstall} {
		values := lifecycleStateValues(LifecycleStateRecord{
			ModuleID:   "alpha",
			Name:       "Alpha",
			Version:    "1.0.0",
			Status:     LifecycleStatusFailed,
			Enabled:    true,
			LastAction: action,
			LastError:  "boom",
		}, now)

		switch action {
		case LifecycleActionInstall:
			require.NotContains(t, values, "installed_at")
		case LifecycleActionUpgrade:
			require.NotContains(t, values, "upgraded_at")
		case LifecycleActionUninstall:
			require.NotContains(t, values, "disabled_at")
		}
	}
}

func TestModuleLifecycleExecuteRunsDestructiveCheckBeforeCommand(t *testing.T) {
	service, _ := newAlphaLifecycleService(modules.Lifecycle{
		Upgrade: "migrate", DestructiveCheck: "module:manifest:check",
	})
	var calls []string
	service.SetRunnerForTest(func(ctx context.Context, command string) error {
		calls = append(calls, command)
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "release",
	})

	require.NoError(t, err)
	require.Equal(t, []string{"module:manifest:check", "migrate"}, calls)
	require.Equal(t, "unit-test", result.Owner)
	require.Equal(t, "release", result.Reason)
	require.Equal(t, "module:manifest:check", result.Items[0].DestructiveCheck)
}

func TestModuleLifecycleRejectsCommandOutsideAllowlist(t *testing.T) {
	service, store := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "queue:work"})
	var calls []string
	service.SetRunnerForTest(func(_ context.Context, command string) error {
		calls = append(calls, command)
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify command allowlist",
	})

	require.ErrorContains(t, err, "module lifecycle command not allowed: queue:work")
	require.Empty(t, calls)
	require.Equal(t, LifecycleStatusFailed, result.Items[0].Status)
	steps := store.steps
	require.Len(t, steps, 1)
	require.Equal(t, LifecycleStatusFailed, steps[0].Status)
	require.Contains(t, steps[0].Error, "queue:work")
}

func TestAdminExecuteGateRequiresConfirmReAuthAndApproval(t *testing.T) {
	t.Cleanup(services.UseEnterpriseSecurityMemoryCacheForTest())
	useCanonicalLifecyclePlanForTest(t)
	services.ResetEnterpriseSecurityControlForTest()
	security := services.NewEnterpriseSecurityControlService()
	reAuthToken, err := security.IssueReAuthToken(services.ReAuthTokenClaims{
		UserID:    10,
		Operation: "module.lifecycle.execute",
		Resource:  "module-lifecycle:alpha:upgrade",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.NoError(t, security.RegisterPermissionApproval("approval-1", lifecycleApprovalForTest(t, 10, 20, "module-lifecycle:alpha:upgrade")))

	err = newAdminExecuteSecurityGate(AdminExecutePayload{
		OperatorID:   10,
		Action:       LifecycleActionUpgrade,
		ModuleID:     "alpha",
		Execute:      true,
		ConfirmToken: "alpha:upgrade",
		ReAuthToken:  reAuthToken,
		ApprovalID:   "approval-1",
	}, LifecycleActionUpgrade).consume(context.Background())
	require.NoError(t, err)

	err = newAdminExecuteSecurityGate(AdminExecutePayload{
		OperatorID:   10,
		Action:       LifecycleActionUpgrade,
		ModuleID:     "alpha",
		Execute:      true,
		ConfirmToken: "wrong",
		ReAuthToken:  reAuthToken,
		ApprovalID:   "approval-1",
	}, LifecycleActionUpgrade).consume(context.Background())
	require.ErrorContains(t, err, "confirm token")
}

func TestAdminExecuteGateConsumesApprovalAndBindsResource(t *testing.T) {
	t.Cleanup(services.UseEnterpriseSecurityMemoryCacheForTest())
	useCanonicalLifecyclePlanForTest(t)
	services.ResetEnterpriseSecurityControlForTest()
	security := services.NewEnterpriseSecurityControlService()
	alphaReAuthToken, err := security.IssueReAuthToken(services.ReAuthTokenClaims{
		UserID:    10,
		Operation: "module.lifecycle.execute",
		Resource:  "module-lifecycle:alpha:upgrade",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	betaReAuthToken, err := security.IssueReAuthToken(services.ReAuthTokenClaims{
		UserID:    10,
		Operation: "module.lifecycle.execute",
		Resource:  "module-lifecycle:beta:upgrade",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.NoError(t, security.RegisterPermissionApproval("approval-1", lifecycleApprovalForTest(t, 10, 20, "module-lifecycle:alpha:upgrade")))

	err = newAdminExecuteSecurityGate(AdminExecutePayload{
		OperatorID:   10,
		Action:       LifecycleActionUpgrade,
		ModuleID:     "beta",
		Execute:      true,
		ConfirmToken: "beta:upgrade",
		ReAuthToken:  betaReAuthToken,
		ApprovalID:   "approval-1",
	}, LifecycleActionUpgrade).consume(context.Background())
	require.ErrorContains(t, err, "approved approval record")

	alphaReAuthToken, err = security.IssueReAuthToken(services.ReAuthTokenClaims{
		UserID:    10,
		Operation: "module.lifecycle.execute",
		Resource:  "module-lifecycle:alpha:upgrade",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	err = newAdminExecuteSecurityGate(AdminExecutePayload{
		OperatorID:   10,
		Action:       LifecycleActionUpgrade,
		ModuleID:     "alpha",
		Execute:      true,
		ConfirmToken: "alpha:upgrade",
		ReAuthToken:  alphaReAuthToken,
		ApprovalID:   "approval-1",
	}, LifecycleActionUpgrade).consume(context.Background())
	require.NoError(t, err)

	alphaReAuthToken, err = security.IssueReAuthToken(services.ReAuthTokenClaims{
		UserID:    10,
		Operation: "module.lifecycle.execute",
		Resource:  "module-lifecycle:alpha:upgrade",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	err = newAdminExecuteSecurityGate(AdminExecutePayload{
		OperatorID:   10,
		Action:       LifecycleActionUpgrade,
		ModuleID:     "alpha",
		Execute:      true,
		ConfirmToken: "alpha:upgrade",
		ReAuthToken:  alphaReAuthToken,
		ApprovalID:   "approval-1",
	}, LifecycleActionUpgrade).consume(context.Background())
	require.ErrorContains(t, err, "approved approval record")
}

func TestAdminExecuteGateDoesNotConsumeReAuthWhenApprovalFails(t *testing.T) {
	t.Cleanup(services.UseEnterpriseSecurityMemoryCacheForTest())
	useCanonicalLifecyclePlanForTest(t)
	services.ResetEnterpriseSecurityControlForTest()
	security := services.NewEnterpriseSecurityControlService()
	reAuthToken, err := security.IssueReAuthToken(services.ReAuthTokenClaims{
		UserID:    10,
		Operation: "module.lifecycle.execute",
		Resource:  "module-lifecycle:alpha:upgrade",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	payload := AdminExecutePayload{
		OperatorID:   10,
		Action:       LifecycleActionUpgrade,
		ModuleID:     "alpha",
		Execute:      true,
		ConfirmToken: "alpha:upgrade",
		ReAuthToken:  reAuthToken,
		ApprovalID:   "approval-1",
	}

	err = newAdminExecuteSecurityGate(payload, LifecycleActionUpgrade).consume(context.Background())
	require.ErrorContains(t, err, "approved approval record")
	require.NoError(t, security.RegisterPermissionApproval("approval-1", lifecycleApprovalForTest(t, 10, 20, "module-lifecycle:alpha:upgrade")))
	require.NoError(t, newAdminExecuteSecurityGate(payload, LifecycleActionUpgrade).consume(context.Background()))
}

func useCanonicalLifecyclePlanForTest(t *testing.T) {
	t.Helper()
	registry := services.NewSensitiveOperationPolicyRegistry()
	t.Cleanup(services.SetSensitiveOperationPlanProviderForTest(func(
		ctx context.Context,
		policyKey string,
		actorID uint64,
		tenantID uint64,
		selector services.SensitiveOperationPlanSelector,
	) (services.SensitiveOperationPlan, error) {
		return registry.Prepare(ctx, policyKey, actorID, tenantID, services.SensitiveOperationPrepareInput{Resource: selector.Resource})
	}))
}

func lifecycleApprovalForTest(t *testing.T, requesterID, approverID uint64, resource string) services.PermissionApprovalRequest {
	t.Helper()
	plan, err := services.NewSensitiveOperationPolicyRegistry().Prepare(
		context.Background(),
		"module.lifecycle.execute",
		requesterID,
		0,
		services.SensitiveOperationPrepareInput{Resource: resource},
	)
	require.NoError(t, err)
	return services.PermissionApprovalRequest{
		RequesterID: requesterID, ApproverID: approverID, PolicyKey: plan.PolicyKey, BindingDigest: plan.BindingDigest,
		Scope: plan.Scope, Resource: plan.Resource, Status: "approved", Reason: "unit test", ExpiresAt: time.Now().Add(time.Minute),
	}
}

func TestSecurityApprovalGateErrorClassification(t *testing.T) {
	require.True(t, isSecurityApprovalGateError(services.ErrApprovalRequired))
	require.True(t, isSecurityApprovalGateError(services.ErrApprovalSelfApproved))
	require.False(t, isSecurityApprovalGateError(context.Canceled))
}

func TestModuleLifecycleCommandTimeoutCancelsRunner(t *testing.T) {
	service, store := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "migrate"})
	service.commandTimeout = 10 * time.Millisecond
	service.SetRunnerForTest(func(ctx context.Context, command string) error {
		<-ctx.Done()
		return ctx.Err()
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify command timeout",
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, LifecycleStatusFailed, result.Items[0].Status)
	lock, err := store.Acquire(context.Background(), LifecycleLockAcquire{
		Key: lifecycleLockKey("alpha"), Owner: "retry-worker", RunKey: "retry-run", TTL: time.Minute,
	})
	require.NoError(t, err)
	require.True(t, lock.Acquired)
}

func TestModuleLifecycleCommandTimeoutKeepsLockWhenRunnerIgnoresContext(t *testing.T) {
	service, store := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "migrate"})
	service.lockTTL = time.Second
	service.commandTimeout = 10 * time.Millisecond
	started := make(chan struct{})
	releaseRunner := make(chan struct{})
	runnerDone := make(chan struct{})
	service.SetRunnerForTest(func(context.Context, string) error {
		close(started)
		<-releaseRunner
		close(runnerDone)
		return nil
	})
	type executeResult struct {
		result LifecycleResult
		err    error
	}
	resultDone := make(chan executeResult, 1)
	go func() {
		result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
			Execute: true,
			Owner:   "unit-test",
			Reason:  "verify command timeout for blocking runner",
		})
		resultDone <- executeResult{result: result, err: err}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}

	startedAt := time.Now()
	var got executeResult
	select {
	case got = <-resultDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Execute did not return promptly after command timeout")
	}

	require.ErrorIs(t, got.err, context.DeadlineExceeded)
	require.Less(t, time.Since(startedAt), 100*time.Millisecond)
	lock, err := store.Acquire(context.Background(), LifecycleLockAcquire{
		Key: lifecycleLockKey("alpha"), Owner: "other-worker", RunKey: "other-run", TTL: time.Minute,
	})
	require.NoError(t, err)
	require.False(t, lock.Acquired)
	require.Equal(t, "unit-test", lock.Owner)
	select {
	case <-runnerDone:
		t.Fatal("Execute waited for blocking runner to exit")
	default:
	}
	close(releaseRunner)
	<-runnerDone
	require.Equal(t, LifecycleStatusReconciliationRequired, got.result.Items[0].Status)
}

func TestModuleLifecycleCommandTimeoutCancelsLockRenewal(t *testing.T) {
	store := &blockingRenewStore{MemoryLifecycleStore: NewMemoryLifecycleStore(), blockAfter: 1}
	service := newAlphaLifecycleServiceWithPorts(modules.Lifecycle{Upgrade: "migrate"}, store)
	service.commandTimeout = 10 * time.Millisecond
	service.lockRenewInterval = time.Millisecond
	service.SetRunnerForTest(func(ctx context.Context, command string) error {
		<-ctx.Done()
		return ctx.Err()
	})

	startedAt := time.Now()
	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify renewal timeout",
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(startedAt), 100*time.Millisecond)
	require.Equal(t, LifecycleStatusFailed, result.Items[0].Status)
}

func TestModuleLifecycleBlockedRenewalCancelsRunnerBeforeLeaseExpires(t *testing.T) {
	store := &commandBlockingRenewStore{MemoryLifecycleStore: NewMemoryLifecycleStore()}
	service := newAlphaLifecycleServiceWithPorts(modules.Lifecycle{Upgrade: "migrate"}, store)
	service.lockTTL = 40 * time.Millisecond
	service.lockRenewInterval = 10 * time.Millisecond
	service.commandTimeout = time.Second
	service.runnerCancelGrace = time.Millisecond
	runnerCanceled := make(chan struct{})
	service.SetRunnerForTest(func(ctx context.Context, command string) error {
		<-ctx.Done()
		close(runnerCanceled)
		return ctx.Err()
	})

	startedAt := time.Now()
	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify renewal deadline stays inside lease",
	})

	require.Error(t, err)
	require.Less(t, time.Since(startedAt), service.lockTTL)
	require.Equal(t, LifecycleStatusFailed, result.Items[0].Status)
	select {
	case <-runnerCanceled:
	default:
		t.Fatal("runner was not canceled before lock lease expired")
	}
}

func TestModuleLifecycleRunnerCompletionDoesNotWaitForBlockedRenewal(t *testing.T) {
	store := &commandBlockingRenewStore{MemoryLifecycleStore: NewMemoryLifecycleStore()}
	service := newAlphaLifecycleServiceWithPorts(modules.Lifecycle{Upgrade: "migrate"}, store)
	service.commandTimeout = time.Second
	service.lockRenewInterval = time.Millisecond
	startedRenew := make(chan struct{})
	var startedRenewOnce sync.Once
	releaseRunner := make(chan struct{})
	var releaseRunnerOnce sync.Once
	store.onBlock = func() {
		startedRenewOnce.Do(func() {
			close(startedRenew)
		})
	}
	service.SetRunnerForTest(func(ctx context.Context, command string) error {
		<-startedRenew
		releaseRunnerOnce.Do(func() {
			close(releaseRunner)
		})
		return nil
	})

	startedAt := time.Now()
	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify completed runner does not wait for renewal timeout",
	})

	require.NoError(t, err)
	require.Less(t, time.Since(startedAt), 100*time.Millisecond)
	require.Equal(t, LifecycleStatusSucceeded, result.Items[0].Status)
	select {
	case <-releaseRunner:
	default:
		t.Fatal("runner did not complete")
	}
}

func TestModuleLifecycleLockRenewalFailureCancelsRunner(t *testing.T) {
	store := &failingRenewStore{MemoryLifecycleStore: NewMemoryLifecycleStore(), failAfter: 1, err: errors.New("renew failed")}
	service := newAlphaLifecycleServiceWithPorts(modules.Lifecycle{Upgrade: "migrate"}, store)
	service.lockRenewInterval = time.Millisecond
	service.commandTimeout = time.Second
	runnerCanceled := make(chan error, 1)
	releaseRunner := make(chan struct{})
	runnerDone := make(chan struct{})
	service.SetRunnerForTest(func(ctx context.Context, command string) error {
		<-ctx.Done()
		runnerCanceled <- ctx.Err()
		<-releaseRunner
		close(runnerDone)
		return ctx.Err()
	})

	type executeResult struct {
		result LifecycleResult
		err    error
	}
	resultDone := make(chan executeResult, 1)
	go func() {
		result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
			Execute: true,
			Owner:   "unit-test",
			Reason:  "verify renewal failure cancellation",
		})
		resultDone <- executeResult{result: result, err: err}
	}()

	select {
	case runnerErr := <-runnerCanceled:
		require.ErrorIs(t, runnerErr, context.Canceled)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("runner was not canceled after lock renewal failed")
	}
	var got executeResult
	select {
	case got = <-resultDone:
	case <-time.After(time.Second):
		t.Fatal("Execute did not return after runner cancellation grace period")
	}
	require.ErrorContains(t, got.err, "renew failed")
	require.Equal(t, LifecycleStatusReconciliationRequired, got.result.Items[0].Status)
	lock, err := store.Acquire(context.Background(), LifecycleLockAcquire{
		Key: lifecycleLockKey("alpha"), Owner: "other-worker", RunKey: "other-run", TTL: time.Minute,
	})
	require.NoError(t, err)
	require.False(t, lock.Acquired)

	close(releaseRunner)
	select {
	case <-runnerDone:
	case <-time.After(time.Second):
		t.Fatal("runner did not exit")
	}
	require.Eventually(t, func() bool {
		lock, acquireErr := store.Acquire(context.Background(), LifecycleLockAcquire{
			Key: lifecycleLockKey("alpha"), Owner: "retry-worker", RunKey: "retry-run", TTL: time.Minute,
		})
		return acquireErr == nil && lock.Acquired
	}, time.Second, 10*time.Millisecond)
}

func TestModuleLifecycleLateRunnerCannotReleaseReacquiredLock(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{
			Name:    "Alpha",
			Version: "1.0.0",
			Lifecycle: modules.Lifecycle{
				Upgrade: "migrate",
			},
		}},
	})
	service := NewLifecycleService(registry)
	store := NewMemoryLifecycleStore()
	now := time.Now()
	store.nowFunc = func() time.Time { return now }
	bindLifecyclePersistence(service, store)
	service.lockTTL = time.Minute
	service.commandTimeout = 5 * time.Millisecond
	service.runnerCancelGrace = time.Millisecond
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan struct{})
	service.SetRunnerForTest(func(context.Context, string) error {
		close(firstStarted)
		<-releaseFirst
		close(firstDone)
		return nil
	})
	opts := LifecycleOptions{Execute: true, Owner: "unit-test", Reason: "verify lock lease identity"}

	_, firstErr := service.Execute(context.Background(), LifecycleActionUpgrade, opts)
	require.ErrorIs(t, firstErr, context.DeadlineExceeded)
	<-firstStarted
	store.mu.Lock()
	firstLock := store.locks[lifecycleLockKey("alpha")]
	store.mu.Unlock()

	now = now.Add(2 * time.Minute)
	secondLock, err := store.Acquire(context.Background(), LifecycleLockAcquire{
		Key: lifecycleLockKey("alpha"), Owner: "unit-test",
		RunKey: lifecycleLockRunKey("upgrade:alpha:1.0.0"), TTL: time.Minute,
	})
	require.NoError(t, err)
	require.True(t, secondLock.Acquired)
	require.NotEqual(t, firstLock.RunKey, secondLock.RunKey)

	close(releaseFirst)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first runner did not exit")
	}
	require.Eventually(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		current, exists := store.locks[lifecycleLockKey("alpha")]
		return exists && current.RunKey == secondLock.RunKey
	}, time.Second, 10*time.Millisecond)
	third, err := store.Acquire(context.Background(), LifecycleLockAcquire{
		Key: lifecycleLockKey("alpha"), Owner: "third-worker", RunKey: "third-run", TTL: time.Minute,
	})
	require.NoError(t, err)
	require.False(t, third.Acquired)
	require.NoError(t, store.Release(context.Background(), secondLock))
}

func TestLifecycleLockRunKeyRemainsTraceableAndBounded(t *testing.T) {
	key := lifecycleLockRunKey("upgrade:alpha:1.0.0")

	require.Contains(t, key, "upgrade:alpha:1.0.0")
	require.LessOrEqual(t, len(key), 220)
}

func TestModuleLifecycleKeepsLockAliveUntilLateRunnerStops(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{
			Name:      "Alpha",
			Version:   "1.0.0",
			Lifecycle: modules.Lifecycle{Upgrade: "migrate"},
		}},
	})
	service := NewLifecycleService(registry)
	store := NewMemoryLifecycleStore()
	bindLifecyclePersistence(service, store)
	service.lockTTL = 20 * time.Millisecond
	service.lockRenewInterval = 5 * time.Millisecond
	service.commandTimeout = 5 * time.Millisecond
	service.runnerCancelGrace = time.Millisecond
	started := make(chan struct{})
	releaseRunner := make(chan struct{})
	service.SetRunnerForTest(func(context.Context, string) error {
		close(started)
		<-releaseRunner
		return nil
	})

	_, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify late runner lock renewal",
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	<-started
	time.Sleep(50 * time.Millisecond)
	blocked, err := store.Acquire(context.Background(), LifecycleLockAcquire{
		Key: lifecycleLockKey("alpha"), Owner: "other-worker", RunKey: "other-run", TTL: time.Minute,
	})
	require.NoError(t, err)
	require.False(t, blocked.Acquired)

	close(releaseRunner)
	require.Eventually(t, func() bool {
		lock, acquireErr := store.Acquire(context.Background(), LifecycleLockAcquire{
			Key: lifecycleLockKey("alpha"), Owner: "retry-worker", RunKey: "retry-run", TTL: time.Minute,
		})
		return acquireErr == nil && lock.Acquired
	}, time.Second, 10*time.Millisecond)
}

func TestModuleLifecycleLateRunnerRequiresReconciliationBeforeRetry(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{
			Name:      "Alpha",
			Version:   "1.0.0",
			Lifecycle: modules.Lifecycle{Upgrade: "migrate"},
		}},
	})
	service := NewLifecycleService(registry)
	store := NewMemoryLifecycleStore()
	bindLifecyclePersistence(service, store)
	service.lockTTL = 50 * time.Millisecond
	service.lockRenewInterval = 10 * time.Millisecond
	service.commandTimeout = 5 * time.Millisecond
	service.runnerCancelGrace = time.Millisecond
	releaseRunner := make(chan struct{})
	service.SetRunnerForTest(func(context.Context, string) error {
		<-releaseRunner
		return nil
	})
	opts := LifecycleOptions{Execute: true, Owner: "unit-test", Reason: "verify late runner reconciliation"}

	first, err := service.Execute(context.Background(), LifecycleActionUpgrade, opts)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, LifecycleStatusReconciliationRequired, first.Items[0].Status)
	require.Equal(t, LifecycleStatusReconciliationRequired, store.runs[first.Items[0].IdempotencyKey].Status)
	require.Equal(t, LifecycleStatusReconciliationRequired, store.states["alpha"].Status)
	require.Equal(t, LifecycleStatusReconciliationRequired, store.steps[0].Status)

	close(releaseRunner)
	require.Eventually(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		_, exists := store.locks[lifecycleLockKey("alpha")]
		return !exists
	}, time.Second, 10*time.Millisecond)

	var retryCalls int
	service.SetRunnerForTest(func(context.Context, string) error {
		retryCalls++
		return nil
	})
	second, retryErr := service.Execute(context.Background(), LifecycleActionUpgrade, opts)
	require.NoError(t, retryErr)
	require.Equal(t, LifecycleStatusSkipped, second.Items[0].Status)
	require.Zero(t, retryCalls)
}

func TestModuleLifecycleRenewsLockBeforeEachCommand(t *testing.T) {
	service, store := newAlphaLifecycleService(modules.Lifecycle{
		Upgrade: "migrate", DestructiveCheck: "module:manifest:check",
	})
	service.lockTTL = 50 * time.Millisecond
	service.lockRenewInterval = 10 * time.Millisecond
	var expiresAfterFirstCommand time.Time
	service.SetRunnerForTest(func(ctx context.Context, command string) error {
		if command == "module:manifest:check" {
			time.Sleep(80 * time.Millisecond)
			lock, err := store.Acquire(ctx, LifecycleLockAcquire{
				Key: lifecycleLockKey("alpha"), Owner: "other-worker", RunKey: "other-run", TTL: time.Minute,
			})
			require.NoError(t, err)
			require.False(t, lock.Acquired)
			require.Equal(t, "unit-test", lock.Owner)
			expiresAfterFirstCommand = lock.ExpiresAt
		}
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify lock renewal",
	})

	require.NoError(t, err)
	require.Equal(t, LifecycleStatusSucceeded, result.Items[0].Status)
	require.True(t, expiresAfterFirstCommand.After(time.Now()))
}

func TestModuleLifecycleExecuteDeduplicatesSharedCommandsPerRun(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{
			Name:    "Alpha",
			Version: "1.0.0",
			Lifecycle: modules.Lifecycle{
				Rollback:         "go run . artisan migrate:rollback",
				DestructiveCheck: "go run . artisan module:manifest:check",
			},
		}},
		lifecycleStubModule{id: "bravo", metadata: modules.Metadata{
			Name:    "Bravo",
			Version: "1.0.0",
			Lifecycle: modules.Lifecycle{
				Rollback:         "go run . artisan migrate:rollback",
				DestructiveCheck: "go run . artisan module:manifest:check",
			},
		}},
	})
	service := NewLifecycleService(registry)
	store := NewMemoryLifecycleStore()
	bindLifecyclePersistence(service, store)
	var calls []string
	service.SetRunnerForTest(func(ctx context.Context, command string) error {
		calls = append(calls, command)
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionRollback, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify command deduplication",
	})

	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.Equal(t, []string{"bravo", "alpha"}, lifecycleIDs(result.Items))
	require.Equal(t, []string{"module:manifest:check", "migrate:rollback"}, calls)
	require.Equal(t, LifecycleStatusSucceeded, result.Items[0].Status)
	require.Equal(t, LifecycleStatusSucceeded, result.Items[1].Status)
	require.Len(t, store.runs, 2)
}

func TestModuleLifecycleManualCommandRequiresOperatorAction(t *testing.T) {
	service, store := newAlphaLifecycleService(modules.Lifecycle{Uninstall: "manual data review required"})
	service.SetRunnerForTest(func(context.Context, string) error {
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUninstall, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify manual lifecycle handling",
	})

	require.ErrorContains(t, err, "manual lifecycle step requires operator action")
	require.Len(t, result.Items, 1)
	require.Equal(t, LifecycleStatusManualRequired, result.Items[0].Status)
	require.Contains(t, result.Items[0].Error, "manual data review required")

	state := store.states["alpha"]
	require.Equal(t, LifecycleStatusManualRequired, state.Status)
	require.True(t, state.Enabled)
}

func TestModuleLifecycleBatchPreflightRejectsManualStepBeforeExecution(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{Name: "Alpha", Version: "1.0.0", Lifecycle: modules.Lifecycle{Rollback: "migrate:rollback"}}},
		lifecycleStubModule{id: "bravo", metadata: modules.Metadata{Name: "Bravo", Version: "1.0.0", Lifecycle: modules.Lifecycle{Rollback: "manual restore verified backup"}}},
	})
	service := NewLifecycleService(registry)
	store := NewMemoryLifecycleStore()
	bindLifecyclePersistence(service, store)
	var calls []string
	service.SetRunnerForTest(func(_ context.Context, command string) error {
		calls = append(calls, command)
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionRollback, LifecycleOptions{
		Execute: true, Owner: "unit-test", Reason: "verify atomic batch preflight",
	})

	require.ErrorContains(t, err, "batch contains manual step")
	require.Empty(t, result.Items)
	require.Empty(t, calls)
	require.Empty(t, store.runs)
}

func TestModuleLifecycleBatchPreflightRejectsDisallowedCommandBeforeExecution(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{Name: "Alpha", Version: "1.0.0", Lifecycle: modules.Lifecycle{Upgrade: "migrate"}}},
		lifecycleStubModule{id: "bravo", metadata: modules.Metadata{Name: "Bravo", Version: "1.0.0", Lifecycle: modules.Lifecycle{Upgrade: "queue:work"}}},
	})
	service := NewLifecycleService(registry)
	store := NewMemoryLifecycleStore()
	bindLifecyclePersistence(service, store)
	var calls []string
	service.SetRunnerForTest(func(_ context.Context, command string) error {
		calls = append(calls, command)
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true, Owner: "unit-test", Reason: "verify allowlist batch preflight",
	})

	require.ErrorContains(t, err, "batch command not allowed")
	require.Empty(t, result.Items)
	require.Empty(t, calls)
	require.Empty(t, store.runs)
}

func TestModuleLifecycleSuccessfulUninstallDisablesState(t *testing.T) {
	service, store := newAlphaLifecycleService(modules.Lifecycle{Uninstall: "migrate:rollback"})
	service.SetRunnerForTest(func(context.Context, string) error {
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUninstall, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify uninstall state",
	})

	require.NoError(t, err)
	require.Equal(t, LifecycleStatusSucceeded, result.Items[0].Status)
	state := store.states["alpha"]
	require.Equal(t, "uninstalled", state.Status)
	require.False(t, state.Enabled)
}

func TestModuleLifecycleRollsBackReplacementWhenDeprecatedUninstallFails(t *testing.T) {
	plan := replacementLifecyclePlan(t)
	plan.EndOfSupport = time.Now().Add(-time.Hour)
	plan.DataMigration = "tenant:migrate --scope=data"
	plan.ConfigMigration = "db:seed --scope=config"
	plan.PermissionMapping = "module:manifest:check --scope=permissions"
	plan.Validation = "reference-case:upgrade --scope=validation"
	plan.Cutover = "reference-case:rollback --scope=cutover"
	plan.Rollback = "migrate:rollback --scope=replacement"
	plan.CommandPolicyHashes = plan.CommandHashes()
	registry := modules.NewRegistry([]modules.Module{
		lifecycleReplacementStubModule{
			lifecycleStubModule: lifecycleStubModule{id: "legacy", metadata: modules.Metadata{
				Name: "Legacy", Version: "1.0.0", Lifecycle: modules.Lifecycle{Uninstall: "migrate:rollback --scope=legacy"},
			}},
			replacement: plan,
		},
		lifecycleStubModule{id: "modern", metadata: modules.Metadata{
			Name: "Modern", Version: "1.0.0", Lifecycle: modules.Lifecycle{Install: "migrate --scope=install"},
		}},
	})
	service := NewLifecycleService(registry)
	store := NewMemoryLifecycleStore()
	bindLifecyclePersistence(service, store)
	var calls []string
	service.SetRunnerForTest(func(_ context.Context, command string) error {
		calls = append(calls, command)
		if command == "migrate:rollback --scope=legacy" {
			return errors.New("legacy uninstall failed")
		}
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUninstall, LifecycleOptions{
		ModuleID: "legacy", Execute: true, Owner: "unit-test", Reason: "verify replacement rollback",
	})

	require.ErrorContains(t, err, "legacy uninstall failed")
	require.Equal(t, []string{
		"module:manifest:check",
		"migrate --scope=install",
		"tenant:migrate --scope=data",
		"db:seed --scope=config",
		"module:manifest:check --scope=permissions",
		"reference-case:upgrade --scope=validation",
		"reference-case:rollback --scope=cutover",
		"migrate:rollback --scope=legacy",
		"migrate:rollback --scope=replacement",
	}, calls)
	require.Len(t, result.Items, 8)
	require.Equal(t, LifecycleStatusFailed, result.Items[6].Status)
	require.Equal(t, "replacement:rollback_window:rollback", result.Items[7].Action)
	require.Equal(t, LifecycleStatusSucceeded, result.Items[7].Status)
}

func TestMemoryLifecycleStoreBlocksSameOwnerConcurrentLock(t *testing.T) {
	store := NewMemoryLifecycleStore()

	first, err := store.Acquire(context.Background(), LifecycleLockAcquire{
		Key: "module-lifecycle:alpha", Owner: "worker", RunKey: "run-1", TTL: time.Minute,
	})
	require.NoError(t, err)
	require.True(t, first.Acquired)

	second, err := store.Acquire(context.Background(), LifecycleLockAcquire{
		Key: "module-lifecycle:alpha", Owner: "worker", RunKey: "run-2", TTL: time.Minute,
	})
	require.NoError(t, err)
	require.False(t, second.Acquired)
	require.Equal(t, "worker", second.Owner)
	require.Equal(t, "run-1", second.RunKey)
}

func lifecycleIDs(items []LifecycleResultItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ModuleID)
	}
	return ids
}

type failingSuccessfulRunStore struct {
	*MemoryLifecycleStore
	err error
}

func (s *failingSuccessfulRunStore) SuccessfulRunExists(ctx context.Context, key string) (bool, error) {
	return false, s.err
}

type failingUpsertStateStore struct {
	*MemoryLifecycleStore
	err error
}

func (s *failingUpsertStateStore) UpsertState(ctx context.Context, state LifecycleStateRecord) error {
	return s.err
}

type blockingRenewStore struct {
	*MemoryLifecycleStore
	renewCount int
	blockAfter int
}

func (s *blockingRenewStore) Renew(ctx context.Context, request LifecycleLockRenewal) (LifecycleLock, error) {
	s.renewCount++
	if s.renewCount > s.blockAfter {
		<-ctx.Done()
		return LifecycleLock{}, ctx.Err()
	}
	return s.MemoryLifecycleStore.Renew(ctx, request)
}

type commandBlockingRenewStore struct {
	*MemoryLifecycleStore
	onBlock func()
}

func (s *commandBlockingRenewStore) Renew(ctx context.Context, request LifecycleLockRenewal) (LifecycleLock, error) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		if s.onBlock != nil {
			s.onBlock()
		}
		<-ctx.Done()
		return LifecycleLock{}, ctx.Err()
	}
	return s.MemoryLifecycleStore.Renew(ctx, request)
}

type failingRenewStore struct {
	*MemoryLifecycleStore
	renewCount int
	failAfter  int
	err        error
}

func (s *failingRenewStore) Renew(ctx context.Context, request LifecycleLockRenewal) (LifecycleLock, error) {
	s.renewCount++
	if s.renewCount > s.failAfter {
		return LifecycleLock{}, s.err
	}
	return s.MemoryLifecycleStore.Renew(ctx, request)
}

func (m lifecycleStubModule) ID() string { return m.id }
func (m lifecycleStubModule) Metadata() modules.Metadata {
	if strings.TrimSpace(m.metadata.Name) == "" {
		m.metadata.Name = m.id
	}
	return m.metadata
}
func (m lifecycleStubModule) Routes() []modules.Route           { return nil }
func (m lifecycleStubModule) Menus() []modules.Menu             { return nil }
func (m lifecycleStubModule) Permissions() []modules.Permission { return nil }
func (m lifecycleStubModule) Migrations() []schema.Migration    { return nil }
func (m lifecycleStubModule) Seeders() []seeder.Seeder          { return nil }
func (m lifecycleStubModule) OpenAPIFiles() []string            { return nil }
func (m lifecycleStubModule) TestTemplates() []string           { return nil }

func (m lifecycleReplacementStubModule) ReplacementPlan() modules.ReplacementPlan {
	return m.replacement
}
