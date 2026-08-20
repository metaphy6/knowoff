package transport

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WebSocketHandler accepts WebSocket upgrade requests and establishes a
// connection. For Phase 1 it performs an echo round-trip so client contract
// tests have something to talk to.
func WebSocketHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := deps.Logger.With("conn_id", uuid.NewString())

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warn("websocket upgrade failed", "error", err)
			return
		}
		defer conn.Close()

		if g, ok := deps.Connections.(interface {
			Inc()
			Dec()
		}); ok {
			g.Inc()
			defer g.Dec()
		}

		logger.Info("websocket connection established")
		if err := echoLoop(conn, logger); err != nil {
			logger.Info("websocket connection closed", "error", err)
		}
	}
}

// echoLoop reads JSON messages and echoes them back with an "echo": true flag.
func echoLoop(conn *websocket.Conn, logger *slog.Logger) error {
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType == websocket.TextMessage {
			// Append a simple echo marker to the JSON payload. We append bytes
			// instead of parsing/re-encoding to keep Phase 1 dependency-free.
			trimmed := bytes.TrimRight(data, " \t\r\n")
			if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '}' {
				data = append(trimmed[:len(trimmed)-1], []byte(`,"echo":true}`)...)
			}
		}
		if err := conn.WriteMessage(messageType, data); err != nil {
			return err
		}
	}
}
