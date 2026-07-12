package modulecatalog

import "goravel/app/modules"

type manifestProjector struct {
	mapper dtoMapper
}

func newManifestProjector() manifestProjector {
	return manifestProjector{}
}

func (p manifestProjector) project(catalog modules.Catalog) Manifest {
	return Manifest{Modules: mapSlice(catalog.Modules, p.mapper.manifestItem)}
}

type stateProjector struct {
	mapper dtoMapper
}

func newStateProjector() stateProjector {
	return stateProjector{}
}

func (p stateProjector) project(states []modules.ModuleState, persisted map[string]*PersistedModuleState) []ModuleStateItem {
	items := make([]ModuleStateItem, 0, len(states))
	for _, state := range states {
		items = append(items, p.mapper.stateItem(state, persisted[state.ID]))
	}
	return items
}
