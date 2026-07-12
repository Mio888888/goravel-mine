package modulecatalog

import (
	"context"
	"errors"
	"strings"
	"time"
)

type lifecycleLockRecord struct {
	Key       string    `gorm:"column:key"`
	Owner     string    `gorm:"column:owner"`
	RunKey    string    `gorm:"column:run_key"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
}

func (s *DBLifecycleStore) Acquire(ctx context.Context, request LifecycleLockAcquire) (LifecycleLock, error) {
	ctx = contextOrBackground(ctx)
	key := strings.TrimSpace(request.Key)
	owner := strings.TrimSpace(request.Owner)
	if key == "" || owner == "" {
		return LifecycleLock{}, errors.New("module lifecycle lock key and owner are required")
	}
	if request.TTL <= 0 {
		request.TTL = time.Minute
	}
	query := s.orm(ctx).Query()
	now := s.now()
	expiresAt := now.Add(request.TTL)
	result, err := query.Table("module_lifecycle_lock").
		Where("key", key).
		Where("(expires_at IS NULL OR expires_at <= ?)", now).
		Update(map[string]any{"owner": owner, "run_key": request.RunKey, "expires_at": expiresAt, "updated_at": now})
	if err != nil {
		return LifecycleLock{}, err
	}
	if result.RowsAffected == 1 {
		return LifecycleLock{Key: key, Owner: owner, RunKey: request.RunKey, Acquired: true, ExpiresAt: expiresAt}, nil
	}
	err = query.Table("module_lifecycle_lock").Create(map[string]any{
		"key":        key,
		"owner":      owner,
		"run_key":    request.RunKey,
		"expires_at": expiresAt,
		"created_at": now,
		"updated_at": now,
	})
	if err == nil {
		return LifecycleLock{Key: key, Owner: owner, RunKey: request.RunKey, Acquired: true, ExpiresAt: expiresAt}, nil
	}
	var record lifecycleLockRecord
	if readErr := query.Table("module_lifecycle_lock").Where("key", key).First(&record); readErr != nil || record.Key == "" {
		return LifecycleLock{}, err
	}
	if record.ExpiresAt.After(now) {
		return LifecycleLock{Key: key, Owner: record.Owner, RunKey: record.RunKey, ExpiresAt: record.ExpiresAt}, nil
	}
	return LifecycleLock{}, err
}

func (s *DBLifecycleStore) Renew(ctx context.Context, request LifecycleLockRenewal) (LifecycleLock, error) {
	ctx = contextOrBackground(ctx)
	lock := request.Lock
	if !lock.Acquired {
		return lock, errors.New("module lifecycle lock is not acquired")
	}
	if request.TTL <= 0 {
		request.TTL = time.Minute
	}
	now := s.now()
	expiresAt := now.Add(request.TTL)
	result, err := s.orm(ctx).Query().
		Table("module_lifecycle_lock").
		Where("key", lock.Key).
		Where("owner", lock.Owner).
		Where("run_key", lock.RunKey).
		Update(map[string]any{"expires_at": expiresAt, "updated_at": now})
	if err != nil {
		return LifecycleLock{}, err
	}
	if result.RowsAffected != 1 {
		return LifecycleLock{}, errors.New("module lifecycle lock lost: " + lock.Key)
	}
	lock.ExpiresAt = expiresAt
	return lock, nil
}

func (s *DBLifecycleStore) Release(ctx context.Context, lock LifecycleLock) error {
	ctx = contextOrBackground(ctx)
	if !lock.Acquired {
		return nil
	}
	_, err := s.orm(ctx).Query().
		Table("module_lifecycle_lock").
		Where("key", lock.Key).
		Where("owner", lock.Owner).
		Where("run_key", lock.RunKey).
		Delete()
	return err
}
