package providers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueueServiceProviderSkipsSyncDriver(t *testing.T) {
	require.False(t, shouldRunQueueWorker(true, "sync", "sync"))
}

func TestQueueServiceProviderSkipsDisabledWorker(t *testing.T) {
	require.False(t, shouldRunQueueWorker(false, "redis", "custom"))
}

func TestQueueServiceProviderReturnsQueueRunner(t *testing.T) {
	require.False(t, shouldRunQueueWorker(true, "", "custom"))
	require.True(t, shouldRunQueueWorker(true, "redis", "custom"))
}
