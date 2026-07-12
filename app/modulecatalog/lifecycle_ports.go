package modulecatalog

import (
	"context"
	"time"
)

type LifecycleRepository interface {
	SuccessfulRunExists(context.Context, string) (bool, error)
	BeginRun(context.Context, LifecycleRunRecord) error
	CompleteRun(context.Context, LifecycleRunCompletion) error
	RecordStep(context.Context, LifecycleStepRecord) error
	UpsertState(context.Context, LifecycleStateRecord) error
}

type LifecycleLockManager interface {
	Acquire(context.Context, LifecycleLockAcquire) (LifecycleLock, error)
	Renew(context.Context, LifecycleLockRenewal) (LifecycleLock, error)
	Release(context.Context, LifecycleLock) error
}

type LifecycleCommandRunner interface {
	Run(context.Context, string) (string, string, error)
}

type LifecycleClock interface {
	Now() time.Time
	WithTimeout(context.Context, time.Duration) (context.Context, context.CancelFunc)
	NewTimer(time.Duration) LifecycleTimer
	NewTicker(time.Duration) LifecycleTicker
}

type LifecycleTimer interface {
	C() <-chan time.Time
	Stop() bool
}

type LifecycleTicker interface {
	C() <-chan time.Time
	Stop()
}

type LifecycleRunCompletion struct {
	IdempotencyKey string
	Status         string
	Error          string
	FinishedAt     time.Time
}

type LifecycleLockAcquire struct {
	Key    string
	Owner  string
	RunKey string
	TTL    time.Duration
}

type LifecycleLockRenewal struct {
	Lock LifecycleLock
	TTL  time.Duration
}

type systemLifecycleClock struct{}

func (systemLifecycleClock) Now() time.Time {
	return time.Now()
}

func (systemLifecycleClock) WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

func (systemLifecycleClock) NewTimer(timeout time.Duration) LifecycleTimer {
	return systemLifecycleTimer{Timer: time.NewTimer(timeout)}
}

func (systemLifecycleClock) NewTicker(interval time.Duration) LifecycleTicker {
	return systemLifecycleTicker{Ticker: time.NewTicker(interval)}
}

type systemLifecycleTimer struct {
	*time.Timer
}

func (timer systemLifecycleTimer) C() <-chan time.Time {
	return timer.Timer.C
}

type systemLifecycleTicker struct {
	*time.Ticker
}

func (ticker systemLifecycleTicker) C() <-chan time.Time {
	return ticker.Ticker.C
}

type lifecycleCommandRunnerFunc func(context.Context, string) error

func (runner lifecycleCommandRunnerFunc) Run(ctx context.Context, command string) (string, string, error) {
	return "", "", runner(ctx, command)
}

type artisanLifecycleCommandRunner struct{}

func (artisanLifecycleCommandRunner) Run(ctx context.Context, command string) (string, string, error) {
	return runLifecycleCommand(ctx, command)
}
