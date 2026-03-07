package memory

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkGetLongterm_CacheHit benchmarks cache hit performance
func BenchmarkGetLongterm_CacheHit(b *testing.B) {
	tmpDir := b.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Warm up cache
	cache.GetLongterm()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.GetLongterm()
	}
}

// BenchmarkGetHot_CacheHit benchmarks cache hit performance
func BenchmarkGetHot_CacheHit(b *testing.B) {
	tmpDir := b.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Warm up cache
	cache.GetHot()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.GetHot()
	}
}

// BenchmarkGetRecent benchmarks recent message retrieval
func BenchmarkGetRecent(b *testing.B) {
	tmpDir := b.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.GetRecent()
	}
}

// BenchmarkAppend benchmarks message appending
func BenchmarkAppend(b *testing.B) {
	tmpDir := b.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := Message{
			ID:        fmt.Sprintf("bench-%d", i), // 修复：唯一 ID
			Content:   "Benchmark test message",
			Timestamp: time.Now(),
		}
		if err := cache.Append(msg); err != nil {
			b.Fatalf("Append failed: %v", err)
		}
	}
}

// BenchmarkSearch benchmarks FTS5 search
func BenchmarkSearch(b *testing.B) {
	tmpDir := b.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Insert test data
	for i := 0; i < 100; i++ {
		msg := Message{
			ID:        fmt.Sprintf("search-%d", i), // 修复：唯一 ID
			Content:   "Benchmark search test message",
			Timestamp: time.Now(),
		}
		if err := cache.Append(msg); err != nil {
			b.Fatalf("Append failed: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cache.Search("Benchmark")
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

// BenchmarkContextBuilder_Build benchmarks context building
func BenchmarkContextBuilder_Build(b *testing.B) {
	tmpDir := b.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	builder := NewContextBuilder(cache, 4000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := Message{
			ID:        fmt.Sprintf("ctx-%d", i),
			Content:   "Benchmark context test",
			Timestamp: time.Now(),
		}
		_, err := builder.Build(msg)
		if err != nil {
			b.Fatalf("Build failed: %v", err)
		}
	}
}
