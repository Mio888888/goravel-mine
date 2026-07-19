package unit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
	"goravel/bootstrap"
)

func TestSensitiveOperationPoliciesCoverDeclaredMutationRoutes(t *testing.T) {
	registry := services.NewSensitiveOperationPolicyRegistry()
	routes := bootstrap.Modules().Routes()
	byName := make(map[string]struct {
		method     string
		path       string
		permission []string
	}, len(routes))
	for _, route := range routes {
		byName[route.Name] = struct {
			method     string
			path       string
			permission []string
		}{route.Method, route.Path, route.PermissionKeys()}
	}

	contracts := registry.RouteContracts()
	require.Len(t, contracts, 24)
	featureTests := sensitiveFeatureTestSource(t)
	for _, contract := range contracts {
		t.Run(contract.RouteName, func(t *testing.T) {
			route, ok := byName[contract.RouteName]
			require.True(t, ok, "sensitive route is not registered")
			require.Equal(t, contract.Method, route.method)
			require.Equal(t, contract.Path, route.path)
			require.Contains(t, route.permission, contract.Permission)

			tenantID := uint64(0)
			if contract.Domain == "tenant" {
				tenantID = 1
			}
			permission, action, err := registry.PermissionFor(contract.PolicyKey, tenantID, contract.Resource)
			require.NoError(t, err)
			require.Equal(t, contract.Permission, permission)
			require.Equal(t, contract.Method, action)
			require.NotEmpty(t, contract.FeatureTest)
			require.Contains(t, featureTests, contract.FeatureTest, "feature evidence test is missing")
		})
	}

	if os.Getenv("UPDATE_SECURITY_ARTIFACT") == "1" {
		writeSensitiveOperationMatrix(t, contracts)
	}
}

func sensitiveFeatureTestSource(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob("../feature/admin/*_test.go")
	require.NoError(t, err)
	pattern := regexp.MustCompile(`func(?: \([^)]*\))? (Test[A-Za-z0-9_]+)\(`)
	tests := make(map[string]bool)
	var source strings.Builder
	for _, path := range paths {
		content, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		source.Write(content)
	}
	for _, match := range pattern.FindAllStringSubmatch(source.String(), -1) {
		tests[match[1]] = true
	}
	return tests
}

func writeSensitiveOperationMatrix(t *testing.T, contracts []services.SensitiveOperationRouteContract) {
	t.Helper()
	path := filepath.Clean("../../../artifacts/security/sensitive-operation-matrix.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content, err := json.MarshalIndent(struct {
		SchemaVersion int                                        `json:"schema_version"`
		Operations    []services.SensitiveOperationRouteContract `json:"operations"`
	}{SchemaVersion: 1, Operations: contracts}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(content, '\n'), 0o644))
}
