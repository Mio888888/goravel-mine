package observability

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func PrometheusMetricsText(snapshot MetricsSnapshot) string {
	var b strings.Builder
	b.WriteString("# HELP goravel_http_requests_total Total HTTP requests.\n")
	b.WriteString("# TYPE goravel_http_requests_total counter\n")
	for _, metric := range snapshot.ByRoute {
		b.WriteString(fmt.Sprintf(
			"goravel_http_requests_total{method=%q,route=%q,status=%q} %d\n",
			metric.Method,
			metric.Route,
			fmt.Sprint(metric.Status),
			metric.Count,
		))
	}

	b.WriteString("# HELP goravel_http_request_duration_milliseconds_by_status_sum Total HTTP request duration in milliseconds by status.\n")
	b.WriteString("# TYPE goravel_http_request_duration_milliseconds_by_status_sum counter\n")
	for _, metric := range snapshot.ByRoute {
		b.WriteString(fmt.Sprintf(
			"goravel_http_request_duration_milliseconds_by_status_sum{method=%q,route=%q,status=%q} %.3f\n",
			metric.Method,
			metric.Route,
			fmt.Sprint(metric.Status),
			metric.DurationSumMS,
		))
	}

	routeTotals := aggregateRouteTotals(snapshot.ByRoute)
	b.WriteString("# HELP goravel_http_request_duration_milliseconds HTTP request duration histogram in milliseconds.\n")
	b.WriteString("# TYPE goravel_http_request_duration_milliseconds histogram\n")
	for _, metric := range routeTotals {
		for _, bucket := range routeBucketsFor(metric, snapshot.DurationBuckets) {
			b.WriteString(fmt.Sprintf(
				"goravel_http_request_duration_milliseconds_bucket{method=%q,route=%q,le=%q} %d\n",
				bucket.Method,
				bucket.Route,
				formatHistogramLe(bucket.LeMS),
				bucket.Count,
			))
		}
		b.WriteString(fmt.Sprintf(
			"goravel_http_request_duration_milliseconds_bucket{method=%q,route=%q,le=%q} %d\n",
			metric.Method,
			metric.Route,
			"+Inf",
			metric.Count,
		))
		b.WriteString(fmt.Sprintf(
			"goravel_http_request_duration_milliseconds_sum{method=%q,route=%q} %.3f\n",
			metric.Method,
			metric.Route,
			metric.DurationSumMS,
		))
		b.WriteString(fmt.Sprintf(
			"goravel_http_request_duration_milliseconds_count{method=%q,route=%q} %d\n",
			metric.Method,
			metric.Route,
			metric.Count,
		))
	}

	b.WriteString("# HELP goravel_http_requests_inflight Current in-flight HTTP requests.\n")
	b.WriteString("# TYPE goravel_http_requests_inflight gauge\n")
	b.WriteString(fmt.Sprintf("goravel_http_requests_inflight %d\n", snapshot.Inflight))

	b.WriteString("# HELP goravel_http_requests_total_all Total HTTP requests across all routes.\n")
	b.WriteString("# TYPE goravel_http_requests_total_all counter\n")
	b.WriteString(fmt.Sprintf("goravel_http_requests_total_all %d\n", snapshot.TotalRequests))

	b.WriteString("# HELP goravel_http_slow_requests_total Slow request samples retained in memory.\n")
	b.WriteString("# TYPE goravel_http_slow_requests_total gauge\n")
	b.WriteString(fmt.Sprintf("goravel_http_slow_requests_total %d\n", len(snapshot.SlowRequests)))

	b.WriteString("# HELP goravel_http_slow_requests_observed_total Total slow requests observed since process start.\n")
	b.WriteString("# TYPE goravel_http_slow_requests_observed_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_http_slow_requests_observed_total %d\n", snapshot.SlowRequestsTotal))

	b.WriteString("# HELP goravel_sql_slow_queries_retained Slow SQL samples retained in memory.\n")
	b.WriteString("# TYPE goravel_sql_slow_queries_retained gauge\n")
	b.WriteString(fmt.Sprintf("goravel_sql_slow_queries_retained %d\n", snapshot.SlowSQLRetained))

	b.WriteString("# HELP goravel_sql_slow_queries_observed_total Total slow SQL queries observed since process start.\n")
	b.WriteString("# TYPE goravel_sql_slow_queries_observed_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_sql_slow_queries_observed_total %d\n", snapshot.SlowSQLObserved))

	b.WriteString("# HELP goravel_process_uptime_seconds Process uptime in seconds.\n")
	b.WriteString("# TYPE goravel_process_uptime_seconds gauge\n")
	b.WriteString(fmt.Sprintf("goravel_process_uptime_seconds %d\n", snapshot.UptimeSeconds))

	b.WriteString("# HELP goravel_go_goroutines Current number of goroutines.\n")
	b.WriteString("# TYPE goravel_go_goroutines gauge\n")
	b.WriteString(fmt.Sprintf("goravel_go_goroutines %d\n", snapshot.GoRuntime.Goroutines))

	b.WriteString("# HELP goravel_go_heap_alloc_bytes Current heap allocation in bytes.\n")
	b.WriteString("# TYPE goravel_go_heap_alloc_bytes gauge\n")
	b.WriteString(fmt.Sprintf("goravel_go_heap_alloc_bytes %d\n", snapshot.GoRuntime.HeapAlloc))

	b.WriteString("# HELP goravel_go_heap_inuse_bytes Current heap in-use bytes.\n")
	b.WriteString("# TYPE goravel_go_heap_inuse_bytes gauge\n")
	b.WriteString(fmt.Sprintf("goravel_go_heap_inuse_bytes %d\n", snapshot.GoRuntime.HeapInuse))

	b.WriteString("# HELP goravel_go_heap_objects Current number of allocated heap objects.\n")
	b.WriteString("# TYPE goravel_go_heap_objects gauge\n")
	b.WriteString(fmt.Sprintf("goravel_go_heap_objects %d\n", snapshot.GoRuntime.HeapObjects))

	b.WriteString("# HELP goravel_go_next_gc_bytes Target heap size of next GC in bytes.\n")
	b.WriteString("# TYPE goravel_go_next_gc_bytes gauge\n")
	b.WriteString(fmt.Sprintf("goravel_go_next_gc_bytes %d\n", snapshot.GoRuntime.NextGC))

	b.WriteString("# HELP goravel_go_last_gc_pause_seconds Last GC pause in seconds.\n")
	b.WriteString("# TYPE goravel_go_last_gc_pause_seconds gauge\n")
	b.WriteString(fmt.Sprintf("goravel_go_last_gc_pause_seconds %.9f\n", float64(snapshot.GoRuntime.LastGCPauseNS)/1e9))

	b.WriteString("# HELP goravel_go_gc_pause_seconds_total Total GC pause in seconds.\n")
	b.WriteString("# TYPE goravel_go_gc_pause_seconds_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_go_gc_pause_seconds_total %.9f\n", float64(snapshot.GoRuntime.TotalPauseNS)/1e9))

	b.WriteString("# HELP goravel_go_gc_cycles_total Total number of completed GC cycles.\n")
	b.WriteString("# TYPE goravel_go_gc_cycles_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_go_gc_cycles_total %d\n", snapshot.GoRuntime.NumGC))

	b.WriteString("# HELP goravel_db_pool_open_connections Current open DB connections.\n")
	b.WriteString("# TYPE goravel_db_pool_open_connections gauge\n")
	b.WriteString(fmt.Sprintf("goravel_db_pool_open_connections{connection=%q} %d\n", snapshot.DBPool.Connection, snapshot.DBPool.OpenConnections))

	b.WriteString("# HELP goravel_db_pool_in_use_connections Current in-use DB connections.\n")
	b.WriteString("# TYPE goravel_db_pool_in_use_connections gauge\n")
	b.WriteString(fmt.Sprintf("goravel_db_pool_in_use_connections{connection=%q} %d\n", snapshot.DBPool.Connection, snapshot.DBPool.InUse))

	b.WriteString("# HELP goravel_db_pool_idle_connections Current idle DB connections.\n")
	b.WriteString("# TYPE goravel_db_pool_idle_connections gauge\n")
	b.WriteString(fmt.Sprintf("goravel_db_pool_idle_connections{connection=%q} %d\n", snapshot.DBPool.Connection, snapshot.DBPool.Idle))

	b.WriteString("# HELP goravel_db_pool_wait_count_total Total waits for DB connections.\n")
	b.WriteString("# TYPE goravel_db_pool_wait_count_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_db_pool_wait_count_total{connection=%q} %d\n", snapshot.DBPool.Connection, snapshot.DBPool.WaitCount))

	b.WriteString("# HELP goravel_db_pool_wait_duration_milliseconds_total Total wait duration for DB connections in milliseconds.\n")
	b.WriteString("# TYPE goravel_db_pool_wait_duration_milliseconds_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_db_pool_wait_duration_milliseconds_total{connection=%q} %.3f\n", snapshot.DBPool.Connection, snapshot.DBPool.WaitDurationMS))

	b.WriteString("# HELP goravel_db_pool_max_open_connections Configured max open DB connections.\n")
	b.WriteString("# TYPE goravel_db_pool_max_open_connections gauge\n")
	b.WriteString(fmt.Sprintf("goravel_db_pool_max_open_connections{connection=%q} %d\n", snapshot.DBPool.Connection, snapshot.DBPool.MaxOpen))

	b.WriteString("# HELP goravel_db_pool_max_idle_closed_total Total DB connections closed due to idle limit.\n")
	b.WriteString("# TYPE goravel_db_pool_max_idle_closed_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_db_pool_max_idle_closed_total{connection=%q} %d\n", snapshot.DBPool.Connection, snapshot.DBPool.MaxIdleClosed))

	b.WriteString("# HELP goravel_db_pool_max_idle_time_closed_total Total DB connections closed due to idle time limit.\n")
	b.WriteString("# TYPE goravel_db_pool_max_idle_time_closed_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_db_pool_max_idle_time_closed_total{connection=%q} %d\n", snapshot.DBPool.Connection, snapshot.DBPool.MaxIdleTimeClosed))

	b.WriteString("# HELP goravel_db_pool_max_lifetime_closed_total Total DB connections closed due to lifetime limit.\n")
	b.WriteString("# TYPE goravel_db_pool_max_lifetime_closed_total counter\n")
	b.WriteString(fmt.Sprintf("goravel_db_pool_max_lifetime_closed_total{connection=%q} %d\n", snapshot.DBPool.Connection, snapshot.DBPool.MaxLifetimeClosed))

	writeQueuePrometheusMetrics(&b, snapshot.Queue)
	writeCapacityPrometheusMetrics(&b, snapshot)
	writeMiddlewarePrometheusMetrics(&b, snapshot.Middleware, snapshot.Protection)

	b.WriteString("# HELP goravel_scheduler_node_alive Scheduler node heartbeat status.\n")
	b.WriteString("# TYPE goravel_scheduler_node_alive gauge\n")
	for _, node := range snapshot.SchedulerNodes {
		alive := 0
		if node.Alive {
			alive = 1
		}
		b.WriteString(fmt.Sprintf("goravel_scheduler_node_alive{node_ip=%q} %d\n", node.NodeIP, alive))
	}

	b.WriteString("# HELP goravel_scheduler_node_last_seen_seconds Seconds since scheduler node heartbeat.\n")
	b.WriteString("# TYPE goravel_scheduler_node_last_seen_seconds gauge\n")
	for _, node := range snapshot.SchedulerNodes {
		b.WriteString(fmt.Sprintf("goravel_scheduler_node_last_seen_seconds{node_ip=%q} %d\n", node.NodeIP, node.LastSeenSecondsAgo))
	}

	b.WriteString("# HELP goravel_tenant_governance_evidence_expired Expired tenant governance evidence rows.\n")
	b.WriteString("# TYPE goravel_tenant_governance_evidence_expired gauge\n")
	b.WriteString(fmt.Sprintf("goravel_tenant_governance_evidence_expired %d\n", snapshot.TenantGovernance.EvidenceExpired))

	b.WriteString("# HELP goravel_tenant_governance_verification_failed Failed tenant isolation verification runs.\n")
	b.WriteString("# TYPE goravel_tenant_governance_verification_failed gauge\n")
	b.WriteString(fmt.Sprintf("goravel_tenant_governance_verification_failed %d\n", snapshot.TenantGovernance.VerificationFailed))

	b.WriteString("# HELP goravel_tenant_governance_run_age_seconds Age of the oldest pending, running, or evidence-waiting tenant governance run.\n")
	b.WriteString("# TYPE goravel_tenant_governance_run_age_seconds gauge\n")
	b.WriteString(fmt.Sprintf("goravel_tenant_governance_run_age_seconds %.0f\n", snapshot.TenantGovernance.OldestRunAge.Seconds()))

	return bindPrometheusRelease(b.String(), os.Getenv("RELEASE_GIT_SHA"))
}

func bindPrometheusRelease(text, gitSHA string) string {
	if gitSHA == "" {
		return text
	}
	label := "release_git_sha=" + strconv.Quote(gitSHA)
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if brace := strings.IndexByte(line, '{'); brace >= 0 {
			lines[index] = line[:brace+1] + label + "," + line[brace+1:]
			continue
		}
		if space := strings.IndexByte(line, ' '); space >= 0 {
			lines[index] = line[:space] + "{" + label + "}" + line[space:]
		}
	}
	return strings.Join(lines, "\n")
}

func aggregateRouteTotals(metrics []RouteMetric) []RouteMetric {
	totals := make(map[string]RouteMetric)
	for _, metric := range metrics {
		key := metric.Method + " " + metric.Route
		total := totals[key]
		total.Method = metric.Method
		total.Route = metric.Route
		total.Count += metric.Count
		total.DurationSumMS += metric.DurationSumMS
		totals[key] = total
	}
	keys := make([]string, 0, len(totals))
	for key := range totals {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]RouteMetric, 0, len(keys))
	for _, key := range keys {
		result = append(result, totals[key])
	}
	return result
}

func routeBucketsFor(metric RouteMetric, buckets []RouteDurationBucket) []RouteDurationBucket {
	counts := make(map[float64]uint64)
	for _, bucket := range buckets {
		if bucket.Method == metric.Method && bucket.Route == metric.Route {
			counts[bucket.LeMS] = bucket.Count
		}
	}
	result := make([]RouteDurationBucket, 0, len(httpDurationBucketsMS))
	for _, le := range httpDurationBucketsMS {
		result = append(result, RouteDurationBucket{
			Method: metric.Method,
			Route:  metric.Route,
			LeMS:   le,
			Count:  counts[le],
		})
	}
	return result
}

func formatHistogramLe(value float64) string {
	return fmt.Sprintf("%g", value)
}

func MetricsSummary(snapshot MetricsSnapshot) map[string]any {
	slowByRoute := make(map[string]int)
	for _, item := range snapshot.SlowRequests {
		slowByRoute[item.Method+" "+item.Route]++
	}
	keys := make([]string, 0, len(slowByRoute))
	for key := range slowByRoute {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	slowRoutes := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		slowRoutes = append(slowRoutes, map[string]any{
			"route": key,
			"count": slowByRoute[key],
		})
	}

	return map[string]any{
		"total_requests":      snapshot.TotalRequests,
		"inflight":            snapshot.Inflight,
		"route_count":         len(snapshot.ByRoute),
		"slow_count":          len(snapshot.SlowRequests),
		"slow_observed_total": snapshot.SlowRequestsTotal,
		"slow_sql_count":      snapshot.SlowSQLRetained,
		"slow_sql_total":      snapshot.SlowSQLObserved,
		"goroutines":          snapshot.GoRuntime.Goroutines,
		"db_pool": map[string]any{
			"connection":       snapshot.DBPool.Connection,
			"open_connections": snapshot.DBPool.OpenConnections,
			"in_use":           snapshot.DBPool.InUse,
			"idle":             snapshot.DBPool.Idle,
			"wait_count":       snapshot.DBPool.WaitCount,
			"wait_duration_ms": snapshot.DBPool.WaitDurationMS,
			"max_open":         snapshot.DBPool.MaxOpen,
		},
		"queue": map[string]any{
			"failed_jobs":       snapshot.Queue.FailedJobs,
			"outbox_pending":    snapshot.Queue.OutboxPending,
			"outbox_processing": snapshot.Queue.OutboxProcessing,
			"outbox_failed":     snapshot.Queue.OutboxFailed,
			"outbox_sent":       snapshot.Queue.OutboxSent,
		},
		"middleware":      snapshot.Middleware,
		"protection":      snapshot.Protection,
		"scheduler_nodes": len(snapshot.SchedulerNodes),
		"uptime_seconds":  snapshot.UptimeSeconds,
		"slow_routes":     slowRoutes,
	}
}
