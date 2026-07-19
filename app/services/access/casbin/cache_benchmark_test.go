package casbin

import (
	"testing"
	"time"
)

func BenchmarkCasbinCapacity(b *testing.B) {
	cache := NewCache(64, time.Minute, time.Now)
	loader := func() (Authorizer, error) { return cacheTestAuthorizer{}, nil }
	if _, err := cache.Get("tenant_1", loader); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := cache.Get("tenant_1", loader); err != nil {
			b.Fatal(err)
		}
	}
}
