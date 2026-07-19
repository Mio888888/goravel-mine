package queue

import (
	"errors"
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

var (
	queueOutboxRetryPolicy = QueueRetryPolicy{
		MaxAttempts:  4,
		InitialDelay: time.Second,
		MaxDelay:     30 * time.Second,
	}
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
	var classified interface {
		Retryable() bool
	}
	if errors.As(err, &classified) && !classified.Retryable() {
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
