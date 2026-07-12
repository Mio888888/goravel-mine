package modulecatalog

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"goravel/app/modules"
)

func validateLifecycleVersionConstraints(states []modules.ModuleState) error {
	versions := make(map[string]string, len(states))
	for _, state := range states {
		versions[state.ID] = state.Metadata.Version
	}
	var errs []error
	for _, state := range states {
		errs = append(errs, lifecycleDependencyVersionErrors(state, versions)...)
	}
	return errors.Join(errs...)
}

func lifecycleDependencyVersionErrors(state modules.ModuleState, versions map[string]string) []error {
	var errs []error
	for _, dependency := range state.Metadata.Dependencies {
		if !dependency.Required {
			continue
		}
		version, ok := versions[dependency.ID]
		if !ok {
			errs = append(errs, fmt.Errorf("module %s requires missing dependency: %s", state.ID, dependency.ID))
			continue
		}
		errs = append(errs, lifecycleVersionConstraintErrors(state.ID, dependency, version)...)
	}
	return errs
}

func lifecycleVersionConstraintErrors(moduleID string, dependency modules.Dependency, version string) []error {
	if strings.TrimSpace(dependency.VersionConstraint) == "" {
		return nil
	}
	ok, err := versionSatisfies(version, dependency.VersionConstraint)
	if err != nil {
		return []error{fmt.Errorf("module %s dependency %s version constraint invalid: %w", moduleID, dependency.ID, err)}
	}
	if !ok {
		return []error{fmt.Errorf("module %s requires %s %s, got %s", moduleID, dependency.ID, dependency.VersionConstraint, version)}
	}
	return nil
}

func versionSatisfies(version, constraint string) (bool, error) {
	parts := splitVersionConstraints(constraint)
	if len(parts) == 0 {
		if strings.TrimSpace(constraint) == "" {
			return true, nil
		}
		return false, errors.New("empty version constraint")
	}
	for _, part := range parts {
		ok, err := versionSatisfiesSingle(version, part)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
}

func splitVersionConstraints(constraint string) []string {
	return strings.FieldsFunc(strings.TrimSpace(constraint), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}

func versionSatisfiesSingle(version, constraint string) (bool, error) {
	operator, target := splitVersionOperator(constraint)
	if target == "" {
		return false, errors.New("empty version")
	}
	cmp, err := compareVersions(version, target)
	if err != nil {
		return false, err
	}
	return compareVersionResult(cmp, operator), nil
}

func splitVersionOperator(constraint string) (string, string) {
	for _, operator := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(constraint, operator) {
			return operator, strings.TrimSpace(strings.TrimPrefix(constraint, operator))
		}
	}
	return "=", strings.TrimSpace(constraint)
}

func compareVersionResult(cmp int, operator string) bool {
	switch operator {
	case ">=":
		return cmp >= 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	case "<":
		return cmp < 0
	default:
		return cmp == 0
	}
}

func compareVersions(left, right string) (int, error) {
	leftParts, err := parseLifecycleVersion(left)
	if err != nil {
		return 0, err
	}
	rightParts, err := parseLifecycleVersion(right)
	if err != nil {
		return 0, err
	}
	leftParts, rightParts = equalizeVersionParts(leftParts, rightParts)
	for index := range leftParts {
		if leftParts[index] > rightParts[index] {
			return 1, nil
		}
		if leftParts[index] < rightParts[index] {
			return -1, nil
		}
	}
	return 0, nil
}

func equalizeVersionParts(left, right []int) ([]int, []int) {
	length := max(len(left), len(right))
	for len(left) < length {
		left = append(left, 0)
	}
	for len(right) < length {
		right = append(right, 0)
	}
	return left, right
}

func parseLifecycleVersion(version string) ([]int, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	version = strings.Split(version, "+")[0]
	if strings.Contains(version, "-") {
		return nil, fmt.Errorf("prerelease version %q is not supported", version)
	}
	if version == "" {
		return nil, errors.New("empty version")
	}
	parts := make([]int, 0, strings.Count(version, ".")+1)
	for _, raw := range strings.Split(version, ".") {
		if strings.TrimSpace(raw) == "" {
			parts = append(parts, 0)
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid version %q", version)
		}
		parts = append(parts, value)
	}
	return parts, nil
}
