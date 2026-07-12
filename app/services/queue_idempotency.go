package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

type QueueIdempotencyResult struct {
	Status string
	Result string
}

type QueueIdempotencyStore interface {
	Once(context.Context, string, func(context.Context) (QueueIdempotencyResult, error)) (QueueIdempotencyResult, error)
}

type MemoryQueueIdempotencyStore struct {
	mu    sync.Mutex
	items map[string]QueueIdempotencyResult
}

func NewMemoryQueueIdempotencyStore() *MemoryQueueIdempotencyStore {
	return &MemoryQueueIdempotencyStore{items: make(map[string]QueueIdempotencyResult)}
}

func (s *MemoryQueueIdempotencyStore) Once(ctx context.Context, key string, fn func(context.Context) (QueueIdempotencyResult, error)) (QueueIdempotencyResult, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return QueueIdempotencyResult{}, errors.New("queue idempotency key is required")
	}
	if reserved, result := s.reserve(key); !reserved {
		return result, nil
	}
	result, err := fn(contextOrBackground(ctx))
	if err != nil {
		s.finish(key, QueueIdempotencyResult{Status: QueueIdempotencyStatusFailed, Result: err.Error()})
		return QueueIdempotencyResult{}, err
	}
	if result.Status == "" {
		result.Status = QueueIdempotencyStatusSuccess
	}
	s.finish(key, result)
	return result, nil
}

func (s *MemoryQueueIdempotencyStore) reserve(key string) (bool, QueueIdempotencyResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result, ok := s.items[key]; ok && result.Status != QueueIdempotencyStatusFailed {
		return false, result
	}
	s.items[key] = QueueIdempotencyResult{Status: QueueIdempotencyStatusRunning}
	return true, QueueIdempotencyResult{}
}

func (s *MemoryQueueIdempotencyStore) finish(key string, result QueueIdempotencyResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = result
}

type DBQueueIdempotencyStore struct {
	connection string
}

type queueIdempotencyRecord struct {
	Key         string     `gorm:"column:key"`
	Status      string     `gorm:"column:status"`
	Result      string     `gorm:"column:result"`
	LockedUntil *time.Time `gorm:"column:locked_until"`
	ClaimToken  string     `gorm:"column:claim_token"`
}

func NewDBQueueIdempotencyStore(connection string) *DBQueueIdempotencyStore {
	return &DBQueueIdempotencyStore{connection: strings.TrimSpace(connection)}
}

func (s *DBQueueIdempotencyStore) Once(ctx context.Context, key string, fn func(context.Context) (QueueIdempotencyResult, error)) (QueueIdempotencyResult, error) {
	ctx = contextOrBackground(ctx)
	key = strings.TrimSpace(key)
	if key == "" {
		return QueueIdempotencyResult{}, errors.New("queue idempotency key is required")
	}
	reserved, claimToken, result, err := s.reserve(ctx, key)
	if !reserved || err != nil {
		return result, err
	}
	result, err = fn(ctx)
	if err != nil {
		_ = s.finish(ctx, key, claimToken, QueueIdempotencyResult{Status: QueueIdempotencyStatusFailed, Result: err.Error()})
		return QueueIdempotencyResult{}, err
	}
	if result.Status == "" {
		result.Status = QueueIdempotencyStatusSuccess
	}
	return result, s.finish(ctx, key, claimToken, result)
}

func (s *DBQueueIdempotencyStore) reserve(ctx context.Context, key string) (bool, string, QueueIdempotencyResult, error) {
	query := OrmForConnectionWithContext(ctx, s.connection).Query()
	var record queueIdempotencyRecord
	err := query.Table("queue_idempotency").Where("key", key).First(&record)
	if err == nil && record.Key != "" {
		result := QueueIdempotencyResult{Status: record.Status, Result: record.Result}
		if record.Status != QueueIdempotencyStatusFailed {
			if record.Status == QueueIdempotencyStatusRunning && isExpired(record.LockedUntil, time.Now()) {
				return s.reserveStaleRunning(ctx, key, result)
			}
			return false, "", result, nil
		}
		return s.reserveFailed(ctx, key, result)
	}
	now := time.Now()
	claimToken := randomRunToken()
	err = query.Table("queue_idempotency").Create(map[string]any{
		"key":          key,
		"status":       QueueIdempotencyStatusRunning,
		"locked_until": now.Add(5 * time.Minute),
		"claim_token":  claimToken,
		"created_at":   now,
		"updated_at":   now,
	})
	if err == nil {
		return true, claimToken, QueueIdempotencyResult{}, nil
	}
	return s.existingAfterReserveConflict(ctx, key, err)
}

func (s *DBQueueIdempotencyStore) reserveFailed(ctx context.Context, key string, fallback QueueIdempotencyResult) (bool, string, QueueIdempotencyResult, error) {
	now := time.Now()
	claimToken := randomRunToken()
	result, err := OrmForConnectionWithContext(ctx, s.connection).Query().
		Table("queue_idempotency").
		Where("key", key).
		Where("status", QueueIdempotencyStatusFailed).
		Update(map[string]any{
			"status":       QueueIdempotencyStatusRunning,
			"locked_until": now.Add(5 * time.Minute),
			"claim_token":  claimToken,
			"updated_at":   now,
		})
	if err != nil {
		return false, "", QueueIdempotencyResult{}, err
	}
	if result.RowsAffected == 1 {
		return true, claimToken, QueueIdempotencyResult{}, nil
	}
	return false, "", fallback, nil
}

func (s *DBQueueIdempotencyStore) reserveStaleRunning(ctx context.Context, key string, fallback QueueIdempotencyResult) (bool, string, QueueIdempotencyResult, error) {
	now := time.Now()
	claimToken := randomRunToken()
	result, err := OrmForConnectionWithContext(ctx, s.connection).Query().
		Table("queue_idempotency").
		Where("key", key).
		Where("status", QueueIdempotencyStatusRunning).
		Where("(locked_until IS NULL OR locked_until <= ?)", now).
		Update(map[string]any{"locked_until": now.Add(5 * time.Minute), "claim_token": claimToken, "updated_at": now})
	if err != nil {
		return false, "", QueueIdempotencyResult{}, err
	}
	if result.RowsAffected == 1 {
		return true, claimToken, QueueIdempotencyResult{}, nil
	}
	return false, "", fallback, nil
}

func (s *DBQueueIdempotencyStore) existingAfterReserveConflict(ctx context.Context, key string, originalErr error) (bool, string, QueueIdempotencyResult, error) {
	var existing queueIdempotencyRecord
	err := OrmForConnectionWithContext(ctx, s.connection).Query().
		Table("queue_idempotency").
		Where("key", key).
		First(&existing)
	if err == nil && existing.Key != "" {
		return false, "", QueueIdempotencyResult{Status: existing.Status, Result: existing.Result}, nil
	}
	return false, "", QueueIdempotencyResult{}, originalErr
}

func (s *DBQueueIdempotencyStore) finish(ctx context.Context, key, claimToken string, result QueueIdempotencyResult) error {
	_, err := OrmForConnectionWithContext(ctx, s.connection).Query().
		Table("queue_idempotency").
		Where("key", key).
		Where("status", QueueIdempotencyStatusRunning).
		Where("claim_token", claimToken).
		Update(map[string]any{
			"status":       result.Status,
			"result":       result.Result,
			"last_error":   lastErrorForIdempotencyResult(result),
			"locked_until": nil,
			"claim_token":  "",
			"updated_at":   time.Now(),
		})
	return err
}

func isExpired(deadline *time.Time, now time.Time) bool {
	return deadline == nil || !deadline.After(now)
}

func lastErrorForIdempotencyResult(result QueueIdempotencyResult) any {
	if result.Status == QueueIdempotencyStatusFailed {
		return result.Result
	}
	return nil
}
