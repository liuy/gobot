package memory

import (
	"fmt"
	"testing"
	"time"
)

func TestNewContextBuilder_ValidInput(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	builder := NewContextBuilder(cache, 4000)

	if builder == nil {
		t.Fatal("Expected non-nil ContextBuilder")
	}
	if builder.maxTokens != 4000 {
		t.Errorf("Expected maxTokens=4000, got %d", builder.maxTokens)
	}
}

func TestBuild_BasicContext(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	chatID := "test-chat-basic"
	msg := Message{
		ID:        "test-1",
		Content:   "Test message",
		Timestamp: time.Now(),
		ChatID:    chatID,
	}
	if err := cache.Append(msg); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Wait for async write to complete
	waitFor(t, 2*time.Second, func() bool {
		recent := cache.GetRecent(chatID, 20)
		return len(recent) > 0
	})

	builder := NewContextBuilder(cache, 10000)
	ctx, err := builder.Build(msg)

	if err != nil {
		t.Errorf("Build failed: %v", err)
	}
	if ctx == nil {
		t.Fatal("Expected non-nil Context")
	}
	if ctx.Recent == nil {
		t.Error("Expected non-nil Recent slice")
	}
}

func TestBuild_WithinBudget(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	chatID := "test-chat-budget"
	for i := 0; i < 5; i++ {
		msg := Message{
			ID:        string(rune('a' + i)),
			Content:   "short",
			Timestamp: time.Now(),
			ChatID:    chatID,
		}
		if err := cache.Append(msg); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Wait for async writes to complete
	waitFor(t, 2*time.Second, func() bool {
		recent := cache.GetRecent(chatID, 20)
		return len(recent) >= 5
	})

	builder := NewContextBuilder(cache, 1000)
	currentMsg := Message{ID: "current", Content: "test", Timestamp: time.Now(), ChatID: chatID}
	ctx, err := builder.Build(currentMsg)

	if err != nil {
		t.Errorf("Build failed: %v", err)
	}

	if len(ctx.Recent) != 5 {
		t.Errorf("Expected 5 recent messages, got %d", len(ctx.Recent))
	}
}

func TestBuild_DropOldestRecent(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	chatID := "test-chat-dropoldest"
	messageIDs := []string{}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("msg-%02d", i)
		messageIDs = append(messageIDs, id)
		msg := Message{
			ID:        id,
			Content:   "This is a message with enough content to consume tokens",
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			ChatID:    chatID,
		}
		if err := cache.Append(msg); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Wait for async writes to complete
	waitFor(t, 2*time.Second, func() bool {
		recent := cache.GetRecent(chatID, 20)
		return len(recent) >= 20
	})

	builder := NewContextBuilder(cache, 50)
	currentMsg := Message{ID: "current", Content: "test", Timestamp: time.Now(), ChatID: chatID}
	ctx, err := builder.Build(currentMsg)

	if err != nil {
		t.Errorf("Build failed: %v", err)
	}

	if len(ctx.Recent) >= 20 {
		t.Errorf("Expected some recent messages to be dropped, got %d", len(ctx.Recent))
	}

	keptIDs := make(map[string]bool)
	for _, msg := range ctx.Recent {
		keptIDs[msg.ID] = true
	}

	newestKept := false
	oldestDropped := false
	for _, id := range messageIDs {
		if keptIDs[id] {
			if id >= "msg-15" {
				newestKept = true
			}
		} else {
			if id <= "msg-05" {
				oldestDropped = true
			}
		}
	}

	if !newestKept {
		t.Error("Expected newest messages (msg-15+) to be kept")
	}
	if !oldestDropped {
		t.Error("Expected oldest messages (msg-05-) to be dropped")
	}
}

func TestBuild_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		numMessages int
		maxTokens   int
		minRecent   int
	}{
		{
			name:        "exact_fit",
			numMessages: 5,
			maxTokens:   500,
			minRecent:   5,
		},
		{
			name:        "drop_recent",
			numMessages: 20,
			maxTokens:   100,
			minRecent:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			cache, err := NewMemoryCache(tmpDir)
			if err != nil {
				t.Fatalf("Failed to create MemoryCache: %v", err)
			}
			defer cache.Close()

			chatID := fmt.Sprintf("test-chat-%s", tt.name)
			for i := 0; i < tt.numMessages; i++ {
				msg := Message{
					ID:        fmt.Sprintf("msg-%02d", i),
					Content:   "Test message content for token counting",
					Timestamp: time.Now(),
					ChatID:    chatID,
				}
				if err := cache.Append(msg); err != nil {
					t.Fatalf("Append failed: %v", err)
				}
			}

			// Wait for async writes to complete
			waitFor(t, 2*time.Second, func() bool {
				recent := cache.GetRecent(chatID, 20)
				return len(recent) >= tt.numMessages
			})

			builder := NewContextBuilder(cache, tt.maxTokens)
			currentMsg := Message{ID: "current", Content: "test", Timestamp: time.Now(), ChatID: chatID}
			ctx, err := builder.Build(currentMsg)

			if err != nil {
				t.Errorf("Build failed: %v", err)
			}

			if len(ctx.Recent) < tt.minRecent {
				t.Errorf("Expected at least %d recent messages, got %d", tt.minRecent, len(ctx.Recent))
			}
		})
	}
}

func TestBuild_WithLongterm(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	longtermPath := tmpDir + "/memory/longterm.md"
	longtermContent := "# About Me\n\nI am an AI assistant."
	if err := saveLongterm(longtermPath, longtermContent); err != nil {
		t.Fatalf("Failed to save longterm: %v", err)
	}

	chatID := "test-chat-longterm"
	builder := NewContextBuilder(cache, 10000)
	currentMsg := Message{ID: "current", Content: "test", Timestamp: time.Now(), ChatID: chatID}
	ctx, err := builder.Build(currentMsg)

	if err != nil {
		t.Errorf("Build failed: %v", err)
	}

	if ctx.Longterm != longtermContent {
		t.Errorf("Expected longterm %q, got %q", longtermContent, ctx.Longterm)
	}
}

func TestCountTokens_EmptyString(t *testing.T) {
	count := countTokens("")

	if count != 0 {
		t.Errorf("Expected 0 tokens for empty string, got %d", count)
	}
}

func TestCountTokens_English(t *testing.T) {
	text := "Hello world this is a test"
	count := countTokens(text)

	expected := len(text) / 4
	if expected < 1 {
		expected = 1
	}
	if count != expected {
		t.Errorf("Expected %d tokens, got %d", expected, count)
	}
}

func TestCountTokens_Chinese(t *testing.T) {
	text := "今天天气怎么样"
	count := countTokens(text)

	if count <= 0 {
		t.Errorf("Expected positive token count for Chinese text, got %d", count)
	}
}

func TestCountTokens_MixedContent(t *testing.T) {
	text := "Hello 今天天气 world"
	count := countTokens(text)

	if count <= 0 {
		t.Errorf("Expected positive token count, got %d", count)
	}
}

func TestContextBuilder_ZeroMaxTokens(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	builder := NewContextBuilder(cache, 0)

	chatID := "test-chat-zero"
	msg := Message{ID: "1", Content: "test", Timestamp: time.Now(), ChatID: chatID}
	ctx, err := builder.Build(msg)

	if err != nil {
		t.Errorf("Build failed: %v", err)
	}

	if ctx == nil {
		t.Error("Expected non-nil context even with zero maxTokens")
	}
}

func TestBuild_MultipleCalls(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewMemoryCache(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create MemoryCache: %v", err)
	}
	defer cache.Close()

	builder := NewContextBuilder(cache, 1000)

	chatID := "test-chat-multi"
	for i := 0; i < 3; i++ {
		msg := Message{
			ID:        string(rune('a' + i)),
			Content:   "test message",
			Timestamp: time.Now(),
			ChatID:    chatID,
		}

		ctx, err := builder.Build(msg)
		if err != nil {
			t.Errorf("Build %d failed: %v", i, err)
		}
		if ctx == nil {
			t.Errorf("Expected non-nil context on build %d", i)
		}
	}
}
