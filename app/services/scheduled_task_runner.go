package services

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goravel/framework/contracts/foundation"

	"goravel/app/facades"
)

type ScheduledTaskRunner struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	once    sync.Once
	started atomic.Bool
}

func NewScheduledTaskRunner() foundation.Runner {
	ctx, cancel := context.WithCancel(context.Background())
	return &ScheduledTaskRunner{ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

func (r *ScheduledTaskRunner) Signature() string {
	return "scheduled_task_runner"
}

func (r *ScheduledTaskRunner) ShouldRun() bool {
	return facades.Config().GetBool("scheduler.enabled", true)
}

func (r *ScheduledTaskRunner) Run() error {
	r.started.Store(true)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer close(r.done)

	service := NewScheduledTaskService()
	RecordSchedulerHeartbeat(service.nodeIP, scheduledTaskNow())
	for {
		select {
		case <-r.ctx.Done():
			return nil
		case now := <-ticker.C:
			RecordSchedulerHeartbeat(service.nodeIP, now)
			_ = service.RunDue(r.ctx, now)
		}
	}
}

func (r *ScheduledTaskRunner) Shutdown() error {
	r.once.Do(func() {
		r.cancel()
		if r.started.Load() {
			<-r.done
		}
	})
	return nil
}
