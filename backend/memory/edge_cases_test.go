package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppend_InvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		msg     Message
		wantErr bool
	}{
		{"empty ID", Message{ID: "", Content: "test", Timestamp: time.Now()}, true},
		{"whitespace ID", Message{ID: "   ", Content: "test", Timestamp: time.Now()}, true},
		{"empty Content", Message{ID: "1", Content: "", Timestamp: time.Now()}, true},
		{"whitespace Content", Message{ID: "1", Content: "   ", Timestamp: time.Now()}, true},
		{"zero Timestamp", Message{ID: "1", Content: "test", Timestamp: time.Time{}}, true},
		{"valid message", Message{ID: "1", Content: "test", Timestamp: time.Now()}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cache, _ := NewMemoryCache(tmpDir)
			defer cache.Close()

			err := cache.Append(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Append() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSearch_WhitespaceQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"empty", "", true},
		{"spaces", "   ", true},
		{"tabs", "\t\t", true},
		{"newlines", "\n\n", true},
		{"mixed", " \t\n ", true},
		{"valid", "test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cache, _ := NewMemoryCache(tmpDir)
			defer cache.Close()

			_, err := cache.Search(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("Search(%q) error = %v, wantErr %v", tt.query, err, tt.wantErr)
			}
		})
	}
}

func TestClose_AfterClose(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)

	if err := cache.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := cache.Close(); err != nil {
		t.Errorf("Second Close should not error, got: %v", err)
	}

	// After close, AddMessage should return error
	msg := Message{ID: "post-close", Content: "test", Timestamp: time.Now()}
	if err := cache.AddMessage(msg); err == nil {
		t.Error("AddMessage after Close should error")
	}

	// Search should also fail (DB closed)
	if _, err := cache.Search("test"); err == nil {
		t.Error("Search after Close should error")
	}
}

func TestAppend_VeryLongContent(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	longContent := strings.Repeat("test ", 20000)
	msg := Message{ID: "1", Content: longContent, Timestamp: time.Now()}

	if err := cache.Append(msg); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// 100KB content takes longer to tokenize, use longer timeout
	waitForMessage(t, cache, "test", 1)

	results, err := cache.Search("test")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search should find the message")
	}
}

func TestAppend_HighConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	const goroutines = 10
	const messagesPerGoroutine = 10

	done := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			for i := 0; i < messagesPerGoroutine; i++ {
				msg := Message{
					ID:        fmt.Sprintf("g%d-m%d", gid, i),
					Content:   "concurrent test",
					Timestamp: time.Now(),
				}
				if err := cache.Append(msg); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}(g)
	}

	for g := 0; g < goroutines; g++ {
		if err := <-done; err != nil {
			t.Fatalf("Goroutine failed: %v", err)
		}
	}

	waitForMessage(t, cache, "concurrent", 1)

	results, err := cache.Search("concurrent")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search should find messages")
	}
}

func TestGetRecent_RowsError(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	chatID := "test-chat"
	for i := 0; i < 30; i++ {
		msg := Message{
			ID:        fmt.Sprintf("recent-%d", i),
			Content:   fmt.Sprintf("message %d", i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			ChatID:    chatID,
		}
		if err := cache.Append(msg); err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}

	// Wait for async writes to complete
	waitFor(t, 3*time.Second, func() bool {
		recent := cache.GetRecent(chatID, 20)
		return len(recent) >= 20
	})

	recent := cache.GetRecent(chatID, 20)

	if len(recent) != 20 {
		t.Errorf("Expected 20 recent messages, got %d", len(recent))
	}

	if len(recent) > 1 && recent[0].Timestamp.After(recent[1].Timestamp) {
		t.Error("Recent messages should be in ascending order (oldest first)")
	}
}

func TestGetLongterm_RaceCondition(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("NewMemoryCache failed: %v", err)
	}
	defer cache.Close()

	longtermPath := filepath.Join(tmpDir, "memory", "longterm.md")
	if err := os.MkdirAll(filepath.Dir(longtermPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(longtermPath, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	const goroutines = 50
	done := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := cache.GetLongterm()
			done <- err
		}()
	}

	for i := 0; i < goroutines; i++ {
		if err := <-done; err != nil {
			t.Errorf("GetLongterm failed: %v", err)
		}
	}
}

func TestGetLongterm_EmptyContentCaching(t *testing.T) {
	const testTTL = 30 * time.Second

	tmpDir := t.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("NewMemoryCache failed: %v", err)
	}
	defer cache.Close()

	longtermPath := filepath.Join(tmpDir, "memory", "longterm.md")
	if err := os.MkdirAll(filepath.Dir(longtermPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(longtermPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	content1, err := cache.GetLongterm()
	if err != nil {
		t.Fatalf("GetLongterm #1 failed: %v", err)
	}
	if content1 != "" {
		t.Errorf("Expected empty content, got %q", content1)
	}

	mockTime := time.Now().Add(testTTL + time.Second)
	cache.nowFunc = func() time.Time { return mockTime }

	content2, err := cache.GetLongterm()
	if err != nil {
		t.Fatalf("GetLongterm #2 failed: %v", err)
	}
	if content2 != "" {
		t.Errorf("Expected empty content, got %q", content2)
	}
}

func TestInsertMessage_DuplicateID(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	msg := Message{
		ID:        "dup-1",
		Content:   "first message",
		Timestamp: time.Now(),
	}

	// First insert via AddMessage
	if err := cache.AddMessage(msg); err != nil {
		t.Fatalf("First AddMessage failed: %v", err)
	}

	waitForMessage(t, cache, "first", 1)

	// Second insert with same ID should be ignored (INSERT OR IGNORE)
	msg2 := Message{
		ID:        "dup-1",
		Content:   "second message",
		Timestamp: time.Now(),
	}
	if err := cache.AddMessage(msg2); err != nil {
		t.Fatalf("Second AddMessage failed: %v", err)
	}

	// Wait a bit for second insert attempt
	time.Sleep(200 * time.Millisecond)

	// Should only have 1 message (first one)
	results, err := cache.Search("first")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	// Second content should not exist
	results2, _ := cache.Search("second")
	if len(results2) != 0 {
		t.Errorf("Expected 0 results for 'second', got %d", len(results2))
	}
}
