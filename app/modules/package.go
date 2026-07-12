package modules

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

var (
	packageVersionPattern    = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:\+[0-9A-Za-z.-]+)?$`)
	packageConstraintPattern = regexp.MustCompile(`^(?:>=|<=|>|<|=)?v?[0-9]+\.[0-9]+\.[0-9]+(?:\+[0-9A-Za-z.-]+)?$`)
	packageDigestPattern     = regexp.MustCompile(`^sha256:[0-9a-fA-F]{64}$`)
)

type Package struct {
	ImportPath    string   `json:"import_path"`
	RegistryKey   string   `json:"registry_key"`
	Version       string   `json:"version,omitempty"`
	Owner         string   `json:"owner"`
	ReleaseTrack  string   `json:"release_track"`
	Compatibility []string `json:"compatibility"`
	Digest        string   `json:"digest,omitempty"`
	Signature     string   `json:"signature,omitempty"`
	Deprecated    bool     `json:"deprecated,omitempty"`
	ReplacedBy    string   `json:"replaced_by,omitempty"`
}

type PackageProvider interface {
	Package() Package
}

type SourceManifest struct {
	Modules []SourceManifestModule `json:"modules"`
}

type SourceManifestModule struct {
	ID      string  `json:"id"`
	Package Package `json:"package"`
}

func (r Registry) SourceManifest() SourceManifest {
	source := r.kernel.sourceModules()
	items := make([]SourceManifestModule, 0, len(source))
	for _, module := range source {
		items = append(items, SourceManifestModule{
			ID:      module.ID(),
			Package: modulePackage(module),
		})
	}

	return SourceManifest{Modules: items}
}

func (r Registry) validatePackages() error {
	return r.validatePackagesAt(time.Now())
}

func (r Registry) validatePackagesAt(now time.Time) error {
	source := r.kernel.sourceModules()
	var errs []error
	for _, module := range source {
		if _, ok := module.(PackageProvider); !ok {
			continue
		}
		pkg := modulePackage(module)
		if emptyPackage(pkg) {
			continue
		}
		plan, hasPlan := moduleReplacementPlan(module)
		errs = append(errs, validatePackageWithReplacement(module.ID(), ModuleMetadata(module).Version, pkg, plan, hasPlan, now)...)
	}

	return errors.Join(errs...)
}

func emptyPackage(pkg Package) bool {
	return strings.TrimSpace(pkg.ImportPath) == "" &&
		strings.TrimSpace(pkg.RegistryKey) == "" &&
		strings.TrimSpace(pkg.Owner) == "" &&
		strings.TrimSpace(pkg.ReleaseTrack) == "" &&
		len(pkg.Compatibility) == 0
}

func modulePackage(module Module) Package {
	provider, ok := module.(PackageProvider)
	if !ok {
		return Package{}
	}
	return provider.Package()
}

func validatePackage(moduleID string, moduleVersion string, pkg Package) []error {
	return validatePackageWithReplacement(moduleID, moduleVersion, pkg, ReplacementPlan{}, false, time.Now())
}

func validatePackageWithReplacement(moduleID string, moduleVersion string, pkg Package, replacement ReplacementPlan, hasReplacement bool, now time.Time) []error {
	moduleID = strings.TrimSpace(moduleID)
	var errs []error
	if strings.TrimSpace(pkg.RegistryKey) != moduleID {
		errs = append(errs, fmt.Errorf("module %s package registry key mismatch: %s", moduleID, pkg.RegistryKey))
	}
	if strings.TrimSpace(pkg.ImportPath) == "" {
		errs = append(errs, fmt.Errorf("module %s package import path is required", moduleID))
	} else if !packageImportPathMatches(moduleID, pkg.ImportPath) {
		errs = append(errs, fmt.Errorf("module %s package import path mismatch: %s", moduleID, pkg.ImportPath))
	}
	version := strings.TrimSpace(pkg.Version)
	if version == "" {
		errs = append(errs, fmt.Errorf("module %s package version is required", moduleID))
	} else if !packageVersionPattern.MatchString(version) {
		errs = append(errs, fmt.Errorf("module %s package version invalid: %s", moduleID, pkg.Version))
	} else if strings.TrimPrefix(version, "v") != strings.TrimPrefix(strings.TrimSpace(moduleVersion), "v") {
		errs = append(errs, fmt.Errorf("module %s package version mismatch: package %s metadata %s", moduleID, pkg.Version, moduleVersion))
	}
	if strings.TrimSpace(pkg.Owner) == "" {
		errs = append(errs, fmt.Errorf("module %s package owner is required", moduleID))
	}
	if !validReleaseTrack(pkg.ReleaseTrack) {
		errs = append(errs, fmt.Errorf("module %s package release track unsupported: %s", moduleID, pkg.ReleaseTrack))
	}
	if len(pkg.Compatibility) == 0 {
		errs = append(errs, fmt.Errorf("module %s package compatibility matrix is required", moduleID))
	} else {
		for _, constraint := range pkg.Compatibility {
			if !validPackageConstraint(constraint) {
				errs = append(errs, fmt.Errorf("module %s package compatibility invalid: %s", moduleID, constraint))
			}
		}
	}
	if packageRequiresSignature(pkg) {
		if strings.TrimSpace(pkg.Digest) == "" {
			errs = append(errs, fmt.Errorf("module %s package digest is required for release track %s", moduleID, pkg.ReleaseTrack))
		} else if !packageDigestPattern.MatchString(strings.TrimSpace(pkg.Digest)) {
			errs = append(errs, fmt.Errorf("module %s package digest invalid: %s", moduleID, pkg.Digest))
		}
		if strings.TrimSpace(pkg.Signature) == "" {
			errs = append(errs, fmt.Errorf("module %s package signature is required for release track %s", moduleID, pkg.ReleaseTrack))
		} else if !validPackageSignature(pkg.Signature) {
			errs = append(errs, fmt.Errorf("module %s package signature invalid: %s", moduleID, pkg.Signature))
		}
	}
	if pkg.Deprecated {
		if strings.TrimSpace(pkg.ReplacedBy) == "" {
			errs = append(errs, fmt.Errorf("module %s deprecated package requires replacement target", moduleID))
		} else if !hasReplacement {
			errs = append(errs, fmt.Errorf("module %s deprecated package requires replacement plan", moduleID))
		} else if err := replacement.Validate(moduleID, pkg.ReplacedBy, now); err != nil {
			errs = append(errs, fmt.Errorf("module %s replacement plan invalid: %w", moduleID, err))
		}
	}

	return errs
}

func validPackageConstraint(constraint string) bool {
	parts := strings.FieldsFunc(strings.TrimSpace(constraint), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if !packageConstraintPattern.MatchString(part) {
			return false
		}
	}
	return true
}

func validPackageSignature(signature string) bool {
	value := strings.TrimSpace(signature)
	return strings.HasPrefix(value, "cosign:") && strings.TrimSpace(strings.TrimPrefix(value, "cosign:")) != ""
}

func packageRequiresSignature(pkg Package) bool {
	track := strings.TrimSpace(pkg.ReleaseTrack)
	return track != "" && track != "internal"
}

func packageImportPathMatches(moduleID string, importPath string) bool {
	name := path.Base(strings.TrimSpace(importPath))
	return name == packageName(moduleID)
}

func packageName(moduleID string) string {
	return strings.NewReplacer("-", "", "_", "").Replace(strings.TrimSpace(moduleID))
}

func validReleaseTrack(track string) bool {
	return slices.Contains([]string{"stable", "beta", "experimental", "internal", "deprecated"}, strings.TrimSpace(track))
}
