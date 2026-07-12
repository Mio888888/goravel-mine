package unit

import (
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/facades"
	"goravel/app/services"
)

func TestJWTSecretRequiresConfiguredKeyOutsideLocal(t *testing.T) {
	originalSecret := facades.Config().GetString("jwt.secret")
	originalAppKey := facades.Config().GetString("app.key")
	originalEnv := facades.Config().GetString("app.env")
	t.Cleanup(func() {
		facades.Config().Add("jwt.secret", originalSecret)
		facades.Config().Add("app.key", originalAppKey)
		facades.Config().Add("app.env", originalEnv)
	})

	facades.Config().Add("jwt.secret", "")
	facades.Config().Add("app.key", "")
	facades.Config().Add("app.env", "production")

	_, err := services.JWTSecret()
	require.Error(t, err)
}

func TestJWTSecretAllowsDevelopmentFallbackOnlyInLocal(t *testing.T) {
	originalSecret := facades.Config().GetString("jwt.secret")
	originalAppKey := facades.Config().GetString("app.key")
	originalEnv := facades.Config().GetString("app.env")
	t.Cleanup(func() {
		facades.Config().Add("jwt.secret", originalSecret)
		facades.Config().Add("app.key", originalAppKey)
		facades.Config().Add("app.env", originalEnv)
	})

	facades.Config().Add("jwt.secret", "")
	facades.Config().Add("app.key", "")
	facades.Config().Add("app.env", "local")

	secret, err := services.JWTSecret()
	require.NoError(t, err)
	require.Equal(t, "local-development-jwt-secret", secret)
}

func TestJWTTTLSecondsUseConfigMinutes(t *testing.T) {
	originalTTL := facades.Config().GetInt("jwt.ttl", 60)
	originalRefreshTTL := facades.Config().GetInt("jwt.refresh_ttl", 20160)
	t.Cleanup(func() {
		facades.Config().Add("jwt.ttl", originalTTL)
		facades.Config().Add("jwt.refresh_ttl", originalRefreshTTL)
	})

	facades.Config().Add("jwt.ttl", 2)
	facades.Config().Add("jwt.refresh_ttl", 3)

	require.Equal(t, 120, services.AccessTokenTTLSeconds())
	require.Equal(t, 180, services.RefreshTokenTTLSeconds())
}
