package queue

import (
	"context"
	"strings"
	"sync"
	"time"
)

type MemoryQueueOutboxStore struct {
	mu     sync.Mutex
	events []*QueueOutboxEvent
}

func NewMemoryQueueOutboxStore(events []*QueueOutboxEvent) *MemoryQueueOutboxStore {
	return &MemoryQueueOutboxStore{events: events}
}

func (s *MemoryQueueOutboxStore) ClaimDue(ctx context.Context, owner string, limit int) ([]QueueOutboxEvent, error) {
	_ = contextOrBackground(ctx)
	if limit < 1 {
		limit = 10
	}
	now := time.Now()
	out := make([]QueueOutboxEvent, 0, limit)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range s.events {
		if len(out) >= limit {
			break
		}
		if event == nil {
			continue
		}
		isPending := event.Status == "" || event.Status == QueueOutboxStatusPending
		isExpiredProcessing := event.Status == QueueOutboxStatusProcessing && isExpired(event.LockedUntil, now)
		if !isPending && !isExpiredProcessing {
			continue
		}
		if !event.AvailableAt.IsZero() && event.AvailableAt.After(now) {
			continue
		}
		event.Status = QueueOutboxStatusProcessing
		event.LockOwner = owner
		event.ClaimToken = randomRunToken()
		lockUntil := now.Add(5 * time.Minute)
		event.LockedUntil = &lockUntil
		out = append(out, *event)
	}
	return out, nil
}

func (s *MemoryQueueOutboxStore) MarkSent(ctx context.Context, id uint64, claimToken string) error {
	_ = contextOrBackground(ctx)
	claimToken = strings.TrimSpace(claimToken)
	if claimToken == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if event := s.findClaim(id, claimToken); event != nil {
		event.Status = QueueOutboxStatusSent
		event.LastError = ""
		event.LockedUntil = nil
		event.LockOwner = ""
		event.ClaimToken = ""
	}
	return nil
}

func (s *MemoryQueueOutboxStore) MarkFailed(ctx context.Context, id uint64, claimToken string, err error) error {
	_ = contextOrBackground(ctx)
	claimToken = strings.TrimSpace(claimToken)
	if claimToken == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if event := s.findClaim(id, claimToken); event != nil {
		event.Status = QueueOutboxStatusFailed
		event.LastError = err.Error()
		event.Attempts++
		event.LockedUntil = nil
		event.LockOwner = ""
		event.ClaimToken = ""
	}
	return nil
}

func (s *MemoryQueueOutboxStore) findClaim(id uint64, claimToken string) *QueueOutboxEvent {
	for _, event := range s.events {
		if event != nil && event.ID == id && event.Status == QueueOutboxStatusProcessing && event.ClaimToken == claimToken {
			return event
		}
	}
	return nil
}
