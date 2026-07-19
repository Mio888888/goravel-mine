package unit

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
)

func TestNextScheduledRunSupportsSecondPrecision(t *testing.T) {
	base := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)

	next, err := services.NextScheduledRun("*/5 * * * * *", time.UTC, base)

	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 7, 4, 9, 0, 5, 0, time.UTC), next)
}

func TestNextScheduledRunSupportsDailyTime(t *testing.T) {
	base := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)

	next, err := services.NextScheduledRun("30 15 10 * * *", time.UTC, base)

	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 7, 4, 10, 15, 30, 0, time.UTC), next)
}

func TestNextScheduledRunSupportsMinutePrecision(t *testing.T) {
	base := time.Date(2026, 7, 4, 9, 0, 30, 0, time.UTC)

	next, err := services.NextScheduledRun("*/5 * * * *", time.UTC, base)

	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 7, 4, 9, 5, 0, 0, time.UTC), next)
}

func TestNextScheduledRunRejectsInvalidFieldCount(t *testing.T) {
	_, err := services.NextScheduledRun("* * * *", time.UTC, time.Now())

	require.Error(t, err)
	require.Contains(t, err.Error(), "Cron 表达式格式错误")
}

func TestNextScheduledRunRejectsImpossibleMonthDayQuickly(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := services.NextScheduledRun("0 0 0 31 2 *", time.UTC, base)

	require.Error(t, err)
	require.Contains(t, err.Error(), "no valid day")
}

func TestNextScheduledRunFindsSparseYearlySchedule(t *testing.T) {
	base := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)

	next, err := services.NextScheduledRun("0 0 0 1 1 *", time.UTC, base)

	require.NoError(t, err)
	require.Equal(t, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), next)
}

func TestScheduledTaskTargetsNode(t *testing.T) {
	require.True(t, services.ScheduledTaskTargetsNode([]string{}, "10.0.0.8"))
	require.True(t, services.ScheduledTaskTargetsNode([]string{"10.0.0.8"}, "10.0.0.8"))
	require.False(t, services.ScheduledTaskTargetsNode([]string{"10.0.0.7"}, "10.0.0.8"))
}

func TestCronErrorTextDocumentsImpossibleDay(t *testing.T) {
	_, err := services.NextScheduledRun("0 0 0 31 2 *", time.UTC, time.Now())

	require.True(t, strings.Contains(err.Error(), "no valid day"))
}
