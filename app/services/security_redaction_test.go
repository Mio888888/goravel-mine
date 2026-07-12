package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactSensitiveDataMasksConfiguredFieldsRecursively(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.sensitive_data.fields": []string{"password", "token", "client_secret"},
	})
	defer restore()

	input := map[string]any{
		"username": "alice",
		"password": "secret",
		"profile": map[string]any{
			"token": "jwt",
			"email": "a@example.com",
		},
		"providers": []any{
			map[string]any{"client_secret": "hidden"},
		},
	}

	output := RedactSensitiveData(input).(map[string]any)

	require.Equal(t, "alice", output["username"])
	require.Equal(t, RedactedValue, output["password"])
	profile := output["profile"].(map[string]any)
	require.Equal(t, RedactedValue, profile["token"])
	require.Equal(t, "a@example.com", profile["email"])
	providers := output["providers"].([]any)
	require.Equal(t, RedactedValue, providers[0].(map[string]any)["client_secret"])
}
