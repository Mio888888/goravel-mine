package services

import (
	"testing"
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"
	"github.com/stretchr/testify/require"

	"goravel/app/models"
)

func TestTenantRateLimiterUsesRollingWindow(t *testing.T) {
	cache := newTestCache()
	originalCache := tenantRateCache
	t.Cleanup(func() { tenantRateCache = originalCache })
	tenantRateCache = func() contractscache.Driver { return cache }
	now := time.Date(2026, 7, 3, 12, 0, 59, 0, time.UTC)
	t.Cleanup(func() { tenantRateNow = time.Now })
	tenantRateNow = func() time.Time { return now }

	tenant := Tenant{ID: 7, Code: "rolling", Quotas: models.JSONMap{"api_rate_per_minute": 2}}
	limiter := NewTenantRateLimiter()

	require.NoError(t, limiter.Allow(tenant, 2))
	now = now.Add(time.Second)
	require.NoError(t, limiter.Allow(tenant, 2))
	now = now.Add(time.Second)
	require.ErrorIs(t, limiter.Allow(tenant, 2), ErrQuotaExceeded)
}

func TestTenantRateLimiterExpiresOldBuckets(t *testing.T) {
	cache := newTestCache()
	originalCache := tenantRateCache
	t.Cleanup(func() { tenantRateCache = originalCache })
	tenantRateCache = func() contractscache.Driver { return cache }
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	t.Cleanup(func() { tenantRateNow = time.Now })
	tenantRateNow = func() time.Time { return now }

	tenant := Tenant{ID: 8, Code: "expire", Quotas: models.JSONMap{"api_rate_per_minute": 1}}
	limiter := NewTenantRateLimiter()

	require.NoError(t, limiter.Allow(tenant, 1))
	now = now.Add(time.Minute)
	require.NoError(t, limiter.Allow(tenant, 1))
}

func TestTenantRateLimiterDoesNotAppendRejectedHits(t *testing.T) {
	cache := newTestCache()
	originalCache := tenantRateCache
	t.Cleanup(func() { tenantRateCache = originalCache })
	tenantRateCache = func() contractscache.Driver { return cache }
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	t.Cleanup(func() { tenantRateNow = time.Now })
	tenantRateNow = func() time.Time { return now }

	tenant := Tenant{ID: 9, Code: "bounded", Quotas: models.JSONMap{"api_rate_per_minute": 1}}
	limiter := NewTenantRateLimiter()

	require.NoError(t, limiter.Allow(tenant, 1))
	require.ErrorIs(t, limiter.Allow(tenant, 1), ErrQuotaExceeded)
	require.ErrorIs(t, limiter.Allow(tenant, 1), ErrQuotaExceeded)
	require.JSONEq(t, `[1783080000000000000]`, cache.GetString(tenantRateWindowKey(tenant)))
}
