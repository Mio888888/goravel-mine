package modules

import (
	"reflect"
	"strings"
	"testing"
)

func TestRegistryKernelBuildsStableProjections(t *testing.T) {
	kernel := newRegistryKernel([]Module{
		stubModule{id: "gamma", metadata: Metadata{Dependencies: []Dependency{{ID: "beta", Required: true}}}},
		stubModule{id: "beta", metadata: Metadata{Dependencies: []Dependency{{ID: "alpha", Required: true}}}},
		stubModule{id: "alpha"},
		stubModule{id: "delta"},
	}, map[string]bool{"alpha": true})

	assertModuleIDs(t, kernel.sourceModules(), []string{"gamma", "beta", "alpha", "delta"})
	assertModuleIDs(t, kernel.registeredModules(), []string{"beta", "gamma", "delta"})
	assertModuleIDs(t, kernel.lifecycleModules(), []string{"alpha", "beta", "gamma", "delta"})
	assertModuleIDs(t, kernel.activeModules(), []string{"delta"})

	if got := kernel.disabledReason("alpha"); got != "disabled by MODULE_DISABLED" {
		t.Fatalf("disabledReason(alpha) = %q", got)
	}
	if got := kernel.disabledReason("beta"); got != "disabled because dependency alpha is disabled" {
		t.Fatalf("disabledReason(beta) = %q", got)
	}
	if got := kernel.disabledReason("gamma"); got != "disabled because dependency beta is disabled" {
		t.Fatalf("disabledReason(gamma) = %q", got)
	}
}

func TestRegistryKernelKeepsOptionalDependentsActive(t *testing.T) {
	kernel := newRegistryKernel([]Module{
		stubModule{id: "beta", metadata: Metadata{Dependencies: []Dependency{{ID: "alpha"}}}},
		stubModule{id: "alpha"},
	}, map[string]bool{"alpha": true})

	assertModuleIDs(t, kernel.activeModules(), []string{"beta"})
	if got := kernel.disabledReason("beta"); got != "" {
		t.Fatalf("disabledReason(beta) = %q, want empty", got)
	}
}

func TestRegistryKernelFallsBackToInputOrderAndReportsCycle(t *testing.T) {
	kernel := newRegistryKernel([]Module{
		stubModule{id: "beta", metadata: Metadata{Dependencies: []Dependency{{ID: "alpha", Required: true}}}},
		stubModule{id: "alpha", metadata: Metadata{Dependencies: []Dependency{{ID: "beta", Required: true}}}},
	}, nil)

	assertModuleIDs(t, kernel.registeredModules(), []string{"beta", "alpha"})
	assertModuleIDs(t, kernel.lifecycleModules(), []string{"beta", "alpha"})
	if err := kernel.validateDependencies(); err == nil || !strings.Contains(err.Error(), "module dependency cycle detected") {
		t.Fatalf("validateDependencies() error = %v", err)
	}
}

func TestRegistryKernelFallsBackToInputOrderAndReportsDuplicateID(t *testing.T) {
	kernel := newRegistryKernel([]Module{
		stubModule{id: "alpha"},
		stubModule{id: "alpha"},
	}, nil)

	assertModuleIDs(t, kernel.registeredModules(), []string{"alpha", "alpha"})
	assertModuleIDs(t, kernel.lifecycleModules(), []string{"alpha", "alpha"})
	if err := kernel.validateDependencies(); err == nil || !strings.Contains(err.Error(), "duplicate module id: alpha") {
		t.Fatalf("validateDependencies() error = %v", err)
	}
}

func assertModuleIDs(t *testing.T, items []Module, expected []string) {
	t.Helper()
	actual := make([]string, 0, len(items))
	for _, item := range items {
		actual = append(actual, item.ID())
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("module IDs = %#v, want %#v", actual, expected)
	}
}
