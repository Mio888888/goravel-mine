package cronexpr

import (
	"strings"

	"github.com/robfig/cron/v3"
)

var optionalSecondsParser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

var secondsParser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

func Parse(expression string) (cron.Schedule, error) {
	return optionalSecondsParser.Parse(strings.TrimSpace(expression))
}

func ParseWithSeconds(expression string) (cron.Schedule, error) {
	return secondsParser.Parse(strings.TrimSpace(expression))
}
