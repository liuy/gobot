package protocol

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gobot/providers"
	"golang.org/x/net/websocket"
)

type stubChatProvider struct{}

func (s *stubChatProvider) Chat(ctx context.Context, messages []providers.Message, model string, options map[string]any) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{ReasoningContent: "thinking", Content: "ok"}, nil
}

func (s *stubChatProvider) ChatStream(ctx context.Context, messages []providers.Message, model string, options map[string]any, handler providers.StreamHandler) error {
	handler(&providers.LLMResponse{ReasoningContent: "thinking", IsStreaming: true})
	handler(&providers.LLMResponse{Content: "ok", IsStreaming: true})
	handler(&providers.LLMResponse{IsDone: true})
	return nil
}

func (s *stubChatProvider) GetDefaultModel() string { return "" }

func newWSConn(t *testing.T, fn func(*websocket.Conn)) *websocket.Conn {
	t.Helper()
	srv := httptest.NewServer(websocket.Handler(fn))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, err := websocket.Dial(wsURL, "", "http://localhost/")
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	return conn
}

func TestSendConnectChallenge_POST(t *testing.T) {
	conn := newWSConn(t, func(ws *websocket.Conn) {
		if err := SendConnectChallenge(ws); err != nil {
			t.Errorf("SendConnectChallenge error = %v", err)
		}
	})

	var got map[string]any
	if err := websocket.JSON.Receive(conn, &got); err != nil {
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
	conn := newWSConn(t, func(ws *websocket.Conn) {
		// Use params.auth.token format (OpenClaw Gateway format)
		req := WSRequest{Type: "req", ID: "1", Method: "connect", Params: map[string]any{"auth": map[string]any{"token": ValidToken}}}
		if err := HandleConnect(ws, req); err != nil {
			t.Errorf("HandleConnect error = %v", err)
		}
	})

	var got map[string]any
	if err := websocket.JSON.Receive(conn, &got); err != nil {
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
	conn := newWSConn(t, func(ws *websocket.Conn) {
		req := WSRequest{Type: "req", ID: "2", Method: "connect", Params: map[string]any{"auth": map[string]any{"token": "bad"}}}
		_ = HandleConnect(ws, req) // returns error after sending connect.error
	})

	var got map[string]any
	if err := websocket.JSON.Receive(conn, &got); err != nil {
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

	conn := newWSConn(t, func(ws *websocket.Conn) {
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
		if err := websocket.JSON.Receive(conn, &got); err != nil {
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
			conn := newWSConn(t, func(ws *websocket.Conn) {
				_ = HandleMessage(ws, tc)
			})
			var got map[string]any
			_ = websocket.JSON.Receive(conn, &got)
		})
	}
}
