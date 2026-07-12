package modulecatalog

import (
	"context"
	"sort"

	"goravel/app/http/request"
	"goravel/app/modules"
)

type adminStateQuery struct {
	registry modules.Registry
	ctx      context.Context
}

type adminStateSnapshot struct {
	items     []ModuleStateItem
	persisted map[string]*PersistedModuleState
}

func newAdminStateQuery(registry modules.Registry, ctx context.Context) adminStateQuery {
	return adminStateQuery{registry: registry, ctx: contextOrBackground(ctx)}
}

func (q adminStateQuery) state() (request.PageResult[ModuleStateItem], error) {
	snapshot, err := q.load()
	if err != nil {
		return request.PageResult[ModuleStateItem]{}, err
	}
	return request.PageResult[ModuleStateItem]{List: snapshot.items, Total: int64(len(snapshot.items))}, nil
}

func (q adminStateQuery) diff() (request.PageResult[AdminStateDiffItem], error) {
	snapshot, err := q.load()
	if err != nil {
		return request.PageResult[AdminStateDiffItem]{}, err
	}
	items := manifestStateDiffs(snapshot.items)
	items = append(items, orphanStateDiffs(snapshot.items, snapshot.persisted)...)
	return request.PageResult[AdminStateDiffItem]{List: items, Total: int64(len(items))}, nil
}

func (q adminStateQuery) load() (adminStateSnapshot, error) {
	persisted, err := persistedModuleStates(q.ctx)
	if err != nil {
		return adminStateSnapshot{}, err
	}
	items := newStateProjector().project(q.registry.ModuleStates(), persisted)
	return adminStateSnapshot{items: items, persisted: persisted}, nil
}

func manifestStateDiffs(state []ModuleStateItem) []AdminStateDiffItem {
	items := make([]AdminStateDiffItem, 0, len(state))
	for _, item := range state {
		diff := AdminStateDiffItem{
			ModuleID: item.ID, Name: item.Name, ManifestVersion: item.Version,
			ManifestEnabled: item.Enabled, Drift: "missing_state",
		}
		if item.Persisted != nil {
			diff.PersistedVersion = item.Persisted.TargetVersion
			diff.PersistedEnabled = item.Persisted.Enabled
			diff.PersistedStatus = item.Persisted.Status
			diff.LastAction = item.Persisted.LastAction
			diff.Drift = stateDrift(item, diff.PersistedVersion)
		}
		items = append(items, diff)
	}
	return items
}

func orphanStateDiffs(state []ModuleStateItem, persisted map[string]*PersistedModuleState) []AdminStateDiffItem {
	manifestIDs := make(map[string]struct{}, len(state))
	for _, item := range state {
		manifestIDs[item.ID] = struct{}{}
	}
	orphanIDs := make([]string, 0)
	for moduleID := range persisted {
		if _, ok := manifestIDs[moduleID]; !ok {
			orphanIDs = append(orphanIDs, moduleID)
		}
	}
	sort.Strings(orphanIDs)
	items := make([]AdminStateDiffItem, 0, len(orphanIDs))
	for _, moduleID := range orphanIDs {
		item := persisted[moduleID]
		items = append(items, AdminStateDiffItem{
			ModuleID: moduleID, Name: item.Name, PersistedVersion: item.TargetVersion,
			PersistedEnabled: item.Enabled, PersistedStatus: item.Status,
			LastAction: item.LastAction, Drift: "missing_manifest",
		})
	}
	return items
}

func stateDrift(item ModuleStateItem, persistedVersion string) string {
	if item.Persisted == nil {
		return "missing_state"
	}
	if item.Enabled != item.Persisted.Enabled {
		return "enabled_mismatch"
	}
	if persistedVersion != "" && item.Version != persistedVersion {
		return "version_mismatch"
	}
	if item.Persisted.LastError != "" {
		return "last_error"
	}
	return "in_sync"
}
