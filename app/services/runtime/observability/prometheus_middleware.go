package observability

import (
	"fmt"
	"strings"

	messagebusservice "goravel/app/services/runtime/messagebus"
	protectionservice "goravel/app/services/runtime/protection"
)

func writeMiddlewarePrometheusMetrics(
	b *strings.Builder,
	middleware messagebusservice.MiddlewareMetricSnapshot,
	protection []protectionservice.Metric,
) {
	b.WriteString("# HELP goravel_middleware_adapters Current middleware adapters by type and health.\n")
	b.WriteString("# TYPE goravel_middleware_adapters gauge\n")
	for _, metric := range middleware.Adapters {
		b.WriteString(fmt.Sprintf(
			"goravel_middleware_adapters{adapter_type=%q,health_status=%q} %d\n",
			metric.AdapterType,
			metric.HealthStatus,
			metric.Count,
		))
	}

	b.WriteString("# HELP goravel_message_deliveries_total Message deliveries by type, consumer, and status.\n")
	b.WriteString("# TYPE goravel_message_deliveries_total counter\n")
	b.WriteString("# HELP goravel_message_delivery_duration_milliseconds_total Total message delivery duration.\n")
	b.WriteString("# TYPE goravel_message_delivery_duration_milliseconds_total counter\n")
	for _, metric := range middleware.Deliveries {
		labels := fmt.Sprintf(
			"message_type=%q,consumer_key=%q,status=%q",
			metric.MessageType,
			metric.ConsumerKey,
			metric.Status,
		)
		b.WriteString(fmt.Sprintf("goravel_message_deliveries_total{%s} %d\n", labels, metric.Count))
		b.WriteString(fmt.Sprintf(
			"goravel_message_delivery_duration_milliseconds_total{%s} %.3f\n",
			labels,
			metric.DurationSumMS,
		))
	}

	b.WriteString("# HELP goravel_message_dead_letters Current dead letters by failure and resolution status.\n")
	b.WriteString("# TYPE goravel_message_dead_letters gauge\n")
	for _, metric := range middleware.DeadLetters {
		b.WriteString(fmt.Sprintf(
			"goravel_message_dead_letters{failure_class=%q,resolution_status=%q} %d\n",
			metric.FailureClass,
			metric.ResolutionStatus,
			metric.Count,
		))
	}

	b.WriteString("# HELP goravel_protection_decisions_total Protection decisions by rule set and result.\n")
	b.WriteString("# TYPE goravel_protection_decisions_total counter\n")
	b.WriteString("# HELP goravel_protection_calls_total Completed protected calls by rule set and result.\n")
	b.WriteString("# TYPE goravel_protection_calls_total counter\n")
	b.WriteString("# HELP goravel_protection_duration_milliseconds_total Total protected dependency duration.\n")
	b.WriteString("# TYPE goravel_protection_duration_milliseconds_total counter\n")
	for _, metric := range protection {
		labels := fmt.Sprintf(
			"rule_set_id=%q,version=%q,scope=%q,resource_pattern=%q",
			fmt.Sprint(metric.RuleSetID),
			fmt.Sprint(metric.Version),
			metric.Scope,
			metric.ResourcePattern,
		)
		b.WriteString(fmt.Sprintf("goravel_protection_decisions_total{%s,result=%q} %d\n", labels, "passed", metric.Passed))
		b.WriteString(fmt.Sprintf("goravel_protection_decisions_total{%s,result=%q} %d\n", labels, "rate_limited", metric.RateLimited))
		b.WriteString(fmt.Sprintf("goravel_protection_decisions_total{%s,result=%q} %d\n", labels, "circuit_rejected", metric.CircuitRejected))
		b.WriteString(fmt.Sprintf("goravel_protection_decisions_total{%s,result=%q} %d\n", labels, "concurrency_rejected", metric.ConcurrencyRejected))
		b.WriteString(fmt.Sprintf("goravel_protection_decisions_total{%s,result=%q} %d\n", labels, "half_open_probe", metric.HalfOpenProbes))
		b.WriteString(fmt.Sprintf("goravel_protection_calls_total{%s,result=%q} %d\n", labels, "completed", metric.Calls))
		b.WriteString(fmt.Sprintf("goravel_protection_calls_total{%s,result=%q} %d\n", labels, "failure", metric.Failures))
		b.WriteString(fmt.Sprintf("goravel_protection_duration_milliseconds_total{%s} %.3f\n", labels, metric.DurationSumMS))
	}
}
