package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/models"
)

type QueueOutboxEvent struct {
	ID             uint64         `gorm:"column:id"`
	Topic          string         `gorm:"column:topic"`
	Connection     string         `gorm:"column:connection"`
	Queue          string         `gorm:"column:queue"`
	Payload        string         `gorm:"column:payload"`
	Status         string         `gorm:"column:status"`
	Attempts       int            `gorm:"column:attempts"`
	AvailableAt    time.Time      `gorm:"column:available_at"`
	LockedUntil    *time.Time     `gorm:"column:locked_until"`
	LockOwner      string         `gorm:"column:lock_owner"`
	ClaimToken     string         `gorm:"column:claim_token"`
	LastError      string         `gorm:"column:last_error"`
	MessageID      string         `gorm:"column:message_id"`
	MessageType    string         `gorm:"column:message_type"`
	SchemaVersion  int            `gorm:"column:schema_version"`
	RouteID        uint64         `gorm:"column:route_id"`
	AdapterID      uint64         `gorm:"column:adapter_id"`
	Envelope       models.JSONMap `gorm:"column:envelope;type:jsonb"`
	CorrelationID  string         `gorm:"column:correlation_id"`
	TenantID       string         `gorm:"column:tenant_id"`
	PublishedAt    *time.Time     `gorm:"column:published_at"`
	PublishReceipt string         `gorm:"column:publish_receipt"`
	CreatedAt      time.Time      `gorm:"column:created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
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
	envelope, err := json.Marshal(event.Envelope)
	if err != nil {
		return err
	}
	_, err = query.Exec(`
		INSERT INTO queue_outbox (
			topic, connection, queue, payload, status, attempts, available_at,
			message_id, message_type, schema_version, route_id, adapter_id, envelope,
			correlation_id, tenant_id, publish_receipt, created_at, updated_at
		)
		VALUES (?, ?, ?, ?::jsonb, ?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?, ?, ?, ?, ?)
	`, event.Topic, event.Connection, event.Queue, event.Payload, event.Status, event.Attempts,
		event.AvailableAt, strings.TrimSpace(event.MessageID), strings.TrimSpace(event.MessageType),
		event.SchemaVersion, event.RouteID, event.AdapterID, string(envelope),
		strings.TrimSpace(event.CorrelationID), strings.TrimSpace(event.TenantID),
		strings.TrimSpace(event.PublishReceipt), now, now)
	return err
}
