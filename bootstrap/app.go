package bootstrap

import (
	"os"
	"time"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/database/seeder"
	contractsfoundation "github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/foundation/configuration"
	"github.com/goravel/framework/foundation"

	"goravel/app/console/commands"
	"goravel/app/facades"
	"goravel/app/http/middleware"
	"goravel/app/services"
	"goravel/config"
	"goravel/database"
	"goravel/routes"
)

func Boot() contractsfoundation.Application {
	return foundation.Setup().
		WithCommands(func() []console.Command {
			return []console.Command{
				&commands.MakeCrudCommand{},
				&commands.MakeModuleCommand{},
				&commands.TenantMigrateCommand{},
				&commands.SafeMigrateCommand{},
				&commands.SSOTestFixtureCommand{},
				&commands.TenantPermissionsSnapshotLegacyCommand{},
				&commands.SecurityAuditPruneCommand{},
				&commands.SecurityRotateCheckCommand{},
				&commands.ModuleManifestCheckCommand{},
				&commands.ModuleAdmissionCheckCommand{},
				&commands.ModuleOpenAPILintCommand{},
				&commands.ModuleManifestExportCommand{},
				&commands.ModuleCompatibilityExportCommand{},
				&commands.ModuleStateCommand{},
				&commands.ModulePlanCommand{},
				&commands.ModuleLifecycleCommand{},
				&commands.ReferenceCaseUpgradeCommand{},
				&commands.ReferenceCaseRollbackCommand{},
			}
		}).
		WithMigrations(Migrations).
		WithJobs(services.QueueJobs).
		WithMiddleware(func(handler configuration.Middleware) {
			handler.Prepend(middleware.Observability())
			handler.Append(middleware.CSRF())
		}).
		WithRunners(func() []contractsfoundation.Runner {
			return []contractsfoundation.Runner{
				services.NewOperationLogRunner(),
				services.NewScheduledTaskRunner(),
				services.NewQueueOutboxRunner(),
			}
		}).
		WithSeeders(func() []seeder.Seeder {
			return database.Seeders(ModuleSeeders())
		}).
		WithRouting(func() {
			configureObservability()
			routes.Web(RouteModules(os.Args))
			routes.Grpc()
		}).
		WithProviders(Providers).
		WithConfig(config.Boot).
		Create()
}

func configureObservability() {
	_ = facades.Telemetry()
	threshold := facades.Config().GetInt("observability.slow_request.threshold_ms", 1000)
	maxEntries := facades.Config().GetInt("observability.slow_request.max_entries", 100)
	slowSQLMaxEntries := facades.Config().GetInt("observability.slow_sql.max_entries", 100)
	services.ConfigureObservabilityRecorder(time.Duration(threshold)*time.Millisecond, maxEntries)
	services.ConfigureSlowSQLRecorder(slowSQLMaxEntries)
}
