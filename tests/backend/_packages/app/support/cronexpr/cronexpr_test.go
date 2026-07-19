package cronexpr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseSupportsFiveAndSixFields(t *testing.T) {
	fiveFields, err := Parse("*/5 * * * *")
	require.NoError(t, err)
	require.Equal(t,
		time.Date(2026, time.July, 19, 12, 5, 0, 0, time.UTC),
		fiveFields.Next(time.Date(2026, time.July, 19, 12, 0, 1, 0, time.UTC)),
	)

	sixFields, err := Parse("*/5 * * * * *")
	require.NoError(t, err)
	require.Equal(t,
		time.Date(2026, time.July, 19, 12, 0, 5, 0, time.UTC),
		sixFields.Next(time.Date(2026, time.July, 19, 12, 0, 1, 0, time.UTC)),
	)
}

func TestParseRejectsInvalidFieldCount(t *testing.T) {
	_, err := Parse("* * * *")
	require.Error(t, err)
}

func TestParseWithSecondsRejectsFiveFields(t *testing.T) {
	_, err := ParseWithSeconds("*/5 * * * *")
	require.Error(t, err)

	_, err = ParseWithSeconds("*/5 * * * * *")
	require.NoError(t, err)
}
