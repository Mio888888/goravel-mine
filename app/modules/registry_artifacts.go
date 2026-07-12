package modules

import (
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
)

func (r Registry) Routes() []Route {
	return collectModuleArtifacts(r.kernel.activeModules(), Module.Routes)
}

func (r Registry) RegisterRoutes() {
	for _, route := range r.Routes() {
		if route.Install != nil {
			route.Install()
		}
	}
}

func (r Registry) Menus() []Menu {
	return collectModuleArtifacts(r.kernel.activeModules(), Module.Menus)
}

func (r Registry) Permissions() []Permission {
	return collectModuleArtifacts(r.kernel.activeModules(), Module.Permissions)
}

func (r Registry) Migrations() []schema.Migration {
	return collectModuleArtifacts(r.kernel.activeModules(), Module.Migrations)
}

func (r Registry) TenantMigrations() []schema.Migration {
	var migrations []schema.Migration
	for _, module := range r.kernel.activeModules() {
		provider, ok := module.(TenantMigrationProvider)
		if ok {
			migrations = append(migrations, provider.TenantMigrations()...)
		}
	}
	return migrations
}

func (r Registry) MergeMigrationsAfter(core []schema.Migration, anchorSignature string) []schema.Migration {
	moduleMigrations := r.Migrations()
	migrations := make([]schema.Migration, 0, len(core)+len(moduleMigrations))
	for _, migration := range core {
		migrations = append(migrations, migration)
		if migration.Signature() == anchorSignature {
			migrations = append(migrations, moduleMigrations...)
			moduleMigrations = nil
		}
	}
	return append(migrations, moduleMigrations...)
}

func (r Registry) Seeders() []seeder.Seeder {
	return collectModuleArtifacts(r.kernel.activeModules(), Module.Seeders)
}

func (r Registry) OpenAPIFiles() []string {
	return collectModuleArtifacts(r.kernel.activeModules(), Module.OpenAPIFiles)
}

func (r Registry) TestTemplates() []string {
	return collectModuleArtifacts(r.kernel.activeModules(), Module.TestTemplates)
}

func collectModuleArtifacts[T any](source []Module, collect func(Module) []T) []T {
	var items []T
	for _, module := range source {
		items = append(items, collect(module)...)
	}
	return items
}
