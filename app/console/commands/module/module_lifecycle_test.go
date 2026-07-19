package module

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModuleLifecycleCommandRejectsDirectExecution(t *testing.T) {
	err := validateModuleLifecycleCLIExecution(true)

	require.Error(t, err)
	require.Contains(t, err.Error(), "platform management API")
}

func TestModuleLifecycleCommandAllowsDryRun(t *testing.T) {
	require.NoError(t, validateModuleLifecycleCLIExecution(false))
}
