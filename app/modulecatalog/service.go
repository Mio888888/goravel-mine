package modulecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"goravel/app/modules"
)

type Service struct {
	registry modules.Registry
}

func NewService(registry modules.Registry) Service {
	return Service{registry: registry}
}

func (s Service) Validate() error {
	return s.registry.Validate()
}

func (s Service) ValidateRuntime() error {
	return s.registry.ValidateRuntime()
}

func (s Service) Manifest() Manifest {
	return newManifestProjector().project(s.registry.ModuleCatalog())
}

func (s Service) ModuleStates() []modules.ModuleState {
	return s.registry.ModuleStates()
}

func (s Service) ModuleStateManifest() ([]ModuleStateItem, error) {
	return s.moduleStateManifest(context.Background())
}

func (s Service) moduleStateManifest(ctx context.Context) ([]ModuleStateItem, error) {
	states := s.registry.ModuleStates()
	persisted, err := persistedModuleStates(ctx)
	if err != nil {
		return nil, err
	}
	return newStateProjector().project(states, persisted), nil
}

func (s Service) ManifestJSON() ([]byte, error) {
	return json.MarshalIndent(s.Manifest(), "", "  ")
}

func (s Service) CompatibilityMatrix(frameworkVersion string) CompatibilityMatrix {
	projector := newCompatibilityProjector(time.Now, packageSupportsFramework)
	return projector.project(s.registry.ModuleCatalog(), frameworkVersion)
}

func packageSupportsFramework(pkg modules.Package, frameworkVersion string) (bool, error) {
	frameworkVersion = strings.TrimSpace(frameworkVersion)
	if frameworkVersion == "" {
		return false, errors.New("framework version is required")
	}
	if len(pkg.Compatibility) == 0 {
		return false, errors.New("package compatibility matrix is required")
	}
	for _, constraint := range pkg.Compatibility {
		ok, err := versionSatisfies(frameworkVersion, constraint)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func (s Service) CompatibilityMatrixJSON(frameworkVersion string) ([]byte, error) {
	return json.MarshalIndent(s.CompatibilityMatrix(frameworkVersion), "", "  ")
}

func (s Service) LifecyclePlan(action string) ([]LifecyclePlanItem, error) {
	plan, err := newLifecyclePlannerWithReplacements(s.registry.LifecycleStates(), s.registry.ReplacementPlans()).plan(action, "")
	if err != nil {
		return nil, err
	}
	items := make([]LifecyclePlanItem, 0, len(plan.items))
	for _, item := range plan.items {
		items = append(items, item.planDTO())
	}
	return items, nil
}

func (s Service) ValidateManifestParity(seed ManifestSeedParity, frontend ManifestFrontendParity) error {
	return validateManifestParity(s.Manifest(), seed, frontend)
}
