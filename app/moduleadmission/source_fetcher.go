package moduleadmission

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SourceFetcher struct {
	config SourceFetcherConfig
	client *http.Client
}

const maximumSourceRedirects = 10

func NewSourceFetcher(config SourceFetcherConfig) SourceFetcher {
	if config.MaxBundleBytes <= 0 {
		config.MaxBundleBytes = 32 << 20
	}
	if config.DownloadTimeout <= 0 {
		config.DownloadTimeout = 30 * time.Second
	}
	if config.MaxArchiveEntries <= 0 {
		config.MaxArchiveEntries = 4096
	}
	allowed := make(map[string]bool, len(config.AllowedHosts))
	for _, host := range config.AllowedHosts {
		allowed[strings.ToLower(strings.TrimSpace(host))] = true
	}
	client := &http.Client{Timeout: config.DownloadTimeout}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= maximumSourceRedirects {
			return fmt.Errorf("stopped after %d redirects", maximumSourceRedirects)
		}
		if !allowed[strings.ToLower(request.URL.Hostname())] {
			return fmt.Errorf("redirect host is not allowed: %s", request.URL.Hostname())
		}
		return nil
	}
	return SourceFetcher{config: config, client: client}
}

func (f SourceFetcher) Fetch(ctx context.Context, entry ModuleIndexEntry, workspace string) (result SourceFetchResult, returnErr error) {
	if err := validateSourceURI(entry.SourceURI); err != nil {
		return result, err
	}
	root, err := os.MkdirTemp(workspace, "module-admission-")
	if err != nil {
		return result, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	defer func() {
		if returnErr != nil {
			cleanup()
		}
	}()
	bundlePath := filepath.Join(root, "source.bundle")
	size, err := f.download(ctx, entry.SourceURI, bundlePath)
	if err != nil {
		return result, err
	}
	digest, err := digestFile(bundlePath)
	if err != nil {
		return result, err
	}
	if digest != strings.ToLower(entry.Digest) {
		return result, fmt.Errorf("source digest mismatch: got %s want %s", digest, entry.Digest)
	}
	sourceDir := filepath.Join(root, "source")
	if err := os.Mkdir(sourceDir, 0700); err != nil {
		return result, err
	}
	if isZip(entry.SourceURI) {
		if err := f.extractZip(bundlePath, sourceDir); err != nil {
			return result, err
		}
	} else if err := copyFile(bundlePath, filepath.Join(sourceDir, "source.bundle")); err != nil {
		return result, err
	}
	return SourceFetchResult{BundlePath: bundlePath, SourceDir: sourceDir, Digest: digest, Size: size, Cleanup: cleanup}, nil
}

func (f SourceFetcher) FetchExternal(ctx context.Context, entry ModuleIndexEntry, workspace string) (SourceFetchResult, error) {
	if entry.SourceKind != "external" {
		return SourceFetchResult{}, fmt.Errorf("module %s is not an external source", entry.ID)
	}
	return f.Fetch(ctx, entry, workspace)
}

func (f SourceFetcher) FetchEvidenceFile(ctx context.Context, sourceURI, expectedDigest, workspace string) (EvidenceFile, error) {
	if err := validateSourceURI(sourceURI); err != nil {
		return EvidenceFile{}, err
	}
	root, err := os.MkdirTemp(workspace, "module-admission-evidence-")
	if err != nil {
		return EvidenceFile{}, err
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	path := filepath.Join(root, "evidence")
	if _, err := f.download(ctx, sourceURI, path); err != nil {
		cleanup()
		return EvidenceFile{}, err
	}
	digest, err := digestFile(path)
	if err != nil {
		cleanup()
		return EvidenceFile{}, err
	}
	if expectedDigest != "" && digest != strings.ToLower(expectedDigest) {
		cleanup()
		return EvidenceFile{}, fmt.Errorf("evidence digest mismatch: got %s want %s", digest, expectedDigest)
	}
	return EvidenceFile{Path: path, Digest: digest, Cleanup: cleanup}, nil
}

func (f SourceFetcher) FetchAll(ctx context.Context, entries []ModuleIndexEntry, workspace string) (sources []FetchedSource, returnErr error) {
	defer func() {
		if returnErr == nil {
			return
		}
		for _, source := range sources {
			source.Result.Cleanup()
		}
	}()
	for _, entry := range entries {
		result, err := f.Fetch(ctx, entry, workspace)
		if err != nil {
			return sources, err
		}
		sources = append(sources, FetchedSource{Entry: entry, Result: result})
	}
	return sources, nil
}

func (f SourceFetcher) download(ctx context.Context, sourceURI, target string) (int64, error) {
	parsed, _ := url.Parse(sourceURI)
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		if !f.allowedHost(parsed.Hostname()) {
			return 0, fmt.Errorf("source host is not allowed: %s", parsed.Hostname())
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURI, nil)
		if err != nil {
			return 0, err
		}
		response, err := f.client.Do(request)
		if err != nil {
			return 0, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("source download returned HTTP %d", response.StatusCode)
		}
		if response.ContentLength > f.config.MaxBundleBytes {
			return 0, fmt.Errorf("source bundle exceeds maximum size")
		}
		return copyLimited(response.Body, target, f.config.MaxBundleBytes)
	}
	file, err := os.Open(sourceURI)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return copyLimited(file, target, f.config.MaxBundleBytes)
}

func (f SourceFetcher) allowedHost(host string) bool {
	for _, allowed := range f.config.AllowedHosts {
		if strings.EqualFold(strings.TrimSpace(allowed), host) {
			return true
		}
	}
	return false
}

func (f SourceFetcher) extractZip(bundlePath, target string) error {
	archive, err := zip.OpenReader(bundlePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	if len(archive.File) > f.config.MaxArchiveEntries {
		return fmt.Errorf("archive contains too many entries")
	}
	var total uint64
	for _, item := range archive.File {
		total += item.UncompressedSize64
		if total > uint64(f.config.MaxBundleBytes) {
			return fmt.Errorf("archive exceeds maximum expanded size")
		}
		if err := extractZipEntry(item, target); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(item *zip.File, target string) error {
	clean := filepath.Clean(item.Name)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.Contains(item.Name, "\\") {
		return fmt.Errorf("unsafe archive path: %s", item.Name)
	}
	if item.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("archive symlinks are not allowed: %s", item.Name)
	}
	destination := filepath.Join(target, clean)
	if item.FileInfo().IsDir() {
		return os.MkdirAll(destination, 0700)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	input, err := item.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	return writeRestrictedFile(destination, input)
}

func copyLimited(input io.Reader, target string, maximum int64) (int64, error) {
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return 0, err
	}
	defer output.Close()
	count, err := io.Copy(output, io.LimitReader(input, maximum+1))
	if err != nil {
		return 0, err
	}
	if count > maximum {
		return 0, fmt.Errorf("source bundle exceeds maximum size")
	}
	return count, nil
}

func writeRestrictedFile(target string, input io.Reader) error {
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	_, err = io.Copy(output, input)
	return err
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	return writeRestrictedFile(target, input)
}

func digestFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func isZip(sourceURI string) bool {
	return strings.HasSuffix(strings.ToLower(strings.Split(sourceURI, "?")[0]), ".zip")
}
