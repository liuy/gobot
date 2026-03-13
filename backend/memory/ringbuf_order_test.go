package memory

import (
	"fmt"
	"testing"
	"time"
)

// TestRecentBuffer_Order verifies that GetByChatID returns messages in chronological order (oldest first)
func TestRecentBuffer_Order(t *testing.T) {
	// Create a new cache
	tmpDir := t.TempDir()
	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	chatID := "test-chat-order"

	// Add messages in chronological order
	for i := 1; i <= 5; i++ {
		msg := Message{
			ID:        fmt.Sprintf("msg-%d", i),
			Content:   fmt.Sprintf("Message %d", i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Second), // increasing timestamps
			ChatID:    chatID,
		}
		if err := cache.Append(msg); err != nil {
			t.Fatalf("Append msg-%d failed: %v", i, err)
		}
	}

	// Get recent messages
	recent := cache.GetRecent(chatID, 10)

	// Verify order: oldest first
	if len(recent) != 5 {
		t.Fatalf("Expected 5 messages, got %d", len(recent))
	}

	// Check that messages are in chronological order (oldest first)
	for i := 0; i < len(recent)-1; i++ {
		if recent[i].Timestamp.After(recent[i+1].Timestamp) {
			t.Errorf("Messages not in chronological order: msg[%d]=%s (ts=%v) is after msg[%d]=%s (ts=%v)",
				i, recent[i].ID, recent[i].Timestamp,
				i+1, recent[i+1].ID, recent[i+1].Timestamp)
		}
	}

	// Also verify the content order
	expectedOrder := []string{"msg-1", "msg-2", "msg-3", "msg-4", "msg-5"}
	for i, msg := range recent {
		if msg.ID != expectedOrder[i] {
			t.Errorf("At position %d: expected %s, got %s", i, expectedOrder[i], msg.ID)
		}
	}
}

// TestRecentBuffer_AfterRestart simulates the scenario where messages are stored in cold.db
// and then retrieved after restart
func TestRecentBuffer_AfterRestart(t *testing.T) {
	tmpDir := t.TempDir()

	// First: add messages to a cache
	cache1, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create first MemoryCache: %v", err)
	}

	chatID := "test-chat-restart"

	// Add messages
	for i := 1; i <= 10; i++ {
		msg := Message{
			ID:        fmt.Sprintf("msg-%d", i),
			Content:   fmt.Sprintf("Message %d", i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			ChatID:    chatID,
		}
		if err := cache1.Append(msg); err != nil {
			t.Fatalf("Append msg-%d failed: %v", i, err)
		}
	}

	// Close first cache (simulates shutdown)
	if err := cache1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Second: create a new cache (simulates restart)
	cache2, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create second MemoryCache: %v", err)
	}
	defer cache2.Close()

	// Get messages after restart
	recent := cache2.GetRecent(chatID, 10)

	// Verify order: oldest first
	if len(recent) == 0 {
		t.Fatal("Expected messages after restart, got none")
	}

	t.Logf("Got %d messages after restart:", len(recent))
	for i, msg := range recent {
		t.Logf("  [%d] %s", i, msg.ID)
	}

	// Check chronological order
	for i := 0; i < len(recent)-1; i++ {
		if recent[i].Timestamp.After(recent[i+1].Timestamp) {
			t.Errorf("Messages not in chronological order after restart: msg[%d]=%s is after msg[%d]=%s",
				i, recent[i].ID, i+1, recent[i+1].ID)
		}
	}

	// Also verify the content order
	expectedOrder := []string{"msg-1", "msg-2", "msg-3", "msg-4", "msg-5", "msg-6", "msg-7", "msg-8", "msg-9", "msg-10"}
	for i, msg := range recent {
		if msg.ID != expectedOrder[i] {
			t.Errorf("At position %d: expected %s, got %s", i, expectedOrder[i], msg.ID)
		}
	}
}
