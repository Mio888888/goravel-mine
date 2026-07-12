package modulecatalog

import (
	"context"
	"errors"
	"time"

	"goravel/app/modules"
)

type lifecycleExecutor struct {
	repository        LifecycleRepository
	lockManager       LifecycleLockManager
	commandRunner     LifecycleCommandRunner
	clock             LifecycleClock
	lockTTL           time.Duration
	lockRenewInterval time.Duration
	commandTimeout    time.Duration
	runnerCancelGrace time.Duration
}

type lifecycleItemExecution struct {
	ctx              context.Context
	state            modules.ModuleState
	opts             LifecycleOptions
	item             LifecycleResultItem
	executedCommands map[string]struct{}
	securityGate     *lifecycleSecurityGate
}

type lifecycleFailure struct {
	request     lifecycleItemExecution
	err         error
	releaseLock *bool
}

type lifecycleValidationFailure struct {
	request  lifecycleItemExecution
	stepName string
	command  string
	err      error
}

func newLifecycleExecutor(service *LifecycleService) lifecycleExecutor {
	return lifecycleExecutor{
		repository:        service.repository,
		lockManager:       service.lockManager,
		commandRunner:     service.commandRunner,
		clock:             service.clock,
		lockTTL:           service.lockTTL,
		lockRenewInterval: service.lockRenewInterval,
		commandTimeout:    service.commandTimeout,
		runnerCancelGrace: service.runnerCancelGrace,
	}
}

func (e lifecycleExecutor) executeItem(request lifecycleItemExecution) (LifecycleResultItem, error) {
	owner := firstLifecycleNonEmpty(request.opts.Owner, "module-lifecycle")
	lock, err := e.lockManager.Acquire(request.ctx, LifecycleLockAcquire{
		Key:    lifecycleLockKey(request.state.ID),
		Owner:  owner,
		RunKey: lifecycleLockRunKey(request.item.IdempotencyKey),
		TTL:    e.lockTTLValue(),
	})
	if err != nil {
		return failedLifecycleItem(request.item, err)
	}
	if !lock.Acquired {
		request.item.Status = LifecycleStatusLockBlocked
		request.item.Skipped = true
		request.item.Error = "module lifecycle lock held by " + lock.Owner
		return request.item, errors.New(request.item.Error)
	}
	releaseLock := true
	defer func() {
		if releaseLock {
			_ = e.lockManager.Release(context.Background(), lock)
		}
	}()

	if result, done, err := e.preflightItem(request); done {
		return result, err
	}
	var result LifecycleResultItem
	err = request.securityGate.execute(request.ctx, func() error {
		result, err = e.executeMutation(request, lock, &releaseLock)
		return err
	})
	if err != nil && result.ModuleID == "" {
		return failedLifecycleItem(request.item, err)
	}
	return result, err
}

func (e lifecycleExecutor) executeMutation(request lifecycleItemExecution, lock LifecycleLock, releaseLock *bool) (LifecycleResultItem, error) {
	if err := e.recorder().beginRun(request); err != nil {
		return failedLifecycleItem(request.item, err)
	}
	if err := e.runCommands(lifecycleCommandGroup{item: request, lock: lock, stepName: "destructive_check", value: request.item.DestructiveCheck}); err != nil {
		return e.finishFailure(lifecycleFailure{request: request, err: err, releaseLock: releaseLock})
	}
	if err := e.runCommands(lifecycleCommandGroup{item: request, lock: lock, stepName: "command", value: request.item.Command}); err != nil {
		return e.finishFailure(lifecycleFailure{request: request, err: err, releaseLock: releaseLock})
	}
	return e.finishSuccess(request)
}

func (e lifecycleExecutor) preflightItem(request lifecycleItemExecution) (LifecycleResultItem, bool, error) {
	exists, err := e.repository.SuccessfulRunExists(request.ctx, request.item.IdempotencyKey)
	if err != nil {
		request.item, err = failedLifecycleItem(request.item, err)
		return request.item, true, err
	}
	if exists {
		request.item.Status = LifecycleStatusSkipped
		request.item.Skipped = true
		return request.item, true, nil
	}
	if stepName, command, validationErr := validateLifecycleItemCommands(request.item); validationErr != nil {
		request.item, err = e.failValidation(lifecycleValidationFailure{
			request: request, stepName: stepName, command: command, err: validationErr,
		})
		return request.item, true, err
	}
	return request.item, false, nil
}

func (e lifecycleExecutor) finishFailure(failure lifecycleFailure) (LifecycleResultItem, error) {
	transition := (lifecycleStateMachine{}).failure(failure.request.item, failure.err)
	if transition.retainLock && failure.releaseLock != nil {
		*failure.releaseLock = false
	}
	writer := e.recorder()
	request := failure.request
	request.item = transition.item
	_ = writer.finishRun(request)
	_ = writer.upsertState(request)
	if transition.watchLateRunner {
		e.watchLateRunner(failure.err)
	}
	return transition.item, failure.err
}

func (e lifecycleExecutor) finishSuccess(request lifecycleItemExecution) (LifecycleResultItem, error) {
	request.item = (lifecycleStateMachine{}).success(request.item)
	writer := e.recorder()
	if err := writer.upsertState(request); err != nil {
		request.item, err = failedLifecycleItem(request.item, err)
		_ = writer.finishRun(request)
		return request.item, err
	}
	if err := writer.finishRun(request); err != nil {
		return failedLifecycleItem(request.item, err)
	}
	return request.item, nil
}

func (e lifecycleExecutor) failValidation(failure lifecycleValidationFailure) (LifecycleResultItem, error) {
	request := failure.request
	request.item = (lifecycleStateMachine{}).failure(request.item, failure.err).item
	writer := e.recorder()
	if err := writer.beginRun(request); err != nil {
		return failedLifecycleItem(request.item, err)
	}
	outcome := lifecycleStepOutcome{
		attemptKey: randomLifecycleToken(), stepName: failure.stepName, command: failure.command,
		status: request.item.Status, err: failure.err, finished: true,
	}
	_ = writer.recordStep(request, outcome)
	_ = writer.finishRun(request)
	_ = writer.upsertState(request)
	return request.item, failure.err
}

func failedLifecycleItem(item LifecycleResultItem, err error) (LifecycleResultItem, error) {
	item.Status = LifecycleStatusFailed
	item.Error = lifecycleErrText(err)
	return item, err
}

type lifecycleSecurityGate struct {
	callback func(context.Context, func() error) error
	passed   bool
}

func (g *lifecycleSecurityGate) execute(ctx context.Context, mutate func() error) error {
	if g == nil || g.callback == nil || g.passed {
		return mutate()
	}
	if err := g.callback(ctx, mutate); err != nil {
		return err
	}
	g.passed = true
	return nil
}

func validateLifecycleItemCommands(item LifecycleResultItem) (string, string, error) {
	steps := []struct{ name, commands string }{
		{name: "destructive_check", commands: item.DestructiveCheck},
		{name: "command", commands: item.Command},
	}
	for _, step := range steps {
		if command, err := firstInvalidLifecycleCommand(step.commands); err != nil {
			return step.name, command, err
		}
	}
	return "", "", nil
}
