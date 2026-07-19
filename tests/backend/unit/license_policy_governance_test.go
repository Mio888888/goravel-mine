package unit

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLicensePolicyAcceptsApprovedReviewRequiredLicense(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, reviews := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "reviews.json")
	writeCycloneDXSBOM(t, sbom, "BUSL-1.1")
	writeLicenseReviews(t, reviews, "pkg:generic/example@1.0.0", "1.0.0", "BUSL-1.1")
	output, err := runLicensePolicyWithReviews(t, root, workdir, sbom, "", reviews)
	require.NoError(t, err, string(output))
}

func TestLicenseReviewDoesNotApproveSameNamePackageFromAnotherEcosystem(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, reviews := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "reviews.json")
	writeJSONFile(t, sbom, map[string]any{"components": []map[string]any{
		{"type": "library", "bom-ref": "pkg:npm/example@1.0.0", "purl": "pkg:npm/example@1.0.0", "name": "example", "version": "1.0.0", "licenses": []map[string]any{{"license": map[string]any{"id": "BUSL-1.1"}}}},
		{"type": "library", "bom-ref": "pkg:golang/example@1.0.0", "purl": "pkg:golang/example@1.0.0", "name": "example", "version": "1.0.0", "licenses": []map[string]any{{"license": map[string]any{"id": "BUSL-1.1"}}}},
	}})
	writeLicenseReviews(t, reviews, "pkg:npm/example@1.0.0", "1.0.0", "BUSL-1.1")

	output, err := runLicensePolicyWithReviews(t, root, workdir, sbom, "", reviews)

	require.Error(t, err)
	require.Contains(t, string(output), "license policy violations found")
	findings := readJSONMap(t, filepath.Join(workdir, "license-policy.json"))["license_findings"].([]any)
	require.Len(t, findings, 1)
	require.Equal(t, "example", findings[0].(map[string]any)["component"])
}

func TestVulnerabilityExceptionDoesNotApproveSameNamePackageFromAnotherEcosystem(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, exceptions := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "exceptions.json")
	writeJSONFile(t, sbom, map[string]any{
		"components": []map[string]any{
			{"type": "library", "bom-ref": "pkg:npm/example@1.0.0", "purl": "pkg:npm/example@1.0.0", "name": "example", "version": "1.0.0", "licenses": []map[string]any{{"license": map[string]any{"id": "MIT"}}}},
			{"type": "library", "bom-ref": "pkg:golang/example@1.0.0", "purl": "pkg:golang/example@1.0.0", "name": "example", "version": "1.0.0", "licenses": []map[string]any{{"license": map[string]any{"id": "MIT"}}}},
		},
		"vulnerabilities": []map[string]any{{
			"id": "CVE-2026-0001", "ratings": []map[string]any{{"score": 9.8}},
			"affects": []map[string]any{{"ref": "pkg:npm/example@1.0.0"}, {"ref": "pkg:golang/example@1.0.0"}},
		}},
	})
	writeJSONFile(t, exceptions, map[string]any{"exceptions": []map[string]any{{
		"purl": "pkg:npm/example@1.0.0", "version": "1.0.0", "cve": "CVE-2026-0001", "cvss": 9.8,
		"status": "accepted-risk", "owner": "platform-security", "approval_id": "APR-1", "mitigation": "tracked",
		"expires_at": time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02"),
	}}})

	cmd := licensePolicyCommand(t, root, workdir, sbom, exceptions)
	output, err := cmd.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "high severity vulnerabilities require approved exceptions")
	findings := readJSONMap(t, filepath.Join(workdir, "license-policy.json"))["vulnerability_findings"].([]any)
	require.Len(t, findings, 1)
	require.Equal(t, "pkg:golang/example@1.0.0", findings[0].(map[string]any)["purl"])
}

func TestLicensePolicyRejectsUnapprovedReviewRequiredLicense(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom := filepath.Join(workdir, "sbom.cdx.json")
	writeCycloneDXSBOM(t, sbom, "BUSL-1.1")
	output, err := runLicensePolicy(t, root, workdir, sbom, "")
	require.Error(t, err)
	require.Contains(t, string(output), "license policy violations found")
}

func TestLicensePolicyEvaluatesCompoundExpressions(t *testing.T) {
	root := repositoryRoot(t)
	for _, test := range []struct {
		name, expression string
		wantError        bool
	}{
		{name: "allowed OR denied", expression: "MIT OR GPL-3.0-only"},
		{name: "allowed AND denied", expression: "MIT AND GPL-3.0-only", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			workdir, sbom := t.TempDir(), ""
			sbom = filepath.Join(workdir, "sbom.cdx.json")
			writeCycloneDXSBOM(t, sbom, test.expression)
			output, err := runLicensePolicy(t, root, workdir, sbom, "")
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err, string(output))
		})
	}
}

func TestLicensePolicyRejectsVulnerabilityExceptionWithoutExpiry(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, exceptions := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "exceptions.json")
	writeCycloneDXSBOM(t, sbom, "MIT")
	writeJSONFile(t, exceptions, map[string]any{"exceptions": []map[string]any{{
		"purl": "pkg:generic/example@1.0.0", "version": "1.0.0", "cve": "CVE-2026-0001",
		"status": "accepted-risk", "owner": "platform-security", "approval_id": "APR-1", "mitigation": "tracked",
	}}})
	cmd := licensePolicyCommand(t, root, workdir, sbom, exceptions)
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "missing expires_at")
}

func TestLicensePolicyRejectsVulnerabilityExceptionBeyondSLA(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, exceptions := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "exceptions.json")
	writeCycloneDXSBOM(t, sbom, "MIT")
	writeJSONFile(t, exceptions, map[string]any{"exceptions": []map[string]any{{
		"purl": "pkg:generic/example@1.0.0", "version": "1.0.0", "cve": "CVE-2026-0001",
		"status": "accepted-risk", "owner": "platform-security", "approval_id": "APR-1", "mitigation": "tracked",
		"expires_at": time.Now().UTC().AddDate(0, 0, 32).Format("2006-01-02"),
	}}})
	cmd := licensePolicyCommand(t, root, workdir, sbom, exceptions)
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "exceeds exception SLA")
}

func TestLicensePolicyRejectsIncompleteVulnerabilityException(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, exceptions := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "exceptions.json")
	writeCycloneDXSBOM(t, sbom, "MIT")
	writeJSONFile(t, exceptions, map[string]any{"exceptions": []map[string]any{{
		"purl": "", "version": "1.0.0", "cve": "",
		"status": "accepted-risk", "owner": "platform-security", "approval_id": "APR-1", "mitigation": "tracked",
		"expires_at": time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02"),
	}}})
	cmd := licensePolicyCommand(t, root, workdir, sbom, exceptions)
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "missing purl, version, cve, or cvss")
}

func licensePolicyCommand(t *testing.T, root, workdir, sbom, exceptions string) *exec.Cmd {
	t.Helper()
	sbomJSON, err := json.Marshal([]string{sbom})
	require.NoError(t, err)
	cmd := exec.Command("bash", filepath.Join(root, "scripts/check-license-policy.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"LICENSE_POLICY_FILE="+filepath.Join(root, "config/license_policy.yml"),
		"LICENSE_REPORT_ARTIFACT="+filepath.Join(workdir, "license-policy.json"),
		"LICENSE_REVIEWS_FILE="+filepath.Join(workdir, "license-reviews.json"),
		"VULNERABILITY_EXCEPTIONS_FILE="+exceptions,
		"SBOM_ARTIFACTS_JSON="+string(sbomJSON),
	)
	return cmd
}
