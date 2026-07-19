package module

import "github.com/goravel/framework/contracts/console"

func Commands() []console.Command {
	return []console.Command{
		&ModuleManifestCheckCommand{},
		&ModuleAdmissionCheckCommand{},
		&ModuleOpenAPILintCommand{},
		&ModuleManifestExportCommand{},
		&ModuleCompatibilityExportCommand{},
		&ModuleStateCommand{},
		&ModulePlanCommand{},
		&ModuleLifecycleCommand{},
		&ReferenceCaseUpgradeCommand{},
		&ReferenceCaseRollbackCommand{},
	}
}
