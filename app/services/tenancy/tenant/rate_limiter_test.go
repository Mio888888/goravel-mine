package tenant

import (
	"testing"
	"time"

	"github.com/goravel/framework/cache"
	contractscache "github.com/goravel/framework/contracts/cache"
	"github.com/stretchr/testify/require"

	tenantcontract "goravel/app/contracts/tenant"
	"goravel/app/models"
)

type testConfig map[string]any

func TestRateLimiterUsesRollingWindow(t *testing.T) {
	cache := newTestCache()
	originalCache := rateCache
	t.Cleanup(func() { rateCache = originalCache })
	rateCache = func() contractscache.Driver { return cache }
	now := time.Date(2026, 7, 3, 12, 0, 59, 0, time.UTC)
	t.Cleanup(func() { rateNow = time.Now })
	rateNow = func() time.Time { return now }

	tenant := tenantcontract.Tenant{
		ID: 7, Code: "rolling", Quotas: models.JSONMap{"api_rate_per_minute": 2},
	}
	limiter := NewRateLimiter()

	require.NoError(t, limiter.Allow(tenant, 2))
	now = now.Add(time.Second)
	require.NoError(t, limiter.Allow(tenant, 2))
	now = now.Add(time.Second)
	require.ErrorIs(t, limiter.Allow(tenant, 2), tenantcontract.ErrQuotaExceeded)
}

func TestRateLimiterExpiresOldBuckets(t *testing.T) {
	cache := newTestCache()
	originalCache := rateCache
	t.Cleanup(func() { rateCache = originalCache })
	rateCache = func() contractscache.Driver { return cache }
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	t.Cleanup(func() { rateNow = time.Now })
	rateNow = func() time.Time { return now }

	tenant := tenantcontract.Tenant{
		ID: 8, Code: "expire", Quotas: models.JSONMap{"api_rate_per_minute": 1},
	}
	limiter := NewRateLimiter()

	require.NoError(t, limiter.Allow(tenant, 1))
	now = now.Add(time.Minute)
	require.NoError(t, limiter.Allow(tenant, 1))
}

func TestRateLimiterDoesNotAppendRejectedHits(t *testing.T) {
	cache := newTestCache()
	originalCache := rateCache
	t.Cleanup(func() { rateCache = originalCache })
	rateCache = func() contractscache.Driver { return cache }
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	t.Cleanup(func() { rateNow = time.Now })
	rateNow = func() time.Time { return now }

	tenant := tenantcontract.Tenant{
		ID: 9, Code: "bounded", Quotas: models.JSONMap{"api_rate_per_minute": 1},
	}
	limiter := NewRateLimiter()

	require.NoError(t, limiter.Allow(tenant, 1))
	require.ErrorIs(t, limiter.Allow(tenant, 1), tenantcontract.ErrQuotaExceeded)
	require.ErrorIs(t, limiter.Allow(tenant, 1), tenantcontract.ErrQuotaExceeded)
	require.JSONEq(t, `[1783080000000000000]`, cache.GetString(rateWindowKey(tenant)))
}

func newTestCache() *cache.Memory {
	driver, _ := cache.NewMemory(testConfig{"cache.prefix": "test"})
	return driver
}

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
