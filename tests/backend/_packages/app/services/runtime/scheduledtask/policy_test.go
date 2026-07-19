package scheduledtask

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplaceCleanupDoesNotRemoveNewerRun(t *testing.T) {
	task := ScheduledTask{ID: 42, ConcurrencyPolicy: ScheduledTaskConcurrencyReplace}

	firstCtx, unregisterFirst := registerScheduledTaskRun(task, context.Background())
	secondCtx, unregisterSecond := registerScheduledTaskRun(task, context.Background())
	require.ErrorIs(t, firstCtx.Err(), context.Canceled)
	require.NoError(t, secondCtx.Err())

	unregisterFirst()
	thirdCtx, unregisterThird := registerScheduledTaskRun(task, context.Background())
	require.ErrorIs(t, secondCtx.Err(), context.Canceled)
	require.NoError(t, thirdCtx.Err())

	unregisterSecond()
	unregisterThird()
}
