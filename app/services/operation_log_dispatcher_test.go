package services

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOperationLogJobShouldRetryUsesBoundedBackoff(t *testing.T) {
	job := &OperationLogJob{}

	retry, delay := job.ShouldRetry(errors.New("db down"), 1)
	require.True(t, retry)
	require.Equal(t, 2*time.Second, delay)

	retry, delay = job.ShouldRetry(errors.New("db down"), 3)
	require.True(t, retry)
	require.Equal(t, 8*time.Second, delay)
}

func TestOperationLogJobShouldRetryStopsAfterConfiguredAttempts(t *testing.T) {
	job := &OperationLogJob{}

	retry, delay := job.ShouldRetry(errors.New("db down"), 4)
	require.False(t, retry)
	require.Zero(t, delay)
}
