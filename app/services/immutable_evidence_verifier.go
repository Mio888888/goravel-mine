package services

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrImmutableEvidenceInvalid    = errors.New("immutable evidence is invalid")
	ErrImmutableEvidenceStale      = errors.New("immutable evidence verification is stale")
	ErrImmutableEvidenceUnattested = errors.New("immutable evidence was not attested by trusted storage")
)

type ImmutableEvidence struct {
	URI            string    `json:"uri"`
	ObjectVersion  string    `json:"object_version"`
	SHA256         string    `json:"sha256"`
	ImmutableUntil time.Time `json:"immutable_until"`
	VerifiedAt     time.Time `json:"verified_at"`
}

type ImmutableEvidenceVerifier struct {
	now          func() time.Time
	maxFreshness time.Duration
	attestor     ImmutableEvidenceAttestor
}

type ImmutableEvidenceAttestation struct {
	Manifest       []byte
	ObjectVersion  string
	ImmutableUntil time.Time
	VerifiedAt     time.Time
}

type ImmutableEvidenceAttestor interface {
	Attest(context.Context, ImmutableEvidence) (ImmutableEvidenceAttestation, error)
}

type immutableEvidenceAttestorFunc func(ImmutableEvidence) (ImmutableEvidenceAttestation, error)

func (f immutableEvidenceAttestorFunc) Attest(_ context.Context, evidence ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
	return f(evidence)
}

func NewImmutableEvidenceVerifier() *ImmutableEvidenceVerifier {
	return &ImmutableEvidenceVerifier{now: time.Now, maxFreshness: 24 * time.Hour, attestor: NewS3ImmutableEvidenceAttestor()}
}

func (v *ImmutableEvidenceVerifier) Verify(ctx context.Context, evidence ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
	if v == nil || v.attestor == nil {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	if !immutableURIAllowed(evidence.URI) || strings.TrimSpace(evidence.ObjectVersion) == "" || !isSHA256(evidence.SHA256) {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceInvalid
	}
	attestation, err := v.attestor.Attest(contextOrBackground(ctx), evidence)
	if err != nil {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	now := v.now().UTC()
	if attestation.ObjectVersion != evidence.ObjectVersion || attestation.ImmutableUntil.IsZero() || !attestation.ImmutableUntil.After(now) {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceInvalid
	}
	if attestation.VerifiedAt.IsZero() || attestation.VerifiedAt.After(now.Add(5*time.Minute)) || now.Sub(attestation.VerifiedAt) > v.maxFreshness {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceStale
	}
	return attestation, nil
}

func immutableURIAllowed(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(value, "s3://") || strings.HasPrefix(value, "gs://") || strings.HasPrefix(value, "azblob://") || strings.HasPrefix(value, "worm://")
}
