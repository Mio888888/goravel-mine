package services

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goravel/framework/contracts/foundation"

	"goravel/app/facades"
)

type QueueOutboxRunner struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	once    sync.Once
	started atomic.Bool
}

type queueOutboxErrorLogger func(string, map[string]any)

func NewQueueOutboxRunner() foundation.Runner {
	ctx, cancel := context.WithCancel(context.Background())
	return &QueueOutboxRunner{ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

func (r *QueueOutboxRunner) Signature() string {
	return "queue_outbox_runner"
}

func (r *QueueOutboxRunner) ShouldRun() bool {
	return ShouldRunQueueOutboxRunner(
		facades.Config().GetBool("queue.outbox.enabled", true),
		facades.Config().GetBool("queue.worker.enabled", true),
		facades.Config().GetString("queue.default", "sync"),
	)
}

func ShouldRunQueueOutboxRunner(outboxEnabled, queueWorkerEnabled bool, connection string) bool {
	connection = strings.TrimSpace(connection)
	return outboxEnabled && queueWorkerEnabled && connection != "" && connection != "sync"
}

func (r *QueueOutboxRunner) Run() error {
	r.started.Store(true)
	defer close(r.done)
	interval := time.Duration(facades.Config().GetInt("queue.outbox.interval_seconds", 5)) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	batch := facades.Config().GetInt("queue.outbox.batch", 20)
	if batch < 1 {
		batch = 20
	}
	owner := facades.Config().GetString("queue.outbox.owner", "queue-outbox-runner")
	dispatcher := QueueOutboxDispatcher{Store: NewDBQueueOutboxStore("")}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		runQueueOutboxDispatchOnce(r.ctx, dispatcher, owner, batch, logQueueOutboxDispatchError)
		select {
		case <-r.ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func runQueueOutboxDispatchOnce(ctx context.Context, dispatcher QueueOutboxDispatcher, owner string, batch int, logger queueOutboxErrorLogger) {
	if _, err := dispatcher.DispatchDue(ctx, owner, batch); err != nil && logger != nil {
		logger("queue outbox dispatch failed", map[string]any{
			"error": err.Error(),
			"owner": owner,
			"batch": batch,
		})
	}
}

func RunQueueOutboxDispatchOnceForTest(ctx context.Context, dispatcher QueueOutboxDispatcher, owner string, batch int, logger func(string, map[string]any)) {
	runQueueOutboxDispatchOnce(ctx, dispatcher, owner, batch, logger)
}

func logQueueOutboxDispatchError(message string, fields map[string]any) {
	facades.Log().With(fields).Error(message)
}

func (r *QueueOutboxRunner) Shutdown() error {
	r.once.Do(func() {
		r.cancel()
		if r.started.Load() {
			<-r.done
		}
	})
	return nil
}
