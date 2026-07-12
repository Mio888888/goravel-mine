package moduleadmission

import (
	"context"
	"regexp"
	"time"
)

const AdmissionApprovalPolicy = "module.admission.approve"

var (
	exactVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
	digestPattern       = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type RepositoryIndex struct {
	SchemaVersion string             `json:"schema_version"`
	Modules       []ModuleIndexEntry `json:"modules"`
	Digest        string             `json:"digest"`
}

type ModuleIndexEntry struct {
	ID               string            `json:"id"`
	Version          string            `json:"version"`
	SourceKind       string            `json:"source_kind"`
	SourceURI        string            `json:"source_uri"`
	Digest           string            `json:"digest"`
	CosignIssuer     string            `json:"cosign_issuer,omitempty"`
	CosignIdentity   string            `json:"cosign_identity,omitempty"`
	SignatureURI     string            `json:"signature_uri,omitempty"`
	SBOMDigest       string            `json:"sbom_digest,omitempty"`
	SBOMURI          string            `json:"sbom_uri,omitempty"`
	ProvenanceDigest string            `json:"provenance_digest,omitempty"`
	ProvenanceURI    string            `json:"provenance_uri,omitempty"`
	GoImportPath     string            `json:"go_import_path,omitempty"`
	Dependencies     []IndexDependency `json:"dependencies,omitempty"`
	Deprecated       bool              `json:"deprecated,omitempty"`
	ReplacedBy       string            `json:"replaced_by,omitempty"`
}

type IndexDependency struct {
	ID                string `json:"id"`
	VersionConstraint string `json:"version_constraint"`
	Required          bool   `json:"required"`
}

type ModuleReference struct {
	ID      string
	Version string
}

type SourceModuleMetadata struct {
	ID           string
	Version      string
	GoImportPath string
	GoModulePath string
	Dependencies []IndexDependency
}

type SourceFetcherConfig struct {
	AllowedHosts      []string
	MaxBundleBytes    int64
	DownloadTimeout   time.Duration
	MaxArchiveEntries int
}

type SourceFetchResult struct {
	BundlePath string
	SourceDir  string
	Digest     string
	Size       int64
	Cleanup    func()
}

type FetchedSource struct {
	Entry  ModuleIndexEntry
	Result SourceFetchResult
}

type ProcessOutput struct {
	Stdout  string
	Stderr  string
	Version string
}

type ProcessRunner interface {
	Run(context.Context, string, ...string) (ProcessOutput, error)
	Version(context.Context, string) (string, error)
}

type VerificationArtifact struct {
	Path                    string
	Digest                  string
	SignaturePath           string
	SBOMPath                string
	SBOMDigest              string
	SBOMSignaturePath       string
	ProvenancePath          string
	ProvenanceDigest        string
	ProvenanceSignaturePath string
}

type VerificationEvidence struct {
	ArtifactDigest   string    `json:"artifact_digest"`
	CosignIssuer     string    `json:"cosign_issuer"`
	CosignIdentity   string    `json:"cosign_identity"`
	RekorIncluded    bool      `json:"rekor_included"`
	SBOMDigest       string    `json:"sbom_digest"`
	ProvenanceDigest string    `json:"provenance_digest"`
	CosignVersion    string    `json:"cosign_version"`
	VerifiedAt       time.Time `json:"verified_at"`
}

type EvidenceFile struct {
	Path    string
	Digest  string
	Cleanup func()
}

type Resolution struct {
	IndexDigest string
	Modules     []ModuleIndexEntry
	GraphDigest string
}

type AdmissionLock struct {
	SchemaVersion         string             `json:"schema_version"`
	IndexDigest           string             `json:"index_digest"`
	SourceDigest          string             `json:"source_digest"`
	DependencyGraphDigest string             `json:"dependency_graph_digest"`
	Modules               []ModuleIndexEntry `json:"modules"`
	Digest                string             `json:"digest"`
}

type AdmissionApproval struct {
	ID            string
	PolicyKey     string
	BindingDigest string
	Approved      bool
	ExpiresAt     time.Time
}

type StaticRegistry struct {
	Source string
	Digest string
}

type ExternalEvidenceVerifier interface {
	Verify(context.Context, FetchedSource) (VerificationEvidence, error)
}

type AdmissionPipelineInput struct {
	Index        RepositoryIndex
	Sources      []FetchedSource
	Requested    []ModuleReference
	Approval     AdmissionApproval
	LockPath     string
	RegistryPath string
}

type AdmissionPipelineResult struct {
	Lock     AdmissionLock
	Registry StaticRegistry
	Evidence []VerificationEvidence
}
