package queue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQueueMetricsUsesOldestPendingEventAge(t *testing.T) {
	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	metrics := QueueClassMetricsFromOutbox([]QueueOutboxEvent{
		{Queue: "critical", Status: QueueOutboxStatusPending, CreatedAt: now.Add(-90 * time.Second)},
		{Queue: "critical", Status: QueueOutboxStatusPending, CreatedAt: now.Add(-20 * time.Second)},
		{Queue: "default", Status: QueueOutboxStatusProcessing, CreatedAt: now.Add(-10 * time.Minute)},
		{Queue: "default", Status: QueueOutboxStatusSent, CreatedAt: now.Add(-30 * time.Second), UpdatedAt: now.Add(-20 * time.Second)},
		{Queue: "bulk", Status: QueueOutboxStatusSent, CreatedAt: now.Add(-20 * time.Minute)},
	}, now)
	byClass := queueClassMetricsByName(metrics)

	require.Equal(t, int64(2), byClass["critical"].Pending)
	require.Equal(t, 90*time.Second, byClass["critical"].OldestAge)
	require.Equal(t, 2.0/(5*60), byClass["critical"].ArrivalRate)
	require.Equal(t, 1.0/(5*60), byClass["default"].ArrivalRate)
	require.Equal(t, 1.0/(5*60), byClass["default"].CompletionRate)
	require.Zero(t, byClass["bulk"].Pending)
	require.Zero(t, byClass["bulk"].OldestAge)
}

func queueClassMetricsByName(metrics []QueueClassMetric) map[string]QueueClassMetric {
	byClass := make(map[string]QueueClassMetric, len(metrics))
	for _, metric := range metrics {
		byClass[metric.Class] = metric
	}
	return byClass
}
