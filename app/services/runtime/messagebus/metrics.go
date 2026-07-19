package messagebus

import (
	"context"
	"sort"
)

type AdapterMetric struct {
	AdapterType  string `json:"adapter_type" gorm:"column:adapter_type"`
	HealthStatus string `json:"health_status" gorm:"column:health_status"`
	Count        int64  `json:"count" gorm:"column:count"`
}

type DeliveryMetric struct {
	MessageType   string  `json:"message_type" gorm:"column:message_type"`
	ConsumerKey   string  `json:"consumer_key" gorm:"column:consumer_key"`
	Status        string  `json:"status" gorm:"column:status"`
	Count         int64   `json:"count" gorm:"column:count"`
	DurationSumMS float64 `json:"duration_sum_ms" gorm:"column:duration_sum_ms"`
}

type DeadLetterMetric struct {
	FailureClass     string `json:"failure_class" gorm:"column:failure_class"`
	ResolutionStatus string `json:"resolution_status" gorm:"column:resolution_status"`
	Count            int64  `json:"count" gorm:"column:count"`
}

type MiddlewareMetricSnapshot struct {
	Adapters    []AdapterMetric    `json:"adapters"`
	Deliveries  []DeliveryMetric   `json:"deliveries"`
	DeadLetters []DeadLetterMetric `json:"dead_letters"`
}

func Metrics(ctx context.Context) (snapshot MiddlewareMetricSnapshot) {
	snapshot.Adapters = []AdapterMetric{}
	snapshot.Deliveries = []DeliveryMetric{}
	snapshot.DeadLetters = []DeadLetterMetric{}
	defer func() {
		if recover() != nil {
			snapshot = MiddlewareMetricSnapshot{
				Adapters: []AdapterMetric{}, Deliveries: []DeliveryMetric{}, DeadLetters: []DeadLetterMetric{},
			}
		}
	}()

	query := OrmForConnectionWithContext(ctx, PlatformConnection()).Query()
	_ = query.Raw(`
		SELECT adapter_type, health_status, COUNT(*) AS count
		FROM middleware_adapter
		GROUP BY adapter_type, health_status
	`).Scan(&snapshot.Adapters)
	_ = query.Raw(`
		SELECT message_type, consumer_key, status, COUNT(*) AS count,
			COALESCE(SUM(duration_ms), 0) AS duration_sum_ms
		FROM message_delivery
		GROUP BY message_type, consumer_key, status
	`).Scan(&snapshot.Deliveries)
	_ = query.Raw(`
		SELECT failure_class, resolution_status, COUNT(*) AS count
		FROM message_dead_letter
		GROUP BY failure_class, resolution_status
	`).Scan(&snapshot.DeadLetters)

	sort.Slice(snapshot.Adapters, func(i, j int) bool {
		if snapshot.Adapters[i].AdapterType == snapshot.Adapters[j].AdapterType {
			return snapshot.Adapters[i].HealthStatus < snapshot.Adapters[j].HealthStatus
		}
		return snapshot.Adapters[i].AdapterType < snapshot.Adapters[j].AdapterType
	})
	sort.Slice(snapshot.Deliveries, func(i, j int) bool {
		if snapshot.Deliveries[i].MessageType == snapshot.Deliveries[j].MessageType {
			if snapshot.Deliveries[i].ConsumerKey == snapshot.Deliveries[j].ConsumerKey {
				return snapshot.Deliveries[i].Status < snapshot.Deliveries[j].Status
			}
			return snapshot.Deliveries[i].ConsumerKey < snapshot.Deliveries[j].ConsumerKey
		}
		return snapshot.Deliveries[i].MessageType < snapshot.Deliveries[j].MessageType
	})
	sort.Slice(snapshot.DeadLetters, func(i, j int) bool {
		if snapshot.DeadLetters[i].FailureClass == snapshot.DeadLetters[j].FailureClass {
			return snapshot.DeadLetters[i].ResolutionStatus < snapshot.DeadLetters[j].ResolutionStatus
		}
		return snapshot.DeadLetters[i].FailureClass < snapshot.DeadLetters[j].FailureClass
	})
	return snapshot
}
