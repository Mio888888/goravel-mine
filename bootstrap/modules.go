package bootstrap

import (
	"fmt"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
	"goravel/app/modules"
	"goravel/app/services"
)

func Modules() modules.Registry {
	registry := moduleboot.Modules()
	if err := registry.ValidateRuntime(); err != nil {
		panic(fmt.Sprintf("module registry invalid: %v", err))
	}
	modulecatalog.SetDefaultAdminRegistry(registry)
	services.SetTenantModuleMigrationsProvider(registry.TenantMigrations)

	return registry
}

func RouteModules(args []string) modules.Registry {
	if isModuleGovernanceCommand(args) {
		return modules.NewRegistry(nil)
	}

	return Modules()
}

func isModuleGovernanceCommand(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "module:manifest:check", "module:manifest:export", "module:compatibility:export", "module:state", "module:plan", "module:lifecycle":
			return true
		}
	}

	return false
}
