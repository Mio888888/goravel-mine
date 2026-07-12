package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	QueueOutboxStatusPending    = "pending"
	QueueOutboxStatusProcessing = "processing"
	QueueOutboxStatusSent       = "sent"
	QueueOutboxStatusFailed     = "failed"

	QueueIdempotencyStatusRunning = "running"
	QueueIdempotencyStatusSuccess = "success"
	QueueIdempotencyStatusFailed  = "failed"
)

type QueueRetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

func (p QueueRetryPolicy) ShouldRetry(err error, attempt int) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}
	maxAttempts := p.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 3
	}
	if attempt >= maxAttempts {
		return false, 0
	}
	delay := p.InitialDelay
	if delay <= 0 {
		delay = time.Second
	}
	for i := 1; i < attempt; i++ {
		delay *= 2
	}
	if p.MaxDelay > 0 && delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	return true, delay
}

type QueueTaskLock struct {
	Key       string
	Owner     string
	Acquired  bool
	ExpiresAt time.Time
}

type QueueTaskLockStore interface {
	Acquire(context.Context, string, string, time.Duration) (QueueTaskLock, error)
	Release(context.Context, QueueTaskLock) error
}

type MemoryQueueTaskLockStore struct {
	mu    sync.Mutex
	locks map[string]QueueTaskLock
	now   func() time.Time
}

func NewMemoryQueueTaskLockStore() *MemoryQueueTaskLockStore {
	return &MemoryQueueTaskLockStore{locks: make(map[string]QueueTaskLock), now: time.Now}
}

func (s *MemoryQueueTaskLockStore) Acquire(ctx context.Context, key, owner string, ttl time.Duration) (QueueTaskLock, error) {
	_ = contextOrBackground(ctx)
	key = strings.TrimSpace(key)
	owner = strings.TrimSpace(owner)
	if key == "" || owner == "" {
		return QueueTaskLock{}, errors.New("queue lock key and owner are required")
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.locks[key]; ok && current.ExpiresAt.After(now) {
		return QueueTaskLock{Key: key, Owner: current.Owner, ExpiresAt: current.ExpiresAt}, nil
	}
	lock := QueueTaskLock{Key: key, Owner: owner, Acquired: true, ExpiresAt: now.Add(ttl)}
	s.locks[key] = lock
	return lock, nil
}

type DBQueueTaskLockStore struct {
	connection string
}

type queueTaskLockRecord struct {
	Key       string    `gorm:"column:key"`
	Owner     string    `gorm:"column:owner"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
}

func NewDBQueueTaskLockStore(connection string) *DBQueueTaskLockStore {
	return &DBQueueTaskLockStore{connection: strings.TrimSpace(connection)}
}

func (s *DBQueueTaskLockStore) Acquire(ctx context.Context, key, owner string, ttl time.Duration) (QueueTaskLock, error) {
	ctx = contextOrBackground(ctx)
	key = strings.TrimSpace(key)
	owner = strings.TrimSpace(owner)
	if key == "" || owner == "" {
		return QueueTaskLock{}, errors.New("queue lock key and owner are required")
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	query := OrmForConnectionWithContext(ctx, s.connection).Query()
	now := time.Now()
	expiresAt := now.Add(ttl)
	result, err := query.Table("queue_task_lock").
		Where("key", key).
		Where("(expires_at IS NULL OR expires_at <= ? OR owner = ?)", now, owner).
		Update(map[string]any{"owner": owner, "expires_at": expiresAt, "updated_at": now})
	if err != nil {
		return QueueTaskLock{}, err
	}
	if result.RowsAffected == 1 {
		return QueueTaskLock{Key: key, Owner: owner, Acquired: true, ExpiresAt: expiresAt}, nil
	}
	err = query.Table("queue_task_lock").Create(map[string]any{
		"key":        key,
		"owner":      owner,
		"expires_at": expiresAt,
		"created_at": now,
		"updated_at": now,
	})
	if err == nil {
		return QueueTaskLock{Key: key, Owner: owner, Acquired: true, ExpiresAt: expiresAt}, nil
	}
	var record queueTaskLockRecord
	if readErr := query.Table("queue_task_lock").Where("key", key).First(&record); readErr != nil || record.Key == "" {
		return QueueTaskLock{}, err
	}
	if record.ExpiresAt.After(now) && record.Owner != owner {
		return QueueTaskLock{Key: key, Owner: record.Owner, ExpiresAt: record.ExpiresAt}, nil
	}
	return QueueTaskLock{}, err
}

func (s *DBQueueTaskLockStore) Release(ctx context.Context, lock QueueTaskLock) error {
	ctx = contextOrBackground(ctx)
	if !lock.Acquired || lock.Key == "" {
		return nil
	}
	_, err := OrmForConnectionWithContext(ctx, s.connection).Query().
		Table("queue_task_lock").
		Where("key", lock.Key).
		Where("owner", lock.Owner).
		Delete()
	return err
}

func (s *MemoryQueueTaskLockStore) Release(ctx context.Context, lock QueueTaskLock) error {
	_ = contextOrBackground(ctx)
	if !lock.Acquired || lock.Key == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.locks[lock.Key]; ok && current.Owner == lock.Owner {
		delete(s.locks, lock.Key)
	}
	return nil
}
