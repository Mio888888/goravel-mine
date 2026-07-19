package observability

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var slowSQLPattern = regexp.MustCompile(`\[(\d+(?:\.\d+)?)ms\]\s+\[rows:([^\]]+)\]\s+\[SLOW\]\s+(.+)`)

type SlowSQL struct {
	SQL          string    `json:"sql"`
	Rows         string    `json:"rows"`
	DurationMS   float64   `json:"duration_ms"`
	RequestID    string    `json:"request_id"`
	TraceID      string    `json:"trace_id"`
	RecordedAt   time.Time `json:"recorded_at"`
	RetentionMax int       `json:"retention_max"`
}

type SlowSQLSnapshot struct {
	Items []SlowSQL
	Total uint64
}

type slowSQLRecorder struct {
	mu         sync.RWMutex
	maxEntries int
	items      []SlowSQL
	total      uint64
}

var defaultSlowSQLRecorder = &slowSQLRecorder{maxEntries: 100}

func ConfigureSlowSQLRecorder(maxEntries int) {
	if maxEntries < 1 {
		maxEntries = 100
	}
	defaultSlowSQLRecorder.mu.Lock()
	defer defaultSlowSQLRecorder.mu.Unlock()
	defaultSlowSQLRecorder.maxEntries = maxEntries
	if len(defaultSlowSQLRecorder.items) > maxEntries {
		defaultSlowSQLRecorder.items = append([]SlowSQL(nil), defaultSlowSQLRecorder.items[len(defaultSlowSQLRecorder.items)-maxEntries:]...)
	}
}

func RecordSlowSQLFromMessage(message, requestID, traceID string) {
	RecordSlowSQLFromContext(context.Background(), message, requestID, traceID)
}

func RecordSlowSQLFromContext(ctx context.Context, message, requestID, traceID string) {
	if ctx == nil {
		ctx = context.Background()
	}
	match := slowSQLPattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(match) != 4 {
		return
	}
	if requestID == "" {
		requestID = RequestID(ctx)
	}
	if traceID == "" {
		traceID = TraceID(ctx)
	}
	duration, _ := strconv.ParseFloat(match[1], 64)
	defaultSlowSQLRecorder.record(SlowSQL{
		SQL:        match[3],
		Rows:       match[2],
		DurationMS: duration,
		RequestID:  requestID,
		TraceID:    traceID,
		RecordedAt: time.Now(),
	})
}

func SlowSQLMetrics() []SlowSQL {
	return defaultSlowSQLRecorder.snapshot()
}

func SlowSQLMetricsSnapshot() SlowSQLSnapshot {
	return defaultSlowSQLRecorder.metricsSnapshot()
}

func ResetSlowSQLMetricsForTest() {
	defaultSlowSQLRecorder = &slowSQLRecorder{maxEntries: 100}
}

func (r *slowSQLRecorder) record(item SlowSQL) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item.RetentionMax = r.maxEntries
	r.items = append(r.items, item)
	r.total++
	if len(r.items) > r.maxEntries {
		r.items = append([]SlowSQL(nil), r.items[len(r.items)-r.maxEntries:]...)
	}
}

func (r *slowSQLRecorder) snapshot() []SlowSQL {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]SlowSQL, len(r.items))
	copy(items, r.items)
	sort.Slice(items, func(i, j int) bool {
		return items[i].RecordedAt.After(items[j].RecordedAt)
	})
	return items
}

func (r *slowSQLRecorder) metricsSnapshot() SlowSQLSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]SlowSQL, len(r.items))
	copy(items, r.items)
	sort.Slice(items, func(i, j int) bool {
		return items[i].RecordedAt.After(items[j].RecordedAt)
	})
	return SlowSQLSnapshot{
		Items: items,
		Total: r.total,
	}
}
