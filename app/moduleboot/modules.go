package moduleboot

import (
	"goravel/app/modules"
	"goravel/app/modules/datacenter"
	"goravel/app/modules/platformobservability"
	"goravel/app/modules/platformrbac"
	"goravel/app/modules/platformtenant"
	"goravel/app/modules/referencecase"
	"goravel/app/modules/scheduledtask"
	"goravel/app/modules/security"
	"goravel/app/modules/tenantrbac"
)

func Modules() modules.Registry {
	builtins := []modules.Module{
		platformrbac.New(),
		platformtenant.New(),
		platformobservability.New(),
		referencecase.New(),
		scheduledtask.New(),
		security.New(),
		datacenter.New(),
		tenantrbac.New(),
	}
	return modules.NewRegistry(append(builtins, AdmittedModules()...))
}
