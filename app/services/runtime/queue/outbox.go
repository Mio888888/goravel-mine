package queue

import (
	"context"
	"errors"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
)

type QueueOutboxEvent struct {
	ID          uint64
	Topic       string
	Connection  string
	Queue       string
	Payload     string
	Status      string
	Attempts    int
	AvailableAt time.Time
	LockedUntil *time.Time
	LockOwner   string
	ClaimToken  string
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type QueueOutboxHandler func(context.Context, QueueOutboxEvent) error

type QueueOutboxStore interface {
	ClaimDue(context.Context, string, int) ([]QueueOutboxEvent, error)
	MarkSent(context.Context, uint64, string) error
	MarkFailed(context.Context, uint64, string, error) error
}

type QueueOutboxDispatchResult struct {
	Sent   int
	Failed int
}

type QueueOutboxDispatcher struct {
	Store    QueueOutboxStore
	Dispatch func(context.Context, QueueOutboxEvent) error
}

func (d QueueOutboxDispatcher) DispatchDue(ctx context.Context, owner string, limit int) (QueueOutboxDispatchResult, error) {
	ctx = contextOrBackground(ctx)
	if d.Store == nil {
		return QueueOutboxDispatchResult{}, errors.New("queue outbox store is required")
	}
	dispatch := d.Dispatch
	if dispatch == nil {
		dispatch = DispatchQueueOutboxEvent
	}
	events, err := d.Store.ClaimDue(ctx, owner, limit)
	if err != nil {
		return QueueOutboxDispatchResult{}, err
	}
	result := QueueOutboxDispatchResult{}
	for _, event := range events {
		if err := dispatch(ctx, event); err != nil {
			if markErr := d.Store.MarkFailed(ctx, event.ID, event.ClaimToken, err); markErr != nil {
				return result, markErr
			}
			result.Failed++
			continue
		}
		if err := d.Store.MarkSent(ctx, event.ID, event.ClaimToken); err != nil {
			return result, err
		}
		result.Sent++
	}
	return result, nil
}

func EnqueueQueueOutboxEvent(ctx context.Context, event QueueOutboxEvent) error {
	return EnqueueQueueOutboxEventWithQuery(OrmWithContext(ctx).Query(), event)
}

func EnqueueQueueOutboxEventWithQuery(query contractsorm.Query, event QueueOutboxEvent) error {
	if query == nil {
		return errors.New("queue outbox query is required")
	}
	event.Topic = strings.TrimSpace(event.Topic)
	if event.Topic == "" {
		return errors.New("queue outbox topic is required")
	}
	if event.Connection == "" {
		event.Connection = facades.Config().GetString("queue.default", "redis")
	}
	if event.Queue == "" {
		event.Queue = "default"
	}
	if event.Status == "" {
		event.Status = QueueOutboxStatusPending
	}
	now := time.Now()
	if event.AvailableAt.IsZero() {
		event.AvailableAt = now.Add(-time.Second)
	}
	return query.Table("queue_outbox").Create(map[string]any{
		"topic":        event.Topic,
		"connection":   event.Connection,
		"queue":        event.Queue,
		"payload":      event.Payload,
		"status":       event.Status,
		"attempts":     event.Attempts,
		"available_at": event.AvailableAt,
		"created_at":   now,
		"updated_at":   now,
	})
}
