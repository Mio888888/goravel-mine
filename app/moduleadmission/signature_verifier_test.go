package moduleadmission

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignatureVerifierBindsCosignIdentityIssuerAndArtifactEvidence(t *testing.T) {
	directory := t.TempDir()
	artifactPath := filepath.Join(directory, "module.bundle")
	sbomPath := filepath.Join(directory, "module.sbom.json")
	provenancePath := filepath.Join(directory, "module.provenance.json")
	writeVerificationFixture(t, artifactPath, sbomPath, provenancePath)
	artifact := verificationArtifact(t, artifactPath, sbomPath, provenancePath)
	runner := &fakeProcessRunner{output: validCosignOutput}
	verifier := NewSignatureVerifier(runner)

	evidence, err := verifier.Verify(context.Background(), artifact, "https://issuer.example", "build@example")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if evidence.ArtifactDigest != artifact.Digest || evidence.CosignIssuer != "https://issuer.example" || evidence.CosignIdentity != "build@example" {
		t.Fatalf("evidence = %#v, want digest and identity bindings", evidence)
	}
	if len(runner.calls) != 3 || !strings.Contains(strings.Join(runner.calls[0].args, " "), "--certificate-oidc-issuer=https://issuer.example") || !strings.Contains(strings.Join(runner.calls[1].args, " "), "--attestation") {
		t.Fatalf("cosign calls = %#v, want issuer-bound verification and attestation", runner.calls)
	}
}

func TestSignatureVerifierFailsClosedForVerificationAndEvidenceMismatch(t *testing.T) {
	directory := t.TempDir()
	artifactPath := filepath.Join(directory, "module.bundle")
	sbomPath := filepath.Join(directory, "module.sbom.json")
	provenancePath := filepath.Join(directory, "module.provenance.json")
	writeVerificationFixture(t, artifactPath, sbomPath, provenancePath)
	artifact := verificationArtifact(t, artifactPath, sbomPath, provenancePath)

	tests := []struct {
		name     string
		runner   *fakeProcessRunner
		mutate   func(*VerificationArtifact)
		contains string
	}{
		{name: "wrong identity", runner: &fakeProcessRunner{err: errors.New("certificate identity mismatch")}, contains: "certificate identity mismatch"},
		{name: "wrong issuer", runner: &fakeProcessRunner{err: errors.New("certificate issuer mismatch")}, contains: "certificate issuer mismatch"},
		{name: "missing transparency proof", runner: &fakeProcessRunner{output: `[{"optional":{}}]`}, contains: "transparency proof"},
		{name: "altered SBOM", runner: &fakeProcessRunner{output: validCosignOutput}, mutate: func(value *VerificationArtifact) { value.SBOMDigest = "sha256:" + strings.Repeat("0", 64) }, contains: "SBOM digest mismatch"},
		{name: "SBOM binds another artifact", runner: &fakeProcessRunner{output: validCosignOutput}, mutate: func(value *VerificationArtifact) {
			requireWriteVerificationFile(t, value.SBOMPath, `{"metadata":{"component":{"hashes":[{"alg":"SHA-256","content":"`+strings.Repeat("0", 64)+`"}]}}}`)
			value.SBOMDigest = fileDigest(t, value.SBOMPath)
		}, contains: "SBOM subject digest mismatch"},
		{name: "altered provenance", runner: &fakeProcessRunner{output: validCosignOutput}, mutate: func(value *VerificationArtifact) { value.ProvenanceDigest = "sha256:" + strings.Repeat("0", 64) }, contains: "provenance digest mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := copyVerificationArtifact(t, artifact, t.TempDir())
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			_, err := NewSignatureVerifier(test.runner).Verify(context.Background(), candidate, "https://issuer.example", "build@example")
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Verify() error = %v, want %q", err, test.contains)
			}
		})
	}
}

const validCosignOutput = `[{"optional":{"Bundle":{"Payload":{"logIndex":42}}}}]`

type fakeProcessRunner struct {
	output string
	err    error
	calls  []processCall
}

type processCall struct {
	name string
	args []string
}

func (r *fakeProcessRunner) Run(_ context.Context, name string, args ...string) (ProcessOutput, error) {
	r.calls = append(r.calls, processCall{name: name, args: append([]string(nil), args...)})
	if r.err != nil {
		return ProcessOutput{}, r.err
	}
	return ProcessOutput{Stdout: r.output, Version: "v2.4.1"}, nil
}

func (r *fakeProcessRunner) Version(_ context.Context, _ string) (string, error) {
	return "v2.4.1", nil
}

func writeVerificationFixture(t *testing.T, artifactPath, sbomPath, provenancePath string) {
	t.Helper()
	if err := os.WriteFile(artifactPath, []byte("approved bundle"), 0600); err != nil {
		t.Fatal(err)
	}
	digest := fileDigest(t, artifactPath)
	if err := os.WriteFile(sbomPath, []byte(`{"bomFormat":"CycloneDX","metadata":{"component":{"hashes":[{"alg":"SHA-256","content":"`+strings.TrimPrefix(digest, "sha256:")+`"}]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(provenancePath, []byte(`{"subject":[{"name":"module.bundle","digest":{"sha256":"`+strings.TrimPrefix(digest, "sha256:")+`"}}]}`), 0600); err != nil {
		t.Fatal(err)
	}
}

func verificationArtifact(t *testing.T, artifactPath, sbomPath, provenancePath string) VerificationArtifact {
	t.Helper()
	return VerificationArtifact{
		Path: artifactPath, Digest: fileDigest(t, artifactPath), SignaturePath: artifactPath + ".sig",
		SBOMPath: sbomPath, SBOMDigest: fileDigest(t, sbomPath), SBOMSignaturePath: sbomPath + ".sig",
		ProvenancePath: provenancePath, ProvenanceDigest: fileDigest(t, provenancePath),
		ProvenanceSignaturePath: provenancePath + ".sig",
	}
}

func requireWriteVerificationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func copyVerificationArtifact(t *testing.T, artifact VerificationArtifact, directory string) VerificationArtifact {
	t.Helper()
	copyPath := func(source string) string {
		target := filepath.Join(directory, filepath.Base(source))
		payload, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, payload, 0600); err != nil {
			t.Fatal(err)
		}
		return target
	}
	artifact.Path = copyPath(artifact.Path)
	artifact.SignaturePath = artifact.Path + ".sig"
	artifact.SBOMPath = copyPath(artifact.SBOMPath)
	artifact.SBOMSignaturePath = artifact.SBOMPath + ".sig"
	artifact.ProvenancePath = copyPath(artifact.ProvenancePath)
	artifact.ProvenanceSignaturePath = artifact.ProvenancePath + ".sig"
	return artifact
}
