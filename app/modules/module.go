package modules

import (
	"errors"

	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
)

type InstallRouteFunc func()

type Route struct {
	Name        string
	Method      string
	Path        string
	Permission  string
	Permissions []string
	Middlewares []string
	Install     InstallRouteFunc
}

func (r Route) PermissionKeys() []string {
	seen := make(map[string]bool, len(r.Permissions)+1)
	keys := make([]string, 0, len(r.Permissions)+1)
	add := func(permission string) {
		if permission == "" || seen[permission] {
			return
		}
		seen[permission] = true
		keys = append(keys, permission)
	}

	add(r.Permission)
	for _, permission := range r.Permissions {
		add(permission)
	}

	return keys
}

type Menu struct {
	Key        string
	ParentKey  string
	Title      string
	Path       string
	Component  string
	Permission string
	Type       string
	I18n       string
	Sort       int
}

type Permission struct {
	Key         string
	Description string
}

type Module interface {
	ID() string
	Routes() []Route
	Menus() []Menu
	Permissions() []Permission
	Migrations() []schema.Migration
	Seeders() []seeder.Seeder
	OpenAPIFiles() []string
	TestTemplates() []string
}

type TenantMigrationProvider interface {
	TenantMigrations() []schema.Migration
}

type Registry struct {
	kernel registryKernel
}

func NewRegistry(items []Module) Registry {
	return Registry{kernel: newRegistryKernel(items, disabledModuleSet())}
}

func (r Registry) IDs() []string {
	modules := r.kernel.registeredModules()
	ids := make([]string, 0, len(modules))
	for _, module := range modules {
		ids = append(ids, module.ID())
	}

	return ids
}

func (r Registry) ModuleStates() []ModuleState {
	return r.kernel.sourceStates()
}

func (r Registry) LifecycleStates() []ModuleState {
	return r.kernel.lifecycleStates()
}

func (r Registry) Catalog() Catalog {
	return NewCatalog(r.kernel.activeModules())
}

func (r Registry) ModuleCatalog() Catalog {
	source := r.kernel.sourceModules()
	modules := make([]ModuleCatalog, 0, len(source))
	for _, module := range source {
		reason := r.kernel.disabledReason(module.ID())
		enabled := reason == ""
		item := catalogModule(module, enabled)
		item.Reason = reason
		modules = append(modules, item)
	}

	return Catalog{Modules: modules}
}

func (r Registry) Validate() error {
	return errors.Join(
		r.validateDependencies(),
		r.validatePackages(),
		r.Catalog().Validate(),
	)
}

func (r Registry) ValidateRuntime() error {
	return errors.Join(
		r.validatePackages(),
		r.validateDependencies(),
		r.Catalog().ValidateRuntime(),
	)
}
