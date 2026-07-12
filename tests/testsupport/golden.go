package testsupport

import (
	"encoding/json"
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
