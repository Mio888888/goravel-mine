package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/goravel/framework/cache"
	contractscache "github.com/goravel/framework/contracts/cache"
	"github.com/stretchr/testify/require"
)

func TestTokenBlacklistUsesSharedCacheAndHashesToken(t *testing.T) {
	cache := newTestCache()
	original := tokenBlacklistCache
	t.Cleanup(func() { tokenBlacklistCache = original })
	tokenBlacklistCache = func() contractscache.Driver { return cache }

	token := "token.secret.value"
	require.NoError(t, BlacklistToken(token, time.Minute))

	require.True(t, TokenBlacklisted(token))
	require.False(t, cache.Has("jwt:blacklist:"+token))
	require.NotEmpty(t, BlacklistedTokenKey(token))
	require.False(t, strings.Contains(BlacklistedTokenKey(token), token))
}

func TestTokenBlacklistIgnoresBlankToken(t *testing.T) {
	cache := newTestCache()
	original := tokenBlacklistCache
	t.Cleanup(func() { tokenBlacklistCache = original })
	tokenBlacklistCache = func() contractscache.Driver { return cache }

	require.NoError(t, BlacklistToken("", time.Minute))

	require.False(t, TokenBlacklisted(""))
}

func TestTokenBlacklistReturnsCacheWriteError(t *testing.T) {
	cache := failingPutCache{Driver: newTestCache(), err: errors.New("cache down")}
	original := tokenBlacklistCache
	t.Cleanup(func() { tokenBlacklistCache = original })
	tokenBlacklistCache = func() contractscache.Driver { return cache }

	err := BlacklistToken("token", time.Minute)

	require.EqualError(t, err, "cache down")
}

type failingPutCache struct {
	contractscache.Driver
	err error
}

func (c failingPutCache) Put(string, any, time.Duration) error {
	return c.err
}

func newTestCache() *cache.Memory {
	driver, _ := cache.NewMemory(testConfig{"cache.prefix": "test"})
	return driver
}

type testConfig map[string]any

func (c testConfig) Env(name string, defaultValue ...any) any {
	return c.Get(name, defaultValue...)
}

func (c testConfig) EnvString(name string, defaultValue ...string) string {
	return c.GetString(name, defaultValue...)
}

func (c testConfig) EnvBool(name string, defaultValue ...bool) bool {
	return c.GetBool(name, defaultValue...)
}

func (c testConfig) Add(name string, configuration any) {
	c[name] = configuration
}

func (c testConfig) Get(path string, defaultValue ...any) any {
	if value, ok := c[path]; ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return nil
}

func (c testConfig) GetString(path string, defaultValue ...string) string {
	if value, ok := c[path].(string); ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func (c testConfig) GetInt(path string, defaultValue ...int) int {
	if value, ok := c[path].(int); ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

func (c testConfig) GetBool(path string, defaultValue ...bool) bool {
	if value, ok := c[path].(bool); ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return false
}

func (c testConfig) GetDuration(path string, defaultValue ...time.Duration) time.Duration {
	if value, ok := c[path].(time.Duration); ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

func (c testConfig) UnmarshalKey(string, any) error {
	return nil
}
