package main

// MODULE SPEC: main
//
// RELY:
//   - handler provides SendConnectChallenge, HandleConnect, HandleChatSend, HandleMessage
//   - net/http provides http.ListenAndServe
//   - golang.org/x/net/websocket provides websocket.Handler
//
// GUARANTEE:
//   - WebSocket server listening on DefaultPort
//   - WebSocket endpoint at "/ws"
//   - Frontend static files served at "/" (TODO: implement later)

import (
	"net/http"
	"os"
	"strings"

	"gobot/log"
	"gobot/protocol"
	"gobot/providers"
	"golang.org/x/net/websocket"
)

var (
	llmProvider providers.LLMProvider
	llmModel    string
)

const (
	DefaultPort = ":10086"
)

// FUNC SPEC: main
//
// PRE:
//   - port is available for listening
//
// POST:
//   - returns error if server fails to start
//
// INTENT:
func main() {
	model := strings.TrimSpace(os.Getenv("LLM_MODEL"))
	apiKey := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	if model == "" {
		log.Fatal("LLM_MODEL is required")
	}
	if apiKey == "" {
		log.Fatal("LLM_API_KEY is required")
	}
	cfg := &providers.ModelConfig{
		Model:   model,
		APIKey:  apiKey,
		APIBase: strings.TrimSpace(os.Getenv("LLM_API_BASE")),
	}
	if protocolName, _ := providers.ExtractProtocol(model); protocolName == "openai" && cfg.APIBase == "" {
		cfg.APIBase = "https://api.openai.com/v1"
	}
	var err error
	llmProvider, llmModel, err = providers.CreateProvider(cfg)
	if err != nil {
		log.Fatal("CreateProvider failed: %v", err)
	}
	protocol.ChatProvider = llmProvider
	protocol.ChatModel = llmModel

	// WebSocket endpoint
	http.Handle("/ws", websocket.Handler(handleWebSocket))

	// TODO: Serve frontend files (currently disabled)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("gobot backend - frontend not yet implemented"))
	})

	log.Info("WebSocket server starting on localhost %s", DefaultPort)
	log.Info("WebSocket endpoint: ws://127.0.0.1%s/ws", DefaultPort)
	log.Fatal("Server failed: %v", http.ListenAndServe(DefaultPort, nil))
}

// FUNC SPEC: handleWebSocket
//
// PRE:
//   - ws is a valid WebSocket connection
//
// POST:
//   - sends connect.challenge on connect
//   - sends WSResponse or WSEvent based on request
//   - Case error: logs error message and closes WebSocket
//
// INTENT:
func handleWebSocket(ws *websocket.Conn) {
	if err := protocol.SendConnectChallenge(ws); err != nil {
		log.Error("SendConnectChallenge: %v", err)
		_ = ws.Close()
		return
	}
	for {
		var req protocol.WSRequest
		if err := websocket.JSON.Receive(ws, &req); err != nil {
			if err.Error() == "EOF" {
				log.Info("Client disconnected")
			} else {
				log.Error("Receive: %v", err)
			}
			_ = ws.Close()
			return
		}
		if err := protocol.HandleMessage(ws, req); err != nil {
			log.Error("HandleMessage: %v", err)
			_ = ws.Close()
			return
		}
	}
}
