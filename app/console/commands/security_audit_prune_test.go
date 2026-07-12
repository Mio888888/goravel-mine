package commands

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
)

func TestWriteAuditPruneArchiveProducesUploadReadyManifest(t *testing.T) {
	now := time.Now().UTC()
	plan := services.AuditPrunePlan{PlanID: "plan-1", TargetDigest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", Cutoff: now}
	path := filepath.Join(t.TempDir(), "archive", "manifest.json")
	require.NoError(t, writeAuditPruneArchive(path, services.AuditPruneArchiveManifestForPlan(plan, time.Time{}, time.Time{})))
	payload, err := os.ReadFile(path)
	require.NoError(t, err)
	var manifest services.AuditPruneArchiveManifest
	require.NoError(t, json.Unmarshal(payload, &manifest))
	require.Equal(t, plan.PlanID, manifest.PlanID)
	require.Equal(t, plan.TargetDigest, manifest.TargetDigest)
	require.Equal(t, now, manifest.WindowFrom)
	require.Equal(t, now, manifest.WindowTo)
	require.Empty(t, manifest.Records)
}

func TestSecurityAuditPruneRequiresExplicitExecuteAndArtifacts(t *testing.T) {
	require.NoError(t, validateSecurityAuditPruneInput(SecurityAuditPruneInput{}))
	require.NoError(t, validateSecurityAuditPruneInput(SecurityAuditPruneInput{DryRun: true}))
	require.ErrorIs(t, validateSecurityAuditPruneInput(SecurityAuditPruneInput{DryRun: true, Execute: true}), ErrSecurityAuditPruneMode)
	require.ErrorIs(t, validateSecurityAuditPruneInput(SecurityAuditPruneInput{Execute: true}), services.ErrAuditPruneEvidenceRequired)
}

func TestSecurityAuditPruneExecuteReadsProofAndEvidenceOnlyAfterValidation(t *testing.T) {
	proofRead := false
	evidenceRead := false
	core := &SecurityAuditPruneCore{
		plans:    services.NewAuditPrunePlanService(),
		executor: services.NewAuditPruneExecutor(),
		readProof: func(string) (services.AuditPruneWORMProof, error) {
			proofRead = true
			return services.AuditPruneWORMProof{}, errors.New("proof read")
		},
		readStdin: func() ([]byte, error) {
			evidenceRead = true
			return nil, nil
		},
	}

	_, err := core.Run(SecurityAuditPruneInput{Execute: true})
	require.ErrorIs(t, err, services.ErrAuditPruneEvidenceRequired)
	require.False(t, proofRead)
	require.False(t, evidenceRead)

	_, err = core.Run(SecurityAuditPruneInput{Execute: true, PlanID: "plan", ProofFile: "proof.json", EvidenceStdin: true})
	require.EqualError(t, err, "proof read")
	require.True(t, proofRead)
	require.False(t, evidenceRead)
}
