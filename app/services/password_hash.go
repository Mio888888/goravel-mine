package services

import (
	contractshash "github.com/goravel/framework/contracts/hash"

	"goravel/app/facades"
)

var passwordHashProvider = func() contractshash.Hash {
	return facades.Hash()
}

func makePasswordHash(password string) (string, error) {
	return passwordHashProvider().Make(password)
}

func passwordHashMatches(hash, password string) bool {
	return passwordHashProvider().Check(password, hash)
}
