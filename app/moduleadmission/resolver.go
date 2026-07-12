package moduleadmission

import (
	"fmt"
	"sort"
	"strings"
)

func Resolve(index RepositoryIndex, requested []ModuleReference, sources []SourceModuleMetadata) (Resolution, error) {
	entries := make(map[string]ModuleIndexEntry, len(index.Modules))
	for _, entry := range index.Modules {
		entries[moduleVersionKey(entry.ID, entry.Version)] = entry
	}
	sourceByKey := make(map[string]SourceModuleMetadata, len(sources))
	sourceByID := make(map[string]SourceModuleMetadata, len(sources))
	for _, source := range sources {
		sourceByKey[moduleVersionKey(source.ID, source.Version)] = source
		sourceByID[source.ID] = source
	}
	resolved := make(map[string]ModuleIndexEntry)
	visiting := make(map[string]bool)
	selectedVersions := make(map[string]string)
	var visit func(ModuleReference) error
	visit = func(reference ModuleReference) error {
		reference.ID = strings.TrimSpace(reference.ID)
		reference.Version = strings.TrimSpace(reference.Version)
		if !exactVersionPattern.MatchString(reference.Version) {
			return fmt.Errorf("module %s requires exact version", reference.ID)
		}
		if selected, ok := selectedVersions[reference.ID]; ok && selected != reference.Version {
			return fmt.Errorf("module version conflict: %s requires both %s and %s", reference.ID, selected, reference.Version)
		}
		selectedVersions[reference.ID] = reference.Version
		key := moduleVersionKey(reference.ID, reference.Version)
		if _, ok := resolved[key]; ok {
			return nil
		}
		if visiting[key] {
			return fmt.Errorf("module dependency cycle detected at %s", key)
		}
		entry, ok := entries[key]
		if !ok {
			return fmt.Errorf("missing dependency: %s", key)
		}
		if source, present := sourceByKey[key]; present {
			if err := validateSourceMetadata(entry, source); err != nil {
				return err
			}
		} else if source, present := sourceByID[entry.ID]; present && source.Version != entry.Version {
			return fmt.Errorf("source metadata mismatch for %s@%s", entry.ID, entry.Version)
		}
		visiting[key] = true
		dependencies := append([]IndexDependency(nil), entry.Dependencies...)
		sort.Slice(dependencies, func(left, right int) bool { return dependencies[left].ID < dependencies[right].ID })
		for _, dependency := range dependencies {
			if !dependency.Required {
				continue
			}
			if !exactVersionPattern.MatchString(dependency.VersionConstraint) {
				return fmt.Errorf("module %s dependency %s requires exact version", entry.ID, dependency.ID)
			}
			if err := visit(ModuleReference{ID: dependency.ID, Version: dependency.VersionConstraint}); err != nil {
				return err
			}
		}
		visiting[key] = false
		resolved[key] = entry
		return nil
	}
	for _, reference := range requested {
		if err := visit(reference); err != nil {
			return Resolution{}, err
		}
	}
	modules := make([]ModuleIndexEntry, 0, len(resolved))
	for _, entry := range resolved {
		modules = append(modules, entry)
	}
	sort.Slice(modules, func(left, right int) bool {
		if modules[left].ID == modules[right].ID {
			return modules[left].Version < modules[right].Version
		}
		return modules[left].ID < modules[right].ID
	})
	ordered, err := dependencyOrdered(modules)
	if err != nil {
		return Resolution{}, err
	}
	canonical, err := jsonMarshal(ordered)
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{IndexDigest: index.Digest, Modules: ordered, GraphDigest: sha256Digest(canonical)}, nil
}

func (r Resolution) ModuleIDs() []string {
	ids := make([]string, 0, len(r.Modules))
	for _, module := range r.Modules {
		ids = append(ids, module.ID)
	}
	return ids
}

func validateSourceMetadata(entry ModuleIndexEntry, source SourceModuleMetadata) error {
	if entry.ID != source.ID || entry.Version != source.Version || entry.GoImportPath != source.GoImportPath || !sourceMatchesModuleGraph(entry, source) {
		return fmt.Errorf("source metadata mismatch for %s@%s", entry.ID, entry.Version)
	}
	if !equalDependencies(entry.Dependencies, source.Dependencies) {
		return fmt.Errorf("source metadata mismatch for %s@%s dependencies", entry.ID, entry.Version)
	}
	return nil
}

func sourceMatchesModuleGraph(entry ModuleIndexEntry, source SourceModuleMetadata) bool {
	if source.GoModulePath == "" {
		return true
	}
	return entry.GoImportPath == source.GoModulePath || strings.HasPrefix(entry.GoImportPath, strings.TrimSuffix(source.GoModulePath, "/")+"/")
}

func equalDependencies(left, right []IndexDependency) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]IndexDependency(nil), left...)
	right = append([]IndexDependency(nil), right...)
	sort.Slice(left, func(first, second int) bool { return left[first].ID < left[second].ID })
	sort.Slice(right, func(first, second int) bool { return right[first].ID < right[second].ID })
	for item := range left {
		if left[item] != right[item] {
			return false
		}
	}
	return true
}

func dependencyOrdered(modules []ModuleIndexEntry) ([]ModuleIndexEntry, error) {
	byID := make(map[string]ModuleIndexEntry, len(modules))
	for _, module := range modules {
		byID[module.ID] = module
	}
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	ordered := make([]ModuleIndexEntry, 0, len(modules))
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("module dependency cycle detected at %s", id)
		}
		module := byID[id]
		visiting[id] = true
		for _, dependency := range module.Dependencies {
			if dependency.Required {
				if err := visit(dependency.ID); err != nil {
					return err
				}
			}
		}
		visiting[id] = false
		visited[id] = true
		ordered = append(ordered, module)
		return nil
	}
	for _, module := range modules {
		if err := visit(module.ID); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func moduleVersionKey(id, version string) string { return id + "@" + version }
