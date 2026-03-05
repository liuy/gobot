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
//   - Frontend static files served at "/"

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"gobot/protocol"
	"golang.org/x/net/websocket"
)

//go:embed frontend
var staticFiles embed.FS

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
	// WebSocket endpoint
	http.Handle("/ws", websocket.Handler(handleWebSocket))

	// Serve embedded frontend files
	frontendFS, _ := fs.Sub(staticFiles, "frontend")
	http.Handle("/", http.FileServer(http.FS(frontendFS)))

	log.Println("[INFO] WebSocket server starting on localhost", DefaultPort)
	log.Println("[INFO] WebSocket endpoint: ws://127.0.0.1" + DefaultPort + "/ws")
	log.Fatal("[ERROR] Server failed:", http.ListenAndServe(DefaultPort, nil))
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
		log.Println("[ERROR] SendConnectChallenge:", err)
		_ = ws.Close()
		return
	}
	for {
		var req protocol.WSRequest
		if err := websocket.JSON.Receive(ws, &req); err != nil {
			if err.Error() == "EOF" {
				log.Println("[INFO] Client disconnected")
			} else {
				log.Println("[ERROR] Receive:", err)
			}
			_ = ws.Close()
			return
		}
		if err := protocol.HandleMessage(ws, req); err != nil {
			log.Println("[ERROR] HandleMessage:", err)
			_ = ws.Close()
			return
		}
	}
}
