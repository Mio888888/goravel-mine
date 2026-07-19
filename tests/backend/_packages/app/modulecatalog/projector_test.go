package modulecatalog

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/modules"
)

func TestManifestProjectorMapsCatalogContract(t *testing.T) {
	projector := newManifestProjector()
	manifest := projector.project(modules.Catalog{Modules: []modules.ModuleCatalog{projectorCatalogModule()}})

	require.Len(t, manifest.Modules, 1)
	item := manifest.Modules[0]
	require.Equal(t, "alpha", item.ID)
	require.False(t, item.Enabled)
	require.Equal(t, "disabled by MODULE_DISABLED", item.Reason)
	require.Equal(t, []ManifestDependency{{ID: "core", VersionConstraint: ">=1.0.0", Required: true}}, item.Dependencies)
	require.Equal(t, []string{"alpha:list", "alpha:export"}, item.Routes[0].Permissions)
	require.Equal(t, []string{"202607100001_alpha"}, item.Migrations)
}

func TestStateProjectorUsesSharedMetadataMapping(t *testing.T) {
	state := modules.ModuleState{
		ID: "alpha", Enabled: false, Reason: "disabled by MODULE_DISABLED",
		Metadata: projectorMetadata(),
	}
	persisted := &PersistedModuleState{Status: "disabled", Enabled: false, Owner: "platform"}
	items := newStateProjector().project([]modules.ModuleState{state}, map[string]*PersistedModuleState{"alpha": persisted})

	require.Len(t, items, 1)
	require.Equal(t, "Alpha", items[0].Name)
	require.Equal(t, []ManifestDependency{{ID: "core", VersionConstraint: ">=1.0.0", Required: true}}, items[0].DependsOn)
	require.Equal(t, "go run . artisan migrate", items[0].Lifecycle.Upgrade)
	require.Same(t, persisted, items[0].Persisted)
}

func TestCompatibilityProjectorUsesFixedUTCClock(t *testing.T) {
	fixed := time.Date(2026, 7, 10, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	projector := newCompatibilityProjector(
		func() time.Time { return fixed },
		func(modules.Package, string) (bool, error) { return false, errors.New("framework mismatch") },
	)
	matrix := projector.project(modules.Catalog{Modules: []modules.ModuleCatalog{projectorCatalogModule()}}, " 1.17.2 ")

	require.Equal(t, "1.17.2", matrix.FrameworkVersion)
	require.Equal(t, fixed.UTC(), matrix.GeneratedAt)
	require.Equal(t, "passed", matrix.Status, "disabled incompatible modules do not fail matrix")
	require.Len(t, matrix.Modules, 1)
	require.False(t, matrix.Modules[0].FrameworkCompatible)
	require.Equal(t, "framework mismatch", matrix.Modules[0].CompatibilityError)
	require.Equal(t, "disabled by MODULE_DISABLED", matrix.Modules[0].DisabledReason)
}

func projectorCatalogModule() modules.ModuleCatalog {
	return modules.ModuleCatalog{
		ID: "alpha", Enabled: false, Reason: "disabled by MODULE_DISABLED",
		Metadata: projectorMetadata(),
		Package: modules.Package{
			RegistryKey: "alpha", Version: "1.2.3", ReleaseTrack: "internal",
		},
		Routes: []modules.Route{{
			Name: "alpha.list", Method: "GET", Path: "/alpha", Permission: "alpha:list",
			Permissions: []string{"alpha:list", "alpha:export"},
		}},
		Menus:         []modules.Menu{{Key: "alpha", Title: "Alpha", Path: "/alpha", Sort: 10}},
		Permissions:   []modules.Permission{{Key: "alpha:list", Description: "Alpha list"}},
		Migrations:    []string{"202607100001_alpha"},
		OpenAPIFiles:  []string{"alpha.openapi.json"},
		TestTemplates: []string{"alpha_test.go"},
	}
}

func projectorMetadata() modules.Metadata {
	return modules.Metadata{
		Name: "Alpha", Version: "1.2.3", Compatible: ">=1.17.0",
		Dependencies: []modules.Dependency{{ID: "core", VersionConstraint: ">=1.0.0", Required: true}},
		Lifecycle: modules.Lifecycle{
			Install: "install", Uninstall: "uninstall", Upgrade: "go run . artisan migrate", Rollback: "rollback",
			DestructiveCheck: "check", RequiresRestart: true, BreakingChangePolicy: "review",
		},
		SeedStrategy: modules.SeedStrategy{Mode: "idempotent", Idempotent: true, Command: "seed"},
		Frontend:     modules.FrontendArtifact{ModulePath: "MineAdmin-web/src/modules/alpha"},
	}
}
