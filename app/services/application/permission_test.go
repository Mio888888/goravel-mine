package application

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

// Source: casbin_policy_loader_test.go
func TestCasbinPolicyLineTrimsUnusedValues(t *testing.T) {
	line, err := casbinPolicyLine(casbinPolicyRow{Ptype: "p", V0: "role:Auditor", V1: "permission:user:index", V2: "*"})

	require.NoError(t, err)
	require.Equal(t, []string{"p", "role:Auditor", "permission:user:index", "*"}, line)
}

func TestCasbinPolicyLineRejectsEmptyPolicyType(t *testing.T) {
	_, err := casbinPolicyLine(casbinPolicyRow{})

	require.EqualError(t, err, "ptype is empty")
}

func TestResolveCasbinModelPathFindsRepositoryFromPackageDirectory(t *testing.T) {
	repository := t.TempDir()
	modelPath := filepath.Join(repository, "config", "casbin", "rbac_model.conf")
	require.NoError(t, os.MkdirAll(filepath.Dir(modelPath), 0o755))
	require.NoError(t, os.WriteFile(modelPath, []byte("[request_definition]"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module test"), 0o600))

	packageDirectory := filepath.Join(repository, "tests", "feature", "admin")
	require.NoError(t, os.MkdirAll(packageDirectory, 0o755))
	t.Chdir(packageDirectory)

	configuredPath := filepath.Join("config", "casbin", "rbac_model.conf")
	resolved := resolveCasbinModelPath(filepath.Join(packageDirectory, configuredPath), configuredPath)
	require.Equal(t, modelPath, resolved)
}
