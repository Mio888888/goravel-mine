package seeders

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTenantPlanFullPermissionsFeatureJSON(t *testing.T) {
	features, err := tenantPlanFullPermissionsFeatureJSON()
	require.NoError(t, err)

	var decoded map[string]map[string][]string
	require.NoError(t, json.Unmarshal([]byte(features), &decoded))

	allowed := decoded["permissions"]["allowed"]
	require.Contains(t, allowed, "permission:user:index")
	require.Contains(t, allowed, "log:ssoLogin:stats")
	require.NotContains(t, allowed, "platform:tenant:list")
}
