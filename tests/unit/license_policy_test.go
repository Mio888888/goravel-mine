package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLicensePolicyRequiresSBOMArtifacts(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	output, err := runLicensePolicy(t, root, workdir, filepath.Join(workdir, "missing.cdx.json"), "")
	require.Error(t, err)
	require.Contains(t, string(output), "SBOM artifact is required")
}

func TestLicensePolicyAcceptsAllowedSBOMLicenses(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom := filepath.Join(workdir, "sbom.cdx.json")
	writeCycloneDXSBOM(t, sbom, "MIT")
	output, err := runLicensePolicy(t, root, workdir, sbom, "")
	require.NoError(t, err, string(output))
}

func TestLicensePolicyAcceptsSBOMPathContainingSpaces(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbomDir := filepath.Join(workdir, "sbom artifacts")
	require.NoError(t, os.MkdirAll(sbomDir, 0755))
	sbom := filepath.Join(sbomDir, "frontend dependencies.cdx.json")
	writeCycloneDXSBOM(t, sbom, "MIT")

	output, err := runLicensePolicy(t, root, workdir, sbom, "")

	require.NoError(t, err, string(output))
}

func TestLicensePolicyAcceptsStandardPermissiveLicenses(t *testing.T) {
	root := repositoryRoot(t)
	for _, license := range []string{"0BSD", "BlueOak-1.0.0", "FTL", "Python-2.0"} {
		t.Run(license, func(t *testing.T) {
			workdir := t.TempDir()
			sbom := filepath.Join(workdir, "sbom.cdx.json")
			writeCycloneDXSBOM(t, sbom, license)
			output, err := runLicensePolicy(t, root, workdir, sbom, "")
			require.NoError(t, err, string(output))
		})
	}
}

func TestLicensePolicyRejectsDeniedSBOMLicenses(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom := filepath.Join(workdir, "sbom.cdx.json")
	writeCycloneDXSBOM(t, sbom, "GPL-3.0-only")
	output, err := runLicensePolicy(t, root, workdir, sbom, "")
	require.Error(t, err)
	require.Contains(t, string(output), "license policy violations found")
	require.Equal(t, "failed", readJSONMap(t, filepath.Join(workdir, "license-policy.json"))["status"])
}

func TestLicensePolicyExcludesSBOMRootComponent(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom := filepath.Join(workdir, "sbom.cdx.json")
	writeJSONFile(t, sbom, map[string]any{
		"metadata": map[string]any{"component": map[string]any{"bom-ref": "root"}},
		"components": []map[string]any{
			{"type": "application", "bom-ref": "root", "name": "root"},
			{"type": "library", "bom-ref": "dep", "name": "dep", "version": "1.0.0", "licenses": []map[string]any{{"license": map[string]any{"id": "MIT"}}}},
		},
	})
	output, err := runLicensePolicy(t, root, workdir, sbom, "")
	require.NoError(t, err, string(output))
	report := readJSONMap(t, filepath.Join(workdir, "license-policy.json"))
	require.EqualValues(t, 1, report["component_count"])
	require.EqualValues(t, 1, report["excluded_root_component_count"])
}

func TestLicensePolicyExcludesTrivyManifestAndFirstPartyComponents(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom := filepath.Join(workdir, "sbom.cdx.json")
	writeJSONFile(t, sbom, map[string]any{"components": []map[string]any{
		{"type": "application", "bom-ref": "manifest", "name": "yarn.lock", "properties": []map[string]any{{"name": "aquasecurity:trivy:Class", "value": "lang-pkgs"}}},
		{"type": "library", "bom-ref": "pkg:npm/mineadmin-ui@3.2.1", "purl": "pkg:npm/mineadmin-ui@3.2.1", "name": "mineadmin-ui", "version": "3.2.1"},
		{"type": "library", "bom-ref": "dep", "name": "dep", "version": "1.0.0", "licenses": []map[string]any{{"license": map[string]any{"id": "MIT"}}}},
	}})
	output, err := runLicensePolicy(t, root, workdir, sbom, "")
	require.NoError(t, err, string(output))
	report := readJSONMap(t, filepath.Join(workdir, "license-policy.json"))
	require.EqualValues(t, 1, report["component_count"])
	require.EqualValues(t, 1, report["excluded_manifest_component_count"])
	require.EqualValues(t, 1, report["excluded_first_party_component_count"])
}

func TestLicensePolicyAcceptsExactVersionLicenseOverride(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, overrides := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "overrides.json")
	purl := "pkg:npm/%40esbuild/darwin-arm64@0.28.1"
	writeCycloneDXSBOMWithoutLicense(t, sbom, purl, "@esbuild/darwin-arm64", "0.28.1")
	writeLicenseOverrides(t, overrides, purl, "0.28.1", "MIT", "https://github.com/evanw/esbuild")
	output, err := runLicensePolicy(t, root, workdir, sbom, overrides)
	require.NoError(t, err, string(output))
	require.Len(t, readJSONMap(t, filepath.Join(workdir, "license-policy.json"))["applied_license_overrides"], 1)
}

func TestLicensePolicyDoesNotOverrideSameNamePackageFromAnotherEcosystem(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, overrides := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "overrides.json")
	writeJSONFile(t, sbom, map[string]any{"components": []map[string]any{
		{"type": "library", "bom-ref": "pkg:npm/example@1.0.0", "purl": "pkg:npm/example@1.0.0", "name": "example", "version": "1.0.0"},
		{"type": "library", "bom-ref": "pkg:golang/example@1.0.0", "purl": "pkg:golang/example@1.0.0", "name": "example", "version": "1.0.0"},
	}})
	writeLicenseOverrides(t, overrides, "pkg:npm/example@1.0.0", "1.0.0", "MIT", "https://example.test/license")

	output, err := runLicensePolicy(t, root, workdir, sbom, overrides)

	require.Error(t, err)
	require.Contains(t, string(output), "license policy violations found")
	report := readJSONMap(t, filepath.Join(workdir, "license-policy.json"))
	require.Len(t, report["applied_license_overrides"], 1)
	require.Len(t, report["license_findings"], 1)
}

func TestLicensePolicyRejectsComponentNameOverride(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, overrides := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "overrides.json")
	writeCycloneDXSBOMWithoutLicense(t, sbom, "pkg:npm/example@1.0.0", "example", "1.0.0")
	writeJSONFile(t, overrides, map[string]any{"overrides": []map[string]any{{
		"component": "example", "version": "1.0.0", "license": "MIT",
		"owner": "platform-security", "evidence": "https://example.test/license",
	}}})

	output, err := runLicensePolicy(t, root, workdir, sbom, overrides)

	require.Error(t, err)
	require.Contains(t, string(output), "component selector is forbidden")
}

func TestLicensePolicyAcceptsAnchoredPURLPatternWithExactVersion(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, overrides := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "overrides.json")
	writeCycloneDXSBOMWithoutLicense(t, sbom, "pkg:npm/%40esbuild/linux-x64@0.28.1", "linux-x64", "0.28.1")
	writePatternLicenseOverride(t, overrides, `^pkg:npm/%40esbuild/.+@0\.28\.1$`, "0.28.1", "MIT", "https://github.com/evanw/esbuild")
	output, err := runLicensePolicy(t, root, workdir, sbom, overrides)
	require.NoError(t, err, string(output))
}

func TestLicensePolicyRejectsLicenseOverrideVersionMismatch(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, overrides := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "overrides.json")
	writeCycloneDXSBOMWithoutLicense(t, sbom, "pkg:npm/example@1.0.0", "example", "1.0.0")
	writeLicenseOverrides(t, overrides, "pkg:npm/example@1.0.0", "2.0.0", "MIT", "https://example.test/license")
	output, err := runLicensePolicy(t, root, workdir, sbom, overrides)
	require.Error(t, err)
	require.Contains(t, string(output), "license policy violations found")
}

func TestLicensePolicyRejectsLicenseOverrideWithoutEvidence(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, overrides := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "overrides.json")
	writeCycloneDXSBOMWithoutLicense(t, sbom, "pkg:npm/example@1.0.0", "example", "1.0.0")
	writeLicenseOverrides(t, overrides, "pkg:npm/example@1.0.0", "1.0.0", "MIT", "")
	output, err := runLicensePolicy(t, root, workdir, sbom, overrides)
	require.Error(t, err)
	require.Contains(t, string(output), "missing owner or evidence")
}

func TestLicensePolicyAcceptsImmutableEvidenceURIs(t *testing.T) {
	root := repositoryRoot(t)
	for _, evidence := range []string{
		"https://change.example/APR-123",
		"artifact://change/APR-123/license",
		"s3://compliance-worm/APR-123/license.json",
		"gs://compliance-worm/APR-123/license.json",
		"az://compliance-worm/APR-123/license.json",
		"azblob://compliance-worm/APR-123/license.json",
		"worm://compliance/APR-123/license.json",
		"oci://registry.example/compliance/license@sha256:abc",
	} {
		t.Run(evidence, func(t *testing.T) {
			workdir := t.TempDir()
			sbom := filepath.Join(workdir, "sbom.cdx.json")
			overrides := filepath.Join(workdir, "overrides.json")
			writeCycloneDXSBOMWithoutLicense(t, sbom, "pkg:npm/example@1.0.0", "example", "1.0.0")
			writeLicenseOverrides(t, overrides, "pkg:npm/example@1.0.0", "1.0.0", "MIT", evidence)

			output, err := runLicensePolicy(t, root, workdir, sbom, overrides)

			require.NoError(t, err, string(output))
		})
	}
}

func TestLicenseMetadataOverrideUsesPopperPackageEvidence(t *testing.T) {
	payload := readJSONMap(t, filepath.Join(repositoryRoot(t), "config/license_metadata_overrides.json"))
	for _, raw := range payload["overrides"].([]any) {
		override := raw.(map[string]any)
		if override["purl"] == "pkg:npm/%40popperjs/core@2.11.8" {
			require.Equal(t, "https://registry.npmjs.org/%40popperjs%2Fcore/2.11.8", override["evidence"])
			return
		}
	}
	t.Fatal("@popperjs/core override not found")
}

func TestLicensePolicyDoesNotOverrideDetectedLicense(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom, overrides := filepath.Join(workdir, "sbom.cdx.json"), filepath.Join(workdir, "overrides.json")
	writeCycloneDXSBOM(t, sbom, "GPL-3.0-only")
	writeLicenseOverrides(t, overrides, "pkg:generic/example@1.0.0", "1.0.0", "MIT", "https://example.test/license")
	output, err := runLicensePolicy(t, root, workdir, sbom, overrides)
	require.Error(t, err)
	require.Contains(t, string(output), "license policy violations found")
}

func TestLicensePolicyDoesNotCreateEvidenceArtifactsWhenAbsent(t *testing.T) {
	root, workdir := repositoryRoot(t), t.TempDir()
	sbom := filepath.Join(workdir, "sbom.cdx.json")
	writeCycloneDXSBOM(t, sbom, "MIT")
	output, err := runLicensePolicy(t, root, workdir, sbom, "")
	require.NoError(t, err, string(output))
	for _, name := range []string{"license-reviews.json", "exceptions.json"} {
		_, statErr := os.Stat(filepath.Join(workdir, name))
		require.ErrorIs(t, statErr, os.ErrNotExist)
	}
}
