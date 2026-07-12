package modulecatalog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/modules"
)

func TestLifecycleExecutorUsesIndependentPorts(t *testing.T) {
	service := newLifecycleExecutorTestService()
	repository := NewMemoryLifecycleStore()
	locks := &trackingLifecycleLockManager{MemoryLifecycleStore: NewMemoryLifecycleStore()}
	runner := &recordingLifecycleCommandRunner{}
	service.repository = repository
	service.lockManager = locks
	service.commandRunner = runner

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify executor ports",
	})

	require.NoError(t, err)
	require.Equal(t, LifecycleStatusSucceeded, result.Items[0].Status)
	require.Len(t, repository.runs, 1)
	require.Len(t, repository.steps, 2)
	require.Len(t, repository.states, 1)
	require.Equal(t, 1, locks.acquires)
	require.Equal(t, 2, locks.renews)
	require.Equal(t, 1, locks.releases)
	require.Equal(t, []string{"module:manifest:check", "migrate"}, runner.commands)
}

func TestLifecycleExecutorUsesClockForStepRecords(t *testing.T) {
	service := newLifecycleExecutorTestService()
	store := NewMemoryLifecycleStore()
	fixed := time.Date(2026, 7, 10, 18, 30, 0, 0, time.UTC)
	bindLifecyclePersistence(service, store)
	service.clock = fixedLifecycleClock{LifecycleClock: systemLifecycleClock{}, now: fixed}
	service.SetRunnerForTest(func(context.Context, string) error { return nil })

	_, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify executor clock",
	})

	require.NoError(t, err)
	require.Len(t, store.steps, 2)
	for _, step := range store.steps {
		require.Equal(t, fixed, step.Started)
		require.Equal(t, fixed, step.Finished)
	}
}

func TestLifecycleExecutorUsesClockForRunAndStateRecords(t *testing.T) {
	service := newLifecycleExecutorTestService()
	store := NewMemoryLifecycleStore()
	fixed := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	bindLifecyclePersistence(service, store)
	service.clock = fixedLifecycleClock{LifecycleClock: systemLifecycleClock{}, now: fixed}
	service.SetRunnerForTest(func(context.Context, string) error { return nil })

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify run and state clock",
	})

	require.NoError(t, err)
	item := result.Items[0]
	require.Equal(t, fixed, store.runs[item.IdempotencyKey].started)
	require.Equal(t, fixed, store.runs[item.IdempotencyKey].finished)
	require.Equal(t, fixed, store.states[item.ModuleID].updatedAt)
}

func TestLifecycleExecutorUsesClockForTimeoutScheduling(t *testing.T) {
	service := newLifecycleExecutorTestService()
	store := NewMemoryLifecycleStore()
	clock := &trackingLifecycleClock{LifecycleClock: systemLifecycleClock{}}
	bindLifecyclePersistence(service, store)
	service.clock = clock
	service.commandTimeout = 5 * time.Millisecond
	service.runnerCancelGrace = time.Millisecond
	service.SetRunnerForTest(func(context.Context, string) error { return nil })

	_, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify timeout clock port",
	})

	require.NoError(t, err)
	require.GreaterOrEqual(t, clock.timeouts, 2)
	require.GreaterOrEqual(t, clock.tickers, 2)
}

func TestLifecycleStateMachineFailurePolicy(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  string
		retainLock  bool
		watchRunner bool
	}{
		{name: "command failure", err: errors.New("boom"), wantStatus: LifecycleStatusFailed},
		{name: "manual command", err: manualLifecycleCommandError{Command: "manual review"}, wantStatus: LifecycleStatusManualRequired},
		{
			name:       "late runner",
			err:        lifecycleCommandStillRunningError{cause: context.DeadlineExceeded},
			wantStatus: LifecycleStatusReconciliationRequired, retainLock: true, watchRunner: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transition := (lifecycleStateMachine{}).failure(LifecycleResultItem{}, test.err)

			require.Equal(t, test.wantStatus, transition.item.Status)
			require.Equal(t, test.err.Error(), transition.item.Error)
			require.Equal(t, test.retainLock, transition.retainLock)
			require.Equal(t, test.watchRunner, transition.watchLateRunner)
		})
	}
}

func TestLifecycleExecutorFinalizesCommandFailuresIdentically(t *testing.T) {
	for _, failedCommand := range []string{"module:manifest:check", "migrate"} {
		t.Run(failedCommand, func(t *testing.T) {
			service := newLifecycleExecutorTestService()
			store := NewMemoryLifecycleStore()
			bindLifecyclePersistence(service, store)
			service.SetRunnerForTest(func(_ context.Context, command string) error {
				if command == failedCommand {
					return errors.New("command failed")
				}
				return nil
			})

			result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
				Execute: true,
				Owner:   "unit-test",
				Reason:  "verify failure finalizer",
			})

			require.ErrorContains(t, err, "command failed")
			item := result.Items[0]
			require.Equal(t, LifecycleStatusFailed, item.Status)
			require.Equal(t, LifecycleStatusFailed, store.runs[item.IdempotencyKey].Status)
			require.Equal(t, LifecycleStatusFailed, store.states[item.ModuleID].Status)
			require.Equal(t, "command failed", store.states[item.ModuleID].LastError)
		})
	}
}

func TestLifecycleLateRunnerRecordsLockRenewalFailure(t *testing.T) {
	store := &failingRenewStore{
		MemoryLifecycleStore: NewMemoryLifecycleStore(),
		failAfter:            1,
		err:                  errors.New("renew failed"),
	}
	service := newAlphaLifecycleServiceWithPorts(modules.Lifecycle{Upgrade: "migrate"}, store)
	service.lockTTL = 100 * time.Millisecond
	service.lockRenewInterval = 10 * time.Millisecond
	service.commandTimeout = 2 * time.Millisecond
	service.runnerCancelGrace = time.Millisecond
	releaseRunner := make(chan struct{})
	service.SetRunnerForTest(func(context.Context, string) error {
		<-releaseRunner
		return nil
	})

	result, err := service.Execute(context.Background(), LifecycleActionUpgrade, LifecycleOptions{
		Execute: true,
		Owner:   "unit-test",
		Reason:  "verify late runner lease failure evidence",
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	item := result.Items[0]
	require.Eventually(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return strings.Contains(store.runs[item.IdempotencyKey].Error, "lock lease renewal failed") &&
			strings.Contains(store.states[item.ModuleID].LastError, "renew failed")
	}, time.Second, 5*time.Millisecond)
	renewCount := store.renewCount
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, renewCount, store.renewCount)

	close(releaseRunner)
}

func newLifecycleExecutorTestService() *LifecycleService {
	service, _ := newAlphaLifecycleService(modules.Lifecycle{Upgrade: "migrate"})
	return service
}

type trackingLifecycleLockManager struct {
	*MemoryLifecycleStore
	acquires int
	renews   int
	releases int
}

func (m *trackingLifecycleLockManager) Acquire(ctx context.Context, request LifecycleLockAcquire) (LifecycleLock, error) {
	m.acquires++
	return m.MemoryLifecycleStore.Acquire(ctx, request)
}

func (m *trackingLifecycleLockManager) Renew(ctx context.Context, request LifecycleLockRenewal) (LifecycleLock, error) {
	m.renews++
	return m.MemoryLifecycleStore.Renew(ctx, request)
}

func (m *trackingLifecycleLockManager) Release(ctx context.Context, lock LifecycleLock) error {
	m.releases++
	return m.MemoryLifecycleStore.Release(ctx, lock)
}

type recordingLifecycleCommandRunner struct {
	commands []string
}

func (r *recordingLifecycleCommandRunner) Run(_ context.Context, command string) (string, string, error) {
	r.commands = append(r.commands, command)
	return "", "", nil
}

type fixedLifecycleClock struct {
	LifecycleClock
	now time.Time
}

func (c fixedLifecycleClock) Now() time.Time {
	return c.now
}

type trackingLifecycleClock struct {
	LifecycleClock
	timeouts int
	timers   int
	tickers  int
}

func (c *trackingLifecycleClock) WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	c.timeouts++
	return c.LifecycleClock.WithTimeout(ctx, timeout)
}

func (c *trackingLifecycleClock) NewTimer(timeout time.Duration) LifecycleTimer {
	c.timers++
	return c.LifecycleClock.NewTimer(timeout)
}

func (c *trackingLifecycleClock) NewTicker(interval time.Duration) LifecycleTicker {
	c.tickers++
	return c.LifecycleClock.NewTicker(interval)
}
