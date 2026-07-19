package logadmin

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/facades"
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

func TestIsTestingEnvironment(t *testing.T) {
	require.True(t, isTestingEnvironment("testing"))
	require.True(t, isTestingEnvironment(" TESTING "))
	require.False(t, isTestingEnvironment("production"))
}

func TestOperationLogRunnerDoesNotRunInTesting(t *testing.T) {
	originalEnvironment := facades.Config().GetString("app.env")
	originalQueue := facades.Config().GetString("queue.default")
	t.Cleanup(func() {
		facades.Config().Add("app.env", originalEnvironment)
		facades.Config().Add("queue.default", originalQueue)
	})

	facades.Config().Add("app.env", "testing")
	facades.Config().Add("queue.default", "sync")
	require.False(t, (&OperationLogRunner{}).ShouldRun())
}
