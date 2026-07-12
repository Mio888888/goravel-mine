package moduleadmission

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type AdmissionPipeline struct {
	Verifier ExternalEvidenceVerifier
	Now      func() time.Time
}

func RunAdmissionPipeline(ctx context.Context, input AdmissionPipelineInput) (AdmissionPipelineResult, error) {
	return AdmissionPipeline{}.Run(ctx, input)
}

func (p AdmissionPipeline) Run(ctx context.Context, input AdmissionPipelineInput) (AdmissionPipelineResult, error) {
	if err := input.Approval.Valid(); err != nil {
		return AdmissionPipelineResult{}, err
	}
	if len(input.Sources) == 0 {
		return AdmissionPipelineResult{}, fmt.Errorf("admission sources are required")
	}
	if strings.TrimSpace(input.LockPath) == "" || strings.TrimSpace(input.RegistryPath) == "" {
		return AdmissionPipelineResult{}, fmt.Errorf("admission lock and registry output paths are required")
	}
	sources, err := normalizeAdmissionSources(input.Index, input.Sources)
	if err != nil {
		return AdmissionPipelineResult{}, err
	}
	metadata, err := sourceMetadata(sources)
	if err != nil {
		return AdmissionPipelineResult{}, err
	}
	requested := input.Requested
	if len(requested) == 0 {
		requested = sourceReferences(sources)
	}
	resolution, err := Resolve(input.Index, requested, metadata)
	if err != nil {
		return AdmissionPipelineResult{}, err
	}
	lock, err := NewAdmissionLock(input.Index, resolution)
	if err != nil {
		return AdmissionPipelineResult{}, err
	}
	registry, err := GenerateStaticRegistry(lock)
	if err != nil {
		return AdmissionPipelineResult{}, err
	}
	if err := ValidateAdmissionApproval(input.Approval, lock, registry); err != nil {
		return AdmissionPipelineResult{}, err
	}
	evidence, err := p.verifyExternal(ctx, sources)
	if err != nil {
		return AdmissionPipelineResult{}, err
	}
	if err := lock.VerifyGeneratedRegistry(registry); err != nil {
		return AdmissionPipelineResult{}, err
	}
	if err := WriteAdmissionArtifacts(lock, registry, input.LockPath, input.RegistryPath); err != nil {
		return AdmissionPipelineResult{}, err
	}
	return AdmissionPipelineResult{Lock: lock, Registry: registry, Evidence: evidence}, nil
}

func (p AdmissionPipeline) verifyExternal(ctx context.Context, sources []FetchedSource) ([]VerificationEvidence, error) {
	evidence := make([]VerificationEvidence, 0)
	for _, source := range sources {
		if source.Entry.SourceKind != "external" {
			continue
		}
		if p.Verifier == nil {
			return nil, fmt.Errorf("external module %s requires cosign, SBOM, and provenance verification", source.Entry.ID)
		}
		item, err := p.Verifier.Verify(ctx, source)
		if err != nil {
			return nil, fmt.Errorf("verify external module %s: %w", source.Entry.ID, err)
		}
		if item.ArtifactDigest != source.Result.Digest || !item.RekorIncluded || item.SBOMDigest != source.Entry.SBOMDigest || item.ProvenanceDigest != source.Entry.ProvenanceDigest {
			return nil, fmt.Errorf("external module %s verification evidence does not bind fetched source", source.Entry.ID)
		}
		evidence = append(evidence, item)
	}
	return evidence, nil
}

func normalizeAdmissionSources(index RepositoryIndex, sources []FetchedSource) ([]FetchedSource, error) {
	byKey := make(map[string]ModuleIndexEntry, len(index.Modules))
	for _, entry := range index.Modules {
		byKey[moduleVersionKey(entry.ID, entry.Version)] = entry
	}
	normalized := append([]FetchedSource(nil), sources...)
	for position := range normalized {
		source := &normalized[position]
		key := moduleVersionKey(source.Entry.ID, source.Entry.Version)
		indexEntry, ok := byKey[key]
		if !ok || source.Entry.Digest != indexEntry.Digest || source.Result.Digest != indexEntry.Digest {
			return nil, fmt.Errorf("fetched source does not match repository index: %s", key)
		}
		source.Entry = indexEntry
	}
	sort.Slice(normalized, func(left, right int) bool {
		return moduleVersionKey(normalized[left].Entry.ID, normalized[left].Entry.Version) < moduleVersionKey(normalized[right].Entry.ID, normalized[right].Entry.Version)
	})
	return normalized, nil
}

func sourceMetadata(sources []FetchedSource) ([]SourceModuleMetadata, error) {
	metadata := make([]SourceModuleMetadata, 0, len(sources))
	for _, source := range sources {
		if strings.TrimSpace(source.Result.SourceDir) == "" {
			continue
		}
		item, err := ReadSourceModuleMetadata(source.Result.SourceDir)
		if err != nil {
			return nil, err
		}
		metadata = append(metadata, item)
	}
	return metadata, nil
}

func sourceReferences(sources []FetchedSource) []ModuleReference {
	references := make([]ModuleReference, 0, len(sources))
	for _, source := range sources {
		references = append(references, ModuleReference{ID: source.Entry.ID, Version: source.Entry.Version})
	}
	return references
}

func (a AdmissionApproval) Valid() error {
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.PolicyKey) != AdmissionApprovalPolicy || !a.Approved {
		return fmt.Errorf("module admission requires approved approval record")
	}
	if !a.ExpiresAt.IsZero() && !a.ExpiresAt.After(time.Now()) {
		return fmt.Errorf("module admission approval has expired")
	}
	return nil
}
