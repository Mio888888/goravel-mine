package services

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkTenantConnectionCapacity(b *testing.B) {
	registry := newTenantConnectionRegistry(64, time.Now)
	for index := range 64 {
		if err := registry.Acquire(fmt.Sprintf("tenant_%d", index)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		if !registry.Contains(fmt.Sprintf("tenant_%d", index%64)) {
			b.Fatal("registered tenant missing")
		}
	}
}
