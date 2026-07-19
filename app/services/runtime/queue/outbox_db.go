package queue

import (
	"context"
	"strings"
	"time"
)

type DBQueueOutboxStore struct {
	connection string
}

func NewDBQueueOutboxStore(connection string) *DBQueueOutboxStore {
	return &DBQueueOutboxStore{connection: strings.TrimSpace(connection)}
}

func (s *DBQueueOutboxStore) ClaimDue(ctx context.Context, owner string, limit int) ([]QueueOutboxEvent, error) {
	ctx = contextOrBackground(ctx)
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "queue-worker"
	}
	if limit < 1 {
		limit = 10
	}
	query := OrmForConnectionWithContext(ctx, s.connection).Query()
	now := time.Now()
	rows := make([]QueueOutboxEvent, 0)
	err := query.Table("queue_outbox").
		Where("(status = ? OR (status = ? AND locked_until <= ?))", QueueOutboxStatusPending, QueueOutboxStatusProcessing, now).
		Where("(available_at IS NULL OR available_at <= ?)", now).
		Where("(locked_until IS NULL OR locked_until <= ?)", now).
		OrderBy("id").
		Limit(limit).
		Get(&rows)
	if err != nil {
		return nil, err
	}
	claimed := make([]QueueOutboxEvent, 0, len(rows))
	lockUntil := now.Add(5 * time.Minute)
	for _, row := range rows {
		claimToken := randomRunToken()
		result, err := query.Table("queue_outbox").
			Where("id", row.ID).
			Where("(status = ? OR (status = ? AND locked_until <= ?))", QueueOutboxStatusPending, QueueOutboxStatusProcessing, now).
			Where("(locked_until IS NULL OR locked_until <= ?)", now).
			Update(map[string]any{
				"status":       QueueOutboxStatusProcessing,
				"lock_owner":   owner,
				"claim_token":  claimToken,
				"locked_until": lockUntil,
				"updated_at":   now,
			})
		if err != nil {
			return nil, err
		}
		if result.RowsAffected == 1 {
			row.Status = QueueOutboxStatusProcessing
			row.LockOwner = owner
			row.ClaimToken = claimToken
			row.LockedUntil = &lockUntil
			claimed = append(claimed, row)
		}
	}
	return claimed, nil
}

func (s *DBQueueOutboxStore) MarkSent(ctx context.Context, id uint64, claimToken string) error {
	claimToken = strings.TrimSpace(claimToken)
	if claimToken == "" {
		return nil
	}
	now := time.Now()
	_, err := OrmForConnectionWithContext(ctx, s.connection).Query().
		Table("queue_outbox").
		Where("id", id).
		Where("status", QueueOutboxStatusProcessing).
		Where("claim_token", claimToken).
		Update(map[string]any{
			"status":       QueueOutboxStatusSent,
			"locked_until": nil,
			"lock_owner":   "",
			"claim_token":  "",
			"last_error":   nil,
			"published_at": now,
			"updated_at":   now,
		})
	return err
}

func (s *DBQueueOutboxStore) MarkFailed(ctx context.Context, id uint64, claimToken string, dispatchErr error) error {
	claimToken = strings.TrimSpace(claimToken)
	if claimToken == "" {
		return nil
	}
	now := time.Now()
	var row QueueOutboxEvent
	query := OrmForConnectionWithContext(ctx, s.connection).Query()
	if err := query.Table("queue_outbox").
		Where("id", id).
		Where("status", QueueOutboxStatusProcessing).
		Where("claim_token", claimToken).
		First(&row); err != nil {
		return err
	}
	attempts := row.Attempts + 1
	status := QueueOutboxStatusFailed
	availableAt := now
	if retryable, delay := queueOutboxRetryPolicy.ShouldRetry(dispatchErr, attempts); retryable {
		status = QueueOutboxStatusPending
		availableAt = now.Add(delay)
	}
	_, err := query.Table("queue_outbox").
		Where("id", id).
		Where("status", QueueOutboxStatusProcessing).
		Where("claim_token", claimToken).
		Update(map[string]any{
			"status":       status,
			"attempts":     attempts,
			"available_at": availableAt,
			"locked_until": nil,
			"lock_owner":   "",
			"claim_token":  "",
			"last_error":   dispatchErr.Error(),
			"updated_at":   now,
		})
	return err
}
