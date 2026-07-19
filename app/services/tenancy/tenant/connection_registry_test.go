package tenant

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTenantConnectionRegistryDoesNotRejectAdditionalTenants(t *testing.T) {
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	registry := newTenantConnectionRegistry(2, func() time.Time { return now })
	require.NoError(t, registry.Acquire("a"))
	require.NoError(t, registry.Acquire("b"))
	now = now.Add(2 * time.Minute)
	require.NoError(t, registry.Acquire("c"))
	require.True(t, registry.Contains("b"))
	require.True(t, registry.Contains("a"))
	require.True(t, registry.Contains("c"))
	require.Equal(t, 3, registry.Count())
}

func TestTenantConnectionRegistryAllowsExistingPoolAtCapacity(t *testing.T) {
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	registry := newTenantConnectionRegistry(2, func() time.Time { return now })
	require.NoError(t, registry.Acquire("a"))
	now = now.Add(30 * time.Second)
	require.NoError(t, registry.Acquire("b"))
	now = now.Add(2 * time.Minute)
	require.NoError(t, registry.Acquire("a"))
	require.True(t, registry.Contains("a"))
	require.True(t, registry.Contains("b"))
	require.Equal(t, 2, registry.Count())
}

func TestTenantConnectionBudgetRejectsUnsafeConfiguration(t *testing.T) {
	report, err := ValidateConnectionBudget(ConnectionBudget{
		Pods: 4, ActiveTenantPools: 10, MaxOpenPerPool: 20, PostgreSQLBudget: 500,
	})
	require.ErrorIs(t, err, ErrTenantConnectionBudgetExceeded)
	require.Equal(t, 800, report.RequiredConnections)

	report, err = ValidateConnectionBudget(ConnectionBudget{
		Pods: 2, ActiveTenantPools: 5, MaxOpenPerPool: 20, PostgreSQLBudget: 500,
	})
	require.NoError(t, err)
	require.Equal(t, 200, report.RequiredConnections)
}

func TestResetTenantConnectionRegistryForTest(t *testing.T) {
	configuredTenantConnectionRegistry = newTenantConnectionRegistry(2, time.Now)
	require.NoError(t, configuredTenantConnectionRegistry.Acquire("tenant_1"))
	require.Equal(t, 1, ConnectionRegistryCount())
	require.True(t, ConnectionRegistered("tenant_1"))

	ResetConnectionRegistryForTest()

	require.Equal(t, 0, ConnectionRegistryCount())
	require.False(t, ConnectionRegistered("tenant_1"))
}

func TestTenantConnectionBudgetUsesActualRegisteredPools(t *testing.T) {
	configuredTenantConnectionRegistry = newTenantConnectionRegistry(1, time.Now)
	require.NoError(t, configuredTenantConnectionRegistry.Acquire("tenant_1"))
	require.NoError(t, configuredTenantConnectionRegistry.Acquire("tenant_2"))

	budget := ConnectionBudgetFromConfig()
	require.Equal(t, 2, budget.ActiveTenantPools)
}
