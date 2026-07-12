package moduleadmission

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdmissionLockAndRegistryAreDeterministicAndApprovalBound(t *testing.T) {
	index := mustIndex(t, `{"schema_version":"v1","modules":[{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","go_import_path":"example.com/modules/alpha"}]}`)
	resolution, err := Resolve(index, []ModuleReference{{ID: "alpha", Version: "1.0.0"}}, []SourceModuleMetadata{{ID: "alpha", Version: "1.0.0", GoImportPath: "example.com/modules/alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	lock, err := NewAdmissionLock(index, resolution)
	if err != nil {
		t.Fatal(err)
	}
	first, err := lock.JSON()
	if err != nil {
		t.Fatal(err)
	}
	second, err := lock.JSON()
	if err != nil || string(first) != string(second) {
		t.Fatalf("lock JSON differs: %s / %s, err = %v", first, second, err)
	}
	registry, err := GenerateStaticRegistry(lock)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(registry.Source, "plugin.Open") || strings.Contains(registry.Source, "reflect.") {
		t.Fatalf("registry uses runtime loading: %s", registry.Source)
	}
	if err := lock.VerifyGeneratedRegistry(registry); err != nil {
		t.Fatalf("VerifyGeneratedRegistry() error = %v", err)
	}
	registry.Source += "// altered\n"
	if err := lock.VerifyGeneratedRegistry(registry); err == nil {
		t.Fatal("changed registry was accepted")
	}
	registry, err = GenerateStaticRegistry(lock)
	if err != nil {
		t.Fatal(err)
	}
	approval := AdmissionApproval{ID: "approval-1", PolicyKey: AdmissionApprovalPolicy, BindingDigest: lock.ApprovalBinding(registry.Digest), Approved: true}
	if err := ValidateAdmissionApproval(approval, lock, registry); err != nil {
		t.Fatalf("ValidateAdmissionApproval() error = %v", err)
	}
	approval.BindingDigest = "sha256:" + strings.Repeat("0", 64)
	if err := ValidateAdmissionApproval(approval, lock, registry); err == nil {
		t.Fatal("changed approval binding was accepted")
	}
}

func TestWriteAdmissionArtifactsReplacesBothOutputsAfterCompleteGeneration(t *testing.T) {
	index := mustIndex(t, `{"schema_version":"v1","modules":[{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","go_import_path":"example.com/modules/alpha"}]}`)
	resolution, err := Resolve(index, []ModuleReference{{ID: "alpha", Version: "1.0.0"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := NewAdmissionLock(index, resolution)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := GenerateStaticRegistry(lock)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	lockPath := filepath.Join(directory, "module-admission.lock.json")
	registryPath := filepath.Join(directory, "admitted_modules_gen.go")
	if err := WriteAdmissionArtifacts(lock, registry, lockPath, registryPath); err != nil {
		t.Fatalf("WriteAdmissionArtifacts() error = %v", err)
	}
	if payload, err := os.ReadFile(registryPath); err != nil || string(payload) != registry.Source {
		t.Fatalf("registry payload = %q, err = %v", payload, err)
	}
}

func TestWriteAdmissionArtifactsRestoresRegistryWhenLockCommitFails(t *testing.T) {
	index := mustIndex(t, `{"schema_version":"v1","modules":[{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","go_import_path":"example.com/modules/alpha"}]}`)
	resolution, err := Resolve(index, []ModuleReference{{ID: "alpha", Version: "1.0.0"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := NewAdmissionLock(index, resolution)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := GenerateStaticRegistry(lock)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	registryPath := filepath.Join(directory, "admitted_modules_gen.go")
	lockPath := filepath.Join(directory, "module-admission.lock.json")
	previous := []byte("previous registry\n")
	if err := os.WriteFile(registryPath, previous, 0600); err != nil {
		t.Fatal(err)
	}
	renameCalls := 0
	originalRename := admissionRename
	admissionRename = func(oldPath, newPath string) error {
		renameCalls++
		if renameCalls == 2 {
			return os.ErrPermission
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() { admissionRename = originalRename })

	err = WriteAdmissionArtifacts(lock, registry, lockPath, registryPath)

	if err == nil || !strings.Contains(err.Error(), "replace admission lock") {
		t.Fatalf("WriteAdmissionArtifacts() error = %v", err)
	}
	actual, readErr := os.ReadFile(registryPath)
	if readErr != nil || string(actual) != string(previous) {
		t.Fatalf("registry was not restored: %q, err = %v", actual, readErr)
	}
}

func TestAdmissionPipelineFailsClosedWithoutApprovedBinding(t *testing.T) {
	index := mustIndex(t, `{"schema_version":"v1","modules":[{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","go_import_path":"example.com/modules/alpha"}]}`)
	source := FetchedSource{
		Entry:  index.Modules[0],
		Result: SourceFetchResult{Digest: index.Modules[0].Digest},
	}
	target := filepath.Join(t.TempDir(), "admitted_modules_gen.go")
	previous := []byte("previous registry\n")
	if err := os.WriteFile(target, previous, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := RunAdmissionPipeline(context.Background(), AdmissionPipelineInput{
		Index:        index,
		Sources:      []FetchedSource{source},
		Requested:    []ModuleReference{{ID: "alpha", Version: "1.0.0"}},
		Approval:     AdmissionApproval{},
		LockPath:     filepath.Join(t.TempDir(), "module-admission.lock.json"),
		RegistryPath: target,
	})
	if err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("RunAdmissionPipeline() error = %v, want approval failure", err)
	}
	actual, err := os.ReadFile(target)
	if err != nil || string(actual) != string(previous) {
		t.Fatalf("registry changed after failed admission: %q, err = %v", actual, err)
	}
}

func TestAdmissionPipelineVerifiesExternalEvidenceBeforeWritingArtifacts(t *testing.T) {
	index := mustIndex(t, `{"schema_version":"v1","modules":[{"id":"alpha","version":"1.0.0","source_kind":"external","source_uri":"alpha.zip","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","cosign_issuer":"https://issuer.example","cosign_identity":"build@example","signature_uri":"alpha.sig","sbom_digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","sbom_uri":"alpha.sbom","provenance_digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","provenance_uri":"alpha.provenance","go_import_path":"example.com/modules/alpha"}]}`)
	resolution, err := Resolve(index, []ModuleReference{{ID: "alpha", Version: "1.0.0"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := NewAdmissionLock(index, resolution)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := GenerateStaticRegistry(lock)
	if err != nil {
		t.Fatal(err)
	}
	pipeline := AdmissionPipeline{Verifier: admissionVerifierFunc(func(context.Context, FetchedSource) (VerificationEvidence, error) {
		return VerificationEvidence{}, errAdmissionEvidenceRejected
	})}
	_, err = pipeline.Run(context.Background(), AdmissionPipelineInput{
		Index: index,
		Sources: []FetchedSource{{Entry: index.Modules[0], Result: SourceFetchResult{
			Digest: index.Modules[0].Digest,
		}}},
		Requested: []ModuleReference{{ID: "alpha", Version: "1.0.0"}},
		Approval: AdmissionApproval{
			ID: "approval-1", PolicyKey: AdmissionApprovalPolicy,
			BindingDigest: lock.ApprovalBinding(registry.Digest), Approved: true,
		},
		LockPath:     filepath.Join(t.TempDir(), "module-admission.lock.json"),
		RegistryPath: filepath.Join(t.TempDir(), "admitted_modules_gen.go"),
	})
	if err == nil || !strings.Contains(err.Error(), errAdmissionEvidenceRejected.Error()) {
		t.Fatalf("pipeline.Run() error = %v", err)
	}
}

func TestAdmissionPipelineWritesApprovedDeterministicArtifacts(t *testing.T) {
	index := mustIndex(t, `{"schema_version":"v1","modules":[{"id":"alpha","version":"1.0.0","source_kind":"internal","source_uri":"alpha","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","go_import_path":"example.com/modules/alpha"}]}`)
	resolution, err := Resolve(index, []ModuleReference{{ID: "alpha", Version: "1.0.0"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := NewAdmissionLock(index, resolution)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := GenerateStaticRegistry(lock)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	result, err := RunAdmissionPipeline(context.Background(), AdmissionPipelineInput{
		Index:     index,
		Sources:   []FetchedSource{{Entry: index.Modules[0], Result: SourceFetchResult{Digest: index.Modules[0].Digest}}},
		Requested: []ModuleReference{{ID: "alpha", Version: "1.0.0"}},
		Approval: AdmissionApproval{
			ID: "approval-1", PolicyKey: AdmissionApprovalPolicy,
			BindingDigest: lock.ApprovalBinding(registry.Digest), Approved: true,
		},
		LockPath:     filepath.Join(directory, "module-admission.lock.json"),
		RegistryPath: filepath.Join(directory, "admitted_modules_gen.go"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Lock.Digest != lock.Digest || result.Registry.Digest != registry.Digest {
		t.Fatalf("pipeline result = %#v", result)
	}
}

var errAdmissionEvidenceRejected = admissionTestError("external evidence rejected")

type admissionTestError string

func (e admissionTestError) Error() string { return string(e) }

type admissionVerifierFunc func(context.Context, FetchedSource) (VerificationEvidence, error)

func (f admissionVerifierFunc) Verify(ctx context.Context, source FetchedSource) (VerificationEvidence, error) {
	return f(ctx, source)
}

func TestAdmissionApprovalRejectsExpiredEvidence(t *testing.T) {
	approval := AdmissionApproval{ID: "approval-1", PolicyKey: AdmissionApprovalPolicy, Approved: true, ExpiresAt: time.Now().Add(-time.Minute)}
	if err := approval.Valid(); err == nil {
		t.Fatal("expired approval was accepted")
	}
}
