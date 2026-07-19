package auth

import (
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"

	"goravel/app/facades"
	"goravel/app/support/digest"
)

const tokenBlacklistPrefix = "jwt:blacklist:"

var tokenBlacklistCache = func() contractscache.Driver {
	return facades.Cache()
}

func BlacklistToken(token string, ttl time.Duration) error {
	if token == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = 100 * 365 * 24 * time.Hour
	}
	return tokenBlacklistCache().Put(BlacklistedTokenKey(token), true, ttl)
}

func TokenBlacklisted(token string) bool {
	if token == "" {
		return false
	}
	return tokenBlacklistCache().Has(BlacklistedTokenKey(token))
}

func BlacklistedTokenKey(token string) string {
	return tokenBlacklistPrefix + digest.SHA256Hex([]byte(token))
}
