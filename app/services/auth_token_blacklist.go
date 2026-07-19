package services

import (
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"

	"goravel/app/facades"
)

const tokenBlacklistPrefix = "jwt:blacklist:"

var tokenBlacklistCache = func() contractscache.Driver {
	return facades.Cache()
}

func blacklistToken(token string, ttl time.Duration) error {
	if token == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = 100 * 365 * 24 * time.Hour
	}
	return tokenBlacklistCache().Put(blacklistedTokenKey(token), true, ttl)
}

func tokenBlacklisted(token string) bool {
	if token == "" {
		return false
	}
	return tokenBlacklistCache().Has(blacklistedTokenKey(token))
}

func blacklistedTokenKey(token string) string {
	return tokenBlacklistPrefix + sha256Hex([]byte(token))
}
