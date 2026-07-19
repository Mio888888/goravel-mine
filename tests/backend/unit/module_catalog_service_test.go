package unit

import (
	"encoding/json"
	"testing"

	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
	"github.com/stretchr/testify/require"

	"goravel/app/modulecatalog"
	"goravel/app/modules"
	"goravel/bootstrap"
)

func TestModuleCatalogServiceValidatesBootstrapRegistry(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())

	require.NoError(t, service.Validate())
}

func TestModuleCatalogServiceValidatesRuntimeWithoutArtifactFiles(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())

	require.NoError(t, service.ValidateRuntime())
}

func TestModuleCatalogServiceExportsManifestJSON(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())

	payload, err := service.ManifestJSON()
	require.NoError(t, err)

	var manifest modulecatalog.Manifest
	require.NoError(t, json.Unmarshal(payload, &manifest))
	require.NotEmpty(t, manifest.Modules)
	require.NotEmpty(t, manifest.Modules[0].Routes)
	require.NotEmpty(t, manifest.Modules[0].Permissions)
	require.Equal(t, "1.0.0", manifest.Modules[0].Version)
	require.Equal(t, manifest.Modules[0].ID, manifest.Modules[0].Package.RegistryKey)
	require.NotEmpty(t, manifest.Modules[0].Package.ImportPath)
	require.NotEmpty(t, manifest.Modules[0].Package.Compatibility)
	require.True(t, manifest.Modules[0].Enabled)
	require.NotEmpty(t, manifest.Modules[0].Lifecycle.Install)
	require.Equal(t, "idempotent", manifest.Modules[0].SeedStrategy.Mode)
}

func TestModuleCatalogServiceExportsModuleDependencies(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())

	manifest := service.Manifest()
	tenantRBAC := findManifestModule(t, manifest, "tenant-rbac")
	require.Equal(t, "Tenant RBAC", tenantRBAC.Name)
	require.Equal(t, []modulecatalog.ManifestDependency{{
		ID:                "platform-tenant",
		VersionConstraint: ">=1.0.0",
		Required:          true,
	}}, tenantRBAC.Dependencies)
}

func TestModuleCatalogServiceExportsCompatibilityMatrix(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())

	payload, err := service.CompatibilityMatrixJSON("1.17.2")
	require.NoError(t, err)

	var matrix modulecatalog.CompatibilityMatrix
	require.NoError(t, json.Unmarshal(payload, &matrix))
	require.Equal(t, "passed", matrix.Status)
	require.Equal(t, "1.17.2", matrix.FrameworkVersion)
	require.NotEmpty(t, matrix.GeneratedAt)
	require.NotEmpty(t, matrix.Modules)

	platformRBAC := findCompatibilityModule(t, matrix, "platform-rbac")
	require.Equal(t, "Platform RBAC", platformRBAC.Name)
	require.Equal(t, "1.0.0", platformRBAC.Version)
	require.Equal(t, "internal", platformRBAC.ReleaseTrack)
	require.Equal(t, "platform-rbac", platformRBAC.Package.RegistryKey)
	require.NotEmpty(t, platformRBAC.Package.Compatibility)
	require.True(t, platformRBAC.FrameworkCompatible)
	require.True(t, platformRBAC.RequiresRestart)
	require.NotEmpty(t, platformRBAC.BreakingChangePolicy)
}

func TestModuleCatalogServiceExportsDisabledModuleStates(t *testing.T) {
	t.Setenv("MODULE_DISABLED", "scheduled-task")
	service := modulecatalog.NewService(bootstrap.Modules())

	manifest := service.Manifest()
	scheduledTask := findManifestModule(t, manifest, "scheduled-task")
	require.False(t, scheduledTask.Enabled)
	require.Equal(t, "disabled by MODULE_DISABLED", scheduledTask.Reason)
	require.NotEmpty(t, scheduledTask.Routes)

	platformTenant := findManifestModule(t, manifest, "platform-tenant")
	require.True(t, platformTenant.Enabled)
	require.Empty(t, platformTenant.Reason)
	require.NotEmpty(t, platformTenant.Routes)
}

func findCompatibilityModule(t *testing.T, matrix modulecatalog.CompatibilityMatrix, id string) modulecatalog.CompatibilityMatrixModule {
	t.Helper()

	for _, module := range matrix.Modules {
		if module.ID == id {
			return module
		}
	}

	t.Fatalf("compatibility module %s not found", id)
	return modulecatalog.CompatibilityMatrixModule{}
}

func TestModuleCatalogServiceExportsRoutePermissionList(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())

	payload, err := service.ManifestJSON()
	require.NoError(t, err)

	var manifest map[string][]map[string]any
	require.NoError(t, json.Unmarshal(payload, &manifest))

	route := findManifestRoute(t, manifest, "platform.tenant.permission-list")
	require.Equal(t, "platform:tenant:permissions", route["permission"])
	require.Equal(t, []any{
		"platform:tenant:permissions",
		"platform:tenantPlan:save",
		"platform:tenantPlan:update",
	}, route["permissions"])
}

func TestModuleCatalogServiceBuildsLifecyclePlan(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())

	plan, err := service.LifecyclePlan("install")
	require.NoError(t, err)
	require.NotEmpty(t, plan)

	require.Equal(t, "install", plan[0].Action)
	require.True(t, plan[0].Enabled)
	require.NotEmpty(t, plan[0].Command)
	require.NotEmpty(t, plan[0].DestructiveCheck)
	require.NotEmpty(t, plan[0].BreakingChangePolicy)

	require.Equal(t, "platform-rbac", plan[0].ID)
	require.Equal(t, "platform-tenant", plan[1].ID)
	requirePlanBefore(t, plan, "tenant-rbac", "security")
	requirePlanBefore(t, plan, "tenant-rbac", "data-center")
}

func TestModuleCatalogServiceReversesRollbackLifecyclePlan(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())

	installPlan, err := service.LifecyclePlan("install")
	require.NoError(t, err)
	rollbackPlan, err := service.LifecyclePlan("rollback")
	require.NoError(t, err)

	require.Equal(t, installPlan[0].ID, rollbackPlan[len(rollbackPlan)-1].ID)
	require.Equal(t, installPlan[len(installPlan)-1].ID, rollbackPlan[0].ID)
	require.Equal(t, "rollback", rollbackPlan[0].Action)
	requirePlanBefore(t, rollbackPlan, "security", "tenant-rbac")
	requirePlanBefore(t, rollbackPlan, "data-center", "tenant-rbac")
}

func TestModuleCatalogServiceRejectsInvalidLifecyclePlanAction(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())

	_, err := service.LifecyclePlan("destroy")

	require.ErrorContains(t, err, "unsupported lifecycle action")
}

func TestModuleCatalogServiceValidatesManifestSeedMenuParity(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())
	manifest := service.Manifest()

	seed := modulecatalog.ManifestSeedParity{}
	for _, module := range manifest.Modules {
		seed.Menus = append(seed.Menus, module.Menus...)
		seed.Permissions = append(seed.Permissions, module.Permissions...)
	}

	require.NoError(t, service.ValidateManifestParity(seed, modulecatalog.ManifestFrontendParity{}))

	seed.Menus[0].Path = "/wrong"
	err := service.ValidateManifestParity(seed, modulecatalog.ManifestFrontendParity{})

	require.ErrorContains(t, err, "seed menu path mismatch")
}

func TestModuleCatalogServiceValidatesManifestSeedPermissions(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())
	manifest := service.Manifest()
	seed := modulecatalog.ManifestSeedParity{}
	for _, module := range manifest.Modules {
		seed.Menus = append(seed.Menus, module.Menus...)
		seed.Permissions = append(seed.Permissions, module.Permissions...)
	}

	seed.Permissions = seed.Permissions[1:]
	err := service.ValidateManifestParity(seed, modulecatalog.ManifestFrontendParity{})

	require.ErrorContains(t, err, "seed permission missing")
}

func TestModuleCatalogServiceValidatesFrontendManifestParity(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())
	manifest := service.Manifest()
	frontend := modulecatalog.ManifestFrontendParity{}
	for _, module := range manifest.Modules {
		frontend.Menus = append(frontend.Menus, module.Menus...)
		frontend.Permissions = append(frontend.Permissions, module.Permissions...)
	}

	require.NoError(t, service.ValidateManifestParity(modulecatalog.ManifestSeedParity{}, frontend))

	frontend.Permissions = frontend.Permissions[1:]
	err := service.ValidateManifestParity(modulecatalog.ManifestSeedParity{}, frontend)

	require.ErrorContains(t, err, "frontend permission missing")
}

func TestModuleCatalogServiceRejectsExtraFrontendManifestEntries(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())
	manifest := service.Manifest()
	frontend := modulecatalog.ManifestFrontendParity{}
	for _, module := range manifest.Modules {
		frontend.Menus = append(frontend.Menus, module.Menus...)
		frontend.Permissions = append(frontend.Permissions, module.Permissions...)
	}
	frontend.Menus = append(frontend.Menus, modulecatalog.ManifestMenu{
		Key:        "stale:menu",
		Path:       "/stale",
		Component:  "base/views/stale/index",
		Permission: "stale:list",
	})
	frontend.Permissions = append(frontend.Permissions, modulecatalog.ManifestPermission{
		Key:         "stale:list",
		Description: "Stale List",
	})

	err := service.ValidateManifestParity(modulecatalog.ManifestSeedParity{}, frontend)

	require.ErrorContains(t, err, "frontend menu extra: stale:menu")
	require.ErrorContains(t, err, "frontend permission extra: stale:list")
}

func TestModuleCatalogServiceValidatesFrontendFileParity(t *testing.T) {
	service := modulecatalog.NewService(modules.NewRegistry([]modules.Module{frontendParityModule{}}))
	frontend := modulecatalog.ManifestFrontendParity{
		Menus: []modulecatalog.ManifestMenu{{
			Key:        "audit-log",
			Path:       "/audit-log",
			Component:  "audit-log/views/index",
			Permission: "audit-log:list",
		}},
		Permissions: []modulecatalog.ManifestPermission{{
			Key:         "audit-log:list",
			Description: "Audit Log List",
		}},
		ApiFiles: []string{
			"src/modules/audit-log/api/index.ts",
		},
		Views: []string{
			"src/modules/audit-log/views/index.vue",
		},
		Locales: []string{
			"src/modules/audit-log/locales/zh_CN.yaml",
		},
	}

	require.NoError(t, service.ValidateManifestParity(modulecatalog.ManifestSeedParity{}, frontend))

	frontend.ApiFiles = []string{"src/modules/audit-log/api/wrong.ts"}
	frontend.Views = []string{"src/modules/audit-log/views/wrong.vue"}
	frontend.Locales = []string{"src/modules/audit-log/locales/en.yaml"}
	err := service.ValidateManifestParity(modulecatalog.ManifestSeedParity{}, frontend)

	require.ErrorContains(t, err, "frontend api file missing for module audit-log: src/modules/audit-log/api/index.ts")
	require.ErrorContains(t, err, "frontend view file missing for module audit-log: src/modules/audit-log/views/index.vue")
	require.ErrorContains(t, err, "frontend locale file missing for module audit-log: src/modules/audit-log/locales/zh_CN.yaml")
	require.ErrorContains(t, err, "frontend api file extra for module audit-log: src/modules/audit-log/api/wrong.ts")
	require.ErrorContains(t, err, "frontend view file extra for module audit-log: src/modules/audit-log/views/wrong.vue")
	require.ErrorContains(t, err, "frontend locale file extra for module audit-log: src/modules/audit-log/locales/en.yaml")
}

func TestModuleCatalogServiceUsesDeclaredFrontendLocaleFiles(t *testing.T) {
	service := modulecatalog.NewService(modules.NewRegistry([]modules.Module{frontendParityModule{
		localeFiles: []string{"MineAdmin-web/src/modules/audit-log/locales/zh_CN[简体中文].yaml"},
	}}))
	frontend := modulecatalog.ManifestFrontendParity{
		Menus: []modulecatalog.ManifestMenu{{
			Key:        "audit-log",
			Path:       "/audit-log",
			Component:  "audit-log/views/index",
			Permission: "audit-log:list",
		}},
		Permissions: []modulecatalog.ManifestPermission{{
			Key:         "audit-log:list",
			Description: "Audit Log List",
		}},
		ApiFiles: []string{
			"src/modules/audit-log/api/index.ts",
		},
		Views: []string{
			"src/modules/audit-log/views/index.vue",
		},
		Locales: []string{
			"src/modules/audit-log/locales/zh_CN[简体中文].yaml",
		},
	}

	require.NoError(t, service.ValidateManifestParity(modulecatalog.ManifestSeedParity{}, frontend))
}

func TestModuleCatalogServiceIgnoresFrontendFilesForOtherModules(t *testing.T) {
	service := modulecatalog.NewService(modules.NewRegistry([]modules.Module{frontendParityModule{}}))
	frontend := modulecatalog.ManifestFrontendParity{
		Menus: []modulecatalog.ManifestMenu{{
			Key:        "audit-log",
			Path:       "/audit-log",
			Component:  "audit-log/views/index",
			Permission: "audit-log:list",
		}},
		Permissions: []modulecatalog.ManifestPermission{{
			Key:         "audit-log:list",
			Description: "Audit Log List",
		}},
		ApiFiles: []string{
			"src/modules/audit-log/api/index.ts",
			"src/modules/report/api/index.ts",
		},
		Views: []string{
			"src/modules/audit-log/views/index.vue",
			"src/modules/report/views/index.vue",
		},
		Locales: []string{
			"src/modules/audit-log/locales/zh_CN.yaml",
			"src/modules/report/locales/zh_CN.yaml",
		},
	}

	require.NoError(t, service.ValidateManifestParity(modulecatalog.ManifestSeedParity{}, frontend))
}

func findManifestRoute(t *testing.T, manifest map[string][]map[string]any, name string) map[string]any {
	t.Helper()

	for _, module := range manifest["modules"] {
		routes, ok := module["routes"].([]any)
		require.True(t, ok)
		for _, rawRoute := range routes {
			route, ok := rawRoute.(map[string]any)
			require.True(t, ok)
			if route["name"] == name {
				return route
			}
		}
	}

	t.Fatalf("manifest route %s not found", name)
	return nil
}

type frontendParityModule struct {
	localeFiles []string
}

func (m frontendParityModule) ID() string {
	return "audit-log"
}

func (m frontendParityModule) Metadata() modules.Metadata {
	metadata := modules.BuiltinMetadata("Audit Log")
	metadata.Frontend = modules.FrontendArtifact{
		ModulePath: "MineAdmin-web/src/modules/audit-log",
		ApiFiles: []string{
			"MineAdmin-web/src/modules/audit-log/api/index.ts",
		},
		RouteFiles: []string{
			"MineAdmin-web/src/modules/audit-log/views/index.vue",
		},
		LocaleFiles: m.localeFiles,
	}
	return metadata
}

func (m frontendParityModule) Routes() []modules.Route {
	return []modules.Route{{
		Name:       "audit-log.list",
		Method:     "GET",
		Path:       "/admin/audit-log/list",
		Permission: "audit-log:list",
		Install:    func() {},
	}}
}

func (m frontendParityModule) Menus() []modules.Menu {
	return []modules.Menu{{
		Key:        "audit-log",
		Title:      "Audit Log",
		Path:       "/audit-log",
		Component:  "audit-log/views/index",
		Permission: "audit-log:list",
		Type:       "M",
	}}
}

func (m frontendParityModule) Permissions() []modules.Permission {
	return []modules.Permission{{Key: "audit-log:list", Description: "Audit Log List"}}
}

func (m frontendParityModule) Migrations() []schema.Migration {
	return nil
}

func (m frontendParityModule) Seeders() []seeder.Seeder {
	return nil
}

func (m frontendParityModule) OpenAPIFiles() []string {
	return nil
}

func (m frontendParityModule) TestTemplates() []string {
	return nil
}

func requirePlanBefore(t *testing.T, plan []modulecatalog.LifecyclePlanItem, first string, second string) {
	t.Helper()

	firstIndex := -1
	secondIndex := -1
	for index, item := range plan {
		if item.ID == first {
			firstIndex = index
		}
		if item.ID == second {
			secondIndex = index
		}
	}
	require.NotEqualf(t, -1, firstIndex, "plan missing module %s", first)
	require.NotEqualf(t, -1, secondIndex, "plan missing module %s", second)
	require.Lessf(t, firstIndex, secondIndex, "module %s should be before %s", first, second)
}

func findManifestModule(t *testing.T, manifest modulecatalog.Manifest, id string) modulecatalog.ManifestItem {
	t.Helper()

	for _, module := range manifest.Modules {
		if module.ID == id {
			return module
		}
	}

	t.Fatalf("manifest module %s not found", id)
	return modulecatalog.ManifestItem{}
}
