package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdateKeywords_SliceRealloc(t *testing.T) {
	hotData := &HotMemoryData{}
	now := time.Now()

	for i := 0; i < 100; i++ {
		words := []string{fmt.Sprintf("keyword%d", i), "test"}
		updateKeywords(hotData, words, now)
	}

	for _, kw := range hotData.RecentKeywords {
		if kw.Word == "test" {
			if kw.Count != 100 {
				t.Errorf("'test' count should be 100, got %d", kw.Count)
			}
			break
		}
	}
}

func TestAppend_InvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		msg     Message
		wantErr bool
	}{
		{"empty ID", Message{ID: "", Content: "test", Timestamp: time.Now()}, true},
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

	msg := Message{ID: "1", Content: "test", Timestamp: time.Now()}
	if err := cache.Append(msg); err == nil {
		t.Error("Append after Close should error")
	}

	if _, err := cache.Search("test"); err == nil {
		t.Error("Search after Close should error")
	}
}

func TestTopicSummaries_NotImplemented(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	for i := 0; i < 5; i++ {
		cache.Append(Message{
			ID:        string(rune('1' + i)),
			Content:   "test topic keyword",
			Timestamp: time.Now(),
		})
	}

	hot, _ := cache.GetHot()
	if hot == nil {
		t.Fatal("GetHot returned nil")
	}

	if len(hot.TopicSummaries) > 0 {
		t.Errorf("TopicSummaries should be empty, got %d items", len(hot.TopicSummaries))
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

	results, err := cache.Search("concurrent")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search should find messages")
	}
}

func TestUpdateKeywords_MapPointerRealloc(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	for i := 0; i < 50; i++ {
		msg := Message{
			ID:        fmt.Sprintf("realloc-%d", i),
			Content:   fmt.Sprintf("keyword%d test%d word%d", i, i, i),
			Timestamp: time.Now(),
		}
		if err := cache.Append(msg); err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}

	time.Sleep(600 * time.Millisecond)

	hot, err := cache.GetHot()
	if err != nil {
		t.Fatalf("GetHot failed: %v", err)
	}

	for _, kw := range hot.RecentKeywords {
		if kw.Count <= 0 {
			t.Errorf("Keyword %q has invalid count %d", kw.Word, kw.Count)
		}
	}
}

func TestUpdateTopics_MapPointerRealloc(t *testing.T) {
	hotData := &HotMemoryData{}
	now := time.Now()

	for i := 0; i < 50; i++ {
		words := []string{fmt.Sprintf("topic%d", i), "commontopic", "commontopic", "commontopic"}
		for _, w := range words {
			updateKeywords(hotData, []string{w}, now)
		}
	}

	updateTopics(hotData, now)

	found := false
	for _, topic := range hotData.ActiveTopics {
		if topic.Name == "commontopic" {
			found = true
			if topic.Count < 3 {
				t.Errorf("commontopic count should >= 3, got %d", topic.Count)
			}
			break
		}
	}

	if !found {
		t.Error("Expected 'commontopic' to become a topic")
	}
}

func TestGetRecent_RowsError(t *testing.T) {
	tmpDir := t.TempDir()
	cache, _ := NewMemoryCache(tmpDir)
	defer cache.Close()

	for i := 0; i < 30; i++ {
		msg := Message{
			ID:        fmt.Sprintf("recent-%d", i),
			Content:   fmt.Sprintf("message %d", i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := cache.Append(msg); err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}

	recent := cache.GetRecent()

	if len(recent) != 20 {
		t.Errorf("Expected 20 recent messages, got %d", len(recent))
	}

	if len(recent) > 1 && recent[0].Timestamp.Before(recent[1].Timestamp) {
		t.Error("Recent messages should be in descending order")
	}
}

func TestUpdateKeywords_RepeatedUpdate(t *testing.T) {
	hotData := &HotMemoryData{}
	now := time.Now()

	updateKeywords(hotData, []string{"testword"}, now)

	for _, kw := range hotData.RecentKeywords {
		if kw.Word == "testword" && kw.Count != 1 {
			t.Fatalf("Initial count should be 1, got %d", kw.Count)
		}
	}

	for i := 0; i < 100; i++ {
		updateKeywords(hotData, []string{fmt.Sprintf("unique%d", i)}, now)
	}

	updateKeywords(hotData, []string{"testword"}, now)

	for _, kw := range hotData.RecentKeywords {
		if kw.Word == "testword" && kw.Count != 2 {
			t.Errorf("testword count should be 2, got %d", kw.Count)
		}
	}
}

func TestUpdateKeywords_DuplicateWords(t *testing.T) {
	hotData := &HotMemoryData{}
	now := time.Now()

	words := []string{"test", "test", "test", "unique"}
	updateKeywords(hotData, words, now)

	if len(hotData.RecentKeywords) != 2 {
		t.Errorf("Expected 2 keywords, got %d", len(hotData.RecentKeywords))
	}

	for _, kw := range hotData.RecentKeywords {
		if kw.Word == "test" && kw.Count != 1 {
			t.Errorf("test count should be 1, got %d", kw.Count)
		}
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
