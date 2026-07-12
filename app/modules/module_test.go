package modules

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
)

type stubMigration struct{}

func (stubMigration) Signature() string { return "20260101000000_stub" }
func (stubMigration) Up() error         { return nil }
func (stubMigration) Down() error       { return nil }

type namedMigration struct{ signature string }

func (m namedMigration) Signature() string { return m.signature }
func (m namedMigration) Up() error         { return nil }
func (m namedMigration) Down() error       { return nil }

type stubSeeder struct{}

func (stubSeeder) Signature() string { return "stub" }
func (stubSeeder) Run() error        { return nil }

type stubModule struct {
	id          string
	metadata    Metadata
	pkg         Package
	routes      []Route
	menus       []Menu
	permissions []Permission
	migrations  []schema.Migration
	seeders     []seeder.Seeder
	openAPI     []string
	tests       []string
}

func TestRegistryCatalogValidatesDuplicates(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id:       "alpha",
			metadata: Metadata{Version: "1.2.3"},
			routes: []Route{{
				Name:   "alpha.index",
				Method: "GET",
				Path:   "/admin/alpha/list",
			}},
			permissions: []Permission{{Key: "alpha:list"}},
		},
		stubModule{
			id:       "alpha",
			metadata: Metadata{Version: "1.2.3"},
			routes: []Route{{
				Name:   "alpha.index",
				Method: "GET",
				Path:   "/admin/alpha/list",
			}},
			permissions: []Permission{{Key: "alpha:list"}},
		},
	})

	err := registry.Validate()
	if err == nil {
		t.Fatal("expected duplicate validation error")
	}
	message := err.Error()
	for _, expected := range []string{
		"duplicate module id: alpha",
		"duplicate route name: alpha.index",
		"duplicate route path: GET /admin/alpha/list",
		"duplicate permission key: alpha:list",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("validation error %q missing %q", message, expected)
		}
	}
}

func TestCatalogValidationPreservesCombinedErrorOrder(t *testing.T) {
	catalog := Catalog{Modules: []ModuleCatalog{
		{ID: "alpha"},
		{
			ID: "alpha",
			Routes: []Route{{
				Name:       "alpha.index",
				Method:     "GET",
				Path:       "/admin/alpha/list",
				Permission: "alpha:list",
			}},
		},
	}}

	err := catalog.ValidateRuntime()
	if err == nil {
		t.Fatal("expected combined catalog validation error")
	}
	want := strings.Join([]string{
		"duplicate module id: alpha",
		"route alpha.index has no installer",
		"route alpha.index references missing permission: alpha:list",
	}, "\n")
	if err.Error() != want {
		t.Fatalf("validation error = %q, want %q", err.Error(), want)
	}
}

func TestRegistryCatalogValidatesMenuPermissions(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			menus: []Menu{{
				Key:        "alpha",
				Permission: "alpha:list",
			}},
		},
	})

	err := registry.Validate()
	if err == nil {
		t.Fatal("expected missing permission validation error")
	}
	if !strings.Contains(err.Error(), "menu alpha references missing permission: alpha:list") {
		t.Fatalf("validation error = %q", err.Error())
	}
}

func TestRegistryCatalogValidatesRoutePermissions(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			routes: []Route{{
				Name:       "alpha.index",
				Method:     "GET",
				Path:       "/admin/alpha/list",
				Permission: "alpha:list",
			}},
		},
	})

	err := registry.Validate()
	if err == nil {
		t.Fatal("expected missing route permission validation error")
	}
	if !strings.Contains(err.Error(), "route alpha.index references missing permission: alpha:list") {
		t.Fatalf("validation error = %q", err.Error())
	}
}

func TestRegistryCatalogValidatesRouteInstallers(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			routes: []Route{{
				Name:       "alpha.index",
				Method:     "GET",
				Path:       "/admin/alpha/list",
				Permission: "alpha:list",
			}},
			permissions: []Permission{{Key: "alpha:list"}},
		},
	})

	err := registry.Validate()
	if err == nil {
		t.Fatal("expected missing route installer validation error")
	}
	if !strings.Contains(err.Error(), "route alpha.index has no installer") {
		t.Fatalf("validation error = %q", err.Error())
	}
}

func TestRegistryCatalogValidatesMetadataFiles(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id:      "alpha",
			openAPI: []string{"docs/api-contract/openapi/missing-alpha.openapi.json"},
			tests:   []string{"tests/feature/admin/missing_alpha_test.go"},
		},
	})

	err := registry.Validate()
	if err == nil {
		t.Fatal("expected missing metadata file validation error")
	}
	message := err.Error()
	for _, expected := range []string{
		"openapi file does not exist: docs/api-contract/openapi/missing-alpha.openapi.json",
		"test template does not exist: tests/feature/admin/missing_alpha_test.go",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("validation error %q missing %q", message, expected)
		}
	}
}

func TestRegistryRuntimeValidationSkipsMetadataFileExistence(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			routes: []Route{{
				Name:       "alpha.index",
				Method:     "GET",
				Path:       "/admin/alpha/list",
				Permission: "alpha:list",
				Install:    func() {},
			}},
			permissions: []Permission{{Key: "alpha:list"}},
			openAPI:     []string{"docs/api-contract/openapi/missing-alpha.openapi.json"},
			tests:       []string{"tests/feature/admin/missing_alpha_test.go"},
		},
	})

	if err := registry.ValidateRuntime(); err != nil {
		t.Fatalf("ValidateRuntime() error = %v", err)
	}
	err := registry.Validate()
	if err == nil {
		t.Fatal("expected full validation to require metadata files")
	}
	if !strings.Contains(err.Error(), "openapi file does not exist") {
		t.Fatalf("validation error = %q", err.Error())
	}
}

func TestRegistryCatalogValidatesDuplicateMetadataFiles(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			openAPI: []string{
				"docs/api-contract/openapi/admin-base-apis.openapi.json",
				" docs/api-contract/openapi/admin-base-apis.openapi.json ",
			},
		},
	})

	err := registry.Validate()
	if err == nil {
		t.Fatal("expected duplicate metadata file validation error")
	}
	if !strings.Contains(err.Error(), "duplicate openapi file: docs/api-contract/openapi/admin-base-apis.openapi.json") {
		t.Fatalf("validation error = %q", err.Error())
	}
}

func TestRegistryCatalogAllowsSharedMetadataFilesAcrossModules(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id:      "alpha",
			openAPI: []string{"docs/api-contract/openapi/admin-base-apis.openapi.json"},
		},
		stubModule{
			id:      "beta",
			openAPI: []string{"docs/api-contract/openapi/admin-base-apis.openapi.json"},
		},
	})

	if err := registry.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRegistryValidatesPackageRegistryEntries(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			pkg: Package{
				ImportPath:    "goravel/app/modules/bravo",
				RegistryKey:   "alpha",
				ReleaseTrack:  "nightly",
				Owner:         "",
				Compatibility: []string{">=1.17.0"},
				Deprecated:    true,
			},
		},
	})

	err := registry.ValidateRuntime()

	if err == nil {
		t.Fatal("expected package validation error")
	}
	message := err.Error()
	for _, expected := range []string{
		"module alpha package import path mismatch",
		"module alpha package version is required",
		"module alpha package owner is required",
		"module alpha package release track unsupported: nightly",
		"module alpha package digest is required for release track nightly",
		"module alpha package signature is required for release track nightly",
		"module alpha deprecated package requires replacement target",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("validation error %q missing %q", message, expected)
		}
	}
}

func TestRegistrySourceManifestExportsModulePackages(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			pkg: Package{
				ImportPath:    "goravel/app/modules/alpha",
				RegistryKey:   "alpha",
				Version:       "1.2.3",
				Owner:         "platform-team",
				ReleaseTrack:  "internal",
				Compatibility: []string{">=1.17.0 <2.0.0"},
			},
		},
	})

	manifest := registry.SourceManifest()

	if len(manifest.Modules) != 1 {
		t.Fatalf("SourceManifest modules = %#v", manifest.Modules)
	}
	got := manifest.Modules[0]
	if got.ID != "alpha" || got.Package.ImportPath != "goravel/app/modules/alpha" {
		t.Fatalf("SourceManifest module = %#v", got)
	}
	if got.Package.Owner != "platform-team" || got.Package.ReleaseTrack != "internal" || got.Package.Version != "1.2.3" {
		t.Fatalf("SourceManifest package = %#v", got.Package)
	}
}

func TestRegistryValidatesSignedExternalPackage(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id:       "alpha",
			metadata: Metadata{Version: "1.2.3"},
			pkg: Package{
				ImportPath:    "goravel/app/modules/alpha",
				RegistryKey:   "alpha",
				Version:       "1.2.3",
				Owner:         "platform-team",
				ReleaseTrack:  "stable",
				Compatibility: []string{">=1.17.0 <2.0.0"},
				Digest:        "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				Signature:     "cosign:signature-ref",
			},
		},
	})

	if err := registry.ValidateRuntime(); err != nil {
		t.Fatalf("ValidateRuntime() error = %v", err)
	}
}

func TestRegistryRejectsMalformedExternalPackageEvidence(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id:       "alpha",
			metadata: Metadata{Version: "1.2.3"},
			pkg: Package{
				ImportPath:    "goravel/app/modules/alpha",
				RegistryKey:   "alpha",
				Version:       "release-1",
				Owner:         "platform-team",
				ReleaseTrack:  "stable",
				Compatibility: []string{">=1.17.0 bananas"},
				Digest:        "sha256:abc123",
				Signature:     "cosign:",
			},
		},
	})

	err := registry.ValidateRuntime()

	if err == nil {
		t.Fatal("expected malformed package validation error")
	}
	for _, expected := range []string{
		"module alpha package version invalid",
		"module alpha package compatibility invalid",
		"module alpha package digest invalid",
		"module alpha package signature invalid",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("validation error %q missing %q", err, expected)
		}
	}
}

func TestInstallRoutePanicsWhenMethodUnsupported(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected unsupported method panic")
		}
	}()

	InstallRoute("TRACE", "/admin/alpha", nil)
}

func (m stubModule) ID() string                     { return m.id }
func (m stubModule) Metadata() Metadata             { return m.metadata }
func (m stubModule) Package() Package               { return m.pkg }
func (m stubModule) Routes() []Route                { return m.routes }
func (m stubModule) Menus() []Menu                  { return m.menus }
func (m stubModule) Permissions() []Permission      { return m.permissions }
func (m stubModule) Migrations() []schema.Migration { return m.migrations }
func (m stubModule) Seeders() []seeder.Seeder       { return m.seeders }
func (m stubModule) OpenAPIFiles() []string         { return m.openAPI }
func (m stubModule) TestTemplates() []string        { return m.tests }

func TestRegistryLifecycleStatesKeepDependencyOrderForDisabledModules(t *testing.T) {
	t.Setenv("MODULE_DISABLED", "alpha")
	registry := NewRegistry([]Module{
		stubModule{id: "bravo", metadata: Metadata{
			Name:         "Bravo",
			Dependencies: []Dependency{RequiredDependency("alpha")},
		}},
		stubModule{id: "alpha", metadata: Metadata{Name: "Alpha"}},
	})

	states := registry.LifecycleStates()

	if got := states[0].ID; got != "alpha" {
		t.Fatalf("LifecycleStates()[0].ID = %q, want alpha", got)
	}
	if states[0].Enabled {
		t.Fatal("disabled dependency should be marked disabled")
	}
	if got := states[1].ID; got != "bravo" {
		t.Fatalf("LifecycleStates()[1].ID = %q, want bravo", got)
	}
	if states[1].Enabled {
		t.Fatal("dependent module should be marked disabled")
	}
	if !strings.Contains(states[1].Reason, "dependency alpha is disabled") {
		t.Fatalf("LifecycleStates()[1].Reason = %q, want dependency disabled reason", states[1].Reason)
	}
}

func TestRegistryAggregatesModuleMetadata(t *testing.T) {
	registry := NewRegistry([]Module{
		stubModule{
			id: "alpha",
			routes: []Route{{
				Name:    "alpha.index",
				Method:  "GET",
				Path:    "/admin/alpha/list",
				Install: func() {},
			}},
			menus: []Menu{{
				Key:        "alpha",
				Permission: "alpha:list",
			}},
			permissions: []Permission{{
				Key:         "alpha:list",
				Description: "Alpha list",
			}},
			migrations: []schema.Migration{stubMigration{}},
			seeders:    []seeder.Seeder{stubSeeder{}},
			openAPI:    []string{"docs/api-contract/openapi/alpha.openapi.json"},
			tests:      []string{"tests/feature/admin/alpha_test.go"},
		},
	})

	if got := registry.IDs(); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("IDs() = %#v, want alpha", got)
	}
	if got := registry.Routes()[0].Path; got != "/admin/alpha/list" {
		t.Fatalf("Routes()[0].Path = %q", got)
	}
	if got := registry.Menus()[0].Permission; got != "alpha:list" {
		t.Fatalf("Menus()[0].Permission = %q", got)
	}
	if got := registry.Permissions()[0].Key; got != "alpha:list" {
		t.Fatalf("Permissions()[0].Key = %q", got)
	}
	if got := len(registry.Migrations()); got != 1 {
		t.Fatalf("len(Migrations()) = %d", got)
	}
	if got := len(registry.Seeders()); got != 1 {
		t.Fatalf("len(Seeders()) = %d", got)
	}
	if got := registry.OpenAPIFiles()[0]; got != "docs/api-contract/openapi/alpha.openapi.json" {
		t.Fatalf("OpenAPIFiles()[0] = %q", got)
	}
	if got := registry.TestTemplates()[0]; got != "tests/feature/admin/alpha_test.go" {
		t.Fatalf("TestTemplates()[0] = %q", got)
	}
}
