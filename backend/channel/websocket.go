package channel

// MODULE SPEC: channel/websocket
//
// RELY:
//   - golang.org/x/net/websocket for WebSocket communication
//
// GUARANTEE:
//   - Implements OpenClaw Gateway protocol version 3
//   - Sends connect.challenge on new connection
//   - Authenticates via token in connect request
//   - Handles chat.send with streaming response (reasoning + content)
//   - Handles chat.history, server.commands, sessions.list methods
//   - Dispatches incoming messages to appropriate handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gobot/providers"
	gows "golang.org/x/net/websocket"
)

// Type Definitions

type WSRequest struct {
	Type   string         `json:"type"`
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type WSResponse struct {
	Type    string `json:"type"` // "res"
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Payload any    `json:"payload,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type WSEvent struct {
	Type    string `json:"type"` // "res"
	Event   string `json:"event"`
	Payload any    `json:"payload,omitempty"`
}

type ConnectChallengePayload struct {
	Nonce string `json:"nonce"`
	TS    int64  `json:"ts"`
}

type HelloOkPayload struct {
	Type     string    `json:"type"`
	Protocol int       `json:"protocol"`
	Features *Features `json:"features,omitempty"`
	Auth     *Auth     `json:"auth,omitempty"`
	Policy   *Policy   `json:"policy,omitempty"`
}

type Features struct {
	Methods []string `json:"methods,omitempty"`
	Events  []string `json:"events,omitempty"`
}

type Auth struct {
	DeviceToken *string  `json:"deviceToken,omitempty"`
	Role        *string  `json:"role,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	IssuedAtMs  *int64   `json:"issuedAtMs,omitempty"`
}

type Policy struct {
	TickIntervalMs *int64 `json:"tickIntervalMs,omitempty"`
}

type ConnectErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ChatEventPayload struct {
	State   string       `json:"state"`
	Message *ChatMessage `json:"message,omitempty"`
}

type ChatMessage struct {
	Role      string `json:"role"`
	Content   any    `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// Constants

const (
	ProtocolVersion = 3
	ValidToken      = "123456"
)

var (
	ChatProvider providers.LLMProvider
	ChatModel    string
)

// FUNC SPEC: SendConnectChallenge
//
// PRE:
//   - conn is a valid WebSocket connection
//   - conn is open and ready for writing
//
// POST:
//   - Sends {"type":"event","event":"connect.challenge","payload":{"nonce":"<nanos>","ts":<millis>}}
//   - Returns error if JSON serialization or write fails
//
// INTENT:
//   - Initiate connection handshake by sending challenge to client
func SendConnectChallenge(conn *gows.Conn) error {
	return gows.JSON.Send(conn, WSEvent{
		Type:  "event",
		Event: "connect.challenge",
		Payload: ConnectChallengePayload{
			Nonce: fmt.Sprintf("%d", time.Now().UnixNano()),
			TS:    time.Now().UnixMilli(),
		},
	})
}

// FUNC SPEC: HandleConnect
//
// PRE:
//   - conn is a valid WebSocket connection
//   - conn is open and ready for writing
//   - req.Method == "connect"
//   - req.Params["auth"]["token"] exists (string)
//
// POST:
//   - Case token == "123456": sends WSResponse{OK:true, Payload:HelloOkPayload}
//   - Case token != "123456": sends WSEvent{Event:"connect.error"} + returns error
//
// INTENT:
//   - Authenticate client connection via token validation
func HandleConnect(conn *gows.Conn, req WSRequest) error {
	auth, _ := req.Params["auth"].(map[string]any)
	token, _ := auth["token"].(string)
	if token == ValidToken {
		return gows.JSON.Send(conn, WSResponse{
			Type: "res",
			ID:   req.ID,
			OK:   true,
			Payload: HelloOkPayload{
				Type:     "hello-ok",
				Protocol: ProtocolVersion,
			},
		})
	}
	_ = gows.JSON.Send(conn, WSEvent{
		Type:  "event",
		Event: "connect.error",
		Payload: ConnectErrorPayload{
			Code:    "invalid_token",
			Message: "Invalid Token",
		},
	})
	return fmt.Errorf("Invalid Token")
}

// FUNC SPEC: HandleChatSend
//
// PRE:
//   - conn is a valid WebSocket connection
//   - conn is open and ready for writing
//   - req.Method == "chat.send"
//   - req.Params["message"] exists (string)
//   - req.Params["sessionKey"] exists (string)
//
// POST:
//   - Sends agent events with stream="reasoning" (empty, then streaming text)
//   - Sends agent events with stream="content" (character-by-character)
//   - Sends final chat event with state="final" and assistant message
//   - Returns error if any send fails
//
// INTENT:
//   - Run LLM chat via configured provider and stream reasoning/content to client
func HandleChatSend(conn *gows.Conn, req WSRequest) error {
	content, _ := req.Params["message"].(string)
	sessionKey, _ := req.Params["sessionKey"].(string)
	runId := req.ID
	if ChatProvider == nil {
		return fmt.Errorf("chat provider not configured")
	}
	if strings.TrimSpace(ChatModel) == "" {
		return fmt.Errorf("chat model not configured")
	}

	sendAgent := func(stream string, delta string) error {
		return gows.JSON.Send(conn, WSEvent{
			Type:  "event",
			Event: "agent",
			Payload: map[string]any{
				"runId":      runId,
				"sessionKey": sessionKey,
				"stream":     stream,
				"data":       map[string]string{"delta": delta},
				"ts":         time.Now().UnixMilli(),
			},
		})
	}

	messages := make([]providers.Message, 0, 8)
	if rawMessages, ok := req.Params["messages"].([]any); ok {
		for _, raw := range rawMessages {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			msgContent, _ := m["content"].(string)
			if role == "" || msgContent == "" {
				continue
			}
			messages = append(messages, providers.Message{
				Role:    role,
				Content: msgContent,
			})
		}
	}
	if content != "" {
		messages = append(messages, providers.Message{
			Role:    "user",
			Content: content,
		})
	}
	if len(messages) == 0 {
		return fmt.Errorf("chat.send missing message")
	}

	// Use streaming to get real-time thinking and content
	ch, err := ChatProvider.ChatStream(context.Background(), messages, ChatModel, nil)
	if err != nil {
		return err
	}

	var finalContent string
	var finalReasoning string
	sentReasoningBlock := false
	sentContentBlock := false

	for chunk := range ch {
		// Handle reasoning/thinking stream
		if chunk.Thinking != "" {
			if !sentReasoningBlock {
				if err := gows.JSON.Send(conn, WSEvent{
					Type:  "event",
					Event: "agent",
					Payload: map[string]any{
						"runId":      runId,
						"sessionKey": sessionKey,
						"stream":     "reasoning",
						"data":       map[string]any{"newBlock": true},
						"ts":         time.Now().UnixMilli(),
					},
				}); err != nil {
					return err
				}
				sentReasoningBlock = true
			}
			finalReasoning += chunk.Thinking
			if err := sendAgent("reasoning", chunk.Thinking); err != nil {
				return err
			}
		}

		// Handle content stream
		if chunk.Content != "" {
			if !sentContentBlock {
				if err := gows.JSON.Send(conn, WSEvent{
					Type:  "event",
					Event: "agent",
					Payload: map[string]any{
						"runId":      runId,
						"sessionKey": sessionKey,
						"stream":     "content",
						"data":       map[string]any{"newBlock": true},
						"ts":         time.Now().UnixMilli(),
					},
				}); err != nil {
					return err
				}
				sentContentBlock = true
			}
			finalContent += chunk.Content
			if err := sendAgent("content", chunk.Content); err != nil {
				return err
			}
		}
	}

	return gows.JSON.Send(conn, WSEvent{
		Type:  "event",
		Event: "chat",
		Payload: map[string]any{
			"runId":      runId,
			"sessionKey": sessionKey,
			"state":      "final",
			"message": map[string]any{
				"role":      "assistant",
				"content":   finalContent,
				"timestamp": time.Now().UnixMilli(),
			},
		},
	})
}

// FUNC SPEC: HandleChatHistory
//
// PRE:
//   - conn is a valid WebSocket connection
//   - conn is open and ready for writing
//   - req.Method == "chat.history"
//
// POST:
//   - Sends WSResponse{OK:true, Payload:{"messages":[]}}
//
// INTENT:
//   - Return empty chat history (demo mode, no persistence)
func HandleChatHistory(conn *gows.Conn, req WSRequest) error {
	return gows.JSON.Send(conn, WSResponse{
		Type: "res",
		ID:   req.ID,
		OK:   true,
		Payload: map[string]any{
			"messages": []any{},
		},
	})
}

// FUNC SPEC: HandleServerCommands
//
// PRE:
//   - conn is a valid WebSocket connection
//   - conn is open and ready for writing
//   - req.Method == "server.commands"
//
// POST:
//   - Sends WSResponse{OK:true, Payload:{"commands":[]}}
//
// INTENT:
//   - Return empty server commands list (demo mode)
func HandleServerCommands(conn *gows.Conn, req WSRequest) error {
	return gows.JSON.Send(conn, WSResponse{
		Type: "res",
		ID:   req.ID,
		OK:   true,
		Payload: map[string]any{
			"commands": []any{},
		},
	})
}

// FUNC SPEC: HandleSessionsList
//
// PRE:
//   - conn is a valid WebSocket connection
//   - conn is open and ready for writing
//   - req.Method == "sessions.list"
//
// POST:
//   - Sends WSResponse{OK:true, Payload:{"sessions":[]}}
//
// INTENT:
//   - Return empty sessions list (demo mode, no session management)
func HandleSessionsList(conn *gows.Conn, req WSRequest) error {
	return gows.JSON.Send(conn, WSResponse{
		Type: "res",
		ID:   req.ID,
		OK:   true,
		Payload: map[string]any{
			"sessions": []any{},
		},
	})
}

// FUNC SPEC: HandleMessage
//
// PRE:
//   - conn is a valid WebSocket connection
//   - conn is open and ready for writing
//   - req is a parsed WSRequest with Method field
//
// POST:
//   - Case req.Method == "connect": delegates to HandleConnect
//   - Case req.Method == "chat.send": delegates to HandleChatSend
//   - Case req.Method == "chat.history": delegates to HandleChatHistory
//   - Case req.Method == "server.commands": delegates to HandleServerCommands
//   - Case req.Method == "sessions.list": delegates to HandleSessionsList
//   - Case unknown method: sends WSResponse{OK:false, Error:{"code":"unknown_method"}}
//
// INTENT:
//   - Route incoming WebSocket requests to appropriate handlers
func HandleMessage(conn *gows.Conn, req WSRequest) error {
	switch req.Method {
	case "connect":
		return HandleConnect(conn, req)
	case "chat.send":
		return HandleChatSend(conn, req)
	case "chat.history":
		return HandleChatHistory(conn, req)
	case "server.commands":
		return HandleServerCommands(conn, req)
	case "sessions.list":
		return HandleSessionsList(conn, req)
	default:
		return gows.JSON.Send(conn, WSResponse{
			Type:  "res",
			ID:    req.ID,
			OK:    false,
			Error: map[string]string{"code": "unknown_method", "message": "unknown method"},
		})
	}
}
