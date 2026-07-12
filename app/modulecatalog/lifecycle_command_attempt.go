package modulecatalog

import (
	"context"
	"errors"
	"time"
)

type lifecycleCommandAttemptState struct {
	request    lifecycleItemExecution
	lock       LifecycleLock
	attemptKey string
	stepName   string
	command    string
	started    time.Time
}

type lifecycleLateCompletion struct {
	item       lifecycleItemExecution
	attemptKey string
	stepName   string
	command    string
	started    time.Time
	output     lifecycleCommandOutput
}

func (e lifecycleExecutor) runCommand(attempt lifecycleCommandAttempt) error {
	state, err := e.startCommandAttempt(attempt)
	if err != nil {
		return err
	}
	output := e.executeCommandAttempt(state)
	return e.finishCommandAttempt(state, output)
}

func (e lifecycleExecutor) startCommandAttempt(attempt lifecycleCommandAttempt) (lifecycleCommandAttemptState, error) {
	state := lifecycleCommandAttemptState{
		request: attempt.group.item, attemptKey: randomLifecycleToken(),
		stepName: attempt.group.stepName, command: attempt.command,
	}
	if err := validateLifecycleCommand(state.command); err != nil {
		return state, e.recordCommandAttemptFailure(state, lifecycleErrorStatus(err), err)
	}
	state.started = e.clock.Now()
	if err := e.recorder().recordStep(state.request, state.outcome(LifecycleStatusRunning, lifecycleCommandOutput{}, false)); err != nil {
		return state, err
	}
	renewed, err := e.lockManager.Renew(state.request.ctx, LifecycleLockRenewal{
		Lock: attempt.group.lock,
		TTL:  e.lockTTLValue(),
	})
	if err != nil {
		return state, e.recordCommandAttemptFailure(state, LifecycleStatusFailed, err)
	}
	state.lock = renewed
	return state, nil
}

func (e lifecycleExecutor) executeCommandAttempt(state lifecycleCommandAttemptState) lifecycleCommandOutput {
	lateComplete := func(output lifecycleCommandOutput) {
		e.recordLateCompletion(lifecycleLateCompletion{
			item: state.request, attemptKey: state.attemptKey, stepName: state.stepName,
			command: state.command, started: state.started, output: output,
		})
	}
	leaseLost := func(err error) {
		e.recordLateLeaseFailure(lifecycleLateCompletion{
			item: state.request, attemptKey: state.attemptKey, stepName: state.stepName,
			command: state.command, started: state.started,
		}, err)
	}
	stdout, stderr, err := e.runCommandWithRenewal(lifecycleCommandRun{
		ctx: state.request.ctx, lock: state.lock, command: state.command,
		onLateComplete: lateComplete, onLeaseLost: leaseLost,
	})
	return lifecycleCommandOutput{stdout: stdout, stderr: stderr, err: err}
}

func (e lifecycleExecutor) finishCommandAttempt(state lifecycleCommandAttemptState, output lifecycleCommandOutput) error {
	status := LifecycleStatusSucceeded
	if output.err != nil {
		status = lifecycleErrorStatus(output.err)
	}
	recordErr := e.recorder().recordStep(state.request, state.outcome(status, output, true))
	if recordErr != nil && output.err != nil {
		return errors.Join(output.err, recordErr)
	}
	if recordErr != nil {
		return recordErr
	}
	return output.err
}

func (e lifecycleExecutor) recordCommandAttemptFailure(
	state lifecycleCommandAttemptState,
	status string,
	err error,
) error {
	_ = e.recorder().recordStep(state.request, state.outcome(status, lifecycleCommandOutput{err: err}, true))
	return err
}

func (state lifecycleCommandAttemptState) outcome(
	status string,
	output lifecycleCommandOutput,
	finished bool,
) lifecycleStepOutcome {
	return lifecycleStepOutcome{
		attemptKey: state.attemptKey, stepName: state.stepName, command: state.command,
		status: status, stdout: output.stdout, stderr: output.stderr, err: output.err,
		started: state.started, finished: finished,
	}
}

func (e lifecycleExecutor) recordLateCompletion(completion lifecycleLateCompletion) {
	message := "lifecycle runner completed after cancellation; manual reconciliation required"
	if completion.output.err != nil {
		message += ": " + completion.output.err.Error()
	}
	reconcileErr := errors.New(message)
	completion.item.ctx = context.Background()
	completion.item.item.Status = LifecycleStatusReconciliationRequired
	completion.item.item.Error = message
	outcome := lifecycleStepOutcome{
		attemptKey: completion.attemptKey, stepName: completion.stepName, command: completion.command,
		status: LifecycleStatusReconciliationRequired, stdout: completion.output.stdout,
		stderr: completion.output.stderr, err: reconcileErr, started: completion.started, finished: true,
	}
	writer := e.recorder()
	_ = writer.recordStep(completion.item, outcome)
	_ = writer.finishRun(completion.item)
	_ = writer.upsertState(completion.item)
}

func (e lifecycleExecutor) recordLateLeaseFailure(completion lifecycleLateCompletion, err error) {
	message := "lifecycle late runner lock lease renewal failed; concurrent execution risk requires manual reconciliation"
	if err != nil {
		message += ": " + err.Error()
	}
	reconcileErr := errors.New(message)
	completion.item.ctx = context.Background()
	completion.item.item.Status = LifecycleStatusReconciliationRequired
	completion.item.item.Error = message
	outcome := lifecycleStepOutcome{
		attemptKey: completion.attemptKey, stepName: completion.stepName, command: completion.command,
		status: LifecycleStatusReconciliationRequired, err: reconcileErr,
		started: completion.started, finished: true,
	}
	writer := e.recorder()
	_ = writer.recordStep(completion.item, outcome)
	_ = writer.finishRun(completion.item)
	_ = writer.upsertState(completion.item)
}
