package modules

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
)

func TestRegistrySortsModulesByDependencies(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "beta",
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "alpha", Required: true}},
			},
		},
		stubModule{id: "alpha"},
	})

	if got := registry.IDs(); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("IDs() = %#v, want dependency order", got)
	}
}

func TestRegistryValidatesMissingDependencies(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "beta",
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "alpha", Required: true}},
			},
		},
	})

	err := registry.Validate()
	if err == nil {
		t.Fatal("expected missing dependency validation error")
	}
	if !strings.Contains(err.Error(), "module beta requires missing dependency: alpha") {
		t.Fatalf("validation error = %q", err.Error())
	}
}

func TestRegistryValidatesDisabledDependencies(t *testing.T) {
	t.Setenv("MODULE_DISABLED", "alpha")
	registry := NewRegistry([]Module{
		stubModule{id: "alpha", seeders: []seeder.Seeder{stubSeeder{}}},
		stubModule{
			id:         "beta",
			migrations: []schema.Migration{namedMigration{signature: "202607080001_beta"}},
			seeders:    []seeder.Seeder{stubSeeder{}},
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "alpha", Required: true}},
			},
		},
	})

	err := registry.Validate()
	if err == nil {
		t.Fatal("expected disabled dependency validation error")
	}
	if !strings.Contains(err.Error(), "module beta requires disabled dependency: alpha") {
		t.Fatalf("validation error = %q", err.Error())
	}

	states := registry.ModuleStates()
	if len(states) != 2 || states[0].Enabled || states[0].Reason == "" {
		t.Fatalf("ModuleStates() = %#v, want disabled alpha state", states)
	}
	if states[1].Enabled {
		t.Fatalf("ModuleStates() = %#v, want beta disabled by dependency cascade", states)
	}
	if !strings.Contains(states[1].Reason, "dependency alpha is disabled") {
		t.Fatalf("ModuleStates()[1].Reason = %q, want dependency disabled reason", states[1].Reason)
	}

	catalog := registry.ModuleCatalog()
	if len(catalog.Modules) != 2 || catalog.Modules[1].Enabled {
		t.Fatalf("ModuleCatalog() = %#v, want beta disabled by dependency cascade", catalog.Modules)
	}
	if !strings.Contains(catalog.Modules[1].Reason, "dependency alpha is disabled") {
		t.Fatalf("ModuleCatalog()[1].Reason = %q, want dependency disabled reason", catalog.Modules[1].Reason)
	}

	if got := registry.Migrations(); len(got) != 0 {
		t.Fatalf("Migrations() = %#v, want disabled dependency cascade excluded", got)
	}
	if got := registry.Seeders(); len(got) != 0 {
		t.Fatalf("Seeders() = %#v, want disabled dependency cascade excluded", got)
	}
}

func TestRegistryValidatesDependencyCycles(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "beta", Required: true}},
			},
		},
		stubModule{
			id: "beta",
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "alpha", Required: true}},
			},
		},
	})

	err := registry.Validate()
	if err == nil {
		t.Fatal("expected dependency cycle validation error")
	}
	if !strings.Contains(err.Error(), "module dependency cycle detected") {
		t.Fatalf("validation error = %q", err.Error())
	}
}

func TestRegistryPreservesInputOrderWhenDependencySortFails(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "beta",
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "alpha", Required: true}},
			},
		},
		stubModule{
			id: "alpha",
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "beta", Required: true}},
			},
		},
	})

	if got := registry.IDs(); !reflect.DeepEqual(got, []string{"beta", "alpha"}) {
		t.Fatalf("IDs() = %#v, want input-order fallback", got)
	}
}

func TestRegistryPreservesInputOrderForLifecycleStatesWhenDependencySortFails(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "beta",
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "alpha", Required: true}},
			},
		},
		stubModule{
			id: "alpha",
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "beta", Required: true}},
			},
		},
	})

	states := registry.LifecycleStates()
	got := make([]string, 0, len(states))
	for _, state := range states {
		got = append(got, state.ID)
	}

	if !reflect.DeepEqual(got, []string{"beta", "alpha"}) {
		t.Fatalf("LifecycleStates() IDs = %#v, want input-order fallback", got)
	}
}

func TestRegistryPreservesInputOrderWhileValidateStillReportsDependencyCycleWhenSortFallsBack(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "beta",
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "alpha", Required: true}},
			},
		},
		stubModule{
			id: "alpha",
			metadata: Metadata{
				Dependencies: []Dependency{{ID: "beta", Required: true}},
			},
		},
	})

	err := registry.Validate()
	if err == nil {
		t.Fatal("expected dependency cycle validation error")
	}
	if !strings.Contains(err.Error(), "module dependency cycle detected") {
		t.Fatalf("validation error = %q", err.Error())
	}
}

func TestModuleMetadataAllowsFalseBoolOverrides(t *testing.T) {
	metadata := ModuleMetadata(stubModule{
		id: "alpha",
		metadata: Metadata{
			SeedStrategy: SeedStrategy{Mode: "manual"},
			Overrides: MetadataOverrides{
				RequiresRestart:    Bool(false),
				SupportsHotDisable: Bool(true),
				SeedIdempotent:     Bool(false),
			},
		},
	})

	if metadata.Lifecycle.RequiresRestart {
		t.Fatal("RequiresRestart = true, want false")
	}
	if !metadata.Lifecycle.SupportsHotDisable {
		t.Fatal("SupportsHotDisable = false, want true")
	}
	if metadata.SeedStrategy.Idempotent {
		t.Fatal("SeedStrategy.Idempotent = true, want false")
	}
}
