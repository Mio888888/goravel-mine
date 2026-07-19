package migrationlock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type migrationLockFakeSession struct {
	database string
	tryLock  func(int64) (bool, error)
	unlocks  []int64
	closed   bool
}

func (s *migrationLockFakeSession) DatabaseIdentity(context.Context) (string, error) {
	return s.database, nil
}

func (s *migrationLockFakeSession) TryAdvisoryLock(_ context.Context, key int64) (bool, error) {
	return s.tryLock(key)
}

func (s *migrationLockFakeSession) Unlock(_ context.Context, key int64) error {
	s.unlocks = append(s.unlocks, key)
	return nil
}

func (s *migrationLockFakeSession) Close() error {
	s.closed = true
	return nil
}

func TestMigrationLockKeysUseDatabaseIdentityAndScope(t *testing.T) {
	platform, err := migrationLockKeys("platform-a", MigrationScopePlatform)
	require.NoError(t, err)
	tenants, err := migrationLockKeys("platform-a", MigrationScopeTenants)
	require.NoError(t, err)
	all, err := migrationLockKeys("platform-a", MigrationScopeAll)
	require.NoError(t, err)
	otherDatabase, err := migrationLockKeys("platform-b", MigrationScopePlatform)
	require.NoError(t, err)

	require.Len(t, platform, 1)
	require.Len(t, tenants, 1)
	require.ElementsMatch(t, append(platform, tenants...), all)
	require.NotEqual(t, platform, tenants)
	require.NotEqual(t, platform, otherDatabase)
}

func TestMigrationLockServiceReleasesSessionScopedLockAndRecordsObservability(t *testing.T) {
	ResetMetricsForTest()
	session := &migrationLockFakeSession{
		database: "goravel_mine",
		tryLock:  func(int64) (bool, error) { return true, nil },
	}
	events := make([]AuditEvent, 0)
	service := newMigrationLockService(func(context.Context) (migrationLockSession, error) {
		return session, nil
	})
	service.audit = func(_ context.Context, event AuditEvent) { events = append(events, event) }
	service.newRunID = func() string { return "run-1" }

	lock, err := service.Acquire(context.Background(), MigrationScopePlatform, time.Second)
	require.NoError(t, err)
	require.Equal(t, "goravel_mine", lock.Metadata().Database)
	require.Equal(t, "run-1", lock.Metadata().RunID)
	require.NoError(t, lock.Release(context.Background()))

	require.True(t, session.closed)
	require.Len(t, session.unlocks, 1)
	require.Equal(t, []string{"migration.lock.wait", "migration.lock.acquire", "migration.lock.release"}, auditActions(events))
	metrics := MetricsSnapshot()
	require.Equal(t, uint64(1), metrics.AcquiredTotal)
	require.Equal(t, uint64(1), metrics.ReleasedTotal)
}

func TestMigrationLockServiceFailsClosedWhenLockTimesOut(t *testing.T) {
	ResetMetricsForTest()
	session := &migrationLockFakeSession{
		database: "goravel_mine",
		tryLock:  func(int64) (bool, error) { return false, nil },
	}
	service := newMigrationLockService(func(context.Context) (migrationLockSession, error) {
		return session, nil
	})

	_, err := service.Acquire(context.Background(), MigrationScopePlatform, 0)

	require.ErrorIs(t, err, ErrMigrationLockTimeout)
	require.True(t, session.closed)
	metrics := MetricsSnapshot()
	require.Equal(t, uint64(1), metrics.TimeoutTotal)
}

func TestMigrationLockServiceFailsClosedWhenAdvisoryQueryFails(t *testing.T) {
	session := &migrationLockFakeSession{
		database: "goravel_mine",
		tryLock:  func(int64) (bool, error) { return false, errors.New("database unavailable") },
	}
	service := newMigrationLockService(func(context.Context) (migrationLockSession, error) {
		return session, nil
	})

	_, err := service.Acquire(context.Background(), MigrationScopePlatform, time.Second)

	require.EqualError(t, err, "acquire migration advisory lock: database unavailable")
	require.True(t, session.closed)
}

func auditActions(events []AuditEvent) []string {
	actions := make([]string, 0, len(events))
	for _, event := range events {
		actions = append(actions, event.Action)
	}
	return actions
}
