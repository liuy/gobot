package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitColdDB_CreateDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cold.db")

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file should exist after initialization")
	}

	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("Failed to query journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("Expected journal_mode=wal, got %s", journalMode)
	}
}

func TestInitColdDB_CreateTables(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cold.db")

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO messages (id, content, timestamp) VALUES (?, ?, ?)",
		"test-1", "test content", time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Errorf("Failed to insert into messages table: %v", err)
	}

	_, err = db.Exec("SELECT * FROM messages_fts WHERE messages_fts MATCH ?", "test")
	if err != nil {
		t.Errorf("FTS5 table not working: %v", err)
	}
}

func TestInsertMessage_BasicInsert(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cold.db")

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	msg := Message{
		ID:        "msg-001",
		Content:   "Test message content",
		Timestamp: time.Now(),
		HumanIDs:  []string{"user-1", "user-2"},
		Channel:   "discord",
		ChatID:    "chat-123",
		Role:      "user",
		Type:      "text",
	}

	err = insertMessage(db, msg)
	if err != nil {
		t.Errorf("Failed to insert message: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM messages WHERE id = ?", msg.ID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count messages: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 message, got %d", count)
	}
}

func TestInsertMessage_HumanIDsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cold.db")

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	msg := Message{
		ID:        "msg-002",
		Content:   "Test",
		Timestamp: time.Now(),
		HumanIDs:  []string{"alice", "bob"},
	}

	err = insertMessage(db, msg)
	if err != nil {
		t.Fatalf("Failed to insert message: %v", err)
	}

	var humanIDsJSON string
	err = db.QueryRow("SELECT human_ids FROM messages WHERE id = ?", msg.ID).Scan(&humanIDsJSON)
	if err != nil {
		t.Fatalf("Failed to query human_ids: %v", err)
	}

	expected := `["alice","bob"]`
	if humanIDsJSON != expected {
		t.Errorf("Expected %s, got %s", expected, humanIDsJSON)
	}
}

func TestGetRecentMessages_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cold.db")

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	messages, err := getRecentMessages(db, 20)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(messages))
	}
}

func TestGetRecentMessages_WithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cold.db")

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	for i := 0; i < 25; i++ {
		msg := Message{
			ID:        string(rune('a' + i)),
			Content:   "test",
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := insertMessage(db, msg); err != nil {
			t.Fatalf("Failed to insert message: %v", err)
		}
	}

	messages, err := getRecentMessages(db, 20)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if len(messages) != 20 {
		t.Errorf("Expected 20 messages, got %d", len(messages))
	}
}

func TestGetRecentMessages_DescendingOrder(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cold.db")

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	for i := 0; i < 3; i++ {
		msg := Message{
			ID:        string(rune('a' + i)),
			Content:   "test",
			Timestamp: time.Now().Add(time.Duration(i) * time.Hour),
		}
		if err := insertMessage(db, msg); err != nil {
			t.Fatalf("Failed to insert message: %v", err)
		}
	}

	messages, err := getRecentMessages(db, 10)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if len(messages) < 2 {
		t.Fatal("Need at least 2 messages to test order")
	}

	if messages[0].Timestamp.Before(messages[1].Timestamp) {
		t.Error("Messages should be in descending order by timestamp")
	}
}

func TestSearchMessages_BasicSearch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cold.db")

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	msgs := []Message{
		{ID: "1", Content: "Hello world", Timestamp: time.Now()},
		{ID: "2", Content: "Goodbye world", Timestamp: time.Now()},
		{ID: "3", Content: "Test message", Timestamp: time.Now()},
	}

	for _, msg := range msgs {
		if err := insertMessage(db, msg); err != nil {
			t.Fatalf("Failed to insert message: %v", err)
		}
	}

	results, err := searchMessages(db, "world")
	if err != nil {
		t.Errorf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find messages with 'world'")
	}

	for _, result := range results {
		if result.Content != "Hello world" && result.Content != "Goodbye world" {
			t.Errorf("Unexpected result: %s", result.Content)
		}
	}
}

func TestSearchMessages_ChineseSearch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cold.db")

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	msg := Message{
		ID:        "cn-1",
		Content:   "今天天气怎么样",
		Timestamp: time.Now(),
	}

	if err := insertMessage(db, msg); err != nil {
		t.Fatalf("Failed to insert message: %v", err)
	}

	results, err := searchMessages(db, "天气")
	if err != nil {
		t.Errorf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find Chinese message")
	}
}

func TestSearchMessages_NoResults(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cold.db")

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	msg := Message{
		ID:        "1",
		Content:   "Hello world",
		Timestamp: time.Now(),
	}

	if err := insertMessage(db, msg); err != nil {
		t.Fatalf("Failed to insert message: %v", err)
	}

	results, err := searchMessages(db, "nonexistent")
	if err != nil {
		t.Errorf("Search failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestParseTimestamp_ValidFormat(t *testing.T) {
	timeStr := "2024-03-07 12:30:45"

	parsed := parseTimestamp(timeStr)

	if parsed.IsZero() {
		t.Error("Expected valid time, got zero value")
	}

	expected := time.Date(2024, 3, 7, 12, 30, 45, 0, time.UTC)
	if !parsed.Equal(expected) {
		t.Errorf("Expected %v, got %v", expected, parsed)
	}
}

func TestParseTimestamp_InvalidFormat(t *testing.T) {
	timeStr := "invalid"

	parsed := parseTimestamp(timeStr)

	if !parsed.IsZero() {
		t.Errorf("Expected zero time for invalid format, got %v", parsed)
	}
}

func TestTokenizeChinese_BasicText(t *testing.T) {
	content := "今天天气怎么样"

	result := tokenizeChinese(content)

	// With jieba, the result should be tokenized
	// Expected: "今天 天天 天气 今天天气 怎么 怎么样" (CutForSearch)
	if result == "" {
		t.Errorf("Expected non-empty tokenized result")
	}
	// Verify it contains the key tokens
	if !contains(result, "天气") {
		t.Errorf("Expected result to contain '天气', got %s", result)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
