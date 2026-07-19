package scheduledtask

import (
	"time"

	"github.com/robfig/cron/v3"

	"goravel/app/support/cronexpr"
)

func NextScheduledRun(expression string, location *time.Location, after time.Time) (time.Time, error) {
	schedule, err := parseCronSchedule(expression)
	if err != nil {
		return time.Time{}, err
	}
	if location == nil {
		location = time.UTC
	}
	next := schedule.Next(after.In(location))
	if next.IsZero() {
		return time.Time{}, BusinessError{Message: "Cron 表达式没有可执行日期: no valid day"}
	}
	return next, nil
}

func parseCronSchedule(expression string) (cron.Schedule, error) {
	schedule, err := cronexpr.Parse(expression)
	if err != nil {
		return nil, BusinessError{Message: "Cron 表达式格式错误"}
	}
	return schedule, nil
}
