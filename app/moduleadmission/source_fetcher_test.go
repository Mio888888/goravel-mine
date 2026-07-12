package moduleadmission

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceFetcherChecksDigestSizeAndExtractionBoundary(t *testing.T) {
	workspace := t.TempDir()
	bundle := filepath.Join(workspace, "module.zip")
	writeZip(t, bundle, map[string]string{"module.go": "package module"})
	digest := fileDigest(t, bundle)
	fetcher := NewSourceFetcher(SourceFetcherConfig{MaxBundleBytes: 1024, AllowedHosts: []string{"registry.example"}})
	result, err := fetcher.Fetch(context.Background(), ModuleIndexEntry{
		ID: "audit", Version: "1.0.0", SourceURI: bundle, Digest: digest,
	}, workspace)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	defer result.Cleanup()
	if _, err := os.Stat(filepath.Join(result.SourceDir, "module.go")); err != nil {
		t.Fatalf("extracted module source missing: %v", err)
	}

	_, err = fetcher.Fetch(context.Background(), ModuleIndexEntry{
		ID: "audit", Version: "1.0.0", SourceURI: bundle, Digest: "sha256:" + strings.Repeat("0", 64),
	}, workspace)
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("digest mismatch error = %v", err)
	}

	traversal := filepath.Join(workspace, "traversal.zip")
	writeZip(t, traversal, map[string]string{"../outside.txt": "no"})
	_, err = fetcher.Fetch(context.Background(), ModuleIndexEntry{
		ID: "audit", Version: "1.0.0", SourceURI: traversal, Digest: fileDigest(t, traversal),
	}, workspace)
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("path traversal error = %v", err)
	}
}

func TestSourceFetcherFetchAllCleansPreviouslyFetchedBundlesAfterFailure(t *testing.T) {
	workspace := t.TempDir()
	bundle := filepath.Join(workspace, "module.zip")
	writeZip(t, bundle, map[string]string{"module.go": "package module"})
	fetcher := NewSourceFetcher(SourceFetcherConfig{MaxBundleBytes: 1024})
	_, err := fetcher.FetchAll(context.Background(), []ModuleIndexEntry{
		{ID: "first", Version: "1.0.0", SourceURI: bundle, Digest: fileDigest(t, bundle)},
		{ID: "second", Version: "1.0.0", SourceURI: bundle, Digest: "sha256:" + strings.Repeat("0", 64)},
	}, workspace)
	if err == nil {
		t.Fatal("FetchAll() accepted digest mismatch")
	}
	matches, err := filepath.Glob(filepath.Join(workspace, "module-admission-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary workspaces = %v, err = %v", matches, err)
	}
}

func TestSourceFetcherFetchExternalRejectsInternalModules(t *testing.T) {
	_, err := NewSourceFetcher(SourceFetcherConfig{}).FetchExternal(context.Background(), ModuleIndexEntry{ID: "builtin", SourceKind: "internal"}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not an external source") {
		t.Fatalf("FetchExternal() error = %v", err)
	}
}

func TestSourceFetcherLimitsAllowedHostRedirects(t *testing.T) {
	fetcher := NewSourceFetcher(SourceFetcherConfig{AllowedHosts: []string{"registry.example"}})
	request, err := http.NewRequest(http.MethodGet, "https://registry.example/module.zip", nil)
	if err != nil {
		t.Fatal(err)
	}
	via := make([]*http.Request, maximumSourceRedirects)
	if err := fetcher.client.CheckRedirect(request, via); err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Fatalf("redirect limit error = %v", err)
	}
}

func writeZip(t *testing.T, target string, files map[string]string) {
	t.Helper()
	file, err := os.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", digest[:])
}
