package services

import (
	"testing"
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"goravel/app/models"
)

func TestLoginErrorResultMapsBusinessRule(t *testing.T) {
	result := LoginErrorResult(BusinessError{Message: "密码已过期，请修改密码"})

	require.Equal(t, 422, result.Code)
	require.Equal(t, "密码已过期，请修改密码", result.Message)
}

func TestPasswordHistoryRejectsRecentPasswordHash(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("OldPass1!"), bcrypt.DefaultCost)
	require.NoError(t, err)

	rows := []models.UserPasswordHistory{{Password: string(hash)}}

	require.True(t, passwordMatchesHistory(rows, "OldPass1!"))
	require.False(t, passwordMatchesHistory(rows, "NewPass1!"))
}

func TestPasswordChangedAtUsesExistingUserTimestamp(t *testing.T) {
	createdAt := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC)

	require.Equal(t, updatedAt, passwordChangedAt(models.User{
		Timestamps: models.Timestamps{CreatedAt: createdAt, UpdatedAt: updatedAt},
	}))
	require.Equal(t, createdAt, passwordChangedAt(models.User{
		Timestamps: models.Timestamps{CreatedAt: createdAt},
	}))
}

func TestPasswordChangeChallengeUserIDDoesNotConsumeToken(t *testing.T) {
	cache := newTestCache()
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.password.change_challenge_minutes": 5,
	})
	defer restore()

	token, err := IssuePasswordChangeChallenge("tenant:demo", 42)
	require.NoError(t, err)

	userID, err := PasswordChangeChallengeUserID("tenant:demo", token)
	require.NoError(t, err)
	require.Equal(t, uint64(42), userID)

	userID, err = ConsumePasswordChangeChallenge("tenant:demo", token)
	require.NoError(t, err)
	require.Equal(t, uint64(42), userID)
	_, err = PasswordChangeChallengeUserID("tenant:demo", token)
	require.ErrorIs(t, err, ErrPasswordChangeTokenInvalid)
}
