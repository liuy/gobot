package channel

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gobot/providers"
	"gobot/types"
	gows "golang.org/x/net/websocket"
)

// mockLLMProvider simulates LLM behavior for testing
type mockLLMProvider struct {
	thinkingChunks []string
	contentChunks  []string
}

func (m *mockLLMProvider) Chat(ctx context.Context, messages []providers.Message, model string, options map[string]any) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "ok"}, nil
}

func (m *mockLLMProvider) ChatStream(ctx context.Context, messages []providers.Message, model string, options map[string]any) (<-chan types.StreamChunk, error) {
	ch := make(chan types.StreamChunk, len(m.thinkingChunks)+len(m.contentChunks)+1)
	go func() {
		for _, chunk := range m.thinkingChunks {
			ch <- types.StreamChunk{Thinking: chunk}
		}
		for _, chunk := range m.contentChunks {
			ch <- types.StreamChunk{Content: chunk}
		}
		ch <- types.StreamChunk{StopReason: "stop"}
		close(ch)
	}()
	return ch, nil
}

func (m *mockLLMProvider) GetDefaultModel() string { return "test-model" }

func newWSServer(t *testing.T, handler func(*gows.Conn)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(gows.Handler(handler))
	t.Cleanup(srv.Close)
	return srv
}

func dialWS(t *testing.T, srv *httptest.Server) *gows.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, err := gows.Dial(wsURL, "", "http://localhost/")
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	return conn
}

// TestHandleChatSend_EmitsLifecycleEvents verifies that HandleChatSend emits
// lifecycle events at the start and end of an agent run.
func TestHandleChatSend_EmitsLifecycleEvents(t *testing.T) {
	origProvider, origModel := ChatProvider, ChatModel
	ChatProvider = &mockLLMProvider{
		thinkingChunks: []string{"Let me think...", " Analyzing..."},
		contentChunks:  []string{"Hello", "!"},
	}
	ChatModel = "test-model"
	t.Cleanup(func() {
		ChatProvider = origProvider
		ChatModel = origModel
	})

	srv := newWSServer(t, func(ws *gows.Conn) {
		req := WSRequest{
			Type:   "req",
			ID:     "test-1",
			Method: "chat.send",
			Params: map[string]any{"message": "hi", "sessionKey": "test"},
		}
		_ = HandleChatSend(ws, req)
	})
	conn := dialWS(t, srv)

	// Collect all events
	var events []map[string]any
	for {
		var got map[string]any
		if err := gows.JSON.Receive(conn, &got); err != nil {
			break
		}
		events = append(events, got)
	}

	// Categorize events
	var lifecycleEvents, reasoningEvents, contentEvents, chatEvents int
	for _, evt := range events {
		if evt["type"] != "event" {
			continue
		}
		switch evt["event"] {
		case "agent":
			payload, _ := evt["payload"].(map[string]any)
			switch payload["stream"] {
			case "lifecycle":
				lifecycleEvents++
			case "reasoning":
				reasoningEvents++
			case "content":
				contentEvents++
			}
		case "chat":
			chatEvents++
		}
	}

	t.Logf("Event summary: lifecycle=%d, reasoning=%d, content=%d, chat=%d",
		lifecycleEvents, reasoningEvents, contentEvents, chatEvents)

	// Verify: Should have lifecycle start and end events
	if lifecycleEvents == 0 {
		t.Error("MISSING: No lifecycle events. HandleChatSend should emit lifecycle start/end events.")
		t.Log("Expected: agent event with stream='lifecycle', phase='start' at beginning")
		t.Log("Expected: agent event with stream='lifecycle', phase='end' at end")
	}

	// Verify: Should have reasoning events
	if reasoningEvents == 0 {
		t.Error("MISSING: No reasoning events from LLM")
	}

	// Verify: Should have content events
	if contentEvents == 0 {
		t.Error("MISSING: No content events from LLM")
	}

	// Verify: Should have chat final event
	if chatEvents == 0 {
		t.Error("MISSING: No chat final event")
	}
}

// TestHandleChatSend_EventOrdering verifies events arrive in correct order
func TestHandleChatSend_EventOrdering(t *testing.T) {
	origProvider, origModel := ChatProvider, ChatModel
	ChatProvider = &mockLLMProvider{
		thinkingChunks: []string{"thinking"},
		contentChunks:  []string{"response"},
	}
	ChatModel = "test-model"
	t.Cleanup(func() {
		ChatProvider = origProvider
		ChatModel = origModel
	})

	srv := newWSServer(t, func(ws *gows.Conn) {
		req := WSRequest{
			Type:   "req",
			ID:     "test-2",
			Method: "chat.send",
			Params: map[string]any{"message": "test", "sessionKey": "test"},
		}
		_ = HandleChatSend(ws, req)
	})
	conn := dialWS(t, srv)

	var eventOrder []string
	for {
		var got map[string]any
		if err := gows.JSON.Receive(conn, &got); err != nil {
			break
		}
		if got["type"] != "event" {
			continue
		}
		switch got["event"] {
		case "agent":
			payload, _ := got["payload"].(map[string]any)
			stream, _ := payload["stream"].(string)
			if stream == "lifecycle" {
				data, _ := payload["data"].(map[string]any)
				eventOrder = append(eventOrder, "lifecycle:"+data["phase"].(string))
			} else {
				eventOrder = append(eventOrder, "agent:"+stream)
			}
		case "chat":
			payload, _ := got["payload"].(map[string]any)
			eventOrder = append(eventOrder, "chat:"+payload["state"].(string))
		}
	}

	t.Logf("Event order: %v", eventOrder)

	// Expected order: lifecycle:start → (reasoning) → content → lifecycle:end → chat:final
	if len(eventOrder) > 0 && eventOrder[0] != "lifecycle:start" {
		t.Errorf("First event = %q, want 'lifecycle:start'", eventOrder[0])
		t.Log("Frontend expects lifecycle:start to initialize run state")
	}
}

// mockErrorProvider simulates LLM failure for testing lifecycle:error
type mockErrorProvider struct{}

func (m *mockErrorProvider) Chat(ctx context.Context, messages []providers.Message, model string, options map[string]any) (*providers.LLMResponse, error) {
	return nil, fmt.Errorf("simulated LLM error")
}

func (m *mockErrorProvider) ChatStream(ctx context.Context, messages []providers.Message, model string, options map[string]any) (<-chan types.StreamChunk, error) {
	return nil, fmt.Errorf("simulated LLM stream error")
}

func (m *mockErrorProvider) GetDefaultModel() string { return "test-model" }

// TestHandleChatSend_LifecycleErrorOnError verifies lifecycle:error is sent when LLM fails
func TestHandleChatSend_LifecycleErrorOnError(t *testing.T) {
	origProvider, origModel := ChatProvider, ChatModel
	ChatProvider = &mockErrorProvider{}
	ChatModel = "test-model"
	t.Cleanup(func() {
		ChatProvider = origProvider
		ChatModel = origModel
	})

	srv := newWSServer(t, func(ws *gows.Conn) {
		req := WSRequest{
			Type:   "req",
			ID:     "test-error",
			Method: "chat.send",
			Params: map[string]any{"message": "test", "sessionKey": "test"},
		}
		_ = HandleChatSend(ws, req)
	})
	conn := dialWS(t, srv)

	var events []map[string]any
	for {
		var got map[string]any
		if err := gows.JSON.Receive(conn, &got); err != nil {
			break
		}
		events = append(events, got)
	}

	// Find lifecycle events
	var lifecyclePhases []string
	for _, evt := range events {
		if evt["type"] == "event" && evt["event"] == "agent" {
			payload, _ := evt["payload"].(map[string]any)
			if payload["stream"] == "lifecycle" {
				data, _ := payload["data"].(map[string]any)
				phase, _ := data["phase"].(string)
				lifecyclePhases = append(lifecyclePhases, phase)
			}
		}
	}

	t.Logf("Lifecycle phases: %v", lifecyclePhases)

	// Should have start and error
	if len(lifecyclePhases) < 2 {
		t.Fatalf("Expected 2 lifecycle events (start + error), got %d", len(lifecyclePhases))
	}
	if lifecyclePhases[0] != "start" {
		t.Errorf("First lifecycle phase = %q, want 'start'", lifecyclePhases[0])
	}
	if lifecyclePhases[1] != "error" {
		t.Errorf("Second lifecycle phase = %q, want 'error'", lifecyclePhases[1])
	}

	// Verify error message is included
	lastEvt := events[len(events)-1]
	payload, _ := lastEvt["payload"].(map[string]any)
	data, _ := payload["data"].(map[string]any)
	errMsg, _ := data["error"].(string)
	if errMsg == "" {
		t.Error("lifecycle:error missing error message")
	} else {
		t.Logf("✓ Error message: %s", errMsg)
	}
}
