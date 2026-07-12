package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/models"
)

func TestAuditPruneProofVerifierBindsImmutableArchiveToPlanWindow(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	minimum := now.Add(-48 * time.Hour)
	maximum := now.Add(-24 * time.Hour)
	plan := AuditPrunePlan{
		PlanID: "plan-1", TargetDigest: auditPruneTargetDigest(nil),
		MinTimestamp: &minimum, MaxTimestamp: &maximum,
	}
	proof := AuditPruneWORMProof{
		PlanID: plan.PlanID, TargetDigest: plan.TargetDigest, ArchiveURI: "s3://audit/archive.json",
		ObjectVersion: "v1",
		WindowFrom:    minimum, WindowTo: maximum, ImmutableUntil: now.Add(30 * 24 * time.Hour), VerifiedAt: now,
	}
	proof.ManifestSHA256 = digestBytes(auditPruneManifestJSON(plan, proof))
	verifier := NewAuditPruneProofVerifier()
	verifier.immutable.now = func() time.Time { return now }
	verifier.immutable.attestor = immutableEvidenceAttestorFunc(func(_ ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
		return ImmutableEvidenceAttestation{
			Manifest: auditPruneManifestJSON(plan, proof), ObjectVersion: proof.ObjectVersion,
			ImmutableUntil: proof.ImmutableUntil, VerifiedAt: proof.VerifiedAt,
		}, nil
	})
	require.NoError(t, verifier.Verify(plan, proof))

	invalidCases := map[string]func(*AuditPruneWORMProof){
		"plan":         func(value *AuditPruneWORMProof) { value.PlanID = "other" },
		"digest":       func(value *AuditPruneWORMProof) { value.TargetDigest = auditPruneTestDigest("other") },
		"uri":          func(value *AuditPruneWORMProof) { value.ArchiveURI = "https://example.com/archive" },
		"version":      func(value *AuditPruneWORMProof) { value.ObjectVersion = "" },
		"manifest":     func(value *AuditPruneWORMProof) { value.ManifestSHA256 = "bad" },
		"window":       func(value *AuditPruneWORMProof) { value.WindowFrom = minimum.Add(time.Hour) },
		"immutability": func(value *AuditPruneWORMProof) { value.ImmutableUntil = now },
	}
	for name, mutate := range invalidCases {
		t.Run(name, func(t *testing.T) {
			candidate := proof
			mutate(&candidate)
			require.Error(t, verifier.Verify(plan, candidate))
		})
	}

	verifier.immutable.attestor = immutableEvidenceAttestorFunc(func(_ ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
		return ImmutableEvidenceAttestation{
			Manifest: auditPruneManifestJSON(plan, proof), ObjectVersion: proof.ObjectVersion,
			ImmutableUntil: proof.ImmutableUntil, VerifiedAt: now.Add(-25 * time.Hour),
		}, nil
	})
	require.ErrorIs(t, verifier.Verify(plan, proof), ErrImmutableEvidenceStale)
}

func TestAuditPruneProofVerifierRejectsUnattestedObjectMetadata(t *testing.T) {
	now := time.Now().UTC()
	plan := AuditPrunePlan{PlanID: "plan-1", TargetDigest: auditPruneTargetDigest(nil), Cutoff: now}
	proof := AuditPruneWORMProof{
		PlanID: plan.PlanID, TargetDigest: plan.TargetDigest, ArchiveURI: "s3://audit/archive.json",
		ObjectVersion: "v1", ManifestSHA256: auditPruneTestDigest("manifest"),
		WindowFrom: now.Add(-time.Hour), WindowTo: now, ImmutableUntil: now.Add(time.Hour), VerifiedAt: now,
	}

	verifier := NewAuditPruneProofVerifier()
	verifier.immutable.attestor = immutableEvidenceAttestorFunc(func(ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	})
	require.ErrorIs(t, verifier.Verify(plan, proof), ErrImmutableEvidenceUnattested)
}

func TestAuditPruneArchiveManifestBindsFullRecordContent(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	record := json.RawMessage(`{"id":7,"event":"role.changed"}`)
	digest, err := auditPruneRecordDigest(record)
	require.NoError(t, err)
	plan := AuditPrunePlan{
		PlanID: "plan-record", Cutoff: now, TargetCount: 1,
		Targets: []models.SecurityAuditPruneTarget{{
			Scope: "platform", AuditTable: "user_operation_log", TargetID: 7,
			OccurredAt: now.Add(-time.Hour), Record: record, RecordDigest: digest,
		}},
	}
	plan.TargetDigest = auditPruneTargetDigest(plan.Targets)
	require.True(t, auditPruneArchiveMatchesPlan(plan, auditPruneArchiveRecords(plan.Targets)))

	tampered := auditPruneArchiveRecords(plan.Targets)
	tampered[0].Record = json.RawMessage(`{"id":7,"event":"role.deleted"}`)
	require.False(t, auditPruneArchiveMatchesPlan(plan, tampered))

	forged := auditPruneArchiveRecords(plan.Targets)
	forged[0].RecordDigest = auditPruneTestDigest("forged")
	require.False(t, auditPruneArchiveMatchesPlan(plan, forged))
}

func TestAuditPruneArchiveManifestForPlanUsesPlanWindowAndRecords(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	record := json.RawMessage(`{"id":9,"action":"export"}`)
	digest, err := auditPruneRecordDigest(record)
	require.NoError(t, err)
	minimum, maximum := now.Add(-2*time.Hour), now.Add(-time.Hour)
	plan := AuditPrunePlan{PlanID: "plan-export", MinTimestamp: &minimum, MaxTimestamp: &maximum, Targets: []models.SecurityAuditPruneTarget{{
		Scope: "platform", AuditTable: "user_operation_log", TargetID: 9, OccurredAt: minimum,
		Record: record, RecordDigest: digest,
	}}}
	plan.TargetDigest = auditPruneTargetDigest(plan.Targets)
	manifest := AuditPruneArchiveManifestForPlan(plan, time.Time{}, time.Time{})
	require.Equal(t, minimum, manifest.WindowFrom)
	require.Equal(t, maximum, manifest.WindowTo)
	require.True(t, auditPruneArchiveMatchesPlan(plan, manifest.Records))
}

func auditPruneTestDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
