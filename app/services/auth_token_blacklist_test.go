package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"
	"github.com/stretchr/testify/require"
)

func TestTokenBlacklistUsesSharedCacheAndHashesToken(t *testing.T) {
	cache := newTestCache()
	original := tokenBlacklistCache
	t.Cleanup(func() { tokenBlacklistCache = original })
	tokenBlacklistCache = func() contractscache.Driver { return cache }

	token := "token.secret.value"
	require.NoError(t, blacklistToken(token, time.Minute))

	require.True(t, tokenBlacklisted(token))
	require.False(t, cache.Has("jwt:blacklist:"+token))
	require.NotEmpty(t, blacklistedTokenKey(token))
	require.False(t, strings.Contains(blacklistedTokenKey(token), token))
}

func TestTokenBlacklistIgnoresBlankToken(t *testing.T) {
	cache := newTestCache()
	original := tokenBlacklistCache
	t.Cleanup(func() { tokenBlacklistCache = original })
	tokenBlacklistCache = func() contractscache.Driver { return cache }

	require.NoError(t, blacklistToken("", time.Minute))

	require.False(t, tokenBlacklisted(""))
}

func TestTokenBlacklistReturnsCacheWriteError(t *testing.T) {
	cache := failingPutCache{Driver: newTestCache(), err: errors.New("cache down")}
	original := tokenBlacklistCache
	t.Cleanup(func() { tokenBlacklistCache = original })
	tokenBlacklistCache = func() contractscache.Driver { return cache }

	err := blacklistToken("token", time.Minute)

	require.EqualError(t, err, "cache down")
}

type failingPutCache struct {
	contractscache.Driver
	err error
}

func (c failingPutCache) Put(string, any, time.Duration) error {
	return c.err
}
