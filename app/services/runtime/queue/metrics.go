package queue

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
)

type QueueBacklogMetric struct {
	FailedJobs       int64
	OutboxPending    int64
	OutboxProcessing int64
	OutboxFailed     int64
	OutboxSent       int64
	Classes          []QueueClassMetric
}

type QueueClassMetric struct {
	Class          string
	Pending        int64
	OldestAge      time.Duration
	ArrivalRate    float64
	CompletionRate float64
}

var (
	queueClasses          = []string{"critical", "default", "bulk"}
	queueMetricRateWindow = 5 * time.Minute
)

func QueueBacklogMetrics(ctx context.Context) (metric QueueBacklogMetric) {
	defer func() {
		if recover() != nil {
			metric = QueueBacklogMetric{}
		}
	}()

	ctx = contextOrBackground(ctx)
	db := failedJobsOrm(ctx)
	query := db.Query()

	if count, err := query.Table(failedJobsTable()).Count(); err == nil {
		metric.FailedJobs = count
	}
	db = OrmWithContext(ctx)
	metric.OutboxPending = countQueueOutboxStatus(db, QueueOutboxStatusPending)
	metric.OutboxProcessing = countQueueOutboxStatus(db, QueueOutboxStatusProcessing)
	metric.OutboxFailed = countQueueOutboxStatus(db, QueueOutboxStatusFailed)
	metric.Classes = queueClassMetrics(ctx, db, time.Now())

	return metric
}

func QueueClassMetricsFromOutbox(events []QueueOutboxEvent, now time.Time) []QueueClassMetric {
	metrics := queueClassMetricMap()
	for _, event := range events {
		class := normalizeQueueClass(event.Queue)
		metric := metrics[class]
		if isWithinQueueRateWindow(event.CreatedAt, now) {
			metric.ArrivalRate += 1 / queueMetricRateWindow.Seconds()
		}
		switch event.Status {
		case QueueOutboxStatusPending:
			metric.Pending++
			metric.OldestAge = oldestQueueAge(metric.OldestAge, now, event.CreatedAt)
		case QueueOutboxStatusSent:
			if isWithinQueueRateWindow(event.UpdatedAt, now) {
				metric.CompletionRate += 1 / queueMetricRateWindow.Seconds()
			}
		}
		metrics[class] = metric
	}
	return queueClassMetricSlice(metrics)
}

func isWithinQueueRateWindow(timestamp, now time.Time) bool {
	return !timestamp.IsZero() && !timestamp.After(now) && timestamp.After(now.Add(-queueMetricRateWindow))
}

func queueClassMetrics(ctx context.Context, db orm.Orm, now time.Time) []QueueClassMetric {
	rows := make([]QueueOutboxEvent, 0)
	since := now.Add(-queueMetricRateWindow)
	if err := db.Query().
		Table("queue_outbox").
		Where("(status = ? OR created_at > ? OR updated_at > ?)", QueueOutboxStatusPending, since, since).
		Get(&rows); err != nil {
		return queueClassMetricSlice(queueClassMetricMap())
	}
	return QueueClassMetricsFromOutbox(rows, now)
}

func queueClassMetricMap() map[string]QueueClassMetric {
	metrics := make(map[string]QueueClassMetric, len(queueClasses))
	for _, class := range queueClasses {
		metrics[class] = QueueClassMetric{Class: class}
	}
	return metrics
}

func queueClassMetricSlice(metrics map[string]QueueClassMetric) []QueueClassMetric {
	classes := make([]string, 0, len(metrics))
	for class := range metrics {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	result := make([]QueueClassMetric, 0, len(classes))
	for _, class := range classes {
		result = append(result, metrics[class])
	}
	return result
}

func normalizeQueueClass(queue string) string {
	queue = strings.TrimSpace(queue)
	for _, class := range queueClasses {
		if queue == class {
			return class
		}
	}
	return "default"
}

func oldestQueueAge(current time.Duration, now, createdAt time.Time) time.Duration {
	if createdAt.IsZero() {
		return current
	}
	age := now.Sub(createdAt)
	if age < 0 {
		age = 0
	}
	if age > current {
		return age
	}
	return current
}

func countQueueOutboxStatus(db orm.Orm, status string) int64 {
	count, err := db.Query().Table("queue_outbox").Where("status", status).Count()
	if err != nil {
		return 0
	}
	return count
}

func failedJobsOrm(ctx context.Context) orm.Orm {
	connection := facades.Config().GetString("queue.failed.database")
	if connection == "" {
		return OrmWithContext(ctx)
	}
	return OrmForConnectionWithContext(ctx, connection)
}

func failedJobsTable() string {
	table := strings.TrimSpace(facades.Config().GetString("queue.failed.table", "failed_jobs"))
	if table == "" {
		return "failed_jobs"
	}
	return table
}
