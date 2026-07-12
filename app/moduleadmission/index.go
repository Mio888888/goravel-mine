package moduleadmission

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
)

func LoadRepositoryIndex(payload []byte) (RepositoryIndex, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var index RepositoryIndex
	if err := decoder.Decode(&index); err != nil {
		return RepositoryIndex{}, fmt.Errorf("decode repository index: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return RepositoryIndex{}, fmt.Errorf("repository index contains trailing data")
	}
	if err := normalizeIndex(&index); err != nil {
		return RepositoryIndex{}, err
	}
	canonical, err := indexCanonicalJSON(index)
	if err != nil {
		return RepositoryIndex{}, err
	}
	index.Digest = sha256Digest(canonical)
	return index, nil
}

func normalizeIndex(index *RepositoryIndex) error {
	if index.SchemaVersion != "v1" {
		return fmt.Errorf("unsupported repository index schema version: %s", index.SchemaVersion)
	}
	if len(index.Modules) == 0 {
		return fmt.Errorf("repository index has no modules")
	}
	seen := make(map[string]bool, len(index.Modules))
	for item := range index.Modules {
		entry := &index.Modules[item]
		entry.ID = strings.TrimSpace(entry.ID)
		entry.Version = strings.TrimSpace(entry.Version)
		entry.SourceKind = strings.TrimSpace(entry.SourceKind)
		entry.SourceURI = strings.TrimSpace(entry.SourceURI)
		entry.Digest = strings.ToLower(strings.TrimSpace(entry.Digest))
		if entry.ID == "" || !exactVersionPattern.MatchString(entry.Version) {
			return fmt.Errorf("module %s requires an exact version", entry.ID)
		}
		if entry.SourceKind != "internal" && entry.SourceKind != "external" {
			return fmt.Errorf("module %s has invalid source kind", entry.ID)
		}
		if !digestPattern.MatchString(entry.Digest) {
			return fmt.Errorf("module %s has invalid source digest", entry.ID)
		}
		if err := validateSourceURI(entry.SourceURI); err != nil {
			return fmt.Errorf("module %s: %w", entry.ID, err)
		}
		key := entry.ID + "@" + entry.Version
		if seen[key] {
			return fmt.Errorf("duplicate module version: %s", key)
		}
		seen[key] = true
		if entry.Deprecated && strings.TrimSpace(entry.ReplacedBy) == "" {
			return fmt.Errorf("deprecated module %s requires replacement target", entry.ID)
		}
		if entry.SourceKind == "external" {
			if err := validateExternalEvidence(*entry); err != nil {
				return err
			}
		}
		sort.Slice(entry.Dependencies, func(left, right int) bool {
			return entry.Dependencies[left].ID < entry.Dependencies[right].ID
		})
	}
	sort.Slice(index.Modules, func(left, right int) bool {
		if index.Modules[left].ID == index.Modules[right].ID {
			return index.Modules[left].Version < index.Modules[right].Version
		}
		return index.Modules[left].ID < index.Modules[right].ID
	})
	return nil
}

func validateExternalEvidence(entry ModuleIndexEntry) error {
	fields := []struct {
		name  string
		value string
	}{
		{"cosign issuer", entry.CosignIssuer},
		{"cosign identity", entry.CosignIdentity},
		{"signature URI", entry.SignatureURI},
		{"SBOM URI", entry.SBOMURI},
		{"provenance URI", entry.ProvenanceURI},
		{"Go import path", entry.GoImportPath},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("external module %s requires %s", entry.ID, field.name)
		}
	}
	if !digestPattern.MatchString(strings.ToLower(strings.TrimSpace(entry.SBOMDigest))) {
		return fmt.Errorf("external module %s has invalid SBOM digest", entry.ID)
	}
	if !digestPattern.MatchString(strings.ToLower(strings.TrimSpace(entry.ProvenanceDigest))) {
		return fmt.Errorf("external module %s has invalid provenance digest", entry.ID)
	}
	return nil
}

func validateSourceURI(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || value == "" {
		return fmt.Errorf("invalid source URI")
	}
	switch parsed.Scheme {
	case "":
		clean := path.Clean(value)
		if strings.HasPrefix(clean, "../") || clean == ".." {
			return fmt.Errorf("source URI path traversal")
		}
	case "https", "http":
		if parsed.Host == "" || parsed.User != nil || strings.Contains(parsed.Path, "..") {
			return fmt.Errorf("invalid source URI")
		}
	default:
		return fmt.Errorf("unsupported source URI: %s", parsed.Scheme)
	}
	return nil
}

func indexCanonicalJSON(index RepositoryIndex) ([]byte, error) {
	return json.Marshal(struct {
		SchemaVersion string             `json:"schema_version"`
		Modules       []ModuleIndexEntry `json:"modules"`
	}{SchemaVersion: index.SchemaVersion, Modules: index.Modules})
}

func sha256Digest(payload []byte) string {
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}
