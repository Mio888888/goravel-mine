package scheduledtask

import (
	"context"
	"sync"
	"time"

	"goravel/app/models"
)

type ScheduledTaskRetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

func scheduledTaskRetryPolicy(task ScheduledTask) ScheduledTaskRetryPolicy {
	return ScheduledTaskRetryPolicy{
		MaxAttempts:  jsonMapInt(task.RetryPolicy, "max_attempts", 1),
		InitialDelay: time.Duration(jsonMapInt(task.RetryPolicy, "initial_delay_seconds", 1)) * time.Second,
		MaxDelay:     time.Duration(jsonMapInt(task.RetryPolicy, "max_delay_seconds", 30)) * time.Second,
	}
}

func (p ScheduledTaskRetryPolicy) Delay(attempt int) time.Duration {
	delay := p.InitialDelay
	if delay <= 0 {
		delay = time.Second
	}
	for index := 1; index < attempt; index++ {
		delay *= 2
	}
	if p.MaxDelay > 0 && delay > p.MaxDelay {
		return p.MaxDelay
	}
	return delay
}

func jsonMapInt(value models.JSONMap, key string, fallback int) int {
	switch typed := value[key].(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

var scheduledTaskActiveRuns = struct {
	sync.Mutex
	entries map[uint64]*scheduledTaskActiveRun
}{entries: make(map[uint64]*scheduledTaskActiveRun)}

type scheduledTaskActiveRun struct {
	cancel context.CancelFunc
}

func registerScheduledTaskRun(task ScheduledTask, parent context.Context) (context.Context, context.CancelFunc) {
	runCtx, cancel := context.WithCancel(parent)
	if task.ConcurrencyPolicy != ScheduledTaskConcurrencyReplace {
		return runCtx, cancel
	}
	entry := &scheduledTaskActiveRun{cancel: cancel}
	scheduledTaskActiveRuns.Lock()
	if previous := scheduledTaskActiveRuns.entries[task.ID]; previous != nil {
		previous.cancel()
	}
	scheduledTaskActiveRuns.entries[task.ID] = entry
	scheduledTaskActiveRuns.Unlock()
	return runCtx, func() {
		cancel()
		scheduledTaskActiveRuns.Lock()
		if scheduledTaskActiveRuns.entries[task.ID] == entry {
			delete(scheduledTaskActiveRuns.entries, task.ID)
		}
		scheduledTaskActiveRuns.Unlock()
	}
}
