//go:build integration
// +build integration

package memory

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestIntegration_FullWorkflow tests the complete memory system workflow
func TestIntegration_FullWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// 1. Test Append and GetRecent
	t.Run("AppendAndGetRecent", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			msg := Message{
				ID:        string(rune('a' + i)),
				Content:   "Test message",
				Timestamp: time.Now(),
			}
			if err := cache.Append(msg); err != nil {
				t.Errorf("Append failed: %v", err)
			}
		}

		recent := cache.GetRecent()
		if len(recent) > 20 {
			t.Errorf("Expected max 20 recent messages, got %d", len(recent))
		}
	})

	// 2. Test GetLongterm
	t.Run("GetLongterm", func(t *testing.T) {
		// Write longterm.md
		memoryDir := filepath.Join(tmpDir, "memory")
		os.MkdirAll(memoryDir, 0755)
		longtermPath := filepath.Join(memoryDir, "longterm.md")
		content := "# User Profile\nName: Test User"
		if err := os.WriteFile(longtermPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write longterm: %v", err)
		}

		longterm, err := cache.GetLongterm()
		if err != nil {
			t.Errorf("GetLongterm failed: %v", err)
		}
		if longterm != content {
			t.Errorf("Expected %q, got %q", content, longterm)
		}
	})

	// 3. Test GetHot
	t.Run("GetHot", func(t *testing.T) {
		hot, err := cache.GetHot()
		if err != nil {
			t.Errorf("GetHot failed: %v", err)
		}
		if hot == nil {
			t.Error("Expected non-nil hot data")
		}
	})

	// 4. Test Search
	t.Run("Search", func(t *testing.T) {
		results, err := cache.Search("Test")
		if err != nil {
			t.Errorf("Search failed: %v", err)
		}
		if len(results) == 0 {
			t.Error("Expected search results")
		}
	})

	// 5. Test ContextBuilder
	t.Run("ContextBuilder", func(t *testing.T) {
		builder := NewContextBuilder(cache, 1000)
		msg := Message{
			ID:        "test-msg",
			Content:   "Build context test",
			Timestamp: time.Now(),
		}
		ctx, err := builder.Build(msg)
		if err != nil {
			t.Errorf("Build failed: %v", err)
		}
		if ctx == nil {
			t.Error("Expected non-nil context")
		}
	})
}

// TestIntegration_ConcurrentAccess tests concurrent read/write operations
func TestIntegration_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	var wg sync.WaitGroup
	numOps := 100

	// Concurrent Appends
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := Message{
				ID:        string(rune('A' + id%26)),
				Content:   "Concurrent test",
				Timestamp: time.Now(),
			}
			cache.Append(msg)
		}(i)
	}

	// Concurrent GetRecent
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.GetRecent()
		}()
	}

	// Concurrent GetHot
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.GetHot()
		}()
	}

	// Concurrent GetLongterm
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.GetLongterm()
		}()
	}

	wg.Wait()
}

// TestIntegration_CloseIdempotent tests that Close() is idempotent
func TestIntegration_CloseIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Close multiple times
	for i := 0; i < 5; i++ {
		if err := cache.Close(); err != nil {
			t.Errorf("Close #%d failed: %v", i+1, err)
		}
	}
}

// TestIntegration_HotMemoryDrain tests that Close() drains hot updates
func TestIntegration_HotMemoryDrain(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Append messages
	for i := 0; i < 10; i++ {
		msg := Message{
			ID:        string(rune('a' + i)),
			Content:   "Drain test keyword",
			Timestamp: time.Now(),
		}
		if err := cache.Append(msg); err != nil {
			t.Errorf("Append failed: %v", err)
		}
	}

	// Close should drain and save
	if err := cache.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify hot.json was saved
	hotPath := filepath.Join(tmpDir, "memory", "hot.json")
	if _, err := os.Stat(hotPath); os.IsNotExist(err) {
		t.Error("Expected hot.json to exist after Close()")
	}

	data, err := loadHot(hotPath)
	if err != nil {
		t.Fatalf("Failed to load hot.json: %v", err)
	}

	// Verify keywords were extracted
	if len(data.RecentKeywords) == 0 {
		t.Error("Expected keywords to be extracted after drain")
	}
}
