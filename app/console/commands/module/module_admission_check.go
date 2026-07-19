package module

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/facades"
	"goravel/app/moduleadmission"
	"goravel/app/services"
)

var admissionApprovalLoader = loadAdmissionApproval
var admissionEvidenceReader = func() ([]byte, error) { return io.ReadAll(os.Stdin) }

type ModuleAdmissionCheckCommand struct{}

func (r *ModuleAdmissionCheckCommand) Signature() string {
	return "module:admission:check"
}

func (r *ModuleAdmissionCheckCommand) Description() string {
	return "Validate a signed module repository index and source bundle digests"
}

func (r *ModuleAdmissionCheckCommand) Extend() command.Extend {
	return command.Extend{Category: "module", Flags: []command.Flag{
		&command.StringFlag{Name: "index", Usage: "Repository index file or HTTPS URI"},
		&command.StringFlag{Name: "index-digest", Usage: "Required SHA-256 digest for a remote repository index"},
		&command.StringFlag{Name: "workspace", Usage: "Temporary admission workspace", Value: "tmp/module-admission"},
		&command.StringFlag{Name: "module", Usage: "Exact module references as comma-separated id@version; defaults to every index entry"},
		&command.BoolFlag{Name: "prepare", Usage: "Print canonical admission approval binding without writing artifacts"},
		&command.StringFlag{Name: "requester-id", Usage: "Requester ID bound to approval"},
		&command.BoolFlag{Name: "evidence-stdin", Usage: "Read approval and single-use re-auth evidence JSON from stdin"},
		&command.StringFlag{Name: "lock", Usage: "Atomic admission lock output", Value: "app/moduleboot/module-admission.lock.json"},
		&command.StringFlag{Name: "registry", Usage: "Atomic static registry output", Value: "app/moduleboot/admitted_modules_gen.go"},
	}}
}

func (r *ModuleAdmissionCheckCommand) Handle(ctx console.Context) error {
	indexURI := strings.TrimSpace(ctx.Option("index"))
	if indexURI == "" {
		return fmt.Errorf("--index is required")
	}
	payload, err := readAdmissionIndex(indexURI, ctx.Option("index-digest"))
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	index, err := moduleadmission.LoadRepositoryIndex(payload)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	workspace := strings.TrimSpace(ctx.Option("workspace"))
	if workspace == "" {
		workspace = "tmp/module-admission"
	}
	if err := os.MkdirAll(workspace, 0700); err != nil {
		ctx.Error(err.Error())
		return err
	}
	sources, err := moduleadmission.NewSourceFetcher(moduleAdmissionFetcherConfig()).FetchAll(context.Background(), index.Modules, workspace)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	defer cleanupAdmissionSources(sources)
	requested, err := parseAdmissionReferences(strings.Split(ctx.Option("module"), ","), index.Modules)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	metadata, err := admissionSourceMetadata(sources)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	resolution, err := moduleadmission.Resolve(index, requested, metadata)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	lock, err := moduleadmission.NewAdmissionLock(index, resolution)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	registry, err := moduleadmission.GenerateStaticRegistry(lock)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	if ctx.OptionBool("prepare") {
		requesterID, parseErr := strconv.ParseUint(strings.TrimSpace(ctx.Option("requester-id")), 10, 64)
		if parseErr != nil || requesterID == 0 {
			return fmt.Errorf("module admission prepare requires positive --requester-id")
		}
		return writeAdmissionPreparation(requesterID, lock, registry)
	}
	if !ctx.OptionBool("evidence-stdin") {
		return fmt.Errorf("module admission requires --evidence-stdin and positive --requester-id")
	}
	pipeline := moduleadmission.AdmissionPipeline{Verifier: moduleadmission.NewCosignEvidenceVerifier(moduleadmission.NewSourceFetcher(moduleAdmissionFetcherConfig()), nil)}
	preparedDirectory, err := os.MkdirTemp(workspace, ".prepared-admission-")
	if err != nil {
		return err
	}
	evidencePayload, err := admissionEvidenceReader()
	if err != nil {
		return err
	}
	var stdinEvidence services.SensitiveOperationEvidence
	if err := json.Unmarshal(evidencePayload, &stdinEvidence); err != nil {
		return err
	}
	approval, plan, evidence, err := admissionApprovalInput(moduleAdmissionOptions{
		ApprovalID: stdinEvidence.ApprovalID, RequesterID: ctx.Option("requester-id"), ReAuthToken: stdinEvidence.ReAuthToken,
	}, lock, registry)
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	defer os.RemoveAll(preparedDirectory)
	result, err := pipeline.Run(context.Background(), moduleadmission.AdmissionPipelineInput{
		Index: index, Sources: sources, Requested: requested, Approval: approval,
		LockPath:     filepath.Join(preparedDirectory, "module-admission.lock.json"),
		RegistryPath: filepath.Join(preparedDirectory, "admitted_modules_gen.go"),
	})
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	err = services.NewSensitiveOperationGuard(nil).Execute(context.Background(), plan, evidence, func() error {
		return moduleadmission.WriteAdmissionArtifacts(result.Lock, result.Registry, ctx.Option("lock"), ctx.Option("registry"))
	})
	if err != nil {
		ctx.Error(err.Error())
		return err
	}
	payload, err = json.MarshalIndent(admissionCheckResult{IndexDigest: index.Digest, LockDigest: result.Lock.Digest, RegistryDigest: result.Registry.Digest, Modules: admissionSourceReports(sources), Evidence: result.Evidence}, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(payload, '\n'))
	return err
}

func writeAdmissionPreparation(requesterID uint64, lock moduleadmission.AdmissionLock, registry moduleadmission.StaticRegistry) error {
	moduleBinding := lock.ApprovalBinding(registry.Digest)
	plan, err := services.NewSensitiveOperationGuard(nil).Prepare(context.Background(), moduleadmission.AdmissionApprovalPolicy, requesterID, 0, services.SensitiveOperationPrepareInput{
		Resource: "module-admission:" + moduleBinding,
	})
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(map[string]string{
		"policy_key": moduleadmission.AdmissionApprovalPolicy, "resource": plan.Resource,
		"binding_digest": plan.BindingDigest, "module_binding": moduleBinding,
	}, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(payload, '\n'))
	return err
}

type moduleAdmissionOptions struct {
	ApprovalID  string
	RequesterID string
	ReAuthToken string
}

func admissionApprovalInput(options moduleAdmissionOptions, lock moduleadmission.AdmissionLock, registry moduleadmission.StaticRegistry) (moduleadmission.AdmissionApproval, services.SensitiveOperationPlan, services.SensitiveOperationEvidence, error) {
	approvalID := strings.TrimSpace(options.ApprovalID)
	requesterID, err := strconv.ParseUint(strings.TrimSpace(options.RequesterID), 10, 64)
	reAuthToken := strings.TrimSpace(options.ReAuthToken)
	if approvalID == "" || err != nil || requesterID == 0 || reAuthToken == "" {
		return moduleadmission.AdmissionApproval{}, services.SensitiveOperationPlan{}, services.SensitiveOperationEvidence{}, fmt.Errorf("module admission evidence requires approval_id, reauth_token, and positive --requester-id")
	}
	moduleBinding := lock.ApprovalBinding(registry.Digest)
	plan, err := services.NewSensitiveOperationGuard(nil).Prepare(context.Background(), moduleadmission.AdmissionApprovalPolicy, requesterID, 0, services.SensitiveOperationPrepareInput{
		Resource: "module-admission:" + moduleBinding,
	})
	if err != nil {
		return moduleadmission.AdmissionApproval{}, services.SensitiveOperationPlan{}, services.SensitiveOperationEvidence{}, err
	}
	approval, err := admissionApprovalLoader(context.Background(), approvalID, requesterID, moduleBinding, plan)
	if err != nil {
		return moduleadmission.AdmissionApproval{}, services.SensitiveOperationPlan{}, services.SensitiveOperationEvidence{}, err
	}
	if err := moduleadmission.ValidateAdmissionApproval(approval, lock, registry); err != nil {
		return moduleadmission.AdmissionApproval{}, services.SensitiveOperationPlan{}, services.SensitiveOperationEvidence{}, err
	}
	return approval, plan, services.SensitiveOperationEvidence{ReAuthToken: reAuthToken, ApprovalID: approvalID}, nil
}

func loadAdmissionApproval(ctx context.Context, approvalID string, requesterID uint64, moduleBinding string, plan services.SensitiveOperationPlan) (moduleadmission.AdmissionApproval, error) {
	security := services.NewEnterpriseSecurityControlService()
	record, err := security.PlatformApproval(ctx, approvalID)
	if err != nil {
		return moduleadmission.AdmissionApproval{}, err
	}
	if record.RequesterID != requesterID || record.PolicyKey != moduleadmission.AdmissionApprovalPolicy || record.BindingDigest != plan.BindingDigest || record.Resource != plan.Resource || record.Status != "approved" || !record.UsedAt.IsZero() || (!record.ExpiresAt.IsZero() && !record.ExpiresAt.After(time.Now())) {
		return moduleadmission.AdmissionApproval{}, fmt.Errorf("module admission approval is not valid for current artifacts")
	}
	return moduleadmission.AdmissionApproval{ID: record.ApprovalID, PolicyKey: record.PolicyKey, BindingDigest: moduleBinding, Approved: true, ExpiresAt: record.ExpiresAt}, nil
}

func setAdmissionApprovalLoaderForTest(loader func(context.Context, string, uint64, string, services.SensitiveOperationPlan) (moduleadmission.AdmissionApproval, error)) func() {
	original := admissionApprovalLoader
	admissionApprovalLoader = loader
	return func() { admissionApprovalLoader = original }
}

func checkAdmissionSources(ctx context.Context, fetcher moduleadmission.SourceFetcher, entries []moduleadmission.ModuleIndexEntry, workspace string) ([]admissionCheckReport, error) {
	sources, err := fetcher.FetchAll(ctx, entries, workspace)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, source := range sources {
			source.Result.Cleanup()
		}
	}()
	reports := make([]admissionCheckReport, 0, len(sources))
	for _, source := range sources {
		reports = append(reports, admissionCheckReport{ID: source.Entry.ID, Version: source.Entry.Version, Digest: source.Result.Digest, Size: source.Result.Size})
	}
	return reports, nil
}

func cleanupAdmissionSources(sources []moduleadmission.FetchedSource) {
	for _, source := range sources {
		if source.Result.Cleanup != nil {
			source.Result.Cleanup()
		}
	}
}

func admissionSourceReports(sources []moduleadmission.FetchedSource) []admissionCheckReport {
	reports := make([]admissionCheckReport, 0, len(sources))
	for _, source := range sources {
		reports = append(reports, admissionCheckReport{ID: source.Entry.ID, Version: source.Entry.Version, Digest: source.Result.Digest, Size: source.Result.Size})
	}
	return reports
}

func admissionSourceMetadata(sources []moduleadmission.FetchedSource) ([]moduleadmission.SourceModuleMetadata, error) {
	metadata := make([]moduleadmission.SourceModuleMetadata, 0, len(sources))
	for _, source := range sources {
		if source.Result.SourceDir == "" {
			continue
		}
		item, err := moduleadmission.ReadSourceModuleMetadata(source.Result.SourceDir)
		if err != nil {
			return nil, err
		}
		metadata = append(metadata, item)
	}
	return metadata, nil
}

func parseAdmissionReferences(values []string, entries []moduleadmission.ModuleIndexEntry) ([]moduleadmission.ModuleReference, error) {
	if len(values) == 0 || (len(values) == 1 && strings.TrimSpace(values[0]) == "") {
		references := make([]moduleadmission.ModuleReference, 0, len(entries))
		for _, entry := range entries {
			references = append(references, moduleadmission.ModuleReference{ID: entry.ID, Version: entry.Version})
		}
		return references, nil
	}
	references := make([]moduleadmission.ModuleReference, 0, len(values))
	for _, value := range values {
		parts := strings.Split(strings.TrimSpace(value), "@")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("module reference must be id@version: %s", value)
		}
		references = append(references, moduleadmission.ModuleReference{ID: parts[0], Version: parts[1]})
	}
	return references, nil
}

type admissionCheckReport struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
	Size    int64  `json:"size"`
}

type admissionCheckResult struct {
	IndexDigest    string                                 `json:"index_digest"`
	LockDigest     string                                 `json:"lock_digest"`
	RegistryDigest string                                 `json:"registry_digest"`
	Modules        []admissionCheckReport                 `json:"modules"`
	Evidence       []moduleadmission.VerificationEvidence `json:"evidence"`
}

func readAdmissionIndex(source, expectedDigest string) ([]byte, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		if !strings.HasPrefix(expectedDigest, "sha256:") || len(expectedDigest) != len("sha256:")+64 {
			return nil, fmt.Errorf("remote repository index requires --index-digest=sha256:<64 hex>")
		}
		entry := moduleadmission.ModuleIndexEntry{ID: "repository-index", Version: "1.0.0", SourceKind: "internal", SourceURI: source, Digest: strings.ToLower(expectedDigest)}
		fetcher := moduleadmission.NewSourceFetcher(moduleAdmissionFetcherConfig())
		workspace, err := os.MkdirTemp("", "module-index-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(workspace)
		result, err := fetcher.Fetch(context.Background(), entry, workspace)
		if err == nil {
			defer result.Cleanup()
			return os.ReadFile(result.BundlePath)
		}
		return nil, fmt.Errorf("fetch repository index: %w", err)
	}
	return os.ReadFile(filepath.Clean(source))
}

func moduleAdmissionFetcherConfig() moduleadmission.SourceFetcherConfig {
	timeout := facades.Config().GetDuration("module_admission.download_timeout", 30*time.Second)
	return moduleadmission.SourceFetcherConfig{
		AllowedHosts:    moduleAdmissionAllowedHosts(facades.Config().Get("module_admission.allowed_index_hosts", []string{})),
		MaxBundleBytes:  int64(facades.Config().GetInt("module_admission.max_bundle_bytes", 32<<20)),
		DownloadTimeout: timeout,
	}
}

func moduleAdmissionAllowedHosts(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		hosts := make([]string, 0, len(typed))
		for _, item := range typed {
			if host, ok := item.(string); ok && strings.TrimSpace(host) != "" {
				hosts = append(hosts, strings.TrimSpace(host))
			}
		}
		return hosts
	default:
		return nil
	}
}
