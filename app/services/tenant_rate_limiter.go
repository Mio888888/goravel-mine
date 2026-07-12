package services

import (
	"encoding/json"
	"fmt"
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"

	"goravel/app/facades"
)

const (
	tenantRateWindowSeconds = 60
	tenantRateBucketTTL     = 2 * time.Minute
	tenantRateLockTTL       = 5 * time.Second
)

var tenantRateNow = time.Now
var tenantRateCache = func() contractscache.Driver {
	return facades.Cache()
}

type TenantRateLimiter struct{}

func NewTenantRateLimiter() *TenantRateLimiter {
	return &TenantRateLimiter{}
}

func (l *TenantRateLimiter) Allow(tenant Tenant, limit int64) error {
	if limit <= 0 {
		return nil
	}

	var total int64
	var incErr error
	cache := tenantRateCache()
	lock := cache.Lock(tenantRateLockKey(tenant), tenantRateLockTTL)
	ok := lock.BlockWithTicker(time.Second, 10*time.Millisecond, func() {
		total, incErr = l.incrementAndSum(cache, tenant, limit)
	})
	if !ok {
		return ErrQuotaExceeded
	}
	if incErr != nil {
		return incErr
	}
	if total > limit {
		return ErrQuotaExceeded
	}
	return nil
}

func (l *TenantRateLimiter) incrementAndSum(cache contractscache.Driver, tenant Tenant, limit int64) (int64, error) {
	now := tenantRateNow().UTC().Truncate(time.Second)
	key := tenantRateWindowKey(tenant)
	hits := recentTenantRateHits(cache.GetString(key), now)
	if int64(len(hits)) >= limit {
		return limit + 1, nil
	}
	hits = append(hits, now.UnixNano())
	raw, err := json.Marshal(hits)
	if err != nil {
		return 0, err
	}
	if err := cache.Put(key, string(raw), tenantRateBucketTTL); err != nil {
		return 0, err
	}
	return int64(len(hits)), nil
}

func recentTenantRateHits(raw string, now time.Time) []int64 {
	hits := make([]int64, 0)
	_ = json.Unmarshal([]byte(raw), &hits)
	cutoff := now.Add(-tenantRateWindowSeconds * time.Second).UnixNano()
	recent := hits[:0]
	for _, hit := range hits {
		if hit > cutoff {
			recent = append(recent, hit)
		}
	}
	return recent
}

func tenantRateLockKey(tenant Tenant) string {
	return fmt.Sprintf("tenant:rate:lock:%d:%s", tenant.ID, tenant.Code)
}

func tenantRateWindowKey(tenant Tenant) string {
	return fmt.Sprintf("tenant:rate:%d:%s", tenant.ID, tenant.Code)
}
