package unit

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/tests/backend/testsupport"
)

func TestTestCodeStaysInDedicatedRoots(t *testing.T) {
	root := testsupport.RepositoryPath(t)
	backendRoot := filepath.Join(root, "tests", "backend") + string(filepath.Separator)
	frontendRoot := filepath.Join(root, "MineAdmin-web", "tests") + string(filepath.Separator)
	frontendTestPattern := regexp.MustCompile(`\.(spec|test)\.[cm]?[jt]sx?$`)
	scriptTestPattern := regexp.MustCompile(`(^|/)(test_.*|.*_test)\.(py|sh|rb)$`)

	var scattered []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && shouldSkipTestLayoutDirectory(root, path) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}

		normalized := filepath.ToSlash(path)
		switch {
		case strings.HasSuffix(entry.Name(), "_test.go") && !strings.HasPrefix(path, backendRoot):
			scattered = append(scattered, normalized)
		case frontendTestPattern.MatchString(entry.Name()) && !strings.HasPrefix(path, frontendRoot):
			scattered = append(scattered, normalized)
		case scriptTestPattern.MatchString(normalized) && !strings.HasPrefix(path, backendRoot):
			scattered = append(scattered, normalized)
		}
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, scattered, "test source files must stay under tests/backend or MineAdmin-web/tests")
}

func TestProductionDirectoriesDoNotContainTestData(t *testing.T) {
	root := testsupport.RepositoryPath(t)
	productionRoots := []string{
		"app",
		"bootstrap",
		"config",
		"database",
		"routes",
		"scripts",
		filepath.Join("MineAdmin-web", "src"),
		filepath.Join("MineAdmin-web", "scripts"),
	}

	var scattered []string
	for _, relativeRoot := range productionRoots {
		path := filepath.Join(root, relativeRoot)
		err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() && (entry.Name() == "testdata" || entry.Name() == "fixtures" || entry.Name() == "__tests__") {
				scattered = append(scattered, filepath.ToSlash(current))
				return filepath.SkipDir
			}
			return nil
		})
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
	}
	require.Empty(t, scattered, "test data must stay under a dedicated test root")
}

func TestCentralizedPackageTestsMapToProductionPackages(t *testing.T) {
	root := testsupport.RepositoryPath(t)
	overlayRoot := filepath.Join(root, "tests", "backend", "_packages")
	count := 0

	err := filepath.WalkDir(overlayRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(overlayRoot, path)
		if err != nil {
			return err
		}
		targetDirectory := filepath.Join(root, filepath.Dir(relative))
		info, err := os.Stat(targetDirectory)
		require.NoErrorf(t, err, "overlay target package missing for %s", filepath.ToSlash(relative))
		require.Truef(t, info.IsDir(), "overlay target must be a directory: %s", targetDirectory)
		count++
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, count, "centralized package tests must not be empty")
}

func TestCIUsesCentralizedBackendTestRunner(t *testing.T) {
	root := testsupport.RepositoryPath(t)
	workflowRoot := filepath.Join(root, ".github", "workflows")
	var directGoTest []string
	usesCentralRunner := false

	err := filepath.WalkDir(workflowRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (filepath.Ext(path) != ".yml" && filepath.Ext(path) != ".yaml") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(content)
		if strings.Contains(source, "tests/backend/test.sh") {
			usesCentralRunner = true
		}
		if strings.Contains(source, "go test ") {
			directGoTest = append(directGoTest, filepath.ToSlash(path))
		}
		return nil
	})
	require.NoError(t, err)
	require.True(t, usesCentralRunner, "CI must execute tests/backend/test.sh")
	require.Empty(t, directGoTest, "CI must not bypass the centralized backend test runner")
}

func shouldSkipTestLayoutDirectory(root, path string) bool {
	if path == root {
		return false
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	normalized := filepath.ToSlash(relative)
	for _, prefix := range []string{
		".git",
		"artifacts",
		"docs/docs-master",
		"MineAdmin-web/dist",
		"MineAdmin-web/node_modules",
		"MineAdmin-web/tests/e2e/.output",
		"storage",
	} {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+"/") {
			return true
		}
	}
	return false
}
