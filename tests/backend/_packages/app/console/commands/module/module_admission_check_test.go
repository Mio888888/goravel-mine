package module

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goravel/app/moduleadmission"
	"goravel/app/services"
)

func TestModuleAdmissionCheckCommandContract(t *testing.T) {
	command := &ModuleAdmissionCheckCommand{}
	if command.Signature() != "module:admission:check" {
		t.Fatalf("Signature() = %q", command.Signature())
	}
	flags := governanceFlags(t, command.Extend().Flags)
	if len(flags) != 9 || flags[0].Name != "index" || flags[1].Name != "index-digest" || flags[2].Name != "workspace" || flags[3].Name != "module" || flags[4].Name != "prepare" || flags[5].Name != "requester-id" || flags[6].Name != "evidence-stdin" || flags[7].Name != "lock" || flags[8].Name != "registry" {
		t.Fatalf("flags = %#v", flags)
	}
}

func TestModuleAdmissionCommandDoesNotExposeEvidenceInArgv(t *testing.T) {
	flags := governanceFlags(t, (&ModuleAdmissionCheckCommand{}).Extend().Flags)
	for _, flag := range flags {
		if flag.Name == "reauth-token" || flag.Name == "approval-id" {
			t.Fatalf("sensitive flag exposed in argv: %s", flag.Name)
		}
	}
}

func TestModuleAdmissionInputRequiresBindingApproval(t *testing.T) {
	_, _, _, err := admissionApprovalInput(moduleAdmissionOptions{ApprovalID: "", RequesterID: "1"}, moduleadmission.AdmissionLock{}, moduleadmission.StaticRegistry{})
	if err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("admissionApprovalInput() error = %v", err)
	}
}

func TestModuleAdmissionInputBindsApprovedRecord(t *testing.T) {
	lock := moduleadmission.AdmissionLock{IndexDigest: "sha256:index", SourceDigest: "sha256:source", DependencyGraphDigest: "sha256:graph"}
	registry := moduleadmission.StaticRegistry{Digest: "sha256:registry"}
	approvedAt := time.Now().Add(time.Minute)
	restore := setAdmissionApprovalLoaderForTest(func(_ context.Context, approvalID string, requesterID uint64, _ string, _ services.SensitiveOperationPlan) (moduleadmission.AdmissionApproval, error) {
		if approvalID != "approved" || requesterID != 9 {
			t.Fatalf("approval lookup = %q/%d", approvalID, requesterID)
		}
		return moduleadmission.AdmissionApproval{ID: approvalID, PolicyKey: moduleadmission.AdmissionApprovalPolicy, BindingDigest: lock.ApprovalBinding(registry.Digest), Approved: true, ExpiresAt: approvedAt}, nil
	})
	defer restore()
	approval, _, _, err := admissionApprovalInput(moduleAdmissionOptions{ApprovalID: "approved", RequesterID: "9", ReAuthToken: "reauth"}, lock, registry)
	if err != nil || !approval.Approved {
		t.Fatalf("admissionApprovalInput() = %#v, err = %v", approval, err)
	}
}

func TestCheckAdmissionSourcesReturnsNormalizedDigestReport(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(workspace, "module.txt")
	if err := os.WriteFile(source, []byte("approved source"), 0600); err != nil {
		t.Fatal(err)
	}
	digest := digestForAdmissionTest(t, source)
	reports, err := checkAdmissionSources(context.Background(), moduleadmission.NewSourceFetcher(moduleadmission.SourceFetcherConfig{}), []moduleadmission.ModuleIndexEntry{{ID: "alpha", Version: "1.0.0", SourceKind: "internal", SourceURI: source, Digest: digest}}, workspace)
	if err != nil || len(reports) != 1 || reports[0].Digest != digest {
		t.Fatalf("checkAdmissionSources() = %#v, err = %v", reports, err)
	}
}

func digestForAdmissionTest(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", digest[:])
}

func TestReadAdmissionIndexRequiresDigestForRemoteSource(t *testing.T) {
	_, err := readAdmissionIndex("https://registry.example/index.json", "")
	if err == nil || !strings.Contains(err.Error(), "--index-digest") {
		t.Fatalf("readAdmissionIndex() error = %v", err)
	}
}
