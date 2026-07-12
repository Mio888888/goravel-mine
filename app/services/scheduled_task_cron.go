package services

import (
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const scheduledCronFields = 6

type cronField struct {
	min     int
	max     int
	allowed map[int]struct{}
}

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
	if _, err := parseCronExpression(expression); err != nil {
		return nil, err
	}
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(strings.TrimSpace(expression))
	if err != nil {
		return nil, BusinessError{Message: "Cron 表达式格式错误"}
	}
	return schedule, nil
}

func parseCronExpression(expression string) ([]cronField, error) {
	parts := strings.Fields(strings.TrimSpace(expression))
	if len(parts) != scheduledCronFields {
		return nil, BusinessError{Message: "Cron 表达式必须为 6 位，精确到秒"}
	}

	ranges := [][2]int{{0, 59}, {0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	fields := make([]cronField, 0, scheduledCronFields)
	for i, part := range parts {
		field, err := parseCronField(part, ranges[i][0], ranges[i][1])
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func parseCronField(raw string, min, max int) (cronField, error) {
	field := cronField{min: min, max: max, allowed: map[int]struct{}{}}
	for _, token := range strings.Split(raw, ",") {
		if err := field.addToken(strings.TrimSpace(token)); err != nil {
			return cronField{}, err
		}
	}
	if len(field.allowed) == 0 {
		return cronField{}, BusinessError{Message: "Cron 表达式字段不能为空"}
	}
	return field, nil
}

func (f cronField) addToken(token string) error {
	if token == "" {
		return BusinessError{Message: "Cron 表达式字段不能为空"}
	}

	step := 1
	base := token
	if strings.Contains(token, "/") {
		parts := strings.Split(token, "/")
		if len(parts) != 2 {
			return BusinessError{Message: "Cron 表达式步进格式错误"}
		}
		base = parts[0]
		parsed, err := strconv.Atoi(parts[1])
		if err != nil || parsed < 1 {
			return BusinessError{Message: "Cron 表达式步进必须为正整数"}
		}
		step = parsed
	}

	start, end, err := f.rangeFor(base)
	if err != nil {
		return err
	}
	for value := start; value <= end; value += step {
		f.allowed[value] = struct{}{}
	}
	return nil
}

func (f cronField) rangeFor(base string) (int, int, error) {
	if base == "*" || base == "" {
		return f.min, f.max, nil
	}
	if strings.Contains(base, "-") {
		parts := strings.Split(base, "-")
		if len(parts) != 2 {
			return 0, 0, BusinessError{Message: "Cron 表达式范围格式错误"}
		}
		start, err := parseCronInt(parts[0], f.min, f.max)
		if err != nil {
			return 0, 0, err
		}
		end, err := parseCronInt(parts[1], f.min, f.max)
		if err != nil {
			return 0, 0, err
		}
		if start > end {
			return 0, 0, BusinessError{Message: "Cron 表达式范围起点不能大于终点"}
		}
		return start, end, nil
	}
	value, err := parseCronInt(base, f.min, f.max)
	if err != nil {
		return 0, 0, err
	}
	return value, value, nil
}

func parseCronInt(raw string, min, max int) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < min || value > max {
		return 0, BusinessError{Message: "Cron 表达式字段超出允许范围"}
	}
	return value, nil
}

func (f cronField) matches(value int) bool {
	_, ok := f.allowed[value]
	return ok
}
