package config

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitConfigListTrimsEmptyAndDuplicateValues(t *testing.T) {
	values := splitConfigList(" Authorization, Content-Type, Authorization, ,X-Tenant ")

	require.Equal(t, []string{"Authorization", "Content-Type", "X-Tenant"}, values)
}

func TestSplitConfigListUsesFallbackWhenValueIsEmpty(t *testing.T) {
	values := splitConfigList(" , ", "GET", "POST")

	require.Equal(t, []string{"GET", "POST"}, values)
}

func TestCorsAllowedOriginsDoesNotAllowWildcardWithCredentials(t *testing.T) {
	origins := corsAllowedOrigins(true)

	require.NotEmpty(t, origins)
	require.False(t, slices.Contains(origins, "*"))
}

func TestCorsAllowedOriginsFiltersWildcardWithCredentials(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "*,https://admin.example.com")

	origins := corsAllowedOrigins(true)

	require.Equal(t, []string{"https://admin.example.com"}, origins)
}

func TestSecurityEnterpriseDefaultsHardenAuthentication(t *testing.T) {
	profile := securityProfile(true)

	require.True(t, profile.MFATOTPEnabled)
	require.Equal(t, 12, profile.PasswordMinLength)
	require.True(t, profile.PasswordRequireUppercase)
	require.True(t, profile.PasswordRequireLowercase)
	require.True(t, profile.PasswordRequireNumber)
	require.True(t, profile.PasswordRequireSymbol)
	require.Equal(t, 5, profile.PasswordHistoryLimit)
	require.Equal(t, 90, profile.PasswordMaxAgeDays)
	require.True(t, profile.CSRFEnabled)
}

func TestSecurityCompatibleDefaultsPreserveLegacyBehavior(t *testing.T) {
	profile := securityProfile(false)

	require.False(t, profile.MFATOTPEnabled)
	require.Equal(t, 6, profile.PasswordMinLength)
	require.False(t, profile.PasswordRequireUppercase)
	require.False(t, profile.PasswordRequireLowercase)
	require.False(t, profile.PasswordRequireNumber)
	require.False(t, profile.PasswordRequireSymbol)
	require.Equal(t, 0, profile.PasswordHistoryLimit)
	require.Equal(t, 0, profile.PasswordMaxAgeDays)
	require.False(t, profile.CSRFEnabled)
}

func TestSecurityProfileCanBeOverriddenByExplicitConfigValues(t *testing.T) {
	profile := securityProfile(true)

	security := buildSecurityConfig(testSecurityConfig{
		env: map[string]any{
			"SECURITY_ENTERPRISE":                 true,
			"SECURITY_MFA_TOTP_ENABLED":           false,
			"SECURITY_PASSWORD_MIN_LENGTH":        16,
			"SECURITY_PASSWORD_REQUIRE_SYMBOL":    false,
			"SECURITY_PASSWORD_HISTORY_LIMIT":     9,
			"SECURITY_PASSWORD_MAX_AGE_DAYS":      30,
			"SECURITY_CSRF_ENABLED":               false,
			"SECURITY_CSRF_TRUSTED_ORIGINS":       "https://admin.example.com",
			"SECURITY_SENSITIVE_FIELDS":           "",
			"SECURITY_PASSWORD_REQUIRE_NUMBER":    true,
			"SECURITY_PASSWORD_REQUIRE_UPPERCASE": true,
			"SECURITY_PASSWORD_REQUIRE_LOWERCASE": true,
		},
	}, profile)

	mfa := security["mfa"].(map[string]any)
	password := security["password"].(map[string]any)
	csrf := security["csrf"].(map[string]any)

	require.True(t, security["enterprise"].(bool))
	require.False(t, mfa["totp_enabled"].(bool))
	require.Equal(t, 16, password["min_length"])
	require.False(t, password["require_symbol"].(bool))
	require.Equal(t, 9, password["history_limit"])
	require.Equal(t, 30, password["max_age_days"])
	require.False(t, csrf["enabled"].(bool))
	require.Equal(t, []string{"https://admin.example.com"}, csrf["trusted_origins"])
}

func TestSecurityCSRFTrustedOriginsDefaultsToAppURL(t *testing.T) {
	profile := securityProfile(true)

	security := buildSecurityConfig(testSecurityConfig{
		env: map[string]any{
			"APP_URL":                       "https://ops.example.com",
			"SECURITY_CSRF_TRUSTED_ORIGINS": "",
		},
	}, profile)

	csrf := security["csrf"].(map[string]any)

	require.Equal(t, []string{"https://ops.example.com"}, csrf["trusted_origins"])
}

func TestSecurityCSRFTrustedOriginsStaysEmptyWithoutAppURL(t *testing.T) {
	profile := securityProfile(true)

	security := buildSecurityConfig(testSecurityConfig{
		env: map[string]any{
			"APP_URL":                       "",
			"SECURITY_CSRF_TRUSTED_ORIGINS": "",
		},
	}, profile)

	csrf := security["csrf"].(map[string]any)

	require.Empty(t, csrf["trusted_origins"])
}

func TestSecurityProfileIgnoresEmptyExplicitConfigValues(t *testing.T) {
	profile := securityProfile(false)

	security := buildSecurityConfig(testSecurityConfig{
		env: map[string]any{
			"SECURITY_MFA_TOTP_ENABLED":           "",
			"SECURITY_PASSWORD_MIN_LENGTH":        "",
			"SECURITY_PASSWORD_REQUIRE_UPPERCASE": "",
			"SECURITY_PASSWORD_HISTORY_LIMIT":     "",
			"SECURITY_CSRF_ENABLED":               "",
			"SECURITY_ENTERPRISE":                 "",
		},
	}, profile)

	mfa := security["mfa"].(map[string]any)
	password := security["password"].(map[string]any)
	csrf := security["csrf"].(map[string]any)

	require.False(t, security["enterprise"].(bool))
	require.False(t, mfa["totp_enabled"].(bool))
	require.Equal(t, 6, password["min_length"])
	require.False(t, password["require_uppercase"].(bool))
	require.Equal(t, 0, password["history_limit"])
	require.False(t, csrf["enabled"].(bool))
}

type testSecurityConfig struct {
	env map[string]any
}

func (c testSecurityConfig) Env(name string, fallback ...any) any {
	if value, ok := c.env[name]; ok {
		return value
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return nil
}

func (c testSecurityConfig) EnvString(name string, fallback ...string) string {
	if value, ok := c.env[name]; ok {
		if text, ok := value.(string); ok {
			return text
		}
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}
