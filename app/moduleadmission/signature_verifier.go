package moduleadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type SignatureVerifier struct {
	runner ProcessRunner
	now    func() time.Time
}

func NewSignatureVerifier(runner ProcessRunner) SignatureVerifier {
	if runner == nil {
		runner = execProcessRunner{}
	}
	return SignatureVerifier{runner: runner, now: time.Now}
}

func (v SignatureVerifier) Verify(ctx context.Context, artifact VerificationArtifact, issuer, identity string) (VerificationEvidence, error) {
	issuer = strings.TrimSpace(issuer)
	identity = strings.TrimSpace(identity)
	if issuer == "" || identity == "" {
		return VerificationEvidence{}, fmt.Errorf("cosign issuer and identity are required")
	}
	if err := verifyArtifactDigests(artifact); err != nil {
		return VerificationEvidence{}, err
	}
	output, err := v.runVerifyBlob(ctx, artifact.Path, artifact.SignaturePath, issuer, identity)
	if err != nil {
		return VerificationEvidence{}, err
	}
	if !hasTransparencyProof(output.Stdout) {
		return VerificationEvidence{}, fmt.Errorf("cosign verification has no transparency proof")
	}
	attestation, err := v.runVerifyAttestation(ctx, artifact.Path, artifact.SBOMPath, artifact.SBOMSignaturePath, issuer, identity)
	if err != nil {
		return VerificationEvidence{}, err
	}
	if !hasTransparencyProof(attestation.Stdout) {
		return VerificationEvidence{}, fmt.Errorf("cosign SBOM attestation has no transparency proof")
	}
	provenance, err := v.runVerifyAttestation(ctx, artifact.Path, artifact.ProvenancePath, artifact.ProvenanceSignaturePath, issuer, identity)
	if err != nil {
		return VerificationEvidence{}, err
	}
	if !hasTransparencyProof(provenance.Stdout) {
		return VerificationEvidence{}, fmt.Errorf("cosign provenance attestation has no transparency proof")
	}
	version := output.Version
	if version == "" {
		version, _ = v.runner.Version(ctx, "cosign")
	}
	return VerificationEvidence{
		ArtifactDigest: artifact.Digest, CosignIssuer: issuer, CosignIdentity: identity,
		RekorIncluded: true, SBOMDigest: artifact.SBOMDigest, ProvenanceDigest: artifact.ProvenanceDigest,
		CosignVersion: strings.TrimSpace(version), VerifiedAt: v.now().UTC(),
	}, nil
}

func (v SignatureVerifier) runVerifyBlob(ctx context.Context, path, signature, issuer, identity string) (ProcessOutput, error) {
	if strings.TrimSpace(signature) == "" {
		return ProcessOutput{}, fmt.Errorf("source signature path is required")
	}
	return v.runner.Run(ctx, "cosign", "verify-blob", "--output=json", "--certificate-identity="+identity, "--certificate-oidc-issuer="+issuer, "--signature", signature, path)
}

func (v SignatureVerifier) runVerifyAttestation(ctx context.Context, artifactPath, attestationPath, signaturePath, issuer, identity string) (ProcessOutput, error) {
	if strings.TrimSpace(attestationPath) == "" || strings.TrimSpace(signaturePath) == "" {
		return ProcessOutput{}, fmt.Errorf("attestation and signature paths are required")
	}
	return v.runner.Run(ctx, "cosign", "verify-blob-attestation", "--output=json", "--certificate-identity="+identity, "--certificate-oidc-issuer="+issuer, "--signature", signaturePath, "--attestation", attestationPath, artifactPath)
}

func verifyArtifactDigests(artifact VerificationArtifact) error {
	if !digestPattern.MatchString(artifact.Digest) {
		return fmt.Errorf("artifact digest is invalid")
	}
	digest, err := digestFile(artifact.Path)
	if err != nil {
		return err
	}
	if digest != artifact.Digest {
		return fmt.Errorf("artifact digest mismatch")
	}
	if err := verifyFileDigest("SBOM", artifact.SBOMPath, artifact.SBOMDigest); err != nil {
		return err
	}
	sbom, err := osReadFile(artifact.SBOMPath)
	if err != nil {
		return err
	}
	if !sbomBindsArtifact(sbom, artifact.Digest) {
		return fmt.Errorf("SBOM subject digest mismatch")
	}
	if err := verifyFileDigest("provenance", artifact.ProvenancePath, artifact.ProvenanceDigest); err != nil {
		return err
	}
	payload, err := osReadFile(artifact.ProvenancePath)
	if err != nil {
		return err
	}
	if !provenanceBindsArtifact(payload, artifact.Digest) {
		return fmt.Errorf("provenance subject digest mismatch")
	}
	return nil
}

func verifyFileDigest(name, path, expected string) error {
	if !digestPattern.MatchString(expected) {
		return fmt.Errorf("%s digest is invalid", name)
	}
	actual, err := digestFile(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("%s digest mismatch", name)
	}
	return nil
}

func hasTransparencyProof(payload string) bool {
	var entries []struct {
		Optional struct {
			Bundle struct {
				Payload struct {
					LogIndex *int `json:"logIndex"`
				} `json:"Payload"`
			} `json:"Bundle"`
		} `json:"optional"`
	}
	if json.Unmarshal([]byte(payload), &entries) != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Optional.Bundle.Payload.LogIndex != nil {
			return true
		}
	}
	return false
}

func provenanceBindsArtifact(payload []byte, digest string) bool {
	var provenance struct {
		Subject []struct {
			Digest map[string]string `json:"digest"`
		} `json:"subject"`
	}
	if json.Unmarshal(payload, &provenance) != nil {
		return false
	}
	want := strings.TrimPrefix(digest, "sha256:")
	for _, subject := range provenance.Subject {
		if strings.EqualFold(subject.Digest["sha256"], want) {
			return true
		}
	}
	return false
}

func sbomBindsArtifact(payload []byte, digest string) bool {
	var sbom struct {
		Metadata struct {
			Component struct {
				Hashes []struct {
					Algorithm string `json:"alg"`
					Content   string `json:"content"`
				} `json:"hashes"`
			} `json:"component"`
		} `json:"metadata"`
	}
	if json.Unmarshal(payload, &sbom) != nil {
		return false
	}
	want := strings.TrimPrefix(digest, "sha256:")
	for _, hash := range sbom.Metadata.Component.Hashes {
		if strings.EqualFold(hash.Algorithm, "SHA-256") && strings.EqualFold(hash.Content, want) {
			return true
		}
	}
	return false
}

type execProcessRunner struct{}

func (execProcessRunner) Run(ctx context.Context, name string, args ...string) (ProcessOutput, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return ProcessOutput{Stderr: string(output)}, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	version, _ := execProcessRunner{}.Version(ctx, name)
	return ProcessOutput{Stdout: string(output), Version: version}, nil
}

func (execProcessRunner) Version(ctx context.Context, name string) (string, error) {
	command := exec.CommandContext(ctx, name, "version", "--json")
	output, err := command.Output()
	if err == nil {
		return strings.TrimSpace(string(output)), nil
	}
	command = exec.CommandContext(ctx, name, "version")
	output, err = command.Output()
	return strings.TrimSpace(string(output)), err
}
