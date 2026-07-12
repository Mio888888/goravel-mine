package modulecatalog

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/modules"
)

func TestLifecyclePlannerOrdersActionsAndBuildsItems(t *testing.T) {
	planner := newLifecyclePlanner([]modules.ModuleState{
		lifecyclePlannerState("alpha", true, "", lifecyclePlannerCommands("alpha")),
		lifecyclePlannerState("beta", false, "disabled", lifecyclePlannerCommands("beta")),
	})

	for _, test := range []struct {
		action  string
		ids     []string
		command string
	}{
		{action: LifecycleActionInstall, ids: []string{"alpha", "beta"}, command: "install-alpha"},
		{action: LifecycleActionUpgrade, ids: []string{"alpha", "beta"}, command: "upgrade-alpha"},
		{action: LifecycleActionRollback, ids: []string{"beta", "alpha"}, command: "rollback-beta"},
		{action: LifecycleActionUninstall, ids: []string{"beta", "alpha"}, command: "uninstall-beta"},
	} {
		t.Run(test.action, func(t *testing.T) {
			plan, err := planner.plan(test.action, "")
			require.NoError(t, err)
			require.Equal(t, test.ids, lifecyclePlanIDs(plan.items))
			require.Equal(t, test.action, plan.action)
			require.Equal(t, test.command, plan.items[0].command)
		})
	}

	install, err := planner.plan(LifecycleActionInstall, "")
	require.NoError(t, err)
	require.Equal(t, "install:alpha:1.0.0", install.items[0].idempotencyKey)
	require.Equal(t, "disabled", install.items[1].state.Reason)
}

func TestLifecyclePlannerFiltersOneModule(t *testing.T) {
	planner := newLifecyclePlanner([]modules.ModuleState{
		lifecyclePlannerState("alpha", true, "", modules.Lifecycle{}),
		lifecyclePlannerState("beta", true, "", modules.Lifecycle{}),
	})

	plan, err := planner.plan(LifecycleActionUpgrade, " beta ")
	require.NoError(t, err)
	require.Equal(t, []string{"beta"}, lifecyclePlanIDs(plan.items))

	_, err = planner.plan(LifecycleActionUpgrade, "missing")
	require.EqualError(t, err, "module not found: missing")
}

func TestLifecyclePlannerFiltersAfterActionOrdering(t *testing.T) {
	planner := newLifecyclePlanner([]modules.ModuleState{
		{ID: "alpha", Enabled: true, Metadata: modules.Metadata{Name: "first", Version: "1.0.0"}},
		{ID: "alpha", Enabled: true, Metadata: modules.Metadata{Name: "second", Version: "2.0.0"}},
	})

	install, err := planner.plan(LifecycleActionInstall, "alpha")
	require.NoError(t, err)
	require.Equal(t, "first", install.items[0].state.Metadata.Name)

	rollback, err := planner.plan(LifecycleActionRollback, "alpha")
	require.NoError(t, err)
	require.Equal(t, "second", rollback.items[0].state.Metadata.Name)
}

func TestLifecyclePlannerRejectsInvalidAction(t *testing.T) {
	planner := newLifecyclePlanner(nil)

	_, err := planner.plan("restart", "")

	require.EqualError(t, err, "unsupported lifecycle action: restart")
	_, err = planner.plan(" install ", "")
	require.EqualError(t, err, "unsupported lifecycle action:  install ")
}

func TestLifecyclePlannerAdaptersPreservePlanAndDryRunParity(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{
			Name: "Alpha", Version: "1.0.0",
			Lifecycle: modules.Lifecycle{Upgrade: "migrate", DestructiveCheck: "module:manifest:check"},
		}},
	})

	plan, err := NewService(registry).LifecyclePlan(LifecycleActionUpgrade)
	require.NoError(t, err)
	dryRun, err := NewLifecycleService(registry).Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{})
	require.NoError(t, err)
	require.Len(t, plan, 1)
	require.Len(t, dryRun.Items, 1)
	require.Equal(t, plan[0].ID, dryRun.Items[0].ModuleID)
	require.Equal(t, plan[0].Action, dryRun.Items[0].Action)
	require.Equal(t, plan[0].Command, dryRun.Items[0].Command)
	require.Equal(t, plan[0].DestructiveCheck, dryRun.Items[0].DestructiveCheck)
}

func TestLifecyclePlannerFacadesPreserveActionNormalization(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{Name: "Alpha", Version: "1.0.0"}},
	})

	_, err := NewService(registry).LifecyclePlan(" install ")
	require.EqualError(t, err, "unsupported lifecycle action:  install ")

	result, err := NewLifecycleService(registry).Execute(context.Background(), " install ", LifecycleOptions{})
	require.NoError(t, err)
	require.Equal(t, LifecycleActionInstall, result.Action)
}

func TestLifecyclePlannerFacadesPreserveVersionValidationBoundary(t *testing.T) {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{Name: "Alpha", Version: "1.4.0"}},
		lifecycleStubModule{id: "beta", metadata: modules.Metadata{
			Name: "Beta", Version: "1.0.0",
			Dependencies: []modules.Dependency{{ID: "alpha", VersionConstraint: ">=1.5.0", Required: true}},
		}},
	})

	plan, err := NewService(registry).LifecyclePlan(LifecycleActionUpgrade)
	require.NoError(t, err)
	require.Len(t, plan, 2)

	_, err = NewLifecycleService(registry).Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{})
	require.EqualError(t, err, "module beta requires alpha >=1.5.0, got 1.4.0")
}

func TestLifecyclePlanItemBuildsPublicDTOs(t *testing.T) {
	state := lifecyclePlannerState("alpha", false, "disabled", modules.Lifecycle{
		Upgrade: "migrate", DestructiveCheck: "module:manifest:check",
	})
	item := newLifecyclePlanItem(&state, LifecycleActionUpgrade)

	planDTO := item.planDTO()
	require.Equal(t, "alpha", planDTO.ID)
	require.False(t, planDTO.Enabled)
	require.Equal(t, "disabled", planDTO.Reason)
	require.Equal(t, "migrate", planDTO.Command)

	resultDTO := item.resultDTO()
	require.Equal(t, "alpha", resultDTO.ModuleID)
	require.Equal(t, "upgrade:alpha:1.0.0", resultDTO.IdempotencyKey)
	require.Equal(t, "module:manifest:check", resultDTO.DestructiveCheck)
}

func TestLifecyclePlannerValidatesVersionConstraints(t *testing.T) {
	t.Run("mismatch", func(t *testing.T) {
		planner := newLifecyclePlanner([]modules.ModuleState{
			lifecyclePlannerVersionState("alpha", "1.4.0"),
			lifecyclePlannerDependencyState("beta", "alpha", ">=1.5.0"),
		})

		require.EqualError(t, planner.validateVersionConstraints(), "module beta requires alpha >=1.5.0, got 1.4.0")
	})

	t.Run("missing", func(t *testing.T) {
		planner := newLifecyclePlanner([]modules.ModuleState{
			lifecyclePlannerDependencyState("beta", "alpha", ""),
		})

		require.EqualError(t, planner.validateVersionConstraints(), "module beta requires missing dependency: alpha")
	})

	t.Run("prerelease", func(t *testing.T) {
		planner := newLifecyclePlanner([]modules.ModuleState{
			lifecyclePlannerVersionState("alpha", "1.5.0-rc.1"),
			lifecyclePlannerDependencyState("beta", "alpha", ">=1.5.0"),
		})

		require.ErrorContains(t, planner.validateVersionConstraints(), "prerelease")
	})

	t.Run("build metadata", func(t *testing.T) {
		planner := newLifecyclePlanner([]modules.ModuleState{
			lifecyclePlannerVersionState("alpha", "1.5.0+build.1"),
			lifecyclePlannerDependencyState("beta", "alpha", "=1.5.0"),
		})

		require.NoError(t, planner.validateVersionConstraints())
	})
}

func TestLifecyclePlannerValidatesBatchPolicy(t *testing.T) {
	t.Run("manual", func(t *testing.T) {
		planner := newLifecyclePlanner([]modules.ModuleState{
			lifecyclePlannerState("alpha", true, "", modules.Lifecycle{Rollback: "migrate:rollback"}),
			lifecyclePlannerState("bravo", true, "", modules.Lifecycle{Rollback: "manual restore verified backup"}),
		})
		plan, err := planner.plan(LifecycleActionRollback, "")
		require.NoError(t, err)

		require.EqualError(t, planner.validateBatch(plan),
			"module lifecycle batch contains manual step for bravo: manual lifecycle step requires operator action: manual restore verified backup")
	})

	t.Run("disallowed", func(t *testing.T) {
		planner := newLifecyclePlanner([]modules.ModuleState{
			lifecyclePlannerState("alpha", true, "", modules.Lifecycle{Upgrade: "migrate"}),
			lifecyclePlannerState("bravo", true, "", modules.Lifecycle{Upgrade: "queue:work"}),
		})
		plan, err := planner.plan(LifecycleActionUpgrade, "")
		require.NoError(t, err)

		require.EqualError(t, planner.validateBatch(plan), "module lifecycle batch command not allowed for bravo: queue:work")
	})

	t.Run("disabled skipped", func(t *testing.T) {
		planner := newLifecyclePlanner([]modules.ModuleState{
			lifecyclePlannerState("alpha", true, "", modules.Lifecycle{Upgrade: "migrate"}),
			lifecyclePlannerState("bravo", false, "disabled", modules.Lifecycle{Upgrade: "manual review"}),
		})
		plan, err := planner.plan(LifecycleActionUpgrade, "")
		require.NoError(t, err)

		require.NoError(t, planner.validateBatch(plan))
	})
}

func TestLifecyclePlannerOrdersReplacementTargetBeforeDeprecatedRemoval(t *testing.T) {
	plan := replacementLifecyclePlan(t)
	plan.EndOfSupport = time.Now().Add(-time.Hour)
	planner := newLifecyclePlanner([]modules.ModuleState{
		lifecyclePlannerState("legacy", true, "", modules.Lifecycle{Uninstall: "migrate:rollback"}),
		lifecyclePlannerState("modern", true, "", modules.Lifecycle{Install: "migrate"}),
	})
	planner.replacements = map[string]modules.ReplacementPlan{"legacy": plan}

	ordered, err := planner.plan(LifecycleActionUninstall, "legacy")
	require.NoError(t, err)
	require.Equal(t, []string{"modern", "modern", "modern", "modern", "modern", "modern", "legacy"}, lifecyclePlanIDs(ordered.items))
	require.Equal(t, LifecycleActionInstall, ordered.items[0].action)
	require.Equal(t, "replacement:prepare:data_migration", ordered.items[1].action)
	require.Equal(t, "replacement:prepare:config_migration", ordered.items[2].action)
	require.Equal(t, "replacement:dual_run:permission_mapping", ordered.items[3].action)
	require.Equal(t, "replacement:dual_run:validation", ordered.items[4].action)
	require.Equal(t, "replacement:cutover:cutover", ordered.items[5].action)
	require.Equal(t, LifecycleActionUninstall, ordered.items[6].action)
	require.NotNil(t, ordered.rollback)
	require.Equal(t, "replacement:rollback_window:rollback", ordered.rollback.action)
	require.Equal(t, 5, ordered.rollbackAfter)
	require.NoError(t, planner.validateBatch(ordered))
}

func TestLifecyclePlannerBlocksDeprecatedRemovalBeforeEOS(t *testing.T) {
	plan := replacementLifecyclePlan(t)
	planner := newLifecyclePlanner([]modules.ModuleState{
		lifecyclePlannerState("legacy", true, "", modules.Lifecycle{Uninstall: "migrate:rollback"}),
		lifecyclePlannerState("modern", true, "", modules.Lifecycle{Install: "migrate"}),
	})
	planner.replacements = map[string]modules.ReplacementPlan{"legacy": plan}
	_, err := planner.plan(LifecycleActionUninstall, "legacy")
	require.ErrorContains(t, err, "end of support")
}

func TestLifecyclePlannerAllowsDeprecatedRemovalBeforeEOSOnlyAfterEmergencyApproval(t *testing.T) {
	plan := replacementLifecyclePlan(t)
	planner := newLifecyclePlanner([]modules.ModuleState{
		lifecyclePlannerState("legacy", true, "", modules.Lifecycle{Uninstall: "migrate:rollback"}),
		lifecyclePlannerState("modern", true, "", modules.Lifecycle{Install: "migrate"}),
	})
	planner.replacements = map[string]modules.ReplacementPlan{"legacy": plan}
	planner.emergencyRemovalApproved = true

	_, err := planner.plan(LifecycleActionUninstall, "legacy")

	require.NoError(t, err)
}

func replacementLifecyclePlan(t *testing.T) modules.ReplacementPlan {
	t.Helper()
	plan := modules.ReplacementPlan{
		FromModule: "legacy", ToModule: "modern", AnnouncedAt: time.Now().Add(-time.Hour),
		EndOfSupport: time.Now().Add(time.Hour), RemovalVersion: "2.0.0", DataMigration: "migrate",
		ConfigMigration: "migrate", PermissionMapping: "module:manifest:check", Validation: "module:manifest:check",
		Cutover: "migrate", Rollback: "migrate:rollback",
		Phases: []modules.ReplacementPhase{modules.ReplacementPhasePrepare, modules.ReplacementPhaseDualRun, modules.ReplacementPhaseCutover, modules.ReplacementPhaseRollbackWindow, modules.ReplacementPhaseRetired},
	}
	plan.CommandPolicyHashes = plan.CommandHashes()
	return plan
}

func lifecyclePlannerState(id string, enabled bool, reason string, lifecycle modules.Lifecycle) modules.ModuleState {
	return modules.ModuleState{
		ID: id, Enabled: enabled, Reason: reason,
		Metadata: modules.Metadata{Name: id, Version: "1.0.0", Lifecycle: lifecycle},
	}
}

func lifecyclePlannerCommands(id string) modules.Lifecycle {
	return modules.Lifecycle{
		Install: "install-" + id, Upgrade: "upgrade-" + id,
		Rollback: "rollback-" + id, Uninstall: "uninstall-" + id,
	}
}

func lifecyclePlannerVersionState(id string, version string) modules.ModuleState {
	return modules.ModuleState{ID: id, Enabled: true, Metadata: modules.Metadata{Name: id, Version: version}}
}

func lifecyclePlannerDependencyState(id string, dependencyID string, constraint string) modules.ModuleState {
	return modules.ModuleState{
		ID: id, Enabled: true,
		Metadata: modules.Metadata{
			Name: id, Version: "1.0.0",
			Dependencies: []modules.Dependency{{
				ID: dependencyID, VersionConstraint: constraint, Required: true,
			}},
		},
	}
}

func lifecyclePlanIDs(items []lifecyclePlanItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.state.ID)
	}
	return ids
}
