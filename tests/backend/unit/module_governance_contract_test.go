package unit

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/modulecatalog"
	"goravel/bootstrap"
	"goravel/tests/backend/testsupport"
)

func TestModuleGovernanceManifestContract(t *testing.T) {
	requireModuleGovernanceGoldenJSON(t, "manifest", modulecatalog.NewService(bootstrap.Modules()).Manifest())
}

func TestModuleGovernanceCompatibilityContract(t *testing.T) {
	matrix := modulecatalog.NewService(bootstrap.Modules()).CompatibilityMatrix("1.17.2")
	matrix.GeneratedAt = time.Time{}

	requireModuleGovernanceGoldenJSON(t, "compatibility", matrix)
}

func TestModuleGovernanceStateContract(t *testing.T) {
	requireModuleGovernanceGoldenJSON(t, "module-states", bootstrap.Modules().ModuleStates())
}

func TestModuleGovernanceLifecyclePlanContract(t *testing.T) {
	service := modulecatalog.NewService(bootstrap.Modules())
	plans := map[string][]modulecatalog.LifecyclePlanItem{}

	for _, action := range []string{
		modulecatalog.LifecycleActionInstall,
		modulecatalog.LifecycleActionUpgrade,
		modulecatalog.LifecycleActionRollback,
		modulecatalog.LifecycleActionUninstall,
	} {
		plan, err := service.LifecyclePlan(action)
		require.NoError(t, err)
		plans[action] = plan
	}

	requireModuleGovernanceGoldenJSON(t, "lifecycle-plans", plans)
}

func requireModuleGovernanceGoldenJSON(t *testing.T, name string, value any) {
	t.Helper()
	path := filepath.Join("testdata", "module-governance", name+".golden.json")
	testsupport.RequireGoldenJSON(t, path, value)
}
