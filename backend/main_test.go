package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func newMainWSConn(t *testing.T, fn func(*websocket.Conn)) *websocket.Conn {
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

func TestHandleWebSocket_SendsConnectChallenge_POST(t *testing.T) {
	conn := newMainWSConn(t, handleWebSocket)

	var got map[string]any
	if err := websocket.JSON.Receive(conn, &got); err != nil {
		t.Fatalf("receive initial challenge: %v", err)
	}
	if got["event"] != "connect.challenge" {
		t.Fatalf("event = %v, want connect.challenge", got["event"])
	}
}

func TestHandleWebSocket_ConnectValidation_POST(t *testing.T) {
	conn := newMainWSConn(t, handleWebSocket)

	var challenge map[string]any
	if err := websocket.JSON.Receive(conn, &challenge); err != nil {
		t.Fatalf("receive challenge: %v", err)
	}

	req := map[string]any{"type": "request", "id": "req-1", "method": "connect", "params": map[string]any{"token": "bad-token"}}
	if err := websocket.JSON.Send(conn, req); err != nil {
		t.Fatalf("send connect request: %v", err)
	}

	var got map[string]any
	if err := websocket.JSON.Receive(conn, &got); err != nil {
		t.Fatalf("receive connect result: %v", err)
	}
	if got["event"] != "connect.error" {
		t.Fatalf("event = %v, want connect.error", got["event"])
	}
}

func TestHandleWebSocket_EdgeCaseMalformedMessage(t *testing.T) {
	conn := newMainWSConn(t, handleWebSocket)
	var challenge map[string]any
	_ = websocket.JSON.Receive(conn, &challenge)

	if _, err := conn.Write([]byte("{invalid-json")); err != nil {
		t.Fatalf("write malformed frame: %v", err)
	}

	buf := make([]byte, 1)
	_, err := conn.Read(buf)
	if err == nil {
		t.Fatalf("expected read error after malformed input")
	}
}
