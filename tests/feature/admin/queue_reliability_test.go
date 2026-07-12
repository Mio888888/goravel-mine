package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/services"
	"goravel/tests"
)

type QueueReliabilityTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestQueueReliabilityTestSuite(t *testing.T) {
	suite.Run(t, new(QueueReliabilityTestSuite))
}

func (s *QueueReliabilityTestSuite) SetupTest() {
	s.RefreshDatabase()
}

func (s *QueueReliabilityTestSuite) TestReliabilityTablesExistAfterRefreshDatabase() {
	for _, table := range []string{"queue_outbox", "queue_idempotency", "queue_task_lock"} {
		require.True(s.T(), facades.Schema().HasTable(table), table)
	}
}

func (s *QueueReliabilityTestSuite) TestOutboxAndDBIdempotencyPersistState() {
	ctx := context.Background()
	err := services.EnqueueQueueOutboxEvent(ctx, services.QueueOutboxEvent{
		Topic:   "mail.welcome",
		Payload: `{"user_id":42}`,
	})
	require.NoError(s.T(), err)

	count, err := facades.Orm().Query().Table("queue_outbox").Where("topic", "mail.welcome").Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)

	store := services.NewDBQueueIdempotencyStore("")
	first, err := store.Once(ctx, "tenant:mail:42", func(context.Context) (services.QueueIdempotencyResult, error) {
		return services.QueueIdempotencyResult{Status: services.QueueIdempotencyStatusSuccess, Result: "sent"}, nil
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), "sent", first.Result)

	second, err := store.Once(ctx, "tenant:mail:42", func(context.Context) (services.QueueIdempotencyResult, error) {
		return services.QueueIdempotencyResult{Status: services.QueueIdempotencyStatusSuccess, Result: "duplicate"}, nil
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), "sent", second.Result)
}

func (s *QueueReliabilityTestSuite) TestDBIdempotencyRunningBlocksDuplicateAndFailedCanRetry() {
	ctx := context.Background()
	store := services.NewDBQueueIdempotencyStore("")
	stale := time.Now().Add(-time.Minute)
	err := facades.Orm().Query().Table("queue_idempotency").Create(map[string]any{
		"key":          "tenant:running:42",
		"status":       services.QueueIdempotencyStatusRunning,
		"locked_until": time.Now().Add(time.Minute),
		"created_at":   time.Now(),
		"updated_at":   time.Now(),
	})
	require.NoError(s.T(), err)
	err = facades.Orm().Query().Table("queue_idempotency").Create(map[string]any{
		"key":          "tenant:stale:42",
		"status":       services.QueueIdempotencyStatusRunning,
		"locked_until": stale,
		"created_at":   stale,
		"updated_at":   stale,
	})
	require.NoError(s.T(), err)

	calls := 0
	running, err := store.Once(ctx, "tenant:running:42", func(context.Context) (services.QueueIdempotencyResult, error) {
		calls++
		return services.QueueIdempotencyResult{Status: services.QueueIdempotencyStatusSuccess}, nil
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), services.QueueIdempotencyStatusRunning, running.Status)
	require.Zero(s.T(), calls)

	staleRecovered, err := store.Once(ctx, "tenant:stale:42", func(context.Context) (services.QueueIdempotencyResult, error) {
		calls++
		return services.QueueIdempotencyResult{Status: services.QueueIdempotencyStatusSuccess, Result: "recovered"}, nil
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), "recovered", staleRecovered.Result)

	_, err = store.Once(ctx, "tenant:retry:42", func(context.Context) (services.QueueIdempotencyResult, error) {
		calls++
		return services.QueueIdempotencyResult{}, errors.New("temporary")
	})
	require.Error(s.T(), err)
	success, err := store.Once(ctx, "tenant:retry:42", func(context.Context) (services.QueueIdempotencyResult, error) {
		calls++
		return services.QueueIdempotencyResult{Status: services.QueueIdempotencyStatusSuccess, Result: "ok"}, nil
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), "ok", success.Result)
	require.Equal(s.T(), 3, calls)
}

func (s *QueueReliabilityTestSuite) TestDBIdempotencyStaleOwnerCannotOverwriteNewRun() {
	ctx := context.Background()
	store := services.NewDBQueueIdempotencyStore("")
	key := "tenant:stale-overwrite:42"
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		_, err := store.Once(ctx, key, func(context.Context) (services.QueueIdempotencyResult, error) {
			close(started)
			<-release
			return services.QueueIdempotencyResult{Status: services.QueueIdempotencyStatusSuccess, Result: "stale"}, nil
		})
		done <- err
	}()
	<-started

	expired := time.Now().Add(-time.Minute)
	_, err := facades.Orm().Query().Table("queue_idempotency").
		Where("key", key).
		Update(map[string]any{"locked_until": expired})
	require.NoError(s.T(), err)

	result, err := store.Once(ctx, key, func(context.Context) (services.QueueIdempotencyResult, error) {
		return services.QueueIdempotencyResult{Status: services.QueueIdempotencyStatusSuccess, Result: "fresh"}, nil
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), "fresh", result.Result)

	close(release)
	require.NoError(s.T(), <-done)

	var stored string
	err = facades.Orm().Query().Table("queue_idempotency").
		Where("key", key).
		Pluck("result", &stored)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "fresh", stored)
}

func (s *QueueReliabilityTestSuite) TestDBTaskLockAllowsSingleHolder() {
	store := services.NewDBQueueTaskLockStore("")
	ctx := context.Background()

	first, err := store.Acquire(ctx, "billing:invoice:9", "worker-a", time.Minute)
	require.NoError(s.T(), err)
	require.True(s.T(), first.Acquired)

	second, err := store.Acquire(ctx, "billing:invoice:9", "worker-b", time.Minute)
	require.NoError(s.T(), err)
	require.False(s.T(), second.Acquired)
	require.Equal(s.T(), "worker-a", second.Owner)

	require.NoError(s.T(), store.Release(ctx, first))
	third, err := store.Acquire(ctx, "billing:invoice:9", "worker-b", time.Minute)
	require.NoError(s.T(), err)
	require.True(s.T(), third.Acquired)
}

func (s *QueueReliabilityTestSuite) TestDBOutboxStoreClaimsAndMarksEvents() {
	ctx := context.Background()
	err := services.EnqueueQueueOutboxEvent(ctx, services.QueueOutboxEvent{
		Topic:   "mail.claim",
		Payload: `{"user_id":42}`,
	})
	require.NoError(s.T(), err)

	store := services.NewDBQueueOutboxStore("")
	events, err := store.ClaimDue(ctx, "worker-a", 5)
	require.NoError(s.T(), err)
	require.Len(s.T(), events, 1)
	require.Equal(s.T(), "mail.claim", events[0].Topic)

	err = store.MarkSent(ctx, events[0].ID, events[0].ClaimToken)
	require.NoError(s.T(), err)

	var status string
	err = facades.Orm().Query().Table("queue_outbox").Where("id", events[0].ID).Pluck("status", &status)
	require.NoError(s.T(), err)
	require.Equal(s.T(), services.QueueOutboxStatusSent, status)
}

func (s *QueueReliabilityTestSuite) TestDBOutboxStoreDoesNotClaimTerminalFailedEvents() {
	now := time.Now().Add(-time.Minute)
	err := facades.Orm().Query().Table("queue_outbox").Create(map[string]any{
		"topic":        "mail.dead",
		"connection":   "redis",
		"queue":        "default",
		"payload":      `{}`,
		"status":       services.QueueOutboxStatusFailed,
		"attempts":     4,
		"available_at": now,
		"created_at":   now,
		"updated_at":   now,
	})
	require.NoError(s.T(), err)

	events, err := services.NewDBQueueOutboxStore("").ClaimDue(context.Background(), "worker-a", 5)

	require.NoError(s.T(), err)
	require.Empty(s.T(), events)
}

func (s *QueueReliabilityTestSuite) TestDBOutboxStoreReclaimsExpiredProcessingEvents() {
	now := time.Now()
	expired := now.Add(-time.Minute)
	err := facades.Orm().Query().Table("queue_outbox").Create(map[string]any{
		"topic":        "mail.reclaim",
		"connection":   "redis",
		"queue":        "default",
		"payload":      `{}`,
		"status":       services.QueueOutboxStatusProcessing,
		"attempts":     1,
		"available_at": expired,
		"locked_until": expired,
		"lock_owner":   "dead-worker",
		"created_at":   expired,
		"updated_at":   expired,
	})
	require.NoError(s.T(), err)

	events, err := services.NewDBQueueOutboxStore("").ClaimDue(context.Background(), "worker-b", 5)

	require.NoError(s.T(), err)
	require.Len(s.T(), events, 1)
	require.Equal(s.T(), "mail.reclaim", events[0].Topic)
	require.Equal(s.T(), services.QueueOutboxStatusProcessing, events[0].Status)
	require.Equal(s.T(), "worker-b", events[0].LockOwner)
}

func (s *QueueReliabilityTestSuite) TestDBOutboxStoreStaleOwnerCannotMarkReclaimedEvent() {
	ctx := context.Background()
	store := services.NewDBQueueOutboxStore("")
	err := services.EnqueueQueueOutboxEvent(ctx, services.QueueOutboxEvent{
		Topic:   "mail.claim-token",
		Payload: `{}`,
	})
	require.NoError(s.T(), err)

	events, err := store.ClaimDue(ctx, "worker-a", 1)
	require.NoError(s.T(), err)
	require.Len(s.T(), events, 1)
	firstClaim := events[0]

	expired := time.Now().Add(-time.Minute)
	_, err = facades.Orm().Query().Table("queue_outbox").
		Where("id", firstClaim.ID).
		Update(map[string]any{"locked_until": expired})
	require.NoError(s.T(), err)

	events, err = store.ClaimDue(ctx, "worker-b", 1)
	require.NoError(s.T(), err)
	require.Len(s.T(), events, 1)
	secondClaim := events[0]
	require.NotEqual(s.T(), firstClaim.ClaimToken, secondClaim.ClaimToken)

	err = store.MarkSent(ctx, firstClaim.ID, firstClaim.ClaimToken)
	require.NoError(s.T(), err)

	var row services.QueueOutboxEvent
	err = facades.Orm().Query().Table("queue_outbox").Where("id", firstClaim.ID).First(&row)
	require.NoError(s.T(), err)
	require.Equal(s.T(), services.QueueOutboxStatusProcessing, row.Status)
	require.Equal(s.T(), "worker-b", row.LockOwner)
	require.Equal(s.T(), secondClaim.ClaimToken, row.ClaimToken)
}

func (s *QueueReliabilityTestSuite) TestQueueBacklogMetricsCountsFailedJobsAndOutboxStatuses() {
	now := time.Now()
	err := facades.Orm().Query().Table("failed_jobs").Create(map[string]any{
		"uuid":       "failed-job-1",
		"connection": "redis",
		"queue":      "default",
		"payload":    `{"signature":"mail.send"}`,
		"exception":  "boom",
		"failed_at":  now,
	})
	require.NoError(s.T(), err)

	for _, status := range []string{
		services.QueueOutboxStatusPending,
		services.QueueOutboxStatusProcessing,
		services.QueueOutboxStatusFailed,
		services.QueueOutboxStatusSent,
	} {
		err = facades.Orm().Query().Table("queue_outbox").Create(map[string]any{
			"topic":        "mail.metrics." + status,
			"connection":   "redis",
			"queue":        "default",
			"payload":      `{}`,
			"status":       status,
			"attempts":     0,
			"available_at": now,
			"created_at":   now,
			"updated_at":   now,
		})
		require.NoError(s.T(), err)
	}

	metric := services.QueueBacklogMetrics(context.Background())

	require.Equal(s.T(), int64(1), metric.FailedJobs)
	require.Equal(s.T(), int64(1), metric.OutboxPending)
	require.Equal(s.T(), int64(1), metric.OutboxProcessing)
	require.Equal(s.T(), int64(1), metric.OutboxFailed)
	require.Equal(s.T(), int64(0), metric.OutboxSent)
}

func (s *QueueReliabilityTestSuite) TestQueueBacklogMetricsUsesConfiguredFailedJobTable() {
	_, _ = facades.Orm().Query().Exec("DROP TABLE IF EXISTS custom_failed_jobs")
	_, err := facades.Orm().Query().Exec(`
		CREATE TABLE custom_failed_jobs (
			id bigserial PRIMARY KEY,
			uuid varchar(255),
			connection text,
			queue text,
			payload text,
			exception text,
			failed_at timestamp
		)
	`)
	require.NoError(s.T(), err)
	defer func() {
		_, _ = facades.Orm().Query().Exec("DROP TABLE IF EXISTS custom_failed_jobs")
	}()
	err = facades.Orm().Query().Table("custom_failed_jobs").Create(map[string]any{
		"uuid":       "failed-job-custom",
		"connection": "redis",
		"queue":      "default",
		"payload":    `{}`,
		"exception":  "boom",
		"failed_at":  time.Now(),
	})
	require.NoError(s.T(), err)
	facades.Config().Add("queue.failed.table", "custom_failed_jobs")
	defer facades.Config().Add("queue.failed.table", "failed_jobs")

	metric := services.QueueBacklogMetrics(context.Background())

	require.Equal(s.T(), int64(1), metric.FailedJobs)
}
