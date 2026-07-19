package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var scopeGroups = map[string][]string{
	"backend": {
		"app/modules",
		"app/modulecatalog",
		"app/console/commands/module_manifest_check.go",
		"app/console/commands/module_manifest_export.go",
		"app/console/commands/module_compatibility_export.go",
		"app/console/commands/module_state.go",
		"app/console/commands/module_plan.go",
		"app/console/commands/module_lifecycle.go",
		"app/http/controllers/admin/module_lifecycle_controller.go",
	},
	"frontend": {
		"MineAdmin-web/src/modules/base/api/platformModuleLifecycle.ts",
		"MineAdmin-web/src/modules/base/views/platform/moduleLifecycle",
	},
	"tests": {
		"app/modules",
		"app/modulecatalog",
		"app/console/commands",
		"tests/backend/unit/module_catalog_service_test.go",
		"tests/backend/feature/admin/module_lifecycle_test.go",
		"MineAdmin-web/tests/e2e/enterprise-matrix.spec.ts",
		"MineAdmin-web/tests/e2e/module-lifecycle.spec.ts",
	},
}

var supportedExtensions = []string{".go", ".ts", ".tsx", ".vue"}

var excludedPathFragments = []string{
	"/MineAdmin/",
	"/vendor/",
	"/node_modules/",
	"/dist/",
	"/testdata/",
	"/src/generated/",
	"/src/iconify/",
	"/tests/e2e/.output/",
}

type scopedFile struct {
	Group string
	Path  string
	Full  string
}

func newScopeReport(root string) scopeReport {
	reportRoot := filepath.Clean(root)
	return scopeReport{
		Root:       filepath.ToSlash(reportRoot),
		Extensions: append([]string(nil), supportedExtensions...),
		Excludes:   append([]string(nil), excludedPathFragments...),
	}
}

func collectScopedFiles(root string) ([]scopedFile, error) {
	var files []scopedFile
	seen := map[string]struct{}{}
	for _, name := range orderedGroupNames() {
		paths, err := listGroupFiles(root, name, scopeGroups[name])
		if err != nil {
			return nil, err
		}
		for _, item := range paths {
			if _, exists := seen[item.Path]; exists {
				continue
			}
			seen[item.Path] = struct{}{}
			files = append(files, item)
		}
	}
	sortScopedFiles(files)
	return files, nil
}

func orderedGroupNames() []string {
	names := make([]string, 0, len(scopeGroups))
	for name := range scopeGroups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func listGroupFiles(root, group string, entries []string) ([]scopedFile, error) {
	seen := map[string]struct{}{}
	var files []scopedFile
	for _, entry := range entries {
		matches, err := scanEntry(root, group, entry)
		if err != nil {
			return nil, err
		}
		for _, item := range matches {
			if _, exists := seen[item.Path]; exists {
				continue
			}
			seen[item.Path] = struct{}{}
			files = append(files, item)
		}
	}
	return files, nil
}

func scanEntry(root, group, entry string) ([]scopedFile, error) {
	full := filepath.Join(root, filepath.FromSlash(entry))
	info, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		file, ok := classifyFile(root, group, entry)
		if !ok {
			return nil, nil
		}
		return []scopedFile{file}, nil
	}
	var files []scopedFile
	walkErr := filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if d.IsDir() {
			if shouldSkipDir(relative) {
				return filepath.SkipDir
			}
			return nil
		}
		file, ok := classifyFile(root, group, relative)
		if ok {
			files = append(files, file)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return files, nil
}

func classifyFile(root, group, relative string) (scopedFile, bool) {
	relative = filepath.ToSlash(filepath.Clean(relative))
	if !hasSupportedExtension(relative) || isExcludedPath(relative) {
		return scopedFile{}, false
	}
	testFile := isTestFile(relative)
	if group == "tests" && !testFile {
		return scopedFile{}, false
	}
	if group != "tests" && testFile {
		return scopedFile{}, false
	}
	if testFile {
		group = "tests"
	}
	return scopedFile{
		Group: group,
		Path:  relative,
		Full:  filepath.Join(root, filepath.FromSlash(relative)),
	}, true
}

func hasSupportedExtension(path string) bool {
	for _, extension := range supportedExtensions {
		if strings.HasSuffix(path, extension) {
			return true
		}
	}
	return false
}

func isExcludedPath(path string) bool {
	normalized := "/" + strings.TrimPrefix(filepath.ToSlash(path), "./")
	for _, fragment := range excludedPathFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func isTestFile(path string) bool {
	if strings.HasSuffix(path, "_test.go") {
		return true
	}
	return strings.HasPrefix(path, "tests/") || strings.HasPrefix(path, "MineAdmin-web/tests/")
}

func shouldSkipDir(path string) bool {
	if path == "." {
		return false
	}
	return isExcludedPath(path + "/")
}

func sortScopedFiles(files []scopedFile) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].Path != files[j].Path {
			return files[i].Path < files[j].Path
		}
		return files[i].Group < files[j].Group
	})
}
