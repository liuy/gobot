package channel

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gobot/providers"
	"gobot/types"
	gows "golang.org/x/net/websocket"
)

type stubChatProvider struct{}

func (s *stubChatProvider) Chat(ctx context.Context, messages []providers.Message, model string, options map[string]any) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{ReasoningContent: "thinking", Content: "ok"}, nil
}

func (s *stubChatProvider) ChatStream(ctx context.Context, messages []providers.Message, model string, options map[string]any) (<-chan types.StreamChunk, error) {
	ch := make(chan types.StreamChunk, 3)
	go func() {
		ch <- types.StreamChunk{Thinking: "thinking"}
		ch <- types.StreamChunk{Content: "ok"}
		ch <- types.StreamChunk{IsDone: true}
		close(ch)
	}()
	return ch, nil
}

func (s *stubChatProvider) GetDefaultModel() string { return "" }

func newWSConn(t *testing.T, fn func(*gows.Conn)) *gows.Conn {
	t.Helper()
	srv := httptest.NewServer(gows.Handler(fn))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, err := gows.Dial(wsURL, "", "http://localhost/")
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	return conn
}

func TestSendConnectChallenge_POST(t *testing.T) {
	conn := newWSConn(t, func(ws *gows.Conn) {
		if err := SendConnectChallenge(ws); err != nil {
			t.Errorf("SendConnectChallenge error = %v", err)
		}
	})

	var got map[string]any
	if err := gows.JSON.Receive(conn, &got); err != nil {
		t.Fatalf("receive challenge: %v", err)
	}
	if got["type"] != "event" {
		t.Fatalf("type = %v, want event", got["type"])
	}
	if got["event"] != "connect.challenge" {
		t.Fatalf("event = %v, want connect.challenge", got["event"])
	}
	payload, ok := got["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload missing or invalid: %T", got["payload"])
	}
	if _, ok := payload["nonce"].(string); !ok {
		t.Fatalf("payload.nonce invalid: %v", payload["nonce"])
	}
	if _, ok := payload["ts"].(float64); !ok {
		t.Fatalf("payload.ts invalid: %v", payload["ts"])
	}
}

func TestHandleConnect_ValidToken_POST(t *testing.T) {
	conn := newWSConn(t, func(ws *gows.Conn) {
		// Use params.auth.token format (OpenClaw Gateway format)
		req := WSRequest{Type: "req", ID: "1", Method: "connect", Params: map[string]any{"auth": map[string]any{"token": ValidToken}}}
		if err := HandleConnect(ws, req); err != nil {
			t.Errorf("HandleConnect error = %v", err)
		}
	})

	var got map[string]any
	if err := gows.JSON.Receive(conn, &got); err != nil {
		t.Fatalf("receive connect response: %v", err)
	}
	if got["type"] != "res" {
		t.Fatalf("type = %v, want res", got["type"])
	}
	if got["id"] != "1" {
		t.Fatalf("id = %v, want 1", got["id"])
	}
	if got["ok"] != true {
		t.Fatalf("ok = %v, want true", got["ok"])
	}
	payload, ok := got["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload missing or invalid: %T", got["payload"])
	}
	if payload["type"] != "hello-ok" {
		t.Fatalf("payload.type = %v, want hello-ok", payload["type"])
	}
	if payload["protocol"] != float64(ProtocolVersion) {
		t.Fatalf("payload.protocol = %v, want %d", payload["protocol"], ProtocolVersion)
	}
}

func TestHandleConnect_InvalidToken_POST(t *testing.T) {
	conn := newWSConn(t, func(ws *gows.Conn) {
		req := WSRequest{Type: "req", ID: "2", Method: "connect", Params: map[string]any{"auth": map[string]any{"token": "bad"}}}
		_ = HandleConnect(ws, req) // returns error after sending connect.error
	})

	var got map[string]any
	if err := gows.JSON.Receive(conn, &got); err != nil {
		t.Fatalf("receive connect error event: %v", err)
	}
	if got["type"] != "event" {
		t.Fatalf("type = %v, want event", got["type"])
	}
	if got["event"] != "connect.error" {
		t.Fatalf("event = %v, want connect.error", got["event"])
	}
}

func TestHandleChatSend_POST(t *testing.T) {
	origProvider, origModel := ChatProvider, ChatModel
	ChatProvider = &stubChatProvider{}
	ChatModel = "gpt-4o"
	t.Cleanup(func() {
		ChatProvider = origProvider
		ChatModel = origModel
	})

	conn := newWSConn(t, func(ws *gows.Conn) {
		// Use params.message and params.sessionKey format
		req := WSRequest{Type: "req", ID: "3", Method: "chat.send", Params: map[string]any{"message": "hello", "sessionKey": "main"}}
		if err := HandleChatSend(ws, req); err != nil {
			t.Errorf("HandleChatSend error = %v", err)
		}
	})

	// Receive multiple events: reasoning (2x) + content (stream) + final
	events := []string{}
	for {
		var got map[string]any
		if err := gows.JSON.Receive(conn, &got); err != nil {
			break // EOF or no more messages
		}
		if got["type"] == "event" && got["event"] == "agent" {
			events = append(events, "agent")
		}
		if got["type"] == "event" && got["event"] == "chat" {
			events = append(events, "chat")
			// Verify final chat event
			payload, ok := got["payload"].(map[string]any)
			if !ok {
				t.Fatalf("payload missing or invalid: %T", got["payload"])
			}
			if payload["state"] != "final" {
				t.Fatalf("payload.state = %v, want final", payload["state"])
			}
		}
	}

	// Should have: reasoning (2) + content (stream) + final (1) = at least 3 events
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events (reasoning + content + final), got %d: %v", len(events), events)
	}
}

func TestHandleMessage_InputValidation_EdgeCases(t *testing.T) {
	tests := []WSRequest{
		{Type: "req", ID: "m1", Method: "", Params: nil},
		{Type: "req", ID: "m2", Method: "unknown", Params: nil},
		{Type: "req", ID: "m3", Method: "connect", Params: map[string]any{}},
	}

	for _, tc := range tests {
		t.Run(tc.ID, func(t *testing.T) {
			conn := newWSConn(t, func(ws *gows.Conn) {
				_ = HandleMessage(ws, tc)
			})
			var got map[string]any
			_ = gows.JSON.Receive(conn, &got)
		})
	}
}

// TestHandleChatSend_EmptyMessage tests error handling for missing message
func TestHandleChatSend_EmptyMessage(t *testing.T) {
	origProvider, origModel := ChatProvider, ChatModel
	ChatProvider = &stubChatProvider{}
	ChatModel = "test-model"
	t.Cleanup(func() {
		ChatProvider = origProvider
		ChatModel = origModel
	})

	conn := newWSConn(t, func(ws *gows.Conn) {
		req := WSRequest{Type: "req", ID: "test-empty", Method: "chat.send", Params: map[string]any{}}
		err := HandleChatSend(ws, req)
		if err == nil {
			t.Error("expected error for empty message, got nil")
		}
		if !strings.Contains(err.Error(), "missing message") {
			t.Errorf("expected 'missing message' error, got: %v", err)
		}
	})

	// Should not receive any message since error is returned
	var got map[string]any
	err := gows.JSON.Receive(conn, &got)
	if err == nil {
		t.Fatalf("expected connection error, got message: %v", got)
	}
}

// TestHandleChatSend_NoProvider tests error handling when ChatProvider not configured
func TestHandleChatSend_NoProvider(t *testing.T) {
	origProvider, origModel := ChatProvider, ChatModel
	ChatProvider = nil
	ChatModel = ""
	t.Cleanup(func() {
		ChatProvider = origProvider
		ChatModel = origModel
	})

	conn := newWSConn(t, func(ws *gows.Conn) {
		req := WSRequest{Type: "req", ID: "test-noprovider", Method: "chat.send", Params: map[string]any{"message": "hello"}}
		err := HandleChatSend(ws, req)
		if err == nil {
			t.Error("expected error for nil provider, got nil")
		}
	})

	var got map[string]any
	err := gows.JSON.Receive(conn, &got)
	if err == nil {
		t.Fatalf("expected connection error, got message: %v", got)
	}
}

// TestHandleChatHistory_EmptyResult tests that history returns empty array when no messages
func TestHandleChatHistory_EmptyResult(t *testing.T) {
	conn := newWSConn(t, func(ws *gows.Conn) {
		req := WSRequest{Type: "req", ID: "history-1", Method: "chat.history", Params: map[string]any{}}
		if err := HandleChatHistory(ws, req); err != nil {
			t.Errorf("HandleChatHistory error = %v", err)
		}
	})

	var got map[string]any
	if err := gows.JSON.Receive(conn, &got); err != nil {
		t.Fatalf("receive response: %v", err)
	}
	if got["type"] != "res" {
		t.Fatalf("type = %v, want res", got["type"])
	}
	if got["ok"] != true {
		t.Fatalf("ok = %v, want true", got["ok"])
	}
	payload, ok := got["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload missing or invalid: %T", got["payload"])
	}
	messages, ok := payload["messages"].([]any)
	if !ok {
		t.Fatalf("payload.messages invalid: %T", payload["messages"])
	}
	if len(messages) != 0 {
		t.Fatalf("expected empty messages array, got %d items", len(messages))
	}
}

// TestConcurrentConnections tests WebSocket server with multiple concurrent connections
func TestConcurrentConnections(t *testing.T) {
	origProvider, origModel := ChatProvider, ChatModel
	ChatProvider = &stubChatProvider{}
	ChatModel = "test-model"
	t.Cleanup(func() {
		ChatProvider = origProvider
		ChatModel = origModel
	})

	const numClients = 5
	done := make(chan bool, numClients)

	for i := 0; i < numClients; i++ {
		go func(id int) {
			conn := newWSConn(t, func(ws *gows.Conn) {
				// Connect
				req := WSRequest{Type: "req", ID: "conn-" + string(rune('0'+id)), Method: "connect", Params: map[string]any{"auth": map[string]any{"token": ValidToken}}}
				if err := HandleConnect(ws, req); err != nil {
					t.Errorf("client %d: HandleConnect error = %v", id, err)
				}
			})

			var got map[string]any
			if err := gows.JSON.Receive(conn, &got); err != nil {
				t.Errorf("client %d: receive error = %v", id, err)
			} else if got["ok"] != true {
				t.Errorf("client %d: expected ok=true, got %v", id, got["ok"])
			}
			done <- true
		}(i)
	}

	// Wait for all clients with timeout
	timeout := time.After(5 * time.Second)
	for i := 0; i < numClients; i++ {
		select {
		case <-done:
			// OK
		case <-timeout:
			t.Fatal("timeout waiting for concurrent clients")
		}
	}
}
