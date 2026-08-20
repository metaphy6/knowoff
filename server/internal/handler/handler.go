package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/transport"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ConnectionState tracks a live WebSocket and its routed seat.
type ConnectionState struct {
	Conn         *websocket.Conn
	Room         *lobby.Room
	Seat         int
	sessionToken string
	Lobby        *lobby.Manager
	Logger       *slog.Logger
}

// HandlerDeps bundles dependencies for the realtime WebSocket handler.
type HandlerDeps struct {
	Config      *config.Config
	Logger      *slog.Logger
	Lobby       *lobby.Manager
	Connections interface{}
}

// RealtimeHandler upgrades HTTP requests to WebSockets and routes Knowoff
// intents to the authoritative match. The first message must be a valid
// handshake/join envelope.
func RealtimeHandler(deps HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := deps.Logger.With("conn_id", uuid.NewString())

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warn("websocket upgrade failed", "error", err)
			return
		}

		if g, ok := deps.Connections.(interface {
			Inc()
			Dec()
		}); ok {
			g.Inc()
			defer g.Dec()
		}

		state := &ConnectionState{
			Conn:   conn,
			Lobby:  deps.Lobby,
			Logger: logger,
		}
		defer state.Close()

		if err := state.run(r.Context()); err != nil {
			logger.Info("websocket connection closed", "error", err)
		}
	}
}

// run performs the read loop. It expects a handshake as the first message,
// then routes intents to the room.
func (s *ConnectionState) run(ctx context.Context) error {
	// Handshake: first message must carry protocol version and join intent.
	env, err := s.readEnvelope()
	if err != nil {
		return s.reject("protocol_error", err.Error())
	}
	if env.Version != transport.ProtocolVersion {
		return s.reject("unsupported_version", fmt.Sprintf("expected %d", transport.ProtocolVersion))
	}
	if !transport.IntentIsPhase3(env.Kind) && env.Kind != transport.IntentQueueQuickPlay {
		return s.reject("expected_intent", "first message must be an intent")
	}

	if err := s.handleJoinIntent(env); err != nil {
		return s.reject("join_failed", err.Error())
	}

	// Main read loop.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		env, err := s.readEnvelope()
		if err != nil {
			return err
		}
		if !transport.IntentIsPhase3(env.Kind) {
			_ = s.sendError("expected_intent", "only intents accepted after join")
			continue
		}
		if err := s.handleIntent(env); err != nil {
			_ = s.sendError("rejected", err.Error())
		}
	}
}

func (s *ConnectionState) readEnvelope() (*transport.Envelope, error) {
	_, data, err := s.Conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	return transport.DecodeEnvelope(data, 0)
}

func (s *ConnectionState) handleJoinIntent(env *transport.Envelope) error {
	switch env.Kind {
	case transport.IntentJoinRoom:
		code, _ := env.Payload["code"].(string)
		token, _ := env.Payload["session_token"].(string)
		room := s.Lobby.RoomByCode(code)
		if room == nil {
			return fmt.Errorf("room not found")
		}

		var seat int
		var ok bool
		if token != "" {
			seat, ok = room.ReclaimSeat(token)
			if !ok {
				return fmt.Errorf("invalid session token")
			}
			token = room.SessionToken(seat)
		} else {
			seat, token, ok = room.ClaimSeat()
			if !ok {
				return fmt.Errorf("room full")
			}
		}
		s.Room = room
		s.Seat = seat
		s.sessionToken = token
		s.Room.SetConnection(seat, s.Conn)
		s.Logger = s.Logger.With("room_id", room.ID, "seat", seat)
		s.Logger.Info("joined room")
		return s.sendOK("joined", map[string]any{"room_id": room.ID, "seat": seat, "code": room.Code, "size": room.Size, "session_token": token})

	case transport.IntentQueueQuickPlay:
		sizeF, _ := env.Payload["size"].(float64)
		size := int(sizeF)
		queueID, assigned, err := s.Lobby.QueueQuickPlay(size)
		if err != nil {
			return err
		}
		defer s.Lobby.RemoveFromQueue(queueID)
		// Wait until ProcessQueue assigns us to a room.
		select {
		case a := <-assigned:
			s.Room = a.Room
			s.Seat = a.Seat
			s.sessionToken = a.SessionToken
		case <-time.After(30 * time.Second):
			return fmt.Errorf("queue timeout")
		}
		// Bind connection; the room auto-starts once every seat binds.
		s.Room.SetConnection(s.Seat, s.Conn)
		s.Logger = s.Logger.With("room_id", s.Room.ID, "seat", s.Seat)
		s.Logger.Info("assigned from queue")
		return s.sendOK("joined", map[string]any{"room_id": s.Room.ID, "seat": s.Seat, "size": s.Room.Size, "session_token": s.sessionToken})

	default:
		return fmt.Errorf("first intent must be join_room or queue_quickplay")
	}
}

func (s *ConnectionState) handleIntent(env *transport.Envelope) error {
	if s.Room == nil {
		return fmt.Errorf("not joined")
	}
	m := s.Room.Match()
	if m == nil {
		return fmt.Errorf("match not started")
	}
	return m.HandleIntent(s.Seat, env)
}

func (s *ConnectionState) sendOK(key string, payload map[string]any) error {
	return s.send(transport.NewEvent("joined_"+key, payload))
}

func (s *ConnectionState) sendError(code, message string) error {
	return s.send(transport.NewErrorEnvelope(code, map[string]any{"message": message}, ""))
}

func (s *ConnectionState) reject(code, message string) error {
	_ = s.sendError(code, message)
	return fmt.Errorf("%s: %s", code, message)
}

func (s *ConnectionState) send(env *transport.Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return s.Conn.WriteMessage(websocket.TextMessage, data)
}

// Close removes the connection from its room and closes the socket.
func (s *ConnectionState) Close() {
	if s.Room != nil {
		s.Room.SetConnection(s.Seat, nil)
	}
	if s.Conn != nil {
		_ = s.Conn.Close()
	}
}
