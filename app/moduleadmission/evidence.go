package moduleadmission

import (
	"context"
	"fmt"
	"os"
)

type CosignEvidenceVerifier struct {
	fetcher  SourceFetcher
	verifier SignatureVerifier
}

func NewCosignEvidenceVerifier(fetcher SourceFetcher, runner ProcessRunner) CosignEvidenceVerifier {
	return CosignEvidenceVerifier{fetcher: fetcher, verifier: NewSignatureVerifier(runner)}
}

func (v CosignEvidenceVerifier) Verify(ctx context.Context, source FetchedSource) (VerificationEvidence, error) {
	workspace, err := os.MkdirTemp("", "module-admission-evidence-")
	if err != nil {
		return VerificationEvidence{}, err
	}
	defer os.RemoveAll(workspace)
	signature, err := v.fetcher.FetchEvidenceFile(ctx, source.Entry.SignatureURI, "", workspace)
	if err != nil {
		return VerificationEvidence{}, fmt.Errorf("fetch source signature: %w", err)
	}
	defer signature.Cleanup()
	sbom, err := v.fetcher.FetchEvidenceFile(ctx, source.Entry.SBOMURI, source.Entry.SBOMDigest, workspace)
	if err != nil {
		return VerificationEvidence{}, fmt.Errorf("fetch SBOM: %w", err)
	}
	defer sbom.Cleanup()
	sbomSignature, err := v.fetcher.FetchEvidenceFile(ctx, source.Entry.SBOMURI+".sig", "", workspace)
	if err != nil {
		return VerificationEvidence{}, fmt.Errorf("fetch SBOM signature: %w", err)
	}
	defer sbomSignature.Cleanup()
	provenance, err := v.fetcher.FetchEvidenceFile(ctx, source.Entry.ProvenanceURI, source.Entry.ProvenanceDigest, workspace)
	if err != nil {
		return VerificationEvidence{}, fmt.Errorf("fetch provenance: %w", err)
	}
	defer provenance.Cleanup()
	provenanceSignature, err := v.fetcher.FetchEvidenceFile(ctx, source.Entry.ProvenanceURI+".sig", "", workspace)
	if err != nil {
		return VerificationEvidence{}, fmt.Errorf("fetch provenance signature: %w", err)
	}
	defer provenanceSignature.Cleanup()
	return v.verifier.Verify(ctx, VerificationArtifact{
		Path: source.Result.BundlePath, Digest: source.Result.Digest, SignaturePath: signature.Path,
		SBOMPath: sbom.Path, SBOMDigest: sbom.Digest, SBOMSignaturePath: sbomSignature.Path,
		ProvenancePath: provenance.Path, ProvenanceDigest: provenance.Digest, ProvenanceSignaturePath: provenanceSignature.Path,
	}, source.Entry.CosignIssuer, source.Entry.CosignIdentity)
}
