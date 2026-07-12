package unit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
)

func TestScheduledTaskRunnerShutdownBeforeRunReturns(t *testing.T) {
	runner := services.NewScheduledTaskRunner()
	done := make(chan error, 1)

	go func() {
		done <- runner.Shutdown()
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Shutdown blocked before Run initialized cancellation")
	}
}
