package modules

import (
	"errors"
	"fmt"
)

type registryKernel struct {
	source     []Module
	registered []Module
	lifecycle  []Module
	active     []Module
	disabled   map[string]bool
	reasons    map[string]string
}

func newRegistryKernel(items []Module, disabled map[string]bool) registryKernel {
	source := append([]Module(nil), items...)
	disabled = copyDisabledSet(disabled)
	registered := sortedOrOriginal(excludeDisabledModules(source, disabled))
	lifecycle := sortedOrOriginal(source)
	reasons := propagateDisabledReasons(source, disabled)

	return registryKernel{
		source:     source,
		registered: registered,
		lifecycle:  lifecycle,
		active:     excludeModulesWithReasons(registered, reasons),
		disabled:   disabled,
		reasons:    reasons,
	}
}

func (k registryKernel) sourceModules() []Module {
	return k.source
}

func (k registryKernel) registeredModules() []Module {
	return k.registered
}

func (k registryKernel) lifecycleModules() []Module {
	return k.lifecycle
}

func (k registryKernel) activeModules() []Module {
	return k.active
}

func (k registryKernel) sourceStates() []ModuleState {
	return projectModuleStates(k.source, k.reasons)
}

func (k registryKernel) lifecycleStates() []ModuleState {
	return projectModuleStates(k.lifecycle, k.reasons)
}

func (k registryKernel) disabledReason(id string) string {
	return k.reasons[id]
}

func (k registryKernel) validateDependencies() error {
	ids := moduleIDSet(k.registered)
	var errs []error
	for _, module := range k.registered {
		errs = append(errs, missingDependencyErrors(module, ids, k.disabled)...)
	}
	if err := detectDependencyCycle(k.registered); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func copyDisabledSet(source map[string]bool) map[string]bool {
	items := make(map[string]bool, len(source))
	for id, disabled := range source {
		items[id] = disabled
	}
	return items
}

func excludeDisabledModules(source []Module, disabled map[string]bool) []Module {
	items := make([]Module, 0, len(source))
	for _, module := range source {
		if !disabled[module.ID()] {
			items = append(items, module)
		}
	}
	return items
}

func sortedOrOriginal(source []Module) []Module {
	sorted, err := sortModulesByDependencies(source)
	if err != nil {
		return source
	}
	return sorted
}

func propagateDisabledReasons(source []Module, disabled map[string]bool) map[string]string {
	reasons := explicitDisabledReasons(source, disabled)
	for changed := true; changed; {
		changed = propagateDependencyReasons(source, reasons)
	}
	return reasons
}

func explicitDisabledReasons(source []Module, disabled map[string]bool) map[string]string {
	reasons := map[string]string{}
	for _, module := range source {
		if disabled[module.ID()] {
			reasons[module.ID()] = "disabled by MODULE_DISABLED"
		}
	}
	return reasons
}

func propagateDependencyReasons(source []Module, reasons map[string]string) bool {
	changed := false
	for _, module := range source {
		id := module.ID()
		if reasons[id] != "" {
			continue
		}
		if dependencyID := firstDisabledDependency(module, reasons); dependencyID != "" {
			reasons[id] = "disabled because dependency " + dependencyID + " is disabled"
			changed = true
		}
	}
	return changed
}

func firstDisabledDependency(module Module, reasons map[string]string) string {
	for _, dependency := range ModuleMetadata(module).Dependencies {
		if dependency.Required && reasons[dependency.ID] != "" {
			return dependency.ID
		}
	}
	return ""
}

func excludeModulesWithReasons(source []Module, reasons map[string]string) []Module {
	items := make([]Module, 0, len(source))
	for _, module := range source {
		if reasons[module.ID()] == "" {
			items = append(items, module)
		}
	}
	return items
}

func projectModuleStates(source []Module, reasons map[string]string) []ModuleState {
	states := make([]ModuleState, 0, len(source))
	for _, module := range source {
		reason := reasons[module.ID()]
		states = append(states, ModuleState{
			ID: module.ID(), Enabled: reason == "", Reason: reason, Metadata: ModuleMetadata(module),
		})
	}
	return states
}

func moduleIDSet(source []Module) map[string]bool {
	ids := make(map[string]bool, len(source))
	for _, module := range source {
		ids[module.ID()] = true
	}
	return ids
}

func missingDependencyErrors(module Module, ids, disabled map[string]bool) []error {
	var errs []error
	for _, dependency := range ModuleMetadata(module).Dependencies {
		if !dependency.Required || dependency.ID == "" || ids[dependency.ID] {
			continue
		}
		if disabled[dependency.ID] {
			errs = append(errs, fmt.Errorf("module %s requires disabled dependency: %s", module.ID(), dependency.ID))
			continue
		}
		errs = append(errs, fmt.Errorf("module %s requires missing dependency: %s", module.ID(), dependency.ID))
	}
	return errs
}
