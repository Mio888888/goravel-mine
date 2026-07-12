package modulecatalog

import "goravel/app/modules"

type dtoMapper struct{}

func (dtoMapper) manifestItem(module modules.ModuleCatalog) ManifestItem {
	return ManifestItem{
		ID: module.ID, Name: module.Metadata.Name, Version: module.Metadata.Version,
		Compatible: module.Metadata.Compatible, Package: module.Package, Enabled: module.Enabled, Reason: module.Reason,
		Dependencies: manifestDependencies(module.Metadata.Dependencies),
		Lifecycle:    manifestLifecycle(module.Metadata.Lifecycle), SeedStrategy: manifestSeedStrategy(module.Metadata.SeedStrategy),
		Frontend: manifestFrontend(module.Metadata.Frontend), Routes: manifestRoutes(module.Routes),
		Menus: manifestMenus(module.Menus), Permissions: manifestPermissions(module.Permissions),
		Migrations: module.Migrations, OpenAPIFiles: module.OpenAPIFiles, TestTemplates: module.TestTemplates,
	}
}

func (dtoMapper) stateItem(state modules.ModuleState, persisted *PersistedModuleState) ModuleStateItem {
	return ModuleStateItem{
		ID: state.ID, Name: state.Metadata.Name, Version: state.Metadata.Version,
		Compatible: state.Metadata.Compatible, Enabled: state.Enabled, Reason: state.Reason,
		DependsOn: manifestDependencies(state.Metadata.Dependencies), Lifecycle: manifestLifecycle(state.Metadata.Lifecycle),
		Frontend: manifestFrontend(state.Metadata.Frontend), Seed: manifestSeedStrategy(state.Metadata.SeedStrategy),
		Persisted: persisted,
	}
}

type compatibilityProjection struct {
	module     modules.ModuleCatalog
	compatible bool
	err        error
}

func (dtoMapper) compatibilityItem(projection compatibilityProjection) CompatibilityMatrixModule {
	module := projection.module
	errorMessage := ""
	if projection.err != nil {
		errorMessage = projection.err.Error()
	}
	return CompatibilityMatrixModule{
		ID: module.ID, Name: module.Metadata.Name, Version: module.Metadata.Version,
		Compatible: module.Metadata.Compatible, Package: module.Package,
		Dependencies: manifestDependencies(module.Metadata.Dependencies), ReleaseTrack: module.Package.ReleaseTrack,
		Deprecated: module.Package.Deprecated, ReplacedBy: module.Package.ReplacedBy,
		RequiresRestart:      module.Metadata.Lifecycle.RequiresRestart,
		SupportsHotDisable:   module.Metadata.Lifecycle.SupportsHotDisable,
		BreakingChangePolicy: module.Metadata.Lifecycle.BreakingChangePolicy,
		FrameworkCompatible:  projection.compatible, CompatibilityError: errorMessage,
		Enabled: module.Enabled, DisabledReason: module.Reason,
	}
}

func manifestDependencies(dependencies []modules.Dependency) []ManifestDependency {
	return mapSlice(dependencies, func(dependency modules.Dependency) ManifestDependency {
		return ManifestDependency{ID: dependency.ID, VersionConstraint: dependency.VersionConstraint, Required: dependency.Required}
	})
}

func manifestLifecycle(lifecycle modules.Lifecycle) ManifestLifecycle {
	return ManifestLifecycle{
		Install: lifecycle.Install, Uninstall: lifecycle.Uninstall, Upgrade: lifecycle.Upgrade, Rollback: lifecycle.Rollback,
		DestructiveCheck: lifecycle.DestructiveCheck, SupportsHotDisable: lifecycle.SupportsHotDisable,
		RequiresRestart: lifecycle.RequiresRestart, BreakingChangePolicy: lifecycle.BreakingChangePolicy,
	}
}

func manifestSeedStrategy(strategy modules.SeedStrategy) ManifestSeedStrategy {
	return ManifestSeedStrategy{Mode: strategy.Mode, Idempotent: strategy.Idempotent, Command: strategy.Command, Notes: strategy.Notes}
}

func manifestFrontend(frontend modules.FrontendArtifact) ManifestFrontend {
	return ManifestFrontend{
		ModulePath: frontend.ModulePath, ApiFiles: frontend.ApiFiles, RouteFiles: frontend.RouteFiles,
		LocaleFiles: frontend.LocaleFiles, TypeFiles: frontend.TypeFiles, TestFiles: frontend.TestFiles,
	}
}

func manifestRoutes(routes []modules.Route) []ManifestRoute {
	return mapSlice(routes, func(route modules.Route) ManifestRoute {
		return ManifestRoute{
			Name: route.Name, Method: route.Method, Path: route.Path, Permission: route.Permission,
			Permissions: route.PermissionKeys(), Middlewares: route.Middlewares,
		}
	})
}

func manifestMenus(menus []modules.Menu) []ManifestMenu {
	return mapSlice(menus, func(menu modules.Menu) ManifestMenu {
		return ManifestMenu{
			Key: menu.Key, ParentKey: menu.ParentKey, Title: menu.Title, Path: menu.Path, Component: menu.Component,
			Permission: menu.Permission, Type: menu.Type, I18n: menu.I18n, Sort: menu.Sort,
		}
	})
}

func manifestPermissions(permissions []modules.Permission) []ManifestPermission {
	return mapSlice(permissions, func(permission modules.Permission) ManifestPermission {
		return ManifestPermission{Key: permission.Key, Description: permission.Description}
	})
}

func mapSlice[S, T any](source []S, project func(S) T) []T {
	items := make([]T, 0, len(source))
	for _, item := range source {
		items = append(items, project(item))
	}
	return items
}
