package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCSRFOriginAllowedRejectsWildcardTrustedOrigin(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.csrf.trusted_origins": []string{"*"},
	})
	defer restore()

	require.False(t, CSRFOriginAllowed("https://evil.example"))
}

func TestCSRFOriginAllowedAcceptsConfiguredOrigin(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.csrf.trusted_origins": []string{"https://admin.example"},
	})
	defer restore()

	require.True(t, CSRFOriginAllowed("https://admin.example/path"))
	require.False(t, CSRFOriginAllowed("https://evil.example"))
}

func TestCSRFCookieSecureDefaultsOnForSameSiteNone(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.csrf.same_site":     "none",
		"security.csrf.cookie_secure": "",
	})
	defer restore()

	require.True(t, CSRFCookieSecure())
}

func TestCSRFCookieSecureCanBeConfigured(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.csrf.same_site":     "lax",
		"security.csrf.cookie_secure": "true",
	})
	defer restore()

	require.True(t, CSRFCookieSecure())

	restore = setSecurityPolicyConfig(t, map[string]any{
		"security.csrf.same_site":     "none",
		"security.csrf.cookie_secure": "false",
	})
	defer restore()

	require.False(t, CSRFCookieSecure())
}
