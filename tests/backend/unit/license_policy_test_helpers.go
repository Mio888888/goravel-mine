package unit

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func runLicensePolicy(t *testing.T, root, workdir, sbom, overrides string) ([]byte, error) {
	t.Helper()
	return runLicensePolicyWithReviews(t, root, workdir, sbom, overrides, filepath.Join(workdir, "license-reviews.json"))
}

func runLicensePolicyWithReviews(t *testing.T, root, workdir, sbom, overrides, reviews string) ([]byte, error) {
	t.Helper()
	sbomJSON, err := json.Marshal([]string{sbom})
	require.NoError(t, err)
	cmd := exec.Command("bash", filepath.Join(root, "scripts/check-license-policy.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"LICENSE_POLICY_FILE="+filepath.Join(root, "config/license_policy.yml"),
		"LICENSE_REPORT_ARTIFACT="+filepath.Join(workdir, "license-policy.json"),
		"LICENSE_METADATA_OVERRIDES_FILE="+overrides,
		"LICENSE_REVIEWS_FILE="+reviews,
		"VULNERABILITY_EXCEPTIONS_FILE="+filepath.Join(workdir, "exceptions.json"),
		"SBOM_ARTIFACTS_JSON="+string(sbomJSON),
	)
	return cmd.CombinedOutput()
}

func writeCycloneDXSBOM(t *testing.T, path string, license string) {
	t.Helper()
	writeJSONFile(t, path, map[string]any{
		"bomFormat": "CycloneDX",
		"components": []map[string]any{{
			"type": "library", "bom-ref": "pkg:generic/example@1.0.0", "name": "example", "version": "1.0.0",
			"licenses": []map[string]any{{"license": map[string]any{"id": license}}},
		}},
	})
}

func writeCycloneDXSBOMWithoutLicense(t *testing.T, path, ref, name, version string) {
	t.Helper()
	writeJSONFile(t, path, map[string]any{
		"bomFormat":  "CycloneDX",
		"components": []map[string]any{{"type": "library", "bom-ref": ref, "purl": ref, "name": name, "version": version}},
	})
}

func writeLicenseOverrides(t *testing.T, path, selector, version, license, evidence string) {
	t.Helper()
	writeJSONFile(t, path, map[string]any{"overrides": []map[string]any{{
		"purl": selector, "version": version, "license": license,
		"owner": "platform-security", "evidence": evidence,
	}}})
}

func writePatternLicenseOverride(t *testing.T, path, pattern, version, license, evidence string) {
	t.Helper()
	writeJSONFile(t, path, map[string]any{"overrides": []map[string]any{{
		"purl_pattern": pattern, "version": version, "license": license,
		"owner": "platform-security", "evidence": evidence,
	}}})
}

func writeLicenseReviews(t *testing.T, path, purl, version, license string) {
	t.Helper()
	writeJSONFile(t, path, map[string]any{"reviews": []map[string]any{{
		"purl": purl, "version": version, "license": license, "status": "approved",
		"owner": "platform-security", "approval_id": "APR-123", "evidence": "https://change.example/APR-123",
	}}})
}

func writeJSONFile(t *testing.T, path string, payload any) {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	return payload
}
