package memory

import (
	"testing"
)

// BenchmarkGetHot_CacheHit_BeforeDeepCopy was ~5000 ns/op with 5 allocs
// After removing deep copy, should be faster

func BenchmarkGetHot_CacheHit_NoCopy(b *testing.B) {
	tmpDir := b.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Warm up - load once
	cache.GetHot()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.GetHot()
		}
	})
}
