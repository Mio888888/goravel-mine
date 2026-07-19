package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
	"goravel/tests/backend/testcase"
)

func TestModuleLifecycleServiceAPIShapes(t *testing.T) {
	caseWithDB := &tests.TestCase{}
	caseWithDB.RefreshDatabase()
	seedLifecycleReadModels(t)
	service := modulecatalog.NewAdminService(moduleboot.Modules())

	state, err := service.State()
	require.NoError(t, err)
	require.NotEmpty(t, state.List)
	requireJSONKeys(t, state, "list", "total")
	stateItem := state.List[0]
	stateItem.Reason = "baseline"
	stateItem.DependsOn = []modulecatalog.ManifestDependency{{ID: "dependency", Required: true}}
	stateItem.Extra = map[string]any{"baseline": true}
	require.NotNil(t, stateItem.Persisted)
	stateItem.Persisted.LastError = "baseline"
	stateItem.Persisted.InstalledAt = timePointer(time.Now())
	stateItem.Persisted.UpgradedAt = timePointer(time.Now())
	stateItem.Persisted.DisabledAt = timePointer(time.Now())
	stateItem.Persisted.LastRunAt = timePointer(time.Now())
	stateItem.Persisted.DisabledReason = "baseline"
	requireJSONKeys(t, stateItem, "id", "name", "version", "compatible", "enabled", "reason", "depends_on", "lifecycle", "frontend", "seed_strategy", "persisted", "extra")
	requireJSONKeys(t, stateItem.Persisted, "name", "status", "enabled", "owner", "target_version", "last_action", "last_run_key", "last_error", "installed_at", "upgraded_at", "disabled_at", "last_run_at", "disabled_reason")

	runs, err := service.Runs(map[string]string{}, 1, 15)
	require.NoError(t, err)
	require.NotEmpty(t, runs.List)
	requireJSONKeys(t, runs, "list", "total")
	requireJSONKeys(t, runs.List[0], "id", "idempotency_key", "module_id", "action", "from_version", "to_version", "status", "dry_run", "owner", "reason", "command", "error", "started_at", "finished_at", "created_at")

	steps, err := service.Steps(map[string]string{}, 1, 15)
	require.NoError(t, err)
	require.NotEmpty(t, steps.List)
	requireJSONKeys(t, steps, "list", "total")
	requireJSONKeys(t, steps.List[0], "id", "attempt_key", "run_key", "module_id", "action", "step_name", "command", "status", "stdout", "stderr", "error", "started_at", "finished_at", "created_at")

	locks, err := service.Locks()
	require.NoError(t, err)
	require.NotEmpty(t, locks.List)
	requireJSONKeys(t, locks, "list", "total")
	requireJSONKeys(t, locks.List[0], "id", "key", "owner", "run_key", "expires_at", "created_at", "updated_at")

	diff, err := service.StateDiff()
	require.NoError(t, err)
	require.NotEmpty(t, diff.List)
	requireJSONKeys(t, diff, "list", "total")
	requireJSONKeys(t, diff.List[0], "module_id", "name", "manifest_version", "persisted_version", "manifest_enabled", "persisted_enabled", "persisted_status", "last_action", "drift")
}

func TestModuleLifecycleActionResultShapes(t *testing.T) {
	dryRun, err := modulecatalog.NewAdminService(moduleboot.Modules()).Execute(modulecatalog.AdminExecutePayload{
		Action:   modulecatalog.LifecycleActionUpgrade,
		ModuleID: "platform-rbac",
		Owner:    "baseline",
		Reason:   "api shape",
	})
	require.NoError(t, err)
	dryRun.Items[0].Skipped = true
	dryRun.Items[0].Error = "baseline"
	requireJSONKeys(t, dryRun, "action", "dry_run", "owner", "reason", "items")
	requireJSONKeys(t, dryRun.Items[0], "module_id", "name", "action", "status", "skipped", "command", "destructive_check", "idempotency_key", "error")

	lockResult := modulecatalog.AdminLockReleaseResult{DryRun: true, Released: []modulecatalog.AdminLockRow{{
		ID: 1, Key: "module-lifecycle:stale", Owner: "baseline", RunKey: "run", ExpiresAt: timePointer(time.Now()),
	}}}
	requireJSONKeys(t, lockResult, "dry_run", "released")
	requireJSONKeys(t, lockResult.Released[0], "id", "key", "owner", "run_key", "expires_at", "created_at", "updated_at")
}

func requireJSONKeys(t *testing.T, value any, expected ...string) {
	t.Helper()
	payload, err := json.Marshal(value)
	require.NoError(t, err)
	object := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal(payload, &object))
	actual := make([]string, 0, len(object))
	for key := range object {
		actual = append(actual, key)
	}
	require.ElementsMatch(t, expected, actual)
}

func timePointer(value time.Time) *time.Time {
	return &value
}
