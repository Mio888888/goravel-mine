package migration

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/services"
)

type TenantMigrateCommand struct{}

type TenantMigrateCore struct {
	locks   services.MigrationLockProvider
	migrate func(context.Context, bool) (int, error)
}

func NewTenantMigrateCore() *TenantMigrateCore {
	return &TenantMigrateCore{
		locks: services.NewMigrationLockService(),
		migrate: func(ctx context.Context, dryRun bool) (int, error) {
			return services.NewTenantService().WithContext(ctx).MigrateAllTenants(dryRun)
		},
	}
}

func (c *TenantMigrateCore) Run(ctx context.Context, dryRun bool, lockTimeout time.Duration) (count int, err error) {
	if dryRun {
		return c.migrate(ctx, true)
	}
	lock, err := c.locks.Acquire(ctx, services.MigrationScopeTenants, lockTimeout)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, lock.Release(context.Background())) }()
	return c.migrate(ctx, false)
}

func (r *TenantMigrateCommand) Signature() string {
	return "tenant:migrate"
}

func (r *TenantMigrateCommand) Description() string {
	return "Run tenant business migrations for existing tenants"
}

func (r *TenantMigrateCommand) Extend() command.Extend {
	return command.Extend{
		Category: "tenant",
		Flags: []command.Flag{
			&command.BoolFlag{Name: "dry-run", Usage: "Count tenants without running migrations"},
			&command.StringFlag{Name: "lock-timeout", Usage: "Maximum advisory lock wait duration", Value: "30s"},
		},
	}
}

func (r *TenantMigrateCommand) Handle(ctx console.Context) error {
	dryRun := ctx.OptionBool("dry-run")
	lockTimeout, err := parseMigrationLockTimeout(ctx.Option("lock-timeout"))
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	count, err := NewTenantMigrateCore().Run(context.Background(), dryRun, lockTimeout)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	if dryRun {
		ctx.Success(fmt.Sprintf("%d tenants would be migrated", count))
		return nil
	}
	ctx.Success(fmt.Sprintf("%d tenants migrated", count))
	return nil
}
