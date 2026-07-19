package unit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
)

func TestQueueRetryPolicyUsesExponentialBackoffAndStopsAtMaxAttempts(t *testing.T) {
	policy := services.QueueRetryPolicy{MaxAttempts: 4, InitialDelay: time.Second, MaxDelay: 5 * time.Second}

	retryable, delay := policy.ShouldRetry(errors.New("boom"), 1)
	require.True(t, retryable)
	require.Equal(t, time.Second, delay)

	retryable, delay = policy.ShouldRetry(errors.New("boom"), 3)
	require.True(t, retryable)
	require.Equal(t, 4*time.Second, delay)

	retryable, delay = policy.ShouldRetry(errors.New("boom"), 4)
	require.False(t, retryable)
	require.Zero(t, delay)
}

func TestQueueIdempotencyStoreReturnsCachedResultForDuplicateKey(t *testing.T) {
	store := services.NewMemoryQueueIdempotencyStore()
	ctx := context.Background()
	calls := 0

	first, err := store.Once(ctx, "tenant:job:42", func(context.Context) (services.QueueIdempotencyResult, error) {
		calls++
		return services.QueueIdempotencyResult{Status: "success", Result: "first"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, "first", first.Result)

	second, err := store.Once(ctx, "tenant:job:42", func(context.Context) (services.QueueIdempotencyResult, error) {
		calls++
		return services.QueueIdempotencyResult{Status: "success", Result: "second"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, "first", second.Result)
	require.Equal(t, 1, calls)
}

func TestQueueIdempotencyStoreDoesNotRunConcurrentDuplicate(t *testing.T) {
	store := services.NewMemoryQueueIdempotencyStore()
	ctx := context.Background()
	started := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	calls := 0

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = store.Once(ctx, "tenant:job:99", func(context.Context) (services.QueueIdempotencyResult, error) {
			calls++
			close(started)
			<-release
			return services.QueueIdempotencyResult{Status: services.QueueIdempotencyStatusSuccess, Result: "done"}, nil
		})
	}()
	<-started

	duplicate, err := store.Once(ctx, "tenant:job:99", func(context.Context) (services.QueueIdempotencyResult, error) {
		calls++
		return services.QueueIdempotencyResult{Status: services.QueueIdempotencyStatusSuccess, Result: "duplicate"}, nil
	})
	close(release)
	wg.Wait()

	require.NoError(t, err)
	require.Equal(t, services.QueueIdempotencyStatusRunning, duplicate.Status)
	require.Equal(t, 1, calls)
}

func TestQueueOutboxBuildsSerializableQueueArgs(t *testing.T) {
	event := services.QueueOutboxEvent{
		ID:         7,
		Topic:      "mail.welcome",
		Connection: "redis",
		Queue:      "mail",
		Payload:    `{"user_id":42}`,
		Attempts:   2,
	}

	args := event.QueueArgs()

	require.Len(t, args, 5)
	require.Equal(t, "uint64", args[0].Type)
	require.Equal(t, uint64(7), args[0].Value)
	require.Equal(t, "string", args[1].Type)
	require.Equal(t, "mail.welcome", args[1].Value)
	require.Equal(t, "string", args[4].Type)
	require.Equal(t, `{"user_id":42}`, args[4].Value)
}

func TestQueueOutboxDispatchJobDecodesArgsAndCallsHandler(t *testing.T) {
	job := &services.QueueOutboxDispatchJob{}
	var handled services.QueueOutboxEvent
	services.RegisterQueueOutboxHandler("mail.welcome", func(ctx context.Context, event services.QueueOutboxEvent) error {
		handled = event
		return nil
	})
	defer services.UnregisterQueueOutboxHandler("mail.welcome")

	err := job.Handle(uint64(7), "mail.welcome", "redis", "mail", `{"user_id":42}`)

	require.NoError(t, err)
	require.Equal(t, uint64(7), handled.ID)
	require.Equal(t, "mail.welcome", handled.Topic)
	require.Equal(t, "redis", handled.Connection)
	require.Equal(t, "mail", handled.Queue)
	require.Equal(t, `{"user_id":42}`, handled.Payload)
}

func TestQueueOutboxDispatcherMarksSentOrFailed(t *testing.T) {
	events := []*services.QueueOutboxEvent{
		{ID: 1, Topic: "mail.ok", Connection: "redis", Queue: "mail", Payload: `{}`},
		{ID: 2, Topic: "mail.fail", Connection: "redis", Queue: "mail", Payload: `{}`},
	}
	store := services.NewMemoryQueueOutboxStore(events)
	dispatcher := services.QueueOutboxDispatcher{
		Store: store,
		Dispatch: func(ctx context.Context, event services.QueueOutboxEvent) error {
			if event.Topic == "mail.fail" {
				return errors.New("dispatch failed")
			}
			return nil
		},
	}

	result, err := dispatcher.DispatchDue(context.Background(), "worker-a", 10)

	require.NoError(t, err)
	require.Equal(t, 1, result.Sent)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, services.QueueOutboxStatusSent, events[0].Status)
	require.Equal(t, services.QueueOutboxStatusFailed, events[1].Status)
	require.Equal(t, "dispatch failed", events[1].LastError)
}

func TestMemoryQueueOutboxStoreDoesNotClaimTerminalFailedEvents(t *testing.T) {
	events := []*services.QueueOutboxEvent{
		{ID: 1, Topic: "mail.dead", Status: services.QueueOutboxStatusFailed, Attempts: 4},
		{ID: 2, Topic: "mail.pending", Status: services.QueueOutboxStatusPending},
	}
	store := services.NewMemoryQueueOutboxStore(events)

	claimed, err := store.ClaimDue(context.Background(), "worker-a", 10)

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, uint64(2), claimed[0].ID)
	require.Equal(t, services.QueueOutboxStatusFailed, events[0].Status)
}

func TestMemoryQueueOutboxStoreReclaimsExpiredProcessingEvents(t *testing.T) {
	expired := time.Now().Add(-time.Minute)
	events := []*services.QueueOutboxEvent{
		{
			ID:          1,
			Topic:       "mail.reclaim",
			Status:      services.QueueOutboxStatusProcessing,
			LockedUntil: &expired,
			LockOwner:   "dead-worker",
		},
	}
	store := services.NewMemoryQueueOutboxStore(events)

	claimed, err := store.ClaimDue(context.Background(), "worker-b", 10)

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, uint64(1), claimed[0].ID)
	require.Equal(t, "worker-b", claimed[0].LockOwner)
	require.Equal(t, services.QueueOutboxStatusProcessing, events[0].Status)
}

func TestQueueOutboxRunnerShouldRunOnlyWhenEnabled(t *testing.T) {
	require.False(t, services.ShouldRunQueueOutboxRunner(false, true, "redis"))
	require.False(t, services.ShouldRunQueueOutboxRunner(true, false, "redis"))
	require.False(t, services.ShouldRunQueueOutboxRunner(true, true, ""))
	require.False(t, services.ShouldRunQueueOutboxRunner(true, true, "sync"))
	require.True(t, services.ShouldRunQueueOutboxRunner(true, true, "redis"))
}

func TestQueueOutboxRunnerLogsDispatchErrors(t *testing.T) {
	var logged []map[string]any
	dispatcher := services.QueueOutboxDispatcher{
		Store: failingQueueOutboxStore{err: errors.New("database unavailable")},
	}

	services.RunQueueOutboxDispatchOnceForTest(
		context.Background(),
		dispatcher,
		"worker-a",
		10,
		func(message string, fields map[string]any) {
			logged = append(logged, fields)
		},
	)

	require.Len(t, logged, 1)
	require.Equal(t, "database unavailable", logged[0]["error"])
	require.Equal(t, "worker-a", logged[0]["owner"])
}

type failingQueueOutboxStore struct {
	err error
}

func (s failingQueueOutboxStore) ClaimDue(context.Context, string, int) ([]services.QueueOutboxEvent, error) {
	return nil, s.err
}

func (s failingQueueOutboxStore) MarkSent(context.Context, uint64, string) error {
	return nil
}

func (s failingQueueOutboxStore) MarkFailed(context.Context, uint64, string, error) error {
	return nil
}
