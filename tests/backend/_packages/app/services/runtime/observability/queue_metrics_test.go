package observability

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	queueservice "goravel/app/services/runtime/queue"
)

func TestQueueMetricsPrometheusTextIncludesQueueClassLabels(t *testing.T) {
	text := PrometheusMetricsText(MetricsSnapshot{
		Queue: queueservice.QueueBacklogMetric{
			Classes: []queueservice.QueueClassMetric{
				{Class: "critical", Pending: 2, OldestAge: 90 * time.Second, ArrivalRate: 1.0 / 60, CompletionRate: 1.0 / 60},
				{Class: "default", Pending: 4, OldestAge: 5 * time.Minute, ArrivalRate: 6.0 / 60, CompletionRate: 5.0 / 60},
				{Class: "bulk"},
			},
		},
	})

	require.Contains(t, text, "# TYPE goravel_queue_pending_jobs gauge")
	require.Contains(t, text, `goravel_queue_pending_jobs{queue_class="critical"} 2`)
	require.Contains(t, text, `goravel_queue_oldest_backlog_age_seconds{queue_class="default"} 300`)
	require.Contains(t, text, `goravel_queue_arrival_rate{queue_class="bulk"} 0.000000`)
	require.Contains(t, text, `goravel_queue_completion_rate{queue_class="critical"} 0.016667`)
	require.Equal(t, 4, strings.Count(text, `queue_class="critical"`))
}
