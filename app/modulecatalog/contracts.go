package modulecatalog

import (
	"time"

	"goravel/app/modules"
)

type Manifest struct {
	Modules []ManifestItem `json:"modules"`
}

type ManifestItem struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	Version       string               `json:"version"`
	Compatible    string               `json:"compatible"`
	Package       modules.Package      `json:"package"`
	Enabled       bool                 `json:"enabled"`
	Reason        string               `json:"reason,omitempty"`
	Dependencies  []ManifestDependency `json:"dependencies"`
	Lifecycle     ManifestLifecycle    `json:"lifecycle"`
	SeedStrategy  ManifestSeedStrategy `json:"seed_strategy"`
	Frontend      ManifestFrontend     `json:"frontend"`
	Routes        []ManifestRoute      `json:"routes"`
	Menus         []ManifestMenu       `json:"menus"`
	Permissions   []ManifestPermission `json:"permissions"`
	Migrations    []string             `json:"migrations"`
	OpenAPIFiles  []string             `json:"openapi_files"`
	TestTemplates []string             `json:"test_templates"`
}

type CompatibilityMatrix struct {
	Status           string                      `json:"status"`
	FrameworkVersion string                      `json:"framework_version"`
	GeneratedAt      time.Time                   `json:"generated_at"`
	Modules          []CompatibilityMatrixModule `json:"modules"`
}

type CompatibilityMatrixModule struct {
	ID                   string               `json:"id"`
	Name                 string               `json:"name"`
	Version              string               `json:"version"`
	Compatible           string               `json:"compatible"`
	Package              modules.Package      `json:"package"`
	Dependencies         []ManifestDependency `json:"dependencies"`
	ReleaseTrack         string               `json:"release_track"`
	Deprecated           bool                 `json:"deprecated,omitempty"`
	ReplacedBy           string               `json:"replaced_by,omitempty"`
	RequiresRestart      bool                 `json:"requires_restart"`
	SupportsHotDisable   bool                 `json:"supports_hot_disable"`
	BreakingChangePolicy string               `json:"breaking_change_policy"`
	FrameworkCompatible  bool                 `json:"framework_compatible"`
	CompatibilityError   string               `json:"compatibility_error,omitempty"`
	Enabled              bool                 `json:"enabled"`
	DisabledReason       string               `json:"disabled_reason,omitempty"`
}

type ManifestDependency struct {
	ID                string `json:"id"`
	VersionConstraint string `json:"version_constraint,omitempty"`
	Required          bool   `json:"required"`
}

type ManifestLifecycle struct {
	Install              string `json:"install"`
	Uninstall            string `json:"uninstall"`
	Upgrade              string `json:"upgrade"`
	Rollback             string `json:"rollback"`
	DestructiveCheck     string `json:"destructive_check"`
	SupportsHotDisable   bool   `json:"supports_hot_disable"`
	RequiresRestart      bool   `json:"requires_restart"`
	BreakingChangePolicy string `json:"breaking_change_policy"`
}

type ManifestSeedStrategy struct {
	Mode       string `json:"mode"`
	Idempotent bool   `json:"idempotent"`
	Command    string `json:"command,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

type ManifestFrontend struct {
	ModulePath  string   `json:"module_path,omitempty"`
	ApiFiles    []string `json:"api_files,omitempty"`
	RouteFiles  []string `json:"route_files,omitempty"`
	LocaleFiles []string `json:"locale_files,omitempty"`
	TypeFiles   []string `json:"type_files,omitempty"`
	TestFiles   []string `json:"test_files,omitempty"`
}

type ManifestRoute struct {
	Name        string   `json:"name"`
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Permission  string   `json:"permission,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Middlewares []string `json:"middlewares"`
}

type ManifestMenu struct {
	Key        string `json:"key"`
	ParentKey  string `json:"parent_key,omitempty"`
	Title      string `json:"title"`
	Path       string `json:"path"`
	Component  string `json:"component,omitempty"`
	Permission string `json:"permission,omitempty"`
	Type       string `json:"type,omitempty"`
	I18n       string `json:"i18n,omitempty"`
	Sort       int    `json:"sort"`
}

type ManifestPermission struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

type ModuleStateItem struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Version    string                 `json:"version"`
	Compatible string                 `json:"compatible"`
	Enabled    bool                   `json:"enabled"`
	Reason     string                 `json:"reason,omitempty"`
	DependsOn  []ManifestDependency   `json:"depends_on,omitempty"`
	Lifecycle  ManifestLifecycle      `json:"lifecycle"`
	Frontend   ManifestFrontend       `json:"frontend"`
	Seed       ManifestSeedStrategy   `json:"seed_strategy"`
	Persisted  *PersistedModuleState  `json:"persisted,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

type PersistedModuleState struct {
	Name           string     `json:"name,omitempty"`
	Status         string     `json:"status"`
	Enabled        bool       `json:"enabled"`
	Owner          string     `json:"owner,omitempty"`
	TargetVersion  string     `json:"target_version,omitempty"`
	LastAction     string     `json:"last_action,omitempty"`
	LastRunKey     string     `json:"last_run_key,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	InstalledAt    *time.Time `json:"installed_at,omitempty"`
	UpgradedAt     *time.Time `json:"upgraded_at,omitempty"`
	DisabledAt     *time.Time `json:"disabled_at,omitempty"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	DisabledReason string     `json:"disabled_reason,omitempty"`
}

type LifecyclePlanItem struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Action               string `json:"action"`
	Enabled              bool   `json:"enabled"`
	Reason               string `json:"reason,omitempty"`
	Command              string `json:"command"`
	DestructiveCheck     string `json:"destructive_check"`
	RequiresRestart      bool   `json:"requires_restart"`
	SupportsHotDisable   bool   `json:"supports_hot_disable"`
	BreakingChangePolicy string `json:"breaking_change_policy"`
}

type ManifestFrontendParity struct {
	Menus       []ManifestMenu
	Permissions []ManifestPermission
	ApiFiles    []string
	Views       []string
	Locales     []string
}

func (f ManifestFrontendParity) HasEntries() bool {
	return len(f.Menus) > 0 || len(f.Permissions) > 0 || len(f.ApiFiles) > 0 || len(f.Views) > 0 || len(f.Locales) > 0
}

type ManifestSeedParity struct {
	Menus       []ManifestMenu
	Permissions []ManifestPermission
}
