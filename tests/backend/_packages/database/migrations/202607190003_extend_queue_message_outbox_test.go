package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueueMessageOutboxMigrationIncludesDefaultAndPlatformConnections(t *testing.T) {
	require.Equal(
		t,
		[]string{"tenant", "platform"},
		uniqueQueueMessageOutboxMigrationConnections(" tenant ", " platform "),
	)
	require.Equal(
		t,
		[]string{"postgres"},
		uniqueQueueMessageOutboxMigrationConnections("postgres", "postgres"),
	)
}
