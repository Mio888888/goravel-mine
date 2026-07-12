package modulecatalog

import (
	"context"
	"errors"
	"time"
)

type lifecycleCommandOutput struct {
	stdout string
	stderr string
	err    error
}

type lifecycleLockRenewal struct {
	cancel context.CancelFunc
	done   <-chan error
}

type lifecycleCommandGroup struct {
	item     lifecycleItemExecution
	lock     LifecycleLock
	stepName string
	value    string
}

type lifecycleCommandAttempt struct {
	group   lifecycleCommandGroup
	command string
}

type lifecycleCommandRun struct {
	ctx            context.Context
	lock           LifecycleLock
	command        string
	onLateComplete func(lifecycleCommandOutput)
	onLeaseLost    func(error)
}

type lifecycleCommandAwait struct {
	runCtx    context.Context
	cancelRun context.CancelFunc
	runDone   <-chan lifecycleCommandOutput
	renewDone <-chan error
}

func (e lifecycleExecutor) runCommands(group lifecycleCommandGroup) error {
	for _, command := range normalizeLifecycleCommands(group.value) {
		if _, ok := group.item.executedCommands[command]; ok {
			continue
		}
		if err := e.runCommand(lifecycleCommandAttempt{group: group, command: command}); err != nil {
			return err
		}
		group.item.executedCommands[command] = struct{}{}
	}
	return nil
}

func (e lifecycleExecutor) runCommandWithRenewal(run lifecycleCommandRun) (string, string, error) {
	runCtx, cancelRun := e.clock.WithTimeout(run.ctx, e.commandTimeoutValue())
	defer cancelRun()
	renewal := e.startLockRenewal(run.lock)
	runDone := make(chan lifecycleCommandOutput, 1)
	go func() {
		stdout, stderr, err := e.commandRunner.Run(runCtx, run.command)
		runDone <- lifecycleCommandOutput{stdout: stdout, stderr: stderr, err: err}
	}()

	output, renewErr, runFinished := e.awaitCommand(lifecycleCommandAwait{
		runCtx: runCtx, cancelRun: cancelRun, runDone: runDone, renewDone: renewal.done,
	})
	if runFinished {
		renewal.cancel()
		if renewErr == nil {
			renewErr = <-renewal.done
		}
		return output.stdout, output.stderr, lifecycleCommandResultError(output.err, renewErr, runCtx.Err())
	}
	renewal.cancel()
	if renewErr == nil {
		renewErr = <-renewal.done
	}
	commandErr := lifecycleCommandResultError(output.err, renewErr, runCtx.Err())
	if commandErr == nil {
		commandErr = errors.New("module lifecycle command cancellation did not stop runner")
	}
	return output.stdout, output.stderr, lifecycleCommandStillRunningError{
		cause: commandErr,
		late: &lifecycleLateRunner{
			lock: run.lock, runDone: runDone,
			onComplete: run.onLateComplete, onLeaseLost: run.onLeaseLost,
		},
	}
}

func (e lifecycleExecutor) startLockRenewal(lock LifecycleLock) lifecycleLockRenewal {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		ticker := e.clock.NewTicker(e.lockRenewIntervalValue())
		defer ticker.Stop()
		current := lock
		for {
			select {
			case <-ctx.Done():
				done <- ctx.Err()
				return
			case <-ticker.C():
				current = e.renewLock(ctx, current, done)
				if !current.Acquired {
					return
				}
			}
		}
	}()
	return lifecycleLockRenewal{cancel: cancel, done: done}
}

func (e lifecycleExecutor) renewLock(ctx context.Context, lock LifecycleLock, done chan<- error) LifecycleLock {
	timeout := e.lockRenewTimeout(lock)
	if timeout <= 0 {
		done <- errors.New("module lifecycle lock lease expired before renewal")
		return LifecycleLock{}
	}
	renewCtx, cancel := e.clock.WithTimeout(ctx, timeout)
	renewed, err := e.lockManager.Renew(renewCtx, LifecycleLockRenewal{Lock: lock, TTL: e.lockTTLValue()})
	cancel()
	if err != nil {
		done <- err
		return LifecycleLock{}
	}
	return renewed
}

func (e lifecycleExecutor) awaitCommand(request lifecycleCommandAwait) (lifecycleCommandOutput, error, bool) {
	var output lifecycleCommandOutput
	var renewErr error
	select {
	case output = <-request.runDone:
		return output, nil, true
	case renewErr = <-request.renewDone:
		request.cancelRun()
	case <-request.runCtx.Done():
	}
	timer := e.clock.NewTimer(e.runnerCancelGraceValue())
	defer timer.Stop()
	select {
	case output = <-request.runDone:
		return output, renewErr, true
	case <-timer.C():
		return output, renewErr, false
	}
}

func lifecycleCommandResultError(runErr, renewErr, ctxErr error) error {
	if errors.Is(renewErr, context.Canceled) {
		renewErr = nil
	}
	if ctxErr != nil && !errors.Is(runErr, ctxErr) && !errors.Is(renewErr, ctxErr) {
		runErr = errors.Join(runErr, ctxErr)
	}
	return errors.Join(runErr, renewErr)
}

type lifecycleLateRunner struct {
	lock        LifecycleLock
	runDone     <-chan lifecycleCommandOutput
	onComplete  func(lifecycleCommandOutput)
	onLeaseLost func(error)
}

func (e lifecycleExecutor) watchLateRunner(err error) {
	var runningErr lifecycleCommandStillRunningError
	if !errors.As(err, &runningErr) || runningErr.late == nil {
		return
	}
	late := runningErr.late
	go func() {
		current := late.lock
		interval := e.lockRenewIntervalValue()
		ticker := e.clock.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case output := <-late.runDone:
				if late.onComplete != nil {
					late.onComplete(output)
				}
				_ = e.lockManager.Release(context.Background(), late.lock)
				return
			case <-ticker.C():
				renewed, renewErr := e.renewLateLock(current, interval)
				if renewErr != nil {
					if late.onLeaseLost != nil {
						late.onLeaseLost(renewErr)
					}
					return
				}
				current = renewed
			}
		}
	}()
}

func (e lifecycleExecutor) renewLateLock(lock LifecycleLock, interval time.Duration) (LifecycleLock, error) {
	ctx, cancel := e.clock.WithTimeout(context.Background(), interval)
	renewed, err := e.lockManager.Renew(ctx, LifecycleLockRenewal{Lock: lock, TTL: e.lockTTLValue()})
	cancel()
	if err != nil {
		return LifecycleLock{}, err
	}
	return renewed, nil
}

type lifecycleCommandStillRunningError struct {
	cause error
	late  *lifecycleLateRunner
}

func (e lifecycleCommandStillRunningError) Error() string { return e.cause.Error() }
func (e lifecycleCommandStillRunningError) Unwrap() error { return e.cause }

func lifecycleCommandMayStillBeRunning(err error) bool {
	var runningErr lifecycleCommandStillRunningError
	return errors.As(err, &runningErr)
}
