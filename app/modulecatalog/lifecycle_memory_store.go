package modulecatalog

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

type MemoryLifecycleStore struct {
	mu      sync.Mutex
	locks   map[string]LifecycleLock
	runs    map[string]LifecycleRunRecord
	steps   []LifecycleStepRecord
	states  map[string]LifecycleStateRecord
	nowFunc func() time.Time
}

func NewMemoryLifecycleStore() *MemoryLifecycleStore {
	return &MemoryLifecycleStore{
		locks:   map[string]LifecycleLock{},
		runs:    map[string]LifecycleRunRecord{},
		steps:   []LifecycleStepRecord{},
		states:  map[string]LifecycleStateRecord{},
		nowFunc: time.Now,
	}
}

func (s *MemoryLifecycleStore) Acquire(ctx context.Context, request LifecycleLockAcquire) (LifecycleLock, error) {
	_ = contextOrBackground(ctx)
	key := strings.TrimSpace(request.Key)
	owner := strings.TrimSpace(request.Owner)
	if key == "" || owner == "" {
		return LifecycleLock{}, errors.New("module lifecycle lock key and owner are required")
	}
	if request.TTL <= 0 {
		request.TTL = time.Minute
	}
	now := s.nowFunc()
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.locks[key]; ok && current.ExpiresAt.After(now) {
		return LifecycleLock{Key: key, Owner: current.Owner, RunKey: current.RunKey, ExpiresAt: current.ExpiresAt}, nil
	}
	lock := LifecycleLock{Key: key, Owner: owner, RunKey: request.RunKey, Acquired: true, ExpiresAt: now.Add(request.TTL)}
	s.locks[key] = lock
	return lock, nil
}

func (s *MemoryLifecycleStore) Renew(ctx context.Context, request LifecycleLockRenewal) (LifecycleLock, error) {
	_ = contextOrBackground(ctx)
	lock := request.Lock
	if !lock.Acquired {
		return lock, errors.New("module lifecycle lock is not acquired")
	}
	if request.TTL <= 0 {
		request.TTL = time.Minute
	}
	now := s.nowFunc()
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.locks[lock.Key]
	if !ok || current.Owner != lock.Owner || current.RunKey != lock.RunKey {
		return LifecycleLock{}, errors.New("module lifecycle lock lost: " + lock.Key)
	}
	lock.ExpiresAt = now.Add(request.TTL)
	s.locks[lock.Key] = lock
	return lock, nil
}

func (s *MemoryLifecycleStore) Release(ctx context.Context, lock LifecycleLock) error {
	_ = contextOrBackground(ctx)
	if !lock.Acquired {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.locks[lock.Key]; ok && current.Owner == lock.Owner && current.RunKey == lock.RunKey {
		delete(s.locks, lock.Key)
	}
	return nil
}

func (s *MemoryLifecycleStore) SuccessfulRunExists(ctx context.Context, key string) (bool, error) {
	_ = contextOrBackground(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[key]
	return ok && lifecycleRunBlocksAutomaticRetry(run.Status), nil
}

func (s *MemoryLifecycleStore) BeginRun(ctx context.Context, run LifecycleRunRecord) error {
	_ = contextOrBackground(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	run.Status = LifecycleStatusRunning
	s.runs[run.IdempotencyKey] = run
	return nil
}

func (s *MemoryLifecycleStore) CompleteRun(ctx context.Context, completion LifecycleRunCompletion) error {
	_ = contextOrBackground(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[completion.IdempotencyKey]
	run.Status = completion.Status
	run.Error = completion.Error
	run.finished = completion.FinishedAt
	s.runs[completion.IdempotencyKey] = run
	return nil
}

func (s *MemoryLifecycleStore) RecordStep(ctx context.Context, step LifecycleStepRecord) error {
	_ = contextOrBackground(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, item := range s.steps {
		if item.AttemptKey != step.AttemptKey {
			continue
		}
		if !step.Started.IsZero() {
			item.Started = step.Started
		}
		if !step.Finished.IsZero() {
			item.Finished = step.Finished
		}
		item.Status = step.Status
		item.Stdout = step.Stdout
		item.Stderr = step.Stderr
		item.Error = step.Error
		s.steps[index] = item
		return nil
	}
	s.steps = append(s.steps, step)
	return nil
}

func (s *MemoryLifecycleStore) UpsertState(ctx context.Context, state LifecycleStateRecord) error {
	_ = contextOrBackground(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state.ModuleID] = state
	return nil
}
