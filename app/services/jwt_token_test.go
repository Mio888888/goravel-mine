package services

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"goravel/app/facades"
)

func TestApplicationTokenCodecSeparatesTenantAndPlatformSubjects(t *testing.T) {
	useJWTTestSecret(t)

	tenantToken, err := issueApplicationToken(tenantTokenSubject, 7, 11, "access", 60)
	require.NoError(t, err)
	tenantInfo, err := parseApplicationToken("Bearer "+tenantToken, jwtTokenRequirements{
		Subject:       tenantTokenSubject,
		Type:          "access",
		RequireTenant: true,
	})
	require.NoError(t, err)
	require.Equal(t, TokenInfo{UserID: 7, TenantID: 11}, tenantInfo)

	_, err = parseApplicationToken("Bearer "+tenantToken, jwtTokenRequirements{
		Subject: platformTokenSubject,
		Type:    "access",
	})
	require.ErrorIs(t, err, ErrUnauthorized)

	platformToken, err := issueApplicationToken(platformTokenSubject, 9, 0, "access", 60)
	require.NoError(t, err)
	_, err = parseApplicationToken("Bearer "+platformToken, jwtTokenRequirements{
		Subject:       tenantTokenSubject,
		Type:          "access",
		RequireTenant: true,
	})
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestApplicationTokenCodecRejectsUnexpectedAlgorithm(t *testing.T) {
	secret := useJWTTestSecret(t)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{
		"sub":  tenantTokenSubject,
		"uid":  7,
		"tid":  11,
		"type": "access",
	}).SignedString([]byte(secret))
	require.NoError(t, err)

	_, err = parseApplicationToken("Bearer "+token, jwtTokenRequirements{
		Subject:       tenantTokenSubject,
		Type:          "access",
		RequireTenant: true,
	})
	require.ErrorIs(t, err, ErrUnauthorized)
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
