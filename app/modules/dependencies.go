package modules

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func disabledModuleSet() map[string]bool {
	values := []string{
		os.Getenv("MODULE_DISABLED"),
		os.Getenv("MODULES_DISABLED"),
	}
	disabled := map[string]bool{}
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			id := strings.TrimSpace(item)
			if id != "" {
				disabled[id] = true
			}
		}
	}

	return disabled
}

func (r Registry) validateDependencies() error {
	return r.kernel.validateDependencies()
}

func sortModulesByDependencies(modules []Module) ([]Module, error) {
	byID := map[string]Module{}
	for _, module := range modules {
		if _, exists := byID[module.ID()]; exists {
			return nil, fmt.Errorf("duplicate module id: %s", module.ID())
		}
		byID[module.ID()] = module
	}
	visited := map[string]bool{}
	visiting := map[string]bool{}
	sorted := make([]Module, 0, len(modules))

	var visit func(Module) error
	visit = func(module Module) error {
		id := module.ID()
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("module dependency cycle detected at %s", id)
		}
		visiting[id] = true
		dependencies := append([]Dependency(nil), ModuleMetadata(module).Dependencies...)
		sort.SliceStable(dependencies, func(i, j int) bool {
			return dependencies[i].ID < dependencies[j].ID
		})
		for _, dependency := range dependencies {
			dependencyModule, ok := byID[dependency.ID]
			if !ok {
				continue
			}
			if err := visit(dependencyModule); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		sorted = append(sorted, module)
		return nil
	}

	for _, module := range modules {
		if err := visit(module); err != nil {
			return nil, err
		}
	}

	return sorted, nil
}

func detectDependencyCycle(modules []Module) error {
	_, err := sortModulesByDependencies(modules)
	return err
}
