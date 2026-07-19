package auth

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	authcontract "goravel/app/contracts/auth"
	"goravel/app/facades"
)

func TestApplicationTokenCodecSeparatesTenantAndPlatformSubjects(t *testing.T) {
	useJWTTestSecret(t)

	tenantToken, err := IssueApplicationToken(TenantTokenSubject, 7, 11, "access", 60)
	require.NoError(t, err)
	tenantInfo, err := ParseApplicationToken("Bearer "+tenantToken, TokenRequirements{
		Subject:       TenantTokenSubject,
		Type:          "access",
		RequireTenant: true,
	})
	require.NoError(t, err)
	require.Equal(t, TokenInfo{UserID: 7, TenantID: 11}, tenantInfo)

	_, err = ParseApplicationToken("Bearer "+tenantToken, TokenRequirements{
		Subject: PlatformTokenSubject,
		Type:    "access",
	})
	require.ErrorIs(t, err, authcontract.ErrUnauthorized)

	platformToken, err := IssueApplicationToken(PlatformTokenSubject, 9, 0, "access", 60)
	require.NoError(t, err)
	_, err = ParseApplicationToken("Bearer "+platformToken, TokenRequirements{
		Subject:       TenantTokenSubject,
		Type:          "access",
		RequireTenant: true,
	})
	require.ErrorIs(t, err, authcontract.ErrUnauthorized)
}

func TestApplicationTokenCodecRejectsUnexpectedAlgorithm(t *testing.T) {
	secret := useJWTTestSecret(t)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{
		"sub":  TenantTokenSubject,
		"uid":  7,
		"tid":  11,
		"type": "access",
	}).SignedString([]byte(secret))
	require.NoError(t, err)

	_, err = ParseApplicationToken("Bearer "+token, TokenRequirements{
		Subject:       TenantTokenSubject,
		Type:          "access",
		RequireTenant: true,
	})
	require.ErrorIs(t, err, authcontract.ErrUnauthorized)
}

func useJWTTestSecret(t *testing.T) string {
	t.Helper()
	originalSecret := facades.Config().GetString("jwt.secret")
	secret := "jwt-token-test-secret"
	facades.Config().Add("jwt.secret", secret)
	t.Cleanup(func() {
		facades.Config().Add("jwt.secret", originalSecret)
	})
	return secret
}
