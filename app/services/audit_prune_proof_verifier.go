package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"goravel/app/models"
)

var ErrAuditPruneProofInvalid = errors.New("audit prune proof is invalid")

type AuditPruneWORMProof struct {
	PlanID         string    `json:"plan_id"`
	TargetDigest   string    `json:"target_digest"`
	ArchiveURI     string    `json:"archive_uri"`
	ObjectVersion  string    `json:"object_version"`
	ManifestSHA256 string    `json:"manifest_sha256"`
	WindowFrom     time.Time `json:"window_from"`
	WindowTo       time.Time `json:"window_to"`
	ImmutableUntil time.Time `json:"immutable_until"`
	VerifiedAt     time.Time `json:"verified_at"`
}

type AuditPruneArchiveManifest struct {
	PlanID       string                    `json:"plan_id"`
	TargetDigest string                    `json:"target_digest"`
	WindowFrom   time.Time                 `json:"window_from"`
	WindowTo     time.Time                 `json:"window_to"`
	Records      []AuditPruneArchiveRecord `json:"records"`
}

type AuditPruneArchiveRecord struct {
	Scope        string          `json:"scope"`
	Table        string          `json:"table"`
	TargetID     uint64          `json:"target_id"`
	OccurredAt   time.Time       `json:"occurred_at"`
	Record       json.RawMessage `json:"record"`
	RecordDigest string          `json:"record_digest"`
}

type AuditPruneProofVerifier struct {
	immutable *ImmutableEvidenceVerifier
}

func NewAuditPruneProofVerifier() *AuditPruneProofVerifier {
	return &AuditPruneProofVerifier{immutable: NewImmutableEvidenceVerifier()}
}

func (v *AuditPruneProofVerifier) Verify(plan AuditPrunePlan, proof AuditPruneWORMProof) error {
	_, err := v.VerifyAttested(plan, proof)
	return err
}

func (v *AuditPruneProofVerifier) VerifyAttested(plan AuditPrunePlan, proof AuditPruneWORMProof) (ImmutableEvidenceAttestation, error) {
	if v == nil || v.immutable == nil {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	if strings.TrimSpace(proof.PlanID) != plan.PlanID || !sameDigest(proof.TargetDigest, plan.TargetDigest) {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	if proof.WindowFrom.IsZero() || proof.WindowTo.IsZero() || proof.WindowTo.Before(proof.WindowFrom) {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	if proof.WindowFrom.After(plan.MinTimestampOrCutoff()) || proof.WindowTo.Before(plan.MaxTimestampOrCutoff()) {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	attestation, err := v.immutable.Verify(nil, ImmutableEvidence{
		URI: proof.ArchiveURI, ObjectVersion: proof.ObjectVersion, SHA256: proof.ManifestSHA256,
		ImmutableUntil: proof.ImmutableUntil, VerifiedAt: proof.VerifiedAt,
	})
	if err != nil {
		return ImmutableEvidenceAttestation{}, err
	}
	if !sameDigest(proof.ManifestSHA256, digestBytes(attestation.Manifest)) ||
		!attestation.ImmutableUntil.Equal(proof.ImmutableUntil) {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	var manifest AuditPruneArchiveManifest
	if json.Unmarshal(attestation.Manifest, &manifest) != nil || manifest.PlanID != plan.PlanID ||
		!sameDigest(manifest.TargetDigest, plan.TargetDigest) || !manifest.WindowFrom.Equal(proof.WindowFrom) ||
		!manifest.WindowTo.Equal(proof.WindowTo) || !auditPruneArchiveMatchesPlan(plan, manifest.Records) {
		return ImmutableEvidenceAttestation{}, ErrAuditPruneProofInvalid
	}
	return attestation, nil
}

func ReadAuditPruneProofFile(path string) (AuditPruneWORMProof, error) {
	payload, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return AuditPruneWORMProof{}, err
	}
	var proof AuditPruneWORMProof
	if err := json.Unmarshal(payload, &proof); err != nil {
		return AuditPruneWORMProof{}, err
	}
	return proof, nil
}

func AuditPruneProofDigest(proof AuditPruneWORMProof) string {
	payload, _ := json.Marshal(proof)
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func auditPruneManifestJSON(plan AuditPrunePlan, proof AuditPruneWORMProof) []byte {
	payload, _ := json.Marshal(AuditPruneArchiveManifestForPlan(plan, proof.WindowFrom, proof.WindowTo))
	return payload
}

func AuditPruneArchiveManifestForPlan(plan AuditPrunePlan, windowFrom, windowTo time.Time) AuditPruneArchiveManifest {
	if windowFrom.IsZero() {
		windowFrom = plan.MinTimestampOrCutoff()
	}
	if windowTo.IsZero() {
		windowTo = plan.MaxTimestampOrCutoff()
	}
	return AuditPruneArchiveManifest{
		PlanID: plan.PlanID, TargetDigest: plan.TargetDigest,
		WindowFrom: windowFrom, WindowTo: windowTo, Records: auditPruneArchiveRecords(plan.Targets),
	}
}

func auditPruneArchiveRecords(targets []models.SecurityAuditPruneTarget) []AuditPruneArchiveRecord {
	records := make([]AuditPruneArchiveRecord, 0, len(targets))
	for _, target := range targets {
		records = append(records, AuditPruneArchiveRecord{
			Scope: target.Scope, Table: target.AuditTable, TargetID: target.TargetID,
			OccurredAt: target.OccurredAt, Record: target.Record, RecordDigest: target.RecordDigest,
		})
	}
	return records
}

func auditPruneArchiveMatchesPlan(plan AuditPrunePlan, records []AuditPruneArchiveRecord) bool {
	if len(records) != len(plan.Targets) {
		return false
	}
	expected := make(map[string]models.SecurityAuditPruneTarget, len(plan.Targets))
	for _, target := range plan.Targets {
		expected[auditPruneArchiveKey(target.Scope, target.AuditTable, target.TargetID)] = target
	}
	for _, record := range records {
		target, ok := expected[auditPruneArchiveKey(record.Scope, record.Table, record.TargetID)]
		digest, err := auditPruneRecordDigest(record.Record)
		if !ok || err != nil || !record.OccurredAt.Equal(target.OccurredAt) ||
			!sameDigest(record.RecordDigest, target.RecordDigest) || !sameDigest(digest, target.RecordDigest) {
			return false
		}
		delete(expected, auditPruneArchiveKey(record.Scope, record.Table, record.TargetID))
	}
	return len(expected) == 0
}

func auditPruneArchiveKey(scope, table string, targetID uint64) string {
	return scope + "\x00" + table + "\x00" + strconv.FormatUint(targetID, 10)
}

func digestBytes(payload []byte) string {
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func isSHA256(value string) bool {
	value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sameDigest(left, right string) bool {
	return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(left), "sha256:"), strings.TrimPrefix(strings.TrimSpace(right), "sha256:"))
}
