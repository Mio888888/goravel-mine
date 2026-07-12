package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerDefaultsToDisabledInTesting(t *testing.T) {
	require.False(t, schedulerEnabledByDefault("testing"))
	require.False(t, schedulerEnabledByDefault(" Testing "))
}

func TestSchedulerDefaultsToEnabledOutsideTesting(t *testing.T) {
	require.True(t, schedulerEnabledByDefault("local"))
	require.True(t, schedulerEnabledByDefault("production"))
}
