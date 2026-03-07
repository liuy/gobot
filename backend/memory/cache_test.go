package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func setupTestCache(t *testing.T) (*MemoryCache, string) {
	t.Helper()
	tmpDir := t.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	return cache, tmpDir
}

func TestNewMemoryCache_CreateDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	memoryDir := filepath.Join(tmpDir, "memory")
	if _, err := os.Stat(memoryDir); os.IsNotExist(err) {
		t.Error("Expected memory/ directory to be created")
	}
}

func TestNewMemoryCache_CreateDatabase(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	dbPath := filepath.Join(tmpDir, "memory", "cold.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Expected cold.db to be created")
	}
}

func TestNewMemoryCache_MissingFiles(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	longterm, err := cache.GetLongterm()
	if err != nil {
		t.Errorf("GetLongterm failed: %v", err)
	}
	if longterm != "" {
		t.Errorf("Expected empty longterm, got: %s", longterm)
	}

	hot, err := cache.GetHot()
	if err != nil {
		t.Errorf("GetHot failed: %v", err)
	}
	if hot == nil {
		t.Error("Expected non-nil hot data")
	}

	recent := cache.GetRecent()
	if len(recent) != 0 {
		t.Errorf("Expected 0 recent messages, got %d", len(recent))
	}
}

func TestGetLongterm_CacheHit(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	longtermPath := filepath.Join(tmpDir, "memory", "longterm.md")
	content := "# About\n\nTest content"
	if err := os.WriteFile(longtermPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write longterm.md: %v", err)
	}

	start := time.Now()
	result1, err := cache.GetLongterm()
	if err != nil {
		t.Fatalf("First GetLongterm failed: %v", err)
	}
	firstDuration := time.Since(start)

	start = time.Now()
	result2, err := cache.GetLongterm()
	if err != nil {
		t.Fatalf("Second GetLongterm failed: %v", err)
	}
	secondDuration := time.Since(start)

	if result1 != result2 {
		t.Errorf("Results differ: %q vs %q", result1, result2)
	}

	if secondDuration > firstDuration {
		t.Logf("Warning: Second call (%v) not faster than first (%v)", secondDuration, firstDuration)
	}
}

func TestGetLongterm_CacheInvalidation(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	longtermPath := filepath.Join(tmpDir, "memory", "longterm.md")
	content1 := "Content 1"
	if err := os.WriteFile(longtermPath, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write longterm.md: %v", err)
	}

	result1, err := cache.GetLongterm()
	if err != nil {
		t.Fatalf("GetLongterm failed: %v", err)
	}
	if result1 != content1 {
		t.Errorf("Expected %q, got %q", content1, result1)
	}

	// Update file
	time.Sleep(10 * time.Millisecond)
	content2 := "Content 2"
	if err := os.WriteFile(longtermPath, []byte(content2), 0644); err != nil {
		t.Fatalf("Failed to update longterm.md: %v", err)
	}

	// Wait for TTL to expire (30s default, but we mock time via cache.longtermLastCheck)
	// Force TTL expiry by setting lastCheck to past
	cache.longtermMu.Lock()
	cache.longtermLastCheck = time.Now().Add(-60 * time.Second)
	cache.longtermMu.Unlock()

	result2, err := cache.GetLongterm()
	if err != nil {
		t.Fatalf("GetLongterm after update failed: %v", err)
	}
	if result2 != content2 {
		t.Errorf("Expected %q, got %q", content2, result2)
	}
}

func TestGetLongterm_TTLCache(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	longtermPath := filepath.Join(tmpDir, "memory", "longterm.md")
	content1 := "Content 1"
	if err := os.WriteFile(longtermPath, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write longterm.md: %v", err)
	}

	// First call - loads from file
	result1, err := cache.GetLongterm()
	if err != nil {
		t.Fatalf("First GetLongterm failed: %v", err)
	}
	if result1 != content1 {
		t.Errorf("Expected %q, got %q", content1, result1)
	}

	// Update file (but TTL not expired)
	time.Sleep(10 * time.Millisecond)
	content2 := "Content 2"
	if err := os.WriteFile(longtermPath, []byte(content2), 0644); err != nil {
		t.Fatalf("Failed to update longterm.md: %v", err)
	}

	// Second call - should return cached (TTL not expired)
	result2, err := cache.GetLongterm()
	if err != nil {
		t.Fatalf("Second GetLongterm failed: %v", err)
	}
	// Should still return old content because TTL not expired
	if result2 != content1 {
		t.Errorf("Expected cached %q, got %q", content1, result2)
	}

	// Force TTL expiry
	cache.longtermMu.Lock()
	cache.longtermLastCheck = time.Now().Add(-60 * time.Second)
	cache.longtermMu.Unlock()

	// Third call - TTL expired, should reload
	result3, err := cache.GetLongterm()
	if err != nil {
		t.Fatalf("Third GetLongterm failed: %v", err)
	}
	if result3 != content2 {
		t.Errorf("Expected %q, got %q", content2, result3)
	}
}

func TestGetHot_CacheHit(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	hotPath := filepath.Join(tmpDir, "memory", "hot.json")
	data := &HotMemoryData{
		RecentKeywords: []Keyword{{Word: "test", Count: 1}},
	}
	if err := saveHot(hotPath, data); err != nil {
		t.Fatalf("Failed to write hot.json: %v", err)
	}

	result1, err := cache.GetHot()
	if err != nil {
		t.Fatalf("First GetHot failed: %v", err)
	}

	result2, err := cache.GetHot()
	if err != nil {
		t.Fatalf("Second GetHot failed: %v", err)
	}

	if result1 == nil || result2 == nil {
		t.Fatal("Expected non-nil results")
	}
}

func TestGetRecent_Fixed20(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	for i := 0; i < 25; i++ {
		msg := Message{
			ID:        string(rune('a' + i%26)),
			Content:   "test",
			Timestamp: time.Now(),
		}
		if err := cache.Append(msg); err != nil {
			t.Fatalf("Failed to append message: %v", err)
		}
	}

	recent := cache.GetRecent()

	if len(recent) != 20 {
		t.Errorf("Expected 20 recent messages, got %d", len(recent))
	}
}

func TestGetRecent_ReturnsInternalSlice(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	msg := Message{ID: "1", Content: "test", Timestamp: time.Now()}
	if err := cache.Append(msg); err != nil {
		t.Fatalf("Failed to append message: %v", err)
	}

	recent1 := cache.GetRecent()
	recent2 := cache.GetRecent()

	// Both should point to same underlying data (zero-copy optimization)
	// Note: Callers must NOT modify the returned slice
	if &recent1[0] != &recent2[0] {
		t.Error("GetRecent should return internal slice for performance")
	}
}

func TestAppend_InsertToDatabase(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	msg := Message{
		ID:        "test-1",
		Content:   "Test message",
		Timestamp: time.Now(),
		HumanIDs:  []string{"user-1"},
	}

	if err := cache.Append(msg); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	results, err := cache.Search("Test")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find inserted message")
	}
}

func TestAppend_UpdateRecentCache(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	msg := Message{
		ID:        "test-1",
		Content:   "Test message",
		Timestamp: time.Now(),
	}

	if err := cache.Append(msg); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	recent := cache.GetRecent()
	if len(recent) != 1 {
		t.Errorf("Expected 1 recent message, got %d", len(recent))
	}
	if recent[0].ID != msg.ID {
		t.Errorf("Expected message ID %s, got %s", msg.ID, recent[0].ID)
	}
}

func TestAppend_PrependToRecent(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	msg1 := Message{ID: "1", Content: "First", Timestamp: time.Now()}
	msg2 := Message{ID: "2", Content: "Second", Timestamp: time.Now()}

	if err := cache.Append(msg1); err != nil {
		t.Fatalf("Append msg1 failed: %v", err)
	}
	if err := cache.Append(msg2); err != nil {
		t.Fatalf("Append msg2 failed: %v", err)
	}

	recent := cache.GetRecent()
	if len(recent) < 2 {
		t.Fatalf("Expected at least 2 messages, got %d", len(recent))
	}

	if recent[0].ID != "2" {
		t.Errorf("Expected newest message first, got ID %s", recent[0].ID)
	}
}

func TestAppend_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := Message{
				ID:        fmt.Sprintf("msg-%d", id), // 修复：生成唯一 ID
				Content:   "concurrent test",
				Timestamp: time.Now(),
			}
			if err := cache.Append(msg); err != nil {
				t.Errorf("Append failed in goroutine %d: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	recent := cache.GetRecent()
	if len(recent) != 20 {
		t.Errorf("Expected 20 recent messages, got %d", len(recent))
	}
}

func TestClose_GracefulShutdown(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}

	msg := Message{ID: "1", Content: "test", Timestamp: time.Now()}
	cache.Append(msg)

	if err := cache.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}

	if err := cache.Close(); err != nil {
		t.Fatalf("First close failed: %v", err)
	}

	if err := cache.Close(); err != nil {
		t.Errorf("Second close should not fail, got: %v", err)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	_, err = cache.Search("")
	if err == nil {
		t.Error("Expected error for empty query")
	}
}

func TestSearch_ValidQuery(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	msg := Message{
		ID:        "1",
		Content:   "Hello world",
		Timestamp: time.Now(),
	}
	if err := cache.Append(msg); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	results, err := cache.Search("Hello")
	if err != nil {
		t.Errorf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected to find message")
	}
}

func TestUpdateHotAsync_NonBlocking(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	msg := Message{ID: "1", Content: "test", Timestamp: time.Now()}

	start := time.Now()
	cache.UpdateHotAsync(msg)
	duration := time.Since(start)

	if duration > 10*time.Millisecond {
		t.Errorf("UpdateHotAsync took too long: %v", duration)
	}
}

func TestMemoryCache_ConcurrentReads(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	msg := Message{ID: "1", Content: "test", Timestamp: time.Now()}
	if err := cache.Append(msg); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	var wg sync.WaitGroup
	numReaders := 50

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cache.GetRecent()
			_, _ = cache.GetLongterm()
			_, _ = cache.GetHot()
		}()
	}

	wg.Wait()
}

func TestMemoryCache_ConcurrentReadWriteRace(t *testing.T) {
	cache, _ := setupTestCache(t)
	defer cache.Close()

	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex
	
	numWriters := 50
	numReaders := 50

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := Message{
				ID:        fmt.Sprintf("race-test-%d", id),
				Content:   "concurrent write test",
				Timestamp: time.Now(),
			}
			if err := cache.Append(msg); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("writer %d: %v", id, err))
				mu.Unlock()
			}
		}(i)
	}

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = cache.GetRecent()
			_, _ = cache.GetHot()
		}(i)
	}

	wg.Wait()
	
	if len(errors) > 0 {
		t.Errorf("Concurrent read-write race test failed with %d errors: %v", len(errors), errors)
	}
}

func TestAppend_TriggerHotUpdateEndToEnd(t *testing.T) {
	cache, tmpDir := setupTestCache(t)

	msg := Message{
		ID:        "test-1",
		Content:   "golang programming test",
		Timestamp: time.Now(),
	}

	if err := cache.Append(msg); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if err := cache.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	hotPath := filepath.Join(tmpDir, "memory", "hot.json")
	data, err := loadHot(hotPath)
	if err != nil {
		t.Fatalf("Failed to reload hot.json: %v", err)
	}

	if data == nil {
		t.Fatal("Expected non-nil hot data")
	}

	foundKeyword := false
	for _, kw := range data.RecentKeywords {
		if kw.Word == "golang" || kw.Word == "programming" {
			foundKeyword = true
			break
		}
	}

	if !foundKeyword {
		t.Errorf("Expected to find keywords from message, got: %+v", data.RecentKeywords)
	}
}

func TestMemoryCache_TTLCleanup(t *testing.T) {
	t.Skip("TTL cleanup requires internal nowFunc field which is private. TTL logic is tested in hot_test.go")
}
