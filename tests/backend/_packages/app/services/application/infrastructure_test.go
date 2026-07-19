package application

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

// Source: scheduled_task_heartbeat_test.go
func TestSchedulerHeartbeatSnapshotTracksFreshNodes(t *testing.T) {
	ResetSchedulerHeartbeatForTest()
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	RecordSchedulerHeartbeat("10.0.0.8", now)

	snapshot := SchedulerHeartbeatSnapshot(now.Add(10 * time.Second))
	require.Len(t, snapshot.SchedulerNodes, 1)
	require.Equal(t, "10.0.0.8", snapshot.SchedulerNodes[0].NodeIP)
	require.True(t, snapshot.SchedulerNodes[0].Alive)
	require.Equal(t, int64(10), snapshot.SchedulerNodes[0].LastSeenSecondsAgo)
}

func TestSchedulerHeartbeatSnapshotMarksStaleNodesDead(t *testing.T) {
	ResetSchedulerHeartbeatForTest()
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	RecordSchedulerHeartbeat("10.0.0.9", now.Add(-2*time.Minute))

	snapshot := SchedulerHeartbeatSnapshot(now)
	require.Len(t, snapshot.SchedulerNodes, 1)
	require.False(t, snapshot.SchedulerNodes[0].Alive)
}

func TestPrometheusMetricsTextIncludesSchedulerHeartbeat(t *testing.T) {
	snapshot := MetricsSnapshot{
		SchedulerNodes: []SchedulerNodeMetric{{
			NodeIP:             "10.0.0.8",
			Alive:              true,
			LastSeenSecondsAgo: 3,
		}},
	}

	text := PrometheusMetricsText(snapshot)

	require.True(t, strings.Contains(text, `goravel_scheduler_node_alive{node_ip="10.0.0.8"} 1`))
	require.True(t, strings.Contains(text, `goravel_scheduler_node_last_seen_seconds{node_ip="10.0.0.8"} 3`))
}
