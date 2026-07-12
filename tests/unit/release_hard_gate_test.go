package unit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleaseHardGateUsesLocalApproverOutsideGitHubActions(t *testing.T) {
	root := repositoryRoot(t)
	workdir := t.TempDir()
	rollback := filepath.Join(workdir, "rollback.txt")
	dependencyPolicy := filepath.Join(workdir, "dependency-policy.json")
	compatibilityMatrix := filepath.Join(workdir, "module-compatibility-matrix.json")
	metadataVerifier := writeReleaseEvidenceMetadataVerifier(t, workdir)
	writeReleaseRollbackEvidence(t, rollback, root)
	require.NoError(t, os.WriteFile(dependencyPolicy, []byte(`{"status":"passed"}`+"\n"), 0644))
	require.NoError(t, os.WriteFile(compatibilityMatrix, []byte(`{"status":"passed","framework_version":"1.17.2","modules":[{"id":"platform-rbac","enabled":true,"framework_compatible":true}]}`+"\n"), 0644))

	cmd := exec.Command("bash", filepath.Join(root, "scripts/release-hard-gate.sh"))
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(),
		"GITHUB_ACTIONS=",
		"CHANGE_TICKET=CHG-123",
		"RELEASE_APPROVER=platform-approver",
		"ROLLBACK_DRILL_ARTIFACT="+rollback,
		"SLO_OBSERVATION_ARTIFACT=artifact://change/CHG-123/slo",
		"RELEASE_EVIDENCE_METADATA_VERIFIER="+metadataVerifier,
		"DEPENDENCY_POLICY_ARTIFACT="+dependencyPolicy,
		"COMPATIBILITY_MATRIX_ARTIFACT="+compatibilityMatrix,
	)

	output, err := cmd.CombinedOutput()

	require.NoError(t, err, string(output))
	require.FileExists(t, filepath.Join(workdir, "artifacts/release-hard-gate.json"))
	require.FileExists(t, filepath.Join(workdir, "artifacts/dependency-policy.json"))
	require.NoFileExists(t, filepath.Join(workdir, "artifacts/dependency-policy.txt"))
	manifest := readJSONMap(t, filepath.Join(workdir, "artifacts/release-hard-gate.json"))
	artifacts, ok := manifest["artifacts"].(map[string]any)
	require.True(t, ok)
	dependency, ok := artifacts["dependency_policy"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "artifacts/dependency-policy.json", dependency["path"])
}

func TestReleasePredeployGateDoesNotRequireSLOEvidence(t *testing.T) {
	root := repositoryRoot(t)
	workdir := t.TempDir()
	rollback := filepath.Join(workdir, "rollback.json")
	dependencyPolicy := filepath.Join(workdir, "dependency-policy.json")
	compatibilityMatrix := filepath.Join(workdir, "module-compatibility-matrix.json")
	writeReleaseRollbackEvidence(t, rollback, root)
	require.NoError(t, os.WriteFile(dependencyPolicy, []byte(`{"status":"passed"}`+"\n"), 0644))
	require.NoError(t, os.WriteFile(compatibilityMatrix, []byte(`{"status":"passed","framework_version":"1.17.2","modules":[{"id":"platform-rbac","enabled":true,"framework_compatible":true}]}`+"\n"), 0644))

	cmd := exec.Command("bash", filepath.Join(root, "scripts/release-hard-gate.sh"))
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(),
		"GITHUB_ACTIONS=",
		"RELEASE_GATE_PHASE=predeploy",
		"CHANGE_TICKET=CHG-123",
		"RELEASE_APPROVER=platform-approver",
		"ROLLBACK_DRILL_ARTIFACT="+rollback,
		"SLO_OBSERVATION_ARTIFACT=",
		"DEPENDENCY_POLICY_ARTIFACT="+dependencyPolicy,
		"COMPATIBILITY_MATRIX_ARTIFACT="+compatibilityMatrix,
	)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	require.FileExists(t, filepath.Join(workdir, "artifacts/release-predeploy-gate.json"))
	require.NoFileExists(t, filepath.Join(workdir, "artifacts/slo-observation.json"))
	manifest := readJSONMap(t, filepath.Join(workdir, "artifacts/release-predeploy-gate.json"))
	require.Equal(t, "predeploy", manifest["phase"])
}

func TestReleaseHardGateRejectsIncompatibleEnabledModule(t *testing.T) {
	root := repositoryRoot(t)
	workdir := t.TempDir()
	rollback := filepath.Join(workdir, "rollback.txt")
	dependencyPolicy := filepath.Join(workdir, "dependency-policy.json")
	compatibilityMatrix := filepath.Join(workdir, "module-compatibility-matrix.json")
	metadataVerifier := writeReleaseEvidenceMetadataVerifier(t, workdir)
	writeReleaseRollbackEvidence(t, rollback, root)
	require.NoError(t, os.WriteFile(dependencyPolicy, []byte(`{"status":"passed"}`+"\n"), 0644))
	require.NoError(t, os.WriteFile(compatibilityMatrix, []byte(`{"status":"passed","framework_version":"1.17.2","modules":[{"id":"external-audit","enabled":true,"framework_compatible":false}]}`+"\n"), 0644))

	cmd := exec.Command("bash", filepath.Join(root, "scripts/release-hard-gate.sh"))
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(),
		"GITHUB_ACTIONS=",
		"CHANGE_TICKET=CHG-123",
		"RELEASE_APPROVER=platform-approver",
		"ROLLBACK_DRILL_ARTIFACT="+rollback,
		"SLO_OBSERVATION_ARTIFACT=artifact://change/CHG-123/slo",
		"RELEASE_EVIDENCE_METADATA_VERIFIER="+metadataVerifier,
		"DEPENDENCY_POLICY_ARTIFACT="+dependencyPolicy,
		"COMPATIBILITY_MATRIX_ARTIFACT="+compatibilityMatrix,
	)

	output, err := cmd.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "enabled modules must be framework compatible: external-audit")
}

func TestReleaseHardGateRequiresGitHubApprovalLookupInActions(t *testing.T) {
	root := repositoryRoot(t)
	workdir := t.TempDir()
	rollback := filepath.Join(workdir, "rollback.txt")
	dependencyPolicy := filepath.Join(workdir, "dependency-policy.json")
	metadataVerifier := writeReleaseEvidenceMetadataVerifier(t, workdir)
	writeReleaseRollbackEvidence(t, rollback, root)
	require.NoError(t, os.WriteFile(dependencyPolicy, []byte(`{"status":"passed"}`+"\n"), 0644))

	cmd := exec.Command("bash", filepath.Join(root, "scripts/release-hard-gate.sh"))
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(),
		"GITHUB_ACTIONS=true",
		"GITHUB_TOKEN=",
		"GITHUB_REPOSITORY=vant/goravel-mine",
		"GITHUB_RUN_ID=123",
		"CHANGE_TICKET=CHG-123",
		"RELEASE_APPROVER=platform-approver",
		"ROLLBACK_DRILL_ARTIFACT="+rollback,
		"SLO_OBSERVATION_ARTIFACT=artifact://change/CHG-123/slo",
		"RELEASE_EVIDENCE_METADATA_VERIFIER="+metadataVerifier,
		"DEPENDENCY_POLICY_ARTIFACT="+dependencyPolicy,
	)

	output, err := cmd.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "GITHUB_TOKEN is required")
}

func TestReleaseHardGateRejectsFailedDependencyPolicy(t *testing.T) {
	root := repositoryRoot(t)
	workdir := t.TempDir()
	rollback := filepath.Join(workdir, "rollback.txt")
	dependencyPolicy := filepath.Join(workdir, "dependency-policy.json")
	metadataVerifier := writeReleaseEvidenceMetadataVerifier(t, workdir)
	writeReleaseRollbackEvidence(t, rollback, root)
	require.NoError(t, os.WriteFile(dependencyPolicy, []byte(`{"status":"failed"}`+"\n"), 0644))

	cmd := exec.Command("bash", filepath.Join(root, "scripts/release-hard-gate.sh"))
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(),
		"GITHUB_ACTIONS=",
		"CHANGE_TICKET=CHG-123",
		"RELEASE_APPROVER=platform-approver",
		"ROLLBACK_DRILL_ARTIFACT="+rollback,
		"SLO_OBSERVATION_ARTIFACT=artifact://change/CHG-123/slo",
		"RELEASE_EVIDENCE_METADATA_VERIFIER="+metadataVerifier,
		"DEPENDENCY_POLICY_ARTIFACT="+dependencyPolicy,
	)

	output, err := cmd.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "DEPENDENCY_POLICY_ARTIFACT status must be passed")
}

func TestReleaseHardGateRejectsExternalDependencyPolicyEvidence(t *testing.T) {
	root := repositoryRoot(t)
	workdir := t.TempDir()
	rollback := filepath.Join(workdir, "rollback.txt")
	metadataVerifier := writeReleaseEvidenceMetadataVerifier(t, workdir)
	writeReleaseRollbackEvidence(t, rollback, root)

	cmd := exec.Command("bash", filepath.Join(root, "scripts/release-hard-gate.sh"))
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(),
		"GITHUB_ACTIONS=",
		"CHANGE_TICKET=CHG-123",
		"RELEASE_APPROVER=platform-approver",
		"ROLLBACK_DRILL_ARTIFACT="+rollback,
		"SLO_OBSERVATION_ARTIFACT=artifact://change/CHG-123/slo",
		"RELEASE_EVIDENCE_METADATA_VERIFIER="+metadataVerifier,
		"DEPENDENCY_POLICY_ARTIFACT=artifact://compliance/license-policy",
	)

	output, err := cmd.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "DEPENDENCY_POLICY_ARTIFACT must point to a non-empty local JSON artifact")
}

func TestReleaseWorkflowPassesApprovalLookupToken(t *testing.T) {
	root := repositoryRoot(t)
	workflow, err := os.ReadFile(filepath.Join(root, ".github/workflows/release.yml"))
	require.NoError(t, err)
	payload := string(workflow)

	releaseJob := payload[strings.Index(payload, "  release:"):]
	require.Contains(t, releaseJob, "actions: write")
	require.Contains(t, releaseJob, "GITHUB_TOKEN: ${{ github.token }}")
	require.Contains(t, releaseJob, "GH_TOKEN: ${{ github.token }}")
	require.Contains(t, releaseJob, "Generate backend dependency SBOM")
	require.Contains(t, releaseJob, "Check dependency license policy")
	require.Contains(t, releaseJob, "rm -rf artifacts/sbom artifacts/compliance")
	require.Contains(t, releaseJob, "VULNERABILITY_EXCEPTIONS_JSON: ${{ secrets.VULNERABILITY_EXCEPTIONS_JSON }}")
	require.Contains(t, releaseJob, "LICENSE_REVIEWS_JSON: ${{ secrets.LICENSE_REVIEWS_JSON }}")
	require.Contains(t, releaseJob, "VULNERABILITY_EXCEPTIONS_FILE: ${{ runner.temp }}/compliance-evidence/vulnerability-exceptions.json")
	require.Contains(t, releaseJob, "LICENSE_REVIEWS_FILE: ${{ runner.temp }}/compliance-evidence/license-reviews.json")
	require.Contains(t, releaseJob, `SBOM_ARTIFACTS_JSON: '["artifacts/sbom/backend-dependencies.cdx.json","artifacts/sbom/frontend-dependencies.cdx.json"]'`)
	require.Contains(t, releaseJob, "DEPENDENCY_POLICY_ARTIFACT: artifacts/compliance/license-policy.json")
	require.Contains(t, releaseJob, "Generate module compatibility matrix")
	require.Contains(t, releaseJob, "COMPATIBILITY_MATRIX_ARTIFACT: artifacts/module-compatibility-matrix.json")
	require.Contains(t, releaseJob, "module:compatibility:export")
	require.Contains(t, releaseJob, "artifacts/module-compatibility-matrix.json")
	require.Contains(t, releaseJob, "artifacts/dependency-policy.json")
	require.NotContains(t, releaseJob, "artifacts/dependency-policy.txt")
	require.NotContains(t, releaseJob, "artifacts/compliance/vulnerability-exceptions.json")
	require.NotContains(t, releaseJob, "SBOM_ARTIFACTS:")
	require.NotContains(t, payload, "dependency_policy_artifact:")
	require.NotContains(t, releaseJob, "DEPENDENCY_POLICY_ARTIFACT: ${{ inputs.dependency_policy_artifact }}")
	require.Equal(t, 5, strings.Count(releaseJob, "aquasecurity/trivy-action@ed142fd0673e97e23eac54620cfb913e5ce36c25"))
	require.Equal(t, 2, strings.Count(releaseJob, "scanners: vuln,license"))
	predeployIndex := strings.Index(releaseJob, "name: Enforce pre-deployment release hard gate")
	finalGateIndex := strings.Index(releaseJob, "name: Enforce final release hard gate")
	predeployStep := releaseJob[predeployIndex : strings.Index(releaseJob[predeployIndex+1:], "\n      - name:")+predeployIndex+1]
	buildIndex := strings.Index(releaseJob, "name: Build and push backend image")
	deployIndex := strings.Index(releaseJob, "name: Deploy digest-bound release to production")
	sloIndex := strings.Index(releaseJob, "name: Collect fixed-window production SLO evidence")
	publishIndex := strings.Index(releaseJob, "name: Publish GitHub release")
	require.True(t,
		strings.Index(releaseJob, "actions: write") < strings.Index(releaseJob, "steps:"),
		"release job must grant actions: write before hard gate step",
	)
	require.True(t,
		strings.Index(releaseJob, "Check dependency license policy") < predeployIndex,
		"dependency policy must run before pre-deployment hard gate",
	)
	require.True(t,
		predeployIndex < buildIndex && buildIndex < deployIndex && deployIndex < sloIndex && sloIndex < finalGateIndex && finalGateIndex < publishIndex,
		"pre-deployment gate must precede image publication and production mutation; final gate must follow SLO collection and precede publish",
	)
	require.Contains(t, predeployStep, "GITHUB_TOKEN: ${{ github.token }}")
	require.Contains(t, predeployStep, "GH_TOKEN: ${{ github.token }}")
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "repository root not found")
		dir = parent
	}
}

func writeReleaseEvidenceMetadataVerifier(t *testing.T, workdir string) string {
	t.Helper()
	path := filepath.Join(workdir, "verify-evidence.sh")
	content := `#!/usr/bin/env bash
set -eu
uri=""
sha=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --uri) uri="$2"; shift 2 ;;
    --git-sha) sha="$2"; shift 2 ;;
    *) exit 2 ;;
  esac
done
printf '{"uri":"%s","object_version":"v1","sha256":"%064d","immutable_until":"2099-01-01T00:00:00Z","verified_at":"2026-07-11T00:00:00Z","git_sha":"%s"}\n' "$uri" 0 "$sha"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0755))
	return path
}

func writeReleaseRollbackEvidence(t *testing.T, path, root string) {
	t.Helper()
	shaOutput, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	payload := map[string]any{
		"schema_version": 1,
		"evidence_type":  "rollback-drill",
		"git_sha":        strings.TrimSpace(string(shaOutput)),
		"digest":         "sha256:pending",
		"execution": map[string]any{
			"executed": true, "upgrade_run_key": "upgrade-test", "smoke": "passed", "rollback_run_key": "rollback-test",
		},
		"state_diff": map[string]any{"after_rollback": "in_sync"},
	}
	canonical, err := json.Marshal(payload)
	require.NoError(t, err)
	digest := sha256.Sum256(canonical)
	payload["digest"] = "sha256:" + hex.EncodeToString(digest[:])
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(raw, '\n'), 0644))
}
