package bootstrap

import (
	"strings"
	"testing"
)

func TestModulesPanicsWhenRequiredDependencyDisabled(t *testing.T) {
	t.Setenv("MODULE_DISABLED", "platform-rbac")
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected module registry panic")
		}
		if !strings.Contains(recovered.(string), "requires disabled dependency: platform-rbac") {
			t.Fatalf("panic = %v", recovered)
		}
	}()

	_ = Modules()
}

func TestRouteModulesSkipsRuntimeValidationForModuleGovernanceCommands(t *testing.T) {
	t.Setenv("MODULE_DISABLED", "platform-rbac")

	registry := RouteModules([]string{"app", "artisan", "module:state"})

	if len(registry.Routes()) != 0 {
		t.Fatal("module governance commands should not register module routes")
	}
}

func TestMigrationsUsesUncheckedModuleRegistry(t *testing.T) {
	t.Setenv("MODULE_DISABLED", "platform-rbac")

	migrations := Migrations()

	if len(migrations) == 0 {
		t.Fatal("Migrations() returned no migrations")
	}
}

func TestModuleSeedersUseUncheckedModuleRegistry(t *testing.T) {
	t.Setenv("MODULE_DISABLED", "platform-rbac")

	_ = ModuleSeeders()
}
