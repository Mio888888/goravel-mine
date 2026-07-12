package services

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type HTTPObservation struct {
	Method     string
	Route      string
	Path       string
	Status     int
	Duration   time.Duration
	RequestID  string
	TraceID    string
	IP         string
	RecordedAt time.Time
}

type SlowRequest struct {
	Method       string    `json:"method"`
	Route        string    `json:"route"`
	Path         string    `json:"path"`
	Status       int       `json:"status"`
	DurationMS   int64     `json:"duration_ms"`
	RequestID    string    `json:"request_id"`
	TraceID      string    `json:"trace_id"`
	IP           string    `json:"ip"`
	RecordedAt   time.Time `json:"recorded_at"`
	ThresholdMS  int64     `json:"threshold_ms"`
	RetentionMax int       `json:"retention_max"`
}

type MetricsSnapshot struct {
	TotalRequests     uint64
	Inflight          int64
	ByRoute           []RouteMetric
	DurationBuckets   []RouteDurationBucket
	SlowRequests      []SlowRequest
	SlowRequestsTotal uint64
	SlowSQLRetained   int
	SlowSQLObserved   uint64
	GoRuntime         GoRuntimeMetric
	DBPool            DBPoolMetric
	SchedulerNodes    []SchedulerNodeMetric
	Queue             QueueBacklogMetric
	TenantGovernance  TenantGovernanceObservability
	CasbinCache       CasbinEnforcerCacheMetrics
	TenantConnections TenantConnectionCapacityMetrics
	MigrationLocks    MigrationLockMetrics
	UptimeSeconds     int64
}

type RouteMetric struct {
	Method        string
	Route         string
	Status        int
	Count         uint64
	DurationSumMS float64
}

type RouteDurationBucket struct {
	Method string
	Route  string
	LeMS   float64
	Count  uint64
}

type GoRuntimeMetric struct {
	Goroutines    int
	HeapAlloc     uint64
	HeapInuse     uint64
	HeapObjects   uint64
	NextGC        uint64
	LastGCPauseNS uint64
	TotalPauseNS  uint64
	NumGC         uint32
}

type DBPoolMetric struct {
	Connection        string
	OpenConnections   int
	InUse             int
	Idle              int
	WaitCount         int64
	WaitDurationMS    float64
	MaxOpen           int
	MaxIdleClosed     int64
	MaxIdleTimeClosed int64
	MaxLifetimeClosed int64
}

type ObservabilityRecorder struct {
	mu              sync.RWMutex
	startedAt       time.Time
	totalRequests   uint64
	inflight        int64
	routeMetrics    map[string]RouteMetric
	durationBuckets map[string]RouteDurationBucket
	slowRequests    []SlowRequest
	slowTotal       uint64
	slowThreshold   time.Duration
	slowMaxEntries  int
}

var httpDurationBucketsMS = []float64{50, 100, 250, 500, 1000, 2500, 5000, 10000}

var defaultObservabilityRecorder = NewObservabilityRecorder(time.Second, 100)

func NewObservabilityRecorder(slowThreshold time.Duration, slowMaxEntries int) *ObservabilityRecorder {
	if slowThreshold <= 0 {
		slowThreshold = time.Second
	}
	if slowMaxEntries < 1 {
		slowMaxEntries = 100
	}
	return &ObservabilityRecorder{
		startedAt:       time.Now(),
		routeMetrics:    make(map[string]RouteMetric),
		durationBuckets: make(map[string]RouteDurationBucket),
		slowThreshold:   slowThreshold,
		slowMaxEntries:  slowMaxEntries,
	}
}

func ConfigureObservabilityRecorder(slowThreshold time.Duration, slowMaxEntries int) {
	defaultObservabilityRecorder.Configure(slowThreshold, slowMaxEntries)
}

func RecordHTTPObservation(obs HTTPObservation) {
	defaultObservabilityRecorder.Record(obs)
}

func RecordHTTPObservationStart() {
	defaultObservabilityRecorder.StartRequest()
}

func RecordHTTPObservationFinish() {
	defaultObservabilityRecorder.FinishRequest()
}

func ResetObservabilityMetricsForTest() {
	defaultObservabilityRecorder = NewObservabilityRecorder(time.Second, 100)
}

func (r *ObservabilityRecorder) Configure(slowThreshold time.Duration, slowMaxEntries int) {
	if slowThreshold <= 0 {
		slowThreshold = time.Second
	}
	if slowMaxEntries < 1 {
		slowMaxEntries = 100
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.slowThreshold = slowThreshold
	r.slowMaxEntries = slowMaxEntries
	if len(r.slowRequests) > slowMaxEntries {
		r.slowRequests = append([]SlowRequest(nil), r.slowRequests[len(r.slowRequests)-slowMaxEntries:]...)
	}
}

func (r *ObservabilityRecorder) StartRequest() {
	r.mu.Lock()
	r.inflight++
	r.mu.Unlock()
}

func (r *ObservabilityRecorder) FinishRequest() {
	r.mu.Lock()
	if r.inflight > 0 {
		r.inflight--
	}
	r.mu.Unlock()
}

func (r *ObservabilityRecorder) Record(obs HTTPObservation) {
	route := strings.TrimSpace(obs.Route)
	if route == "" {
		route = obs.Path
	}
	if route == "" {
		route = "unknown"
	}
	key := fmt.Sprintf("%s %s %d", obs.Method, route, obs.Status)

	r.mu.Lock()
	defer r.mu.Unlock()

	metric := r.routeMetrics[key]
	metric.Method = obs.Method
	metric.Route = route
	metric.Status = obs.Status
	metric.Count++
	metric.DurationSumMS += float64(obs.Duration.Microseconds()) / 1000
	r.routeMetrics[key] = metric
	r.totalRequests++
	durationMS := float64(obs.Duration.Microseconds()) / 1000
	for _, le := range httpDurationBucketsMS {
		if durationMS <= le {
			bucketKey := fmt.Sprintf("%s %s %g", obs.Method, route, le)
			bucket := r.durationBuckets[bucketKey]
			bucket.Method = obs.Method
			bucket.Route = route
			bucket.LeMS = le
			bucket.Count++
			r.durationBuckets[bucketKey] = bucket
		}
	}

	if obs.Duration >= r.slowThreshold {
		r.slowTotal++
		r.slowRequests = append(r.slowRequests, SlowRequest{
			Method:       obs.Method,
			Route:        route,
			Path:         obs.Path,
			Status:       obs.Status,
			DurationMS:   obs.Duration.Milliseconds(),
			RequestID:    obs.RequestID,
			TraceID:      obs.TraceID,
			IP:           obs.IP,
			RecordedAt:   obs.RecordedAt,
			ThresholdMS:  r.slowThreshold.Milliseconds(),
			RetentionMax: r.slowMaxEntries,
		})
		if len(r.slowRequests) > r.slowMaxEntries {
			r.slowRequests = append([]SlowRequest(nil), r.slowRequests[len(r.slowRequests)-r.slowMaxEntries:]...)
		}
	}
}

func (r *ObservabilityRecorder) Snapshot() MetricsSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	byRoute := make([]RouteMetric, 0, len(r.routeMetrics))
	for _, metric := range r.routeMetrics {
		byRoute = append(byRoute, metric)
	}
	sort.Slice(byRoute, func(i, j int) bool {
		if byRoute[i].Route == byRoute[j].Route {
			if byRoute[i].Method == byRoute[j].Method {
				return byRoute[i].Status < byRoute[j].Status
			}
			return byRoute[i].Method < byRoute[j].Method
		}
		return byRoute[i].Route < byRoute[j].Route
	})

	buckets := make([]RouteDurationBucket, 0, len(r.durationBuckets))
	for _, bucket := range r.durationBuckets {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Route == buckets[j].Route {
			if buckets[i].Method == buckets[j].Method {
				return buckets[i].LeMS < buckets[j].LeMS
			}
			return buckets[i].Method < buckets[j].Method
		}
		return buckets[i].Route < buckets[j].Route
	})

	slow := make([]SlowRequest, len(r.slowRequests))
	copy(slow, r.slowRequests)
	sort.Slice(slow, func(i, j int) bool {
		return slow[i].RecordedAt.After(slow[j].RecordedAt)
	})

	return MetricsSnapshot{
		TotalRequests:     r.totalRequests,
		Inflight:          r.inflight,
		ByRoute:           byRoute,
		DurationBuckets:   buckets,
		SlowRequests:      slow,
		SlowRequestsTotal: r.slowTotal,
		UptimeSeconds:     int64(time.Since(r.startedAt).Seconds()),
	}
}

func GoRuntimeMetrics() GoRuntimeMetric {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	lastPause := uint64(0)
	if stats.NumGC > 0 {
		lastPause = stats.PauseNs[(stats.NumGC+255)%256]
	}
	return GoRuntimeMetric{
		Goroutines:    runtime.NumGoroutine(),
		HeapAlloc:     stats.HeapAlloc,
		HeapInuse:     stats.HeapInuse,
		HeapObjects:   stats.HeapObjects,
		NextGC:        stats.NextGC,
		LastGCPauseNS: lastPause,
		TotalPauseNS:  stats.PauseTotalNs,
		NumGC:         stats.NumGC,
	}
}
