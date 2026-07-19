package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"goravel/app/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Source: audit_prune_proof_verifier_test.go
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

// Source: audit_retention_service_test.go
func TestAvailableAuditRetentionTargetsSkipsMissingTables(t *testing.T) {
	original := auditRetentionHasTable
	t.Cleanup(func() { auditRetentionHasTable = original })
	auditRetentionHasTable = func(connection, table string) bool {
		return table != "tenant_permission_audit"
	}

	targets := availableAuditRetentionTargets("tenant_demo", []auditRetentionTarget{
		{Table: "user_login_log", Column: "login_time"},
		{Table: "tenant_permission_audit", Column: "created_at"},
	})

	require.Equal(t, []auditRetentionTarget{
		{Table: "user_login_log", Column: "login_time"},
	}, targets)
}

func TestAuditRetentionServiceRejectsDirectPhysicalPrune(t *testing.T) {
	_, err := NewAuditRetentionService("unused").Prune(30, false)

	require.ErrorIs(t, err, ErrAuditPruneExecutionRequired)
}

// Source: immutable_evidence_attestor_s3_test.go
func TestImmutableEvidenceReadLimitRejectsOversizedBody(t *testing.T) {
	_, err := readImmutableEvidenceBody(bytes.NewReader([]byte("123456789")), 8)
	require.ErrorIs(t, err, ErrImmutableEvidenceUnattested)
	payload, err := readImmutableEvidenceBody(bytes.NewReader([]byte("12345678")), 8)
	require.NoError(t, err)
	require.Equal(t, []byte("12345678"), payload)
}

func TestImmutableS3RequestPinsObjectVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, "version-7", request.URL.Query().Get("versionId"))
		response.Header().Set("X-Amz-Version-Id", "version-7")
		response.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	client := newObjectStorageClient(StorageConfig{Endpoint: server.URL, Bucket: "audit"})

	response, err := immutableS3Request(t.Context(), client, http.MethodHead, "archive/manifest.json", "version-7")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
}

func TestImmutableEvidenceVerifierRejectsVersionMismatch(t *testing.T) {
	now := time.Now().UTC()
	verifier := &ImmutableEvidenceVerifier{
		now: time.Now, maxFreshness: time.Hour,
		attestor: immutableEvidenceAttestorFunc(func(ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
			return ImmutableEvidenceAttestation{
				Manifest: []byte(`{}`), ObjectVersion: "v1",
				ImmutableUntil: now.Add(time.Hour), VerifiedAt: now,
			}, nil
		}),
	}

	_, err := verifier.Verify(t.Context(), ImmutableEvidence{
		URI: "s3://audit/archive.json", ObjectVersion: "v2", SHA256: auditPruneTestDigest("manifest"),
	})
	require.ErrorIs(t, err, ErrImmutableEvidenceInvalid)
}
