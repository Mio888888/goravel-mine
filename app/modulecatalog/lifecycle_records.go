package modulecatalog

import (
	"encoding/json"
	"strings"
	"time"
)

type lifecycleRecorder struct {
	repository LifecycleRepository
	clock      LifecycleClock
}

type lifecycleStepOutcome struct {
	attemptKey string
	stepName   string
	command    string
	status     string
	stdout     string
	stderr     string
	err        error
	started    time.Time
	finished   bool
}

func (e lifecycleExecutor) recorder() lifecycleRecorder {
	return lifecycleRecorder{repository: e.repository, clock: e.clock}
}

func (w lifecycleRecorder) beginRun(request lifecycleItemExecution) error {
	return w.repository.BeginRun(request.ctx, lifecycleRunRecord(request, w.clock.Now()))
}

func (w lifecycleRecorder) finishRun(request lifecycleItemExecution) error {
	return w.repository.CompleteRun(request.ctx, LifecycleRunCompletion{
		IdempotencyKey: request.item.IdempotencyKey,
		Status:         request.item.Status,
		Error:          request.item.Error,
		FinishedAt:     w.clock.Now(),
	})
}

func (w lifecycleRecorder) upsertState(request lifecycleItemExecution) error {
	return w.repository.UpsertState(request.ctx, lifecycleStateRecord(request, w.clock.Now()))
}

func (outcome lifecycleStepOutcome) withStarted(started time.Time) lifecycleStepOutcome {
	outcome.started = started
	return outcome
}

func (w lifecycleRecorder) recordStep(request lifecycleItemExecution, outcome lifecycleStepOutcome) error {
	record := LifecycleStepRecord{
		AttemptKey: outcome.attemptKey,
		RunKey:     request.item.IdempotencyKey,
		ModuleID:   request.state.ID,
		Action:     request.item.Action,
		StepName:   outcome.stepName,
		Command:    outcome.command,
		Status:     outcome.status,
		Stdout:     truncateLifecycleOutput(outcome.stdout),
		Stderr:     truncateLifecycleOutput(outcome.stderr),
		Error:      lifecycleErrText(outcome.err),
		Started:    outcome.started,
	}
	if outcome.finished {
		record.Finished = w.clock.Now()
		if record.Started.IsZero() {
			record.Started = record.Finished
		}
	}
	return w.repository.RecordStep(request.ctx, record)
}

func lifecycleRunRecord(request lifecycleItemExecution, started time.Time) LifecycleRunRecord {
	payload, _ := json.Marshal(request.item)
	return LifecycleRunRecord{
		IdempotencyKey: request.item.IdempotencyKey,
		ModuleID:       request.state.ID,
		Action:         request.item.Action,
		ToVersion:      request.state.Metadata.Version,
		Status:         LifecycleStatusRunning,
		DryRun:         !request.opts.Execute,
		Owner:          firstLifecycleNonEmpty(request.opts.Owner, "module-lifecycle"),
		Reason:         strings.TrimSpace(request.opts.Reason),
		Command:        request.item.Command,
		Plan:           string(payload),
		started:        started,
	}
}

func lifecycleStateRecord(request lifecycleItemExecution, updatedAt time.Time) LifecycleStateRecord {
	metadata, _ := json.Marshal(request.state.Metadata)
	return LifecycleStateRecord{
		ModuleID:       request.state.ID,
		Name:           request.state.Metadata.Name,
		Version:        request.state.Metadata.Version,
		TargetVersion:  request.state.Metadata.Version,
		Status:         lifecycleStateRecordStatus(request.item, request.item.Error),
		Enabled:        lifecycleStateEnabled(request.state.Enabled, request.item),
		Owner:          strings.TrimSpace(request.opts.Owner),
		DisabledReason: request.state.Reason,
		LastAction:     request.item.Action,
		LastRunKey:     request.item.IdempotencyKey,
		LastError:      request.item.Error,
		Metadata:       string(metadata),
		updatedAt:      updatedAt,
	}
}

func lifecycleStateStatus(action, errText string) string {
	if errText != "" {
		return LifecycleStatusFailed
	}
	switch action {
	case LifecycleActionInstall:
		return "installed"
	case LifecycleActionUpgrade:
		return "upgraded"
	case LifecycleActionRollback:
		return "rolled_back"
	case LifecycleActionUninstall:
		return "uninstalled"
	default:
		return action
	}
}

func lifecycleStateRecordStatus(item LifecycleResultItem, errText string) string {
	if errText != "" && item.Status != "" {
		return item.Status
	}
	return lifecycleStateStatus(item.Action, errText)
}

func lifecycleStateEnabled(current bool, item LifecycleResultItem) bool {
	return !(item.Action == LifecycleActionUninstall && item.Status == LifecycleStatusSucceeded) && current
}

func lifecycleErrText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncateLifecycleOutput(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > maxLifecycleOutput {
		value = value[:maxLifecycleOutput]
	}
	return strings.ToValidUTF8(value, "")
}
