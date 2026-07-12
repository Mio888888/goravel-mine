package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type cacheTestAuthorizer struct{}

func (cacheTestAuthorizer) Enforce(...interface{}) (bool, error) { return true, nil }

func TestCasbinEnforcerCacheSingleFlightAndInvalidation(t *testing.T) {
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	cache := newCasbinEnforcerCache(4, time.Minute, func() time.Time { return now })
	var loads atomic.Int32
	loader := func() (casbinAuthorizer, error) {
		loads.Add(1)
		time.Sleep(10 * time.Millisecond)
		return cacheTestAuthorizer{}, nil
	}

	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := cache.Get("tenant_1", loader)
			require.NoError(t, err)
		}()
	}
	wait.Wait()
	require.Equal(t, int32(1), loads.Load())

	cache.Invalidate("tenant_1")
	_, err := cache.Get("tenant_1", loader)
	require.NoError(t, err)
	require.Equal(t, int32(2), loads.Load())
}

func TestCasbinEnforcerCacheTTLAndLRU(t *testing.T) {
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	cache := newCasbinEnforcerCache(2, time.Minute, func() time.Time { return now })
	loads := map[string]int{}
	load := func(key string) casbinAuthorizerLoader {
		return func() (casbinAuthorizer, error) {
			loads[key]++
			return cacheTestAuthorizer{}, nil
		}
	}

	_, _ = cache.Get("a", load("a"))
	_, _ = cache.Get("b", load("b"))
	_, _ = cache.Get("a", load("a"))
	_, _ = cache.Get("c", load("c"))
	_, _ = cache.Get("b", load("b"))
	require.Equal(t, 2, loads["b"])

	now = now.Add(2 * time.Minute)
	_, _ = cache.Get("a", load("a"))
	require.Equal(t, 2, loads["a"])
}

func TestCasbinEnforcerCacheReset(t *testing.T) {
	cache := newCasbinEnforcerCache(2, time.Minute, time.Now)
	_, err := cache.Get("a", func() (casbinAuthorizer, error) { return cacheTestAuthorizer{}, nil })
	require.NoError(t, err)
	require.Equal(t, 1, cache.Metrics().Entries)

	cache.Reset()

	require.Equal(t, CasbinEnforcerCacheMetrics{}, cache.Metrics())
}
