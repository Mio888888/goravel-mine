package services

import (
	"strings"
	"testing"

	contractshash "github.com/goravel/framework/contracts/hash"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type testPasswordHasher struct{}

func (testPasswordHasher) Make(value string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.MinCost)
	return string(hash), err
}

func (testPasswordHasher) Check(value, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(value)) == nil
}

func (testPasswordHasher) NeedsRehash(string) bool {
	return false
}

type prefixedPasswordHasher struct{}

func (prefixedPasswordHasher) Make(value string) (string, error) {
	return "configured:" + value, nil
}

func (prefixedPasswordHasher) Check(value, hash string) bool {
	return hash == "configured:"+value
}

func (prefixedPasswordHasher) NeedsRehash(string) bool {
	return false
}

func useTestPasswordHasher(t *testing.T) {
	t.Helper()
	original := passwordHashProvider
	passwordHashProvider = func() contractshash.Hash {
		return testPasswordHasher{}
	}
	t.Cleanup(func() {
		passwordHashProvider = original
	})
}

func TestPasswordHashUsesConfiguredProvider(t *testing.T) {
	useTestPasswordHasher(t)

	hash, err := makePasswordHash("StrongPass1!")

	require.NoError(t, err)
	require.True(t, passwordHashMatches(hash, "StrongPass1!"))
	require.False(t, passwordHashMatches(hash, "WrongPass1!"))
}

func TestSecretHashUsesConfiguredProvider(t *testing.T) {
	original := passwordHashProvider
	passwordHashProvider = func() contractshash.Hash {
		return prefixedPasswordHasher{}
	}
	t.Cleanup(func() {
		passwordHashProvider = original
	})

	hash, err := makeSecretHash("recovery-code")

	require.NoError(t, err)
	require.Equal(t, "configured:recovery-code", hash)
	require.True(t, secretHashMatches(hash, "recovery-code"))
	require.False(t, secretHashMatches(hash, "other-code"))
}

func TestSecretHashMatchesLegacyBcryptAfterProviderChange(t *testing.T) {
	legacy, err := bcrypt.GenerateFromPassword([]byte("legacy-code"), bcrypt.MinCost)
	require.NoError(t, err)
	original := passwordHashProvider
	passwordHashProvider = func() contractshash.Hash {
		return prefixedPasswordHasher{}
	}
	t.Cleanup(func() {
		passwordHashProvider = original
	})

	require.True(t, strings.HasPrefix(string(legacy), "$2"))
	require.True(t, secretHashMatches(string(legacy), "legacy-code"))
	require.False(t, secretHashMatches(string(legacy), "wrong-code"))
}
