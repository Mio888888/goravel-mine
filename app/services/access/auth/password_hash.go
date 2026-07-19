package auth

import (
	"strings"

	contractshash "github.com/goravel/framework/contracts/hash"
	"golang.org/x/crypto/bcrypt"

	"goravel/app/facades"
)

var passwordHashProvider = func() contractshash.Hash {
	return facades.Hash()
}

func SetHashProviderForTest(provider func() contractshash.Hash) func() {
	original := passwordHashProvider
	if provider != nil {
		passwordHashProvider = provider
	}
	return func() {
		passwordHashProvider = original
	}
}

func MakeSecretHash(value string) (string, error) {
	return passwordHashProvider().Make(value)
}

func SecretHashMatches(hash, value string) bool {
	if passwordHashProvider().Check(value, hash) {
		return true
	}
	if strings.HasPrefix(hash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(value)) == nil
	}
	return false
}

func MakePasswordHash(password string) (string, error) {
	return MakeSecretHash(password)
}

func PasswordHashMatches(hash, password string) bool {
	return SecretHashMatches(hash, password)
}
