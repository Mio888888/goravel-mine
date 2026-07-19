package observability

import "goravel/app/facades"

func DBPoolMetrics(connection string) (metric DBPoolMetric) {
	defer func() {
		if recover() != nil {
			metric = DBPoolMetric{}
		}
	}()

	metric.Connection = connection
	orm := facades.Orm()
	if connection != "" {
		orm = orm.Connection(connection)
	}
	db, err := orm.DB()
	if err != nil || db == nil {
		return metric
	}
	stats := db.Stats()
	return DBPoolMetric{
		Connection:        connection,
		OpenConnections:   stats.OpenConnections,
		InUse:             stats.InUse,
		Idle:              stats.Idle,
		WaitCount:         stats.WaitCount,
		WaitDurationMS:    float64(stats.WaitDuration.Microseconds()) / 1000,
		MaxOpen:           stats.MaxOpenConnections,
		MaxIdleClosed:     stats.MaxIdleClosed,
		MaxIdleTimeClosed: stats.MaxIdleTimeClosed,
		MaxLifetimeClosed: stats.MaxLifetimeClosed,
	}
}
