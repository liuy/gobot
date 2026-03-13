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
		chatID := "test-chat"
		for i := 0; i < 25; i++ {
			msg := Message{
				ID:        string(rune('a' + i)),
				Content:   "Test message",
				Timestamp: time.Now(),
				ChatID:    chatID,
			}
			if err := cache.Append(msg); err != nil {
				t.Errorf("Append failed: %v", err)
			}
		}

		recent := cache.GetRecent(chatID, 20)
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

	// 3. Test Search
	t.Run("Search", func(t *testing.T) {
		results, err := cache.Search("Test")
		if err != nil {
			t.Errorf("Search failed: %v", err)
		}
		if len(results) == 0 {
			t.Error("Expected search results")
		}
	})

	// 4. Test ContextBuilder
	t.Run("ContextBuilder", func(t *testing.T) {
		builder := NewContextBuilder(cache, 1000)
		msg := Message{
			ID:        "test-msg",
			Content:   "Build context test",
			Timestamp: time.Now(),
			ChatID:    "test-chat",
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
	chatID := "test-chat"
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.GetRecent(chatID, 20)
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

// TestIntegration_CloseDrainsMessages tests that Close() drains pending messages
func TestIntegration_CloseDrainsMessages(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Append messages
	for i := 0; i < 10; i++ {
		msg := Message{
			ID:        string(rune('a' + i)),
			Content:   "Drain test message",
			Timestamp: time.Now(),
		}
		if err := cache.Append(msg); err != nil {
			t.Errorf("Append failed: %v", err)
		}
	}

	// Close should drain and save to cold.db
	if err := cache.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify messages were saved to cold.db
	dbPath := filepath.Join(tmpDir, "memory", "cold.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Expected cold.db to exist after Close()")
	}
}
