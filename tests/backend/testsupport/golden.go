package testsupport

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func RequireGoldenJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	payload = append(payload, '\n')
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, payload, 0o644))
	}
	expected, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(payload))
}

func RepositoryPath(t *testing.T, elements ...string) string {
	t.Helper()
	if root := os.Getenv("GORAVEL_REPOSITORY_ROOT"); root != "" {
		return filepath.Join(append([]string{root}, elements...)...)
	}

	root, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, statErr := os.Stat(filepath.Join(root, "go.mod")); statErr == nil {
			return filepath.Join(append([]string{root}, elements...)...)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			require.NoError(t, statErr)
		}
		parent := filepath.Dir(root)
		require.NotEqual(t, root, parent, "repository root not found")
		root = parent
	}
}
