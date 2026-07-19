package tenant

import (
	"encoding/json"
	"fmt"
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"

	tenantcontract "goravel/app/contracts/tenant"
	"goravel/app/facades"
)

const (
	rateWindowSeconds = 60
	rateBucketTTL     = 2 * time.Minute
	rateLockTTL       = 5 * time.Second
)

var rateNow = time.Now
var rateCache = func() contractscache.Driver {
	return facades.Cache()
}

type RateLimiter struct{}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{}
}

func (l *RateLimiter) Allow(tenant tenantcontract.Tenant, limit int64) error {
	if limit <= 0 {
		return nil
	}

	var total int64
	var incrementErr error
	cache := rateCache()
	lock := cache.Lock(rateLockKey(tenant), rateLockTTL)
	ok := lock.BlockWithTicker(time.Second, 10*time.Millisecond, func() {
		total, incrementErr = l.incrementAndSum(cache, tenant, limit)
	})
	if !ok {
		return tenantcontract.ErrQuotaExceeded
	}
	if incrementErr != nil {
		return incrementErr
	}
	if total > limit {
		return tenantcontract.ErrQuotaExceeded
	}
	return nil
}

func (l *RateLimiter) incrementAndSum(
	cache contractscache.Driver,
	tenant tenantcontract.Tenant,
	limit int64,
) (int64, error) {
	now := rateNow().UTC().Truncate(time.Second)
	key := rateWindowKey(tenant)
	hits := recentRateHits(cache.GetString(key), now)
	if int64(len(hits)) >= limit {
		return limit + 1, nil
	}
	hits = append(hits, now.UnixNano())
	raw, err := json.Marshal(hits)
	if err != nil {
		return 0, err
	}
	if err := cache.Put(key, string(raw), rateBucketTTL); err != nil {
		return 0, err
	}
	return int64(len(hits)), nil
}

func recentRateHits(raw string, now time.Time) []int64 {
	hits := make([]int64, 0)
	_ = json.Unmarshal([]byte(raw), &hits)
	cutoff := now.Add(-rateWindowSeconds * time.Second).UnixNano()
	recent := hits[:0]
	for _, hit := range hits {
		if hit > cutoff {
			recent = append(recent, hit)
		}
	}
	return recent
}

func rateLockKey(tenant tenantcontract.Tenant) string {
	return fmt.Sprintf("tenant:rate:lock:%d:%s", tenant.ID, tenant.Code)
}

func rateWindowKey(tenant tenantcontract.Tenant) string {
	return fmt.Sprintf("tenant:rate:%d:%s", tenant.ID, tenant.Code)
}
