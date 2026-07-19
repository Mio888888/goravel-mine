package services

import (
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
