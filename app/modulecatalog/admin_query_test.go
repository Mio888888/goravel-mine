package modulecatalog

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdminRunDTOMapsRecord(t *testing.T) {
	now := time.Date(2026, 7, 10, 20, 0, 0, 0, time.UTC)
	record := adminRunRecord{
		ID: 7, IdempotencyKey: "upgrade:alpha:1.0.0", ModuleID: "alpha",
		Action: "upgrade", FromVersion: "0.9.0", ToVersion: "1.0.0",
		Status: "succeeded", DryRun: true, Owner: "operator", Reason: "release",
		Command: "migrate", Error: "", StartedAt: &now, FinishedAt: nil, CreatedAt: &now,
	}

	require.Equal(t, AdminRunRow{
		ID: 7, IdempotencyKey: "upgrade:alpha:1.0.0", ModuleID: "alpha",
		Action: "upgrade", FromVersion: "0.9.0", ToVersion: "1.0.0",
		Status: "succeeded", DryRun: true, Owner: "operator", Reason: "release",
		Command: "migrate", StartedAt: &now, CreatedAt: &now,
	}, adminRunDTO(record))
}

func TestAdminStepDTOMapsRecord(t *testing.T) {
	now := time.Date(2026, 7, 10, 20, 30, 0, 0, time.UTC)
	record := adminStepRecord{
		ID: 8, AttemptKey: "attempt", RunKey: "run", ModuleID: "alpha",
		Action: "upgrade", StepName: "command", Command: "migrate",
		Status: "failed", Stdout: "out", Stderr: "err", Error: "boom",
		StartedAt: nil, FinishedAt: &now, CreatedAt: &now,
	}

	require.Equal(t, AdminStepRow{
		ID: 8, AttemptKey: "attempt", RunKey: "run", ModuleID: "alpha",
		Action: "upgrade", StepName: "command", Command: "migrate",
		Status: "failed", Stdout: "out", Stderr: "err", Error: "boom",
		FinishedAt: &now, CreatedAt: &now,
	}, adminStepDTO(record))
}

func TestAdminLockDTOMapsRecord(t *testing.T) {
	now := time.Date(2026, 7, 10, 21, 0, 0, 0, time.UTC)
	record := adminLockRecord{
		ID: 9, Key: "module-lifecycle:alpha", Owner: "operator", RunKey: "run",
		ExpiresAt: &now, CreatedAt: nil, UpdatedAt: &now,
	}

	require.Equal(t, AdminLockRow{
		ID: 9, Key: "module-lifecycle:alpha", Owner: "operator", RunKey: "run",
		ExpiresAt: &now, UpdatedAt: &now,
	}, adminLockDTO(record))
}

func TestAdminStateDiffSortsOrphans(t *testing.T) {
	state := []ModuleStateItem{{ID: "manifest"}}
	persisted := map[string]*PersistedModuleState{
		"zulu":  {Name: "Zulu", TargetVersion: "2.0.0"},
		"alpha": {Name: "Alpha", TargetVersion: "1.0.0"},
	}

	diffs := orphanStateDiffs(state, persisted)

	require.Equal(t, []string{"alpha", "zulu"}, []string{diffs[0].ModuleID, diffs[1].ModuleID})
	require.Equal(t, "missing_manifest", diffs[0].Drift)
}

func TestAdminSecurityGateBuilders(t *testing.T) {
	execute := newAdminExecuteSecurityGate(AdminExecutePayload{
		ModuleID: "alpha", Execute: true, ConfirmToken: "alpha:upgrade",
		ReAuthToken: "reauth", ApprovalID: "approval", OperatorID: 7,
	}, LifecycleActionUpgrade)
	require.Equal(t, "alpha:upgrade", execute.expectedConfirmToken)
	require.Equal(t, "module.lifecycle.execute", execute.operation)
	require.Equal(t, "module-lifecycle:alpha:upgrade", execute.resource)
	require.Equal(t, "module lifecycle execute requires confirm token", execute.confirmError)
	require.Equal(t, "module lifecycle execute requires valid re-auth token", execute.reAuthError)
	require.Equal(t, "module lifecycle execute requires approved approval record", execute.approvalError)

	release := newAdminLockReleaseSecurityGate(AdminLockReleasePayload{
		Key: " module-lifecycle:alpha ", ConfirmToken: "release-stale-locks",
		ReAuthToken: "reauth", ApprovalID: "approval", OperatorID: 9,
	})
	require.Equal(t, "release-stale-locks", release.expectedConfirmToken)
	require.Equal(t, "module.lifecycle.release-lock", release.operation)
	require.Equal(t, "module-lifecycle:stale-locks:module-lifecycle:alpha", release.resource)
	require.Equal(t, "stale lock release requires confirm token", release.confirmError)
	require.Equal(t, "stale lock release requires valid re-auth token", release.reAuthError)
	require.Equal(t, "stale lock release requires approved approval record", release.approvalError)
}

func TestAdminSecurityGateDryRunDoesNothing(t *testing.T) {
	gate := newAdminExecuteSecurityGate(AdminExecutePayload{}, LifecycleActionUpgrade)
	require.NoError(t, gate.preflight(context.Background()))
	require.NoError(t, gate.consume(context.Background()))
}
