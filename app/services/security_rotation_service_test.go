package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoredSecretRotationFindingsReportConfiguredSecrets(t *testing.T) {
	now := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	secrets := []StoredSecretRotationSource{
		{Scope: "tenant:demo", Name: "sso_provider.client_secret", UpdatedAt: now.AddDate(0, 0, -100), Configured: true},
		{Scope: "tenant:demo", Name: "storage_config.secret_key", UpdatedAt: now.AddDate(0, 0, -10), Configured: true},
		{Scope: "tenant:demo", Name: "sso_provider.jwt_secret", UpdatedAt: now.AddDate(0, 0, -200), Configured: false},
	}

	findings := StoredSecretRotationFindings(secrets, now, 90)

	require.Len(t, findings, 2)
	require.Equal(t, "tenant:demo/sso_provider.client_secret", findings[0].Name)
	require.Equal(t, "expired", findings[0].Status)
	require.Equal(t, "tenant:demo/storage_config.secret_key", findings[1].Name)
	require.Equal(t, "ok", findings[1].Status)
}

func TestSSORotationMetadataSetOnlyWhenSecretProvided(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	provider := withSSORotationMetadata(SSOProvider{JWTSecret: "jwt", ClientSecret: "client"}, now)
	require.Equal(t, now, provider.JWTSecretRotatedAt)
	require.Equal(t, now, provider.ClientSecretRotatedAt)

	provider = withSSORotationMetadata(SSOProvider{Name: "demo"}, now)
	require.True(t, provider.JWTSecretRotatedAt.IsZero())
	require.True(t, provider.ClientSecretRotatedAt.IsZero())
}
