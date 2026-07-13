package services

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/queue"

	"goravel/app/facades"
)

const operationLogQueueName = "operation_logs"

const (
	operationLogRetryAttempts = 4
	operationLogRetryMaxDelay = 30 * time.Second
)

var operationLogDispatcher = newOperationLogWorker(128)

type OperationLogJob struct{}

type OperationLogRunner struct{}

func (j *OperationLogJob) Signature() string {
	return "operation_log"
}

func (j *OperationLogJob) Handle(args ...any) error {
	payload := operationLogPayloadFromArgs(args)
	return NewLogAdminServiceForConnection(payload.Connection).WriteOperationLog(payload)
}

func (j *OperationLogJob) ShouldRetry(err error, attempt int) (bool, time.Duration) {
	if err == nil || attempt >= operationLogRetryAttempts {
		return false, 0
	}
	delay := time.Duration(1<<attempt) * time.Second
	if delay > operationLogRetryMaxDelay {
		delay = operationLogRetryMaxDelay
	}
	return true, delay
}

func DispatchOperationLog(payload OperationLogPayload) {
	if isTestingEnvironment(facades.Config().GetString("app.env")) {
		_ = NewLogAdminServiceForConnection(payload.Connection).WriteOperationLog(payload)
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
		Username:    stringArg(args, 0),
		Method:      stringArg(args, 1),
		Router:      stringArg(args, 2),
		ServiceName: stringArg(args, 3),
		IP:          stringArg(args, 4),
		Connection:  stringArg(args, 5),
	}
}

func stringArg(args []any, index int) string {
	if index >= len(args) {
		return ""
	}
	value, _ := args[index].(string)
	return value
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
		_ = NewLogAdminServiceForConnection(payload.Connection).WriteOperationLog(payload)
		return
	}

	w.mu.Lock()
	if w.stopping {
		w.mu.Unlock()
		_ = NewLogAdminServiceForConnection(payload.Connection).WriteOperationLog(payload)
		return
	}
	select {
	case w.queue <- payload:
		w.mu.Unlock()
	default:
		w.mu.Unlock()
		_ = NewLogAdminServiceForConnection(payload.Connection).WriteOperationLog(payload)
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
		_ = NewLogAdminServiceForConnection(payload.Connection).WriteOperationLog(payload)
	}
}
