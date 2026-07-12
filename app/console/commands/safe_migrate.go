package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/facades"
	"goravel/app/services"
)

type SafeMigrateInput struct {
	Scope       services.MigrationScope
	LockTimeout time.Duration
}

type SafeMigrateResult struct {
	PlatformMigrated bool
	TenantsMigrated  int
}

type SafeMigrateCore struct {
	locks           services.MigrationLockProvider
	platformMigrate func(context.Context) error
	tenantMigrate   func(context.Context) (int, error)
}

func NewSafeMigrateCore() *SafeMigrateCore {
	return &SafeMigrateCore{
		locks: services.NewMigrationLockService(),
		platformMigrate: func(context.Context) error {
			return facades.Artisan().Call("migrate")
		},
		tenantMigrate: func(ctx context.Context) (int, error) {
			return services.NewTenantService().WithContext(ctx).MigrateAllTenants(false)
		},
	}
}

func (c *SafeMigrateCore) Run(ctx context.Context, input SafeMigrateInput) (result SafeMigrateResult, err error) {
	lock, err := c.locks.Acquire(ctx, input.Scope, input.LockTimeout)
	if err != nil {
		return SafeMigrateResult{}, err
	}
	defer func() { err = errors.Join(err, lock.Release(context.Background())) }()

	if input.Scope == services.MigrationScopePlatform || input.Scope == services.MigrationScopeAll {
		if err := c.platformMigrate(ctx); err != nil {
			return result, err
		}
		result.PlatformMigrated = true
	}
	if input.Scope == services.MigrationScopeTenants || input.Scope == services.MigrationScopeAll {
		count, err := c.tenantMigrate(ctx)
		if err != nil {
			return result, err
		}
		result.TenantsMigrated = count
	}
	return result, nil
}

type SafeMigrateCommand struct{}

func (r *SafeMigrateCommand) Signature() string {
	return "migration:safe"
}

func (r *SafeMigrateCommand) Description() string {
	return "Run platform and tenant migrations under PostgreSQL advisory locks"
}

func (r *SafeMigrateCommand) Extend() command.Extend {
	return command.Extend{Category: "migrate", Flags: []command.Flag{
		&command.StringFlag{Name: "scope", Usage: "platform, tenants, or all", Value: string(services.MigrationScopeAll)},
		&command.StringFlag{Name: "lock-timeout", Usage: "Maximum advisory lock wait duration", Value: "30s"},
	}}
}

func (r *SafeMigrateCommand) Handle(ctx console.Context) error {
	input, err := parseSafeMigrateInput(ctx.Option("scope"), ctx.Option("lock-timeout"))
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	result, err := NewSafeMigrateCore().Run(context.Background(), input)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	ctx.Success(fmt.Sprintf("migration complete: platform=%t tenants=%d", result.PlatformMigrated, result.TenantsMigrated))
	return nil
}

func parseSafeMigrateInput(scopeValue, timeoutValue string) (SafeMigrateInput, error) {
	scope, err := services.ParseMigrationScope(scopeValue)
	if err != nil {
		return SafeMigrateInput{}, err
	}
	timeout, err := parseMigrationLockTimeout(timeoutValue)
	if err != nil {
		return SafeMigrateInput{}, err
	}
	return SafeMigrateInput{Scope: scope, LockTimeout: timeout}, nil
}

func parseMigrationLockTimeout(value string) (time.Duration, error) {
	timeout, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid migration lock timeout: %w", err)
	}
	if timeout < 0 {
		return 0, fmt.Errorf("migration lock timeout cannot be negative")
	}
	return timeout, nil
}
