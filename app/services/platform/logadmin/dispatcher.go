package logadmin

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/queue"

	"goravel/app/facades"
	queueservice "goravel/app/services/runtime/queue"
	"goravel/app/support/jobarg"
)

const operationLogQueueName = "operation_logs"

var operationLogDispatcher = newOperationLogWorker(128)

var operationLogRetryPolicy = queueservice.QueueRetryPolicy{
	MaxAttempts:  4,
	InitialDelay: 2 * time.Second,
	MaxDelay:     30 * time.Second,
}

type OperationLogJob struct{}

type OperationLogRunner struct{}

func (j *OperationLogJob) Signature() string {
	return "operation_log"
}

func (j *OperationLogJob) Handle(args ...any) error {
	payload := operationLogPayloadFromArgs(args)
	return NewServiceForConnection(payload.Connection).WriteOperationLog(payload)
}

func (j *OperationLogJob) ShouldRetry(err error, attempt int) (bool, time.Duration) {
	return operationLogRetryPolicy.ShouldRetry(err, attempt)
}

func DispatchOperationLog(payload OperationLogPayload) {
	if isTestingEnvironment(facades.Config().GetString("app.env")) {
		_ = NewServiceForConnection(payload.Connection).WriteOperationLog(payload)
		return
	}
	if shouldUseConfiguredQueue() {
		if err := dispatchOperationLogJob(payload); err == nil {
			return
		}
	}
	operationLogDispatcher.Dispatch(payload)
}

func NewOperationLogRunner() foundation.Runner {
	return &OperationLogRunner{}
}

func (r *OperationLogRunner) Signature() string {
	return "operation_log_worker"
}

func (r *OperationLogRunner) ShouldRun() bool {
	return !isTestingEnvironment(facades.Config().GetString("app.env")) && !shouldUseConfiguredQueue()
}

func (r *OperationLogRunner) Run() error {
	if err := operationLogDispatcher.Start(); err != nil {
		return err
	}
	<-operationLogDispatcher.Done()
	return nil
}

func (r *OperationLogRunner) Shutdown() error {
	return operationLogDispatcher.Shutdown()
}

func dispatchOperationLogJob(payload OperationLogPayload) error {
	args := []queue.Arg{
		{Type: "string", Value: payload.Username},
		{Type: "string", Value: payload.Method},
		{Type: "string", Value: payload.Router},
		{Type: "string", Value: payload.ServiceName},
		{Type: "string", Value: payload.IP},
		{Type: "string", Value: payload.Connection},
	}
	return facades.Queue().Job(&OperationLogJob{}, args).OnQueue(operationLogQueueName).Dispatch()
}

func shouldUseConfiguredQueue() bool {
	return facades.Config().GetString("queue.default") != "sync"
}

func isTestingEnvironment(environment string) bool {
	return strings.EqualFold(strings.TrimSpace(environment), "testing")
}

func operationLogPayloadFromArgs(args []any) OperationLogPayload {
	return OperationLogPayload{
		Username:    jobarg.String(args, 0),
		Method:      jobarg.String(args, 1),
		Router:      jobarg.String(args, 2),
		ServiceName: jobarg.String(args, 3),
		IP:          jobarg.String(args, 4),
		Connection:  jobarg.String(args, 5),
	}
}

type operationLogWorker struct {
	mu       sync.Mutex
	started  bool
	stopping bool
	queue    chan OperationLogPayload
	done     chan struct{}
}

func newOperationLogWorker(size int) *operationLogWorker {
	return &operationLogWorker{
		queue: make(chan OperationLogPayload, size),
		done:  make(chan struct{}),
	}
}

func (w *operationLogWorker) Dispatch(payload OperationLogPayload) {
	if err := w.Start(); err != nil {
		_ = NewServiceForConnection(payload.Connection).WriteOperationLog(payload)
		return
	}

	w.mu.Lock()
	if w.stopping {
		w.mu.Unlock()
		_ = NewServiceForConnection(payload.Connection).WriteOperationLog(payload)
		return
	}
	select {
	case w.queue <- payload:
		w.mu.Unlock()
	default:
		w.mu.Unlock()
		_ = NewServiceForConnection(payload.Connection).WriteOperationLog(payload)
	}
}

func (w *operationLogWorker) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopping {
		return errors.New("operation log worker is stopping")
	}
	if w.started {
		return nil
	}
	w.started = true
	go w.run()
	return nil
}

func (w *operationLogWorker) Done() <-chan struct{} {
	return w.done
}

func (w *operationLogWorker) Shutdown() error {
	w.mu.Lock()
	if w.stopping {
		w.mu.Unlock()
		<-w.done
		return nil
	}
	w.stopping = true
	if !w.started {
		close(w.done)
		w.mu.Unlock()
		return nil
	}
	close(w.queue)
	w.mu.Unlock()
	<-w.done
	return nil
}

func (w *operationLogWorker) run() {
	defer close(w.done)
	for payload := range w.queue {
		_ = NewServiceForConnection(payload.Connection).WriteOperationLog(payload)
	}
}
