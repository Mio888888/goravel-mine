package services

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const schedulerHeartbeatTTL = 90 * time.Second

type SchedulerNodeMetric struct {
	NodeIP             string    `json:"node_ip"`
	Alive              bool      `json:"alive"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	LastSeenSecondsAgo int64     `json:"last_seen_seconds_ago"`
}

type SchedulerHeartbeatMetrics struct {
	SchedulerNodes []SchedulerNodeMetric
}

var schedulerHeartbeatStore = newSchedulerHeartbeatRecorder()

type schedulerHeartbeatRecorder struct {
	mu    sync.RWMutex
	nodes map[string]time.Time
}

func newSchedulerHeartbeatRecorder() *schedulerHeartbeatRecorder {
	return &schedulerHeartbeatRecorder{nodes: make(map[string]time.Time)}
}

func RecordSchedulerHeartbeat(nodeIP string, at time.Time) {
	nodeIP = strings.TrimSpace(nodeIP)
	if nodeIP == "" {
		nodeIP = "unknown"
	}
	schedulerHeartbeatStore.Record(nodeIP, at)
}

func SchedulerHeartbeatSnapshot(now time.Time) SchedulerHeartbeatMetrics {
	return schedulerHeartbeatStore.Snapshot(now)
}

func ResetSchedulerHeartbeatForTest() {
	schedulerHeartbeatStore = newSchedulerHeartbeatRecorder()
}

func (r *schedulerHeartbeatRecorder) Record(nodeIP string, at time.Time) {
	r.mu.Lock()
	r.nodes[nodeIP] = at
	r.mu.Unlock()
}

func (r *schedulerHeartbeatRecorder) Snapshot(now time.Time) SchedulerHeartbeatMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]SchedulerNodeMetric, 0, len(r.nodes))
	for nodeIP, lastSeen := range r.nodes {
		age := now.Sub(lastSeen)
		if age < 0 {
			age = 0
		}
		nodes = append(nodes, SchedulerNodeMetric{
			NodeIP:             nodeIP,
			Alive:              age <= schedulerHeartbeatTTL,
			LastSeenAt:         lastSeen,
			LastSeenSecondsAgo: int64(age.Seconds()),
		})
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeIP < nodes[j].NodeIP
	})

	return SchedulerHeartbeatMetrics{SchedulerNodes: nodes}
}
