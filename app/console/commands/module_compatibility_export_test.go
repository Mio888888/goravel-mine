package commands

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
)

func TestExportCompatibilityMatrixWritesReleaseArtifact(t *testing.T) {
	target := filepath.Join(t.TempDir(), "release", "module-compatibility-matrix.json")

	matrix, err := exportCompatibilityMatrix(moduleboot.Modules(), "1.17.2", target, io.Discard)

	require.NoError(t, err)
	require.Equal(t, "passed", matrix.Status)
	payload, err := os.ReadFile(target)
	require.NoError(t, err)
	var written modulecatalog.CompatibilityMatrix
	require.NoError(t, json.Unmarshal(payload, &written))
	require.Equal(t, "1.17.2", written.FrameworkVersion)
	require.NotEmpty(t, written.Modules)
}

func TestExportCompatibilityMatrixRequiresFrameworkVersion(t *testing.T) {
	_, err := exportCompatibilityMatrix(moduleboot.Modules(), "", "", io.Discard)

	require.ErrorContains(t, err, "framework version is required")
}
