package config

import "testing"

import "github.com/stretchr/testify/require"

func TestQueueConcurrentFromEnvKeepsPositiveValues(t *testing.T) {
	require.Equal(t, 6, queueConcurrentFromEnv(6))
	require.Equal(t, 6, queueConcurrentFromEnv("6"))
}

func TestQueueConcurrentFromEnvFallsBackWhenInvalid(t *testing.T) {
	require.Equal(t, 1, queueConcurrentFromEnv(0))
	require.Equal(t, 1, queueConcurrentFromEnv(-2))
}
