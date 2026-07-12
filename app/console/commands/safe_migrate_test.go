package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
)

type migrationLockCommandFake struct {
	scope   services.MigrationScope
	timeout time.Duration
	lock    *migrationLockCommandFakeHandle
}

func (f *migrationLockCommandFake) Acquire(_ context.Context, scope services.MigrationScope, timeout time.Duration) (services.MigrationLock, error) {
	f.scope = scope
	f.timeout = timeout
	return f.lock, nil
}

type migrationLockCommandFakeHandle struct {
	released bool
	err      error
}

func (f *migrationLockCommandFakeHandle) Metadata() services.MigrationLockMetadata {
	return services.MigrationLockMetadata{}
}

func (f *migrationLockCommandFakeHandle) Release(context.Context) error {
	f.released = true
	return f.err
}

func TestSafeMigrateCoreLocksAllScopesBeforeRunningMigrations(t *testing.T) {
	lock := &migrationLockCommandFakeHandle{}
	locks := &migrationLockCommandFake{lock: lock}
	calls := make([]string, 0, 2)
	core := &SafeMigrateCore{
		locks: locks,
		platformMigrate: func(context.Context) error {
			calls = append(calls, "platform")
			return nil
		},
		tenantMigrate: func(context.Context) (int, error) {
			calls = append(calls, "tenants")
			return 3, nil
		},
	}

	result, err := core.Run(context.Background(), SafeMigrateInput{Scope: services.MigrationScopeAll, LockTimeout: 30 * time.Second})

	require.NoError(t, err)
	require.Equal(t, services.MigrationScopeAll, locks.scope)
	require.Equal(t, []string{"platform", "tenants"}, calls)
	require.True(t, result.PlatformMigrated)
	require.Equal(t, 3, result.TenantsMigrated)
	require.True(t, lock.released)
}

func TestSafeMigrateInputRejectsInvalidScopeAndTimeout(t *testing.T) {
	_, err := parseSafeMigrateInput("unknown", "30s")
	require.ErrorIs(t, err, services.ErrMigrationScope)

	_, err = parseSafeMigrateInput("platform", "invalid")
	require.Error(t, err)
}

func TestSafeMigrateCoreReturnsLockReleaseError(t *testing.T) {
	releaseErr := errors.New("release lock")
	core := &SafeMigrateCore{
		locks:           &migrationLockCommandFake{lock: &migrationLockCommandFakeHandle{err: releaseErr}},
		platformMigrate: func(context.Context) error { return nil },
	}

	_, err := core.Run(context.Background(), SafeMigrateInput{Scope: services.MigrationScopePlatform, LockTimeout: time.Second})

	require.ErrorIs(t, err, releaseErr)
}
