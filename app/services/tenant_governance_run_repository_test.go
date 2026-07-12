package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/models"
)

func TestTenantGovernanceRunTransitionMatrix(t *testing.T) {
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusAwaitingEvidence))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusAwaitingEvidence, models.TenantGovernanceRunStatusCompleted))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusAwaitingEvidence, models.TenantGovernanceRunStatusFailed))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusArtifactWritten))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusArtifactWritten, models.TenantGovernanceRunStatusCompleted))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusCompleted, models.TenantGovernanceRunStatusStale))
	require.False(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusCompleted, models.TenantGovernanceRunStatusRunning))
	require.False(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusFailed, models.TenantGovernanceRunStatusRunning))
}

func TestTenantGovernanceRunKindsAreClosed(t *testing.T) {
	for _, kind := range []string{
		models.TenantGovernanceRunKindRetention,
		models.TenantGovernanceRunKindExport,
		models.TenantGovernanceRunKindIsolationVerify,
	} {
		require.True(t, tenantGovernanceRunKindValid(kind))
	}
	require.False(t, tenantGovernanceRunKindValid("arbitrary_sql"))
}

func TestTenantGovernanceRetentionAwaitingEvidenceTransitionsAreClosed(t *testing.T) {
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusAwaitingEvidence))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusAwaitingEvidence, models.TenantGovernanceRunStatusCompleted))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusAwaitingEvidence, models.TenantGovernanceRunStatusFailed))
	require.False(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusAwaitingEvidence, models.TenantGovernanceRunStatusRunning))
}
