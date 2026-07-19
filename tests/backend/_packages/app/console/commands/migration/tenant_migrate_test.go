package migration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
)

func TestTenantMigrateCoreUsesTenantMigrationLock(t *testing.T) {
	lock := &migrationLockCommandFakeHandle{}
	locks := &migrationLockCommandFake{lock: lock}
	core := &TenantMigrateCore{
		locks:   locks,
		migrate: func(context.Context, bool) (int, error) { return 2, nil },
	}

	count, err := core.Run(context.Background(), false, 30*time.Second)

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, services.MigrationScopeTenants, locks.scope)
	require.True(t, lock.released)
}

func TestTenantMigrateCoreReturnsLockReleaseError(t *testing.T) {
	releaseErr := errors.New("release lock")
	core := &TenantMigrateCore{
		locks:   &migrationLockCommandFake{lock: &migrationLockCommandFakeHandle{err: releaseErr}},
		migrate: func(context.Context, bool) (int, error) { return 1, nil },
	}

	_, err := core.Run(context.Background(), false, time.Second)

	require.ErrorIs(t, err, releaseErr)
}
