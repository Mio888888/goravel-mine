package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/goravel/framework/contracts/queue"

	"goravel/app/facades"
)

var queueOutboxHandlers = struct {
	sync.RWMutex
	items map[string]QueueOutboxHandler
}{items: make(map[string]QueueOutboxHandler)}

type QueueOutboxDispatchJob struct{}

func (j *QueueOutboxDispatchJob) Signature() string {
	return "queue_outbox_dispatch"
}

func (j *QueueOutboxDispatchJob) Handle(args ...any) error {
	event := QueueOutboxEvent{
		ID:         uint64Arg(args, 0),
		Topic:      stringArg(args, 1),
		Connection: stringArg(args, 2),
		Queue:      stringArg(args, 3),
		Payload:    stringArg(args, 4),
	}
	handler, ok := queueOutboxHandler(event.Topic)
	if !ok {
		return errors.New("queue outbox handler not registered: " + event.Topic)
	}
	return handler(context.Background(), event)
}

func (j *QueueOutboxDispatchJob) ShouldRetry(err error, attempt int) (bool, time.Duration) {
	return QueueRetryPolicy{MaxAttempts: 4, InitialDelay: time.Second, MaxDelay: 30 * time.Second}.ShouldRetry(err, attempt)
}

func RegisterQueueOutboxHandler(topic string, handler QueueOutboxHandler) {
	topic = strings.TrimSpace(topic)
	if topic == "" || handler == nil {
		return
	}
	queueOutboxHandlers.Lock()
	defer queueOutboxHandlers.Unlock()
	queueOutboxHandlers.items[topic] = handler
}

func UnregisterQueueOutboxHandler(topic string) {
	queueOutboxHandlers.Lock()
	defer queueOutboxHandlers.Unlock()
	delete(queueOutboxHandlers.items, strings.TrimSpace(topic))
}

func queueOutboxHandler(topic string) (QueueOutboxHandler, bool) {
	queueOutboxHandlers.RLock()
	defer queueOutboxHandlers.RUnlock()
	handler, ok := queueOutboxHandlers.items[strings.TrimSpace(topic)]
	return handler, ok
}

func (e QueueOutboxEvent) QueueArgs() []queue.Arg {
	return []queue.Arg{
		{Type: "uint64", Value: e.ID},
		{Type: "string", Value: e.Topic},
		{Type: "string", Value: e.Connection},
		{Type: "string", Value: e.Queue},
		{Type: "string", Value: e.Payload},
	}
}

func DispatchQueueOutboxEvent(ctx context.Context, event QueueOutboxEvent) error {
	_ = contextOrBackground(ctx)
	pending := facades.Queue().Job(&QueueOutboxDispatchJob{}, event.QueueArgs())
	if event.Connection != "" {
		pending = pending.OnConnection(event.Connection)
	}
	if event.Queue != "" {
		pending = pending.OnQueue(event.Queue)
	}
	return pending.Dispatch()
}

func uint64Arg(args []any, index int) uint64 {
	if index >= len(args) {
		return 0
	}
	switch value := args[index].(type) {
	case uint64:
		return value
	case uint:
		return uint64(value)
	case int:
		if value < 0 {
			return 0
		}
		return uint64(value)
	case int64:
		if value < 0 {
			return 0
		}
		return uint64(value)
	default:
		return 0
	}
}
