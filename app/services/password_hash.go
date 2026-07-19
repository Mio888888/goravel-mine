package services

import (
	"strings"

	contractshash "github.com/goravel/framework/contracts/hash"
	"golang.org/x/crypto/bcrypt"

	"goravel/app/facades"
)

var passwordHashProvider = func() contractshash.Hash {
	return facades.Hash()
}

func makeSecretHash(value string) (string, error) {
	return passwordHashProvider().Make(value)
}

func secretHashMatches(hash, value string) bool {
	if passwordHashProvider().Check(value, hash) {
		return true
	}
	if strings.HasPrefix(hash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(value)) == nil
	}
	return false
}

func makePasswordHash(password string) (string, error) {
	return makeSecretHash(password)
}

func passwordHashMatches(hash, password string) bool {
	return secretHashMatches(hash, password)
}
