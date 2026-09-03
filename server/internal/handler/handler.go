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
	"github.com/knowoff/knowoff/server/internal/audit"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/leaderboard"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/profile"
	"github.com/knowoff/knowoff/server/internal/ratelimit"
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
	AccountID    string
	accessToken  string
	sessionToken string
	Lobby        *lobby.Manager
	Logger       *slog.Logger
	Auth         *auth.Manager
	Profile      *profile.Manager
	Audit        *audit.Logger
	Economy      *economy.Manager
	Redis        interface {
		AllowIntent(ctx context.Context, accountID string, window time.Duration, max int) (bool, error)
	}
	rateLimiter *ratelimit.Limiter
}

// HandlerDeps bundles dependencies for the realtime WebSocket handler.
type HandlerDeps struct {
	Config      *config.Config
	Logger      *slog.Logger
	Lobby       *lobby.Manager
	Connections interface{}
	Auth        *auth.Manager
	Profile     *profile.Manager
	Audit       *audit.Logger
	Leaderboard *leaderboard.Manager
	Economy     *economy.Manager
	Redis       interface {
		AllowIntent(ctx context.Context, accountID string, window time.Duration, max int) (bool, error)
	}
	// ConnLimiter caps concurrent live WebSocket connections; nil means unlimited.
	ConnLimiter *ratelimit.ConnLimiter
	// ConnLimiterRejections counts upgrades rejected because ConnLimiter was
	// at capacity; nil is fine (no metric recorded).
	ConnLimiterRejections interface{ Inc() }
}

// RealtimeHandler upgrades HTTP requests to WebSockets and routes Knowoff
// intents to the authoritative match. The first message must be a valid
// handshake/join envelope.
func RealtimeHandler(deps HandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := deps.Logger.With("conn_id", uuid.NewString())

		// Reject before the (relatively expensive) upgrade handshake so a burst
		// of clients past capacity fails fast with a plain HTTP response.
		if deps.ConnLimiter != nil && !deps.ConnLimiter.TryAcquire() {
			http.Error(w, "server at capacity", http.StatusServiceUnavailable)
			logger.Warn("websocket connection rejected: at capacity")
			if deps.ConnLimiterRejections != nil {
				deps.ConnLimiterRejections.Inc()
			}
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			if deps.ConnLimiter != nil {
				deps.ConnLimiter.Release()
			}
			logger.Warn("websocket upgrade failed", "error", err)
			return
		}
		if deps.ConnLimiter != nil {
			defer deps.ConnLimiter.Release()
		}

		if g, ok := deps.Connections.(interface {
			Inc()
			Dec()
		}); ok {
			g.Inc()
			defer g.Dec()
		}

		limiter := ratelimit.New(float64(deps.Config.RateLimit.MaxIntentsPerSecond), float64(deps.Config.RateLimit.MaxIntentsBurst))
		state := &ConnectionState{
			Conn:        conn,
			Lobby:       deps.Lobby,
			Logger:      logger,
			Auth:        deps.Auth,
			Profile:     deps.Profile,
			Audit:       deps.Audit,
			Economy:     deps.Economy,
			Redis:       deps.Redis,
			rateLimiter: limiter,
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

	// Main read loop. Periodically re-validate the access token so bans and
	// revocations drop live connections within seconds.
	revalidate := time.NewTicker(5 * time.Second)
	defer revalidate.Stop()

	// Set read deadline to detect stalled or dead connections. Use PongWaitS
	// plus buffer to allow for legitimate latency. If no message arrives within
	// this time, the connection is considered stuck and will be closed.
	pongWaitDuration := time.Duration(s.Config().WebSocket.PongWaitS) * time.Second
	if pongWaitDuration == 0 {
		pongWaitDuration = 60 * time.Second // fallback default
	}
	readTimeout := pongWaitDuration + 10*time.Second

	// A silent phase (discussion, ballot, vote window) is entirely normal —
	// a player who is just reading and thinking sends no intents for a
	// while. Without a keepalive, that legitimate silence alone would blow
	// past readTimeout and the read deadline would kill the connection out
	// from under them. A background ping ticker plus a pong handler that
	// refreshes the deadline keeps idle-but-healthy connections alive;
	// WriteControl is safe to call concurrently with the other Write calls
	// on this connection (room broadcasts, sendError, etc).
	done := make(chan struct{})
	defer close(done)
	s.Conn.SetPongHandler(func(string) error {
		s.Conn.SetReadDeadline(time.Now().Add(readTimeout))
		return nil
	})
	pingPeriod := time.Duration(s.Config().WebSocket.PingPeriodS) * time.Second
	if pingPeriod <= 0 {
		pingPeriod = 30 * time.Second // fallback default
	}
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := s.Conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					return
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-revalidate.C:
			if s.accessToken != "" && s.Auth != nil {
				if _, err := s.Auth.ValidateAccessToken(ctx, s.accessToken); err != nil {
					_ = s.sendError("token_revoked", "session invalidated")
					return fmt.Errorf("token revoked")
				}
			}
		default:
		}

		// Set read deadline to prevent indefinite blocking if connection stalls
		s.Conn.SetReadDeadline(time.Now().Add(readTimeout))
		env, err := s.readEnvelope()
		if err != nil {
			// Clear deadline on error to clean up
			s.Conn.SetReadDeadline(time.Time{})
			return err
		}
		if s.rateLimiter != nil && s.Config().RateLimit.Enabled && !s.rateLimiter.Allow() {
			_ = s.sendError("rate_limited", "too many intents")
			s.Audit.LogWithAccount(ctx, audit.EventIntentRejected, uuidToAccount(s.AccountID), s.RoomID(), "", map[string]any{"reason": "rate_limited"})
			continue
		}
		if s.Redis != nil && s.Config().RateLimit.Enabled {
			allowed, err := s.Redis.AllowIntent(ctx, s.AccountID, time.Second, s.Config().RateLimit.MaxIntentsPerSecond)
			if err != nil {
				s.Logger.Warn("redis rate limit check failed", "error", err)
			} else if !allowed {
				_ = s.sendError("rate_limited", "too many intents")
				s.Audit.LogWithAccount(ctx, audit.EventIntentRejected, uuidToAccount(s.AccountID), s.RoomID(), "", map[string]any{"reason": "account_rate_limited"})
				continue
			}
		}
		if !transport.IntentIsPhase3(env.Kind) && env.Kind != transport.IntentConvertPoints && env.Kind != transport.IntentReportMedia && env.Kind != transport.IntentDevGrantSpecialty {
			_ = s.sendError("expected_intent", "only intents accepted after join")
			continue
		}
		if err := s.handleIntent(env); err != nil {
			// Tag the rejection with the intent that caused it so a client can
			// tell a stale/unrelated rejection apart from one about its most
			// recent action (e.g. a queued "ready" resurfacing as rejected
			// during the following Knowoff ballot must not be mistaken for the
			// vote itself getting rejected).
			_ = s.sendErrorFor("rejected", err.Error(), env.Kind)
			s.Audit.LogWithAccount(ctx, audit.EventIntentRejected, uuidToAccount(s.AccountID), s.RoomID(), "", map[string]any{"reason": err.Error(), "intent": env.Kind})
		}
	}
}

func (s *ConnectionState) readEnvelope() (*transport.Envelope, error) {
	_, data, err := s.Conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	return transport.DecodeEnvelope(data, s.Config().RateLimit.MaxBytesPerFrame)
}

// Config returns the server config from the lobby manager.
func (s *ConnectionState) Config() *config.Config {
	return s.Lobby.Config()
}

func (s *ConnectionState) handleJoinIntent(env *transport.Envelope) error {
	if s.Auth != nil {
		if at, _ := env.Payload["access_token"].(string); at != "" {
			accountID, err := s.Auth.ValidateAccessToken(context.Background(), at)
			if err != nil {
				// Continuing anonymously here hides the real problem: the
				// join then fails downstream as a bogus economy denial
				// ("daily quickplay limit reached") against the empty account.
				return fmt.Errorf("invalid access token")
			}
			s.AccountID = accountID
			s.accessToken = at
		}
	}
	switch env.Kind {
	case transport.IntentJoinRoom:
		code, _ := env.Payload["code"].(string)
		token, _ := env.Payload["session_token"].(string)
		room := s.Lobby.RoomByCode(code)
		if room == nil {
			s.Logger.Warn("join room rejected: room not found", "code", code, "account", s.AccountID)
			return fmt.Errorf("room not found")
		}

		var seat int
		var ok bool
		if token != "" {
			seat, ok = room.ReclaimSeat(token)
			if !ok {
				s.Logger.Warn("join room rejected: invalid session token", "code", code, "account", s.AccountID)
				return fmt.Errorf("invalid session token")
			}
			token = room.SessionToken(seat)
		} else {
			seat, token, ok = room.ClaimSeat(s.AccountID, false)
			if !ok {
				s.Logger.Warn("join room rejected: room full", "code", code, "account", s.AccountID, "room_size", room.Size)
				return fmt.Errorf("room full")
			}
		}
		s.Room = room
		s.Seat = seat
		s.sessionToken = token
		s.Room.SetConnection(seat, s.Conn)
		s.Logger = s.Logger.With("room_id", room.ID, "seat", seat)
		s.Logger.Info("player joined room", "code", code, "account", s.AccountID, "room_size", room.Size, "reclaimed", token != "")
		return s.sendOK("joined", map[string]any{"room_id": room.ID, "seat": seat, "code": room.Code, "size": room.Size, "session_token": token})

	case transport.IntentQueueQuickPlay:
		sizeF, _ := env.Payload["size"].(float64)
		size := int(sizeF)
		queueID, assigned, err := s.Lobby.QueueQuickPlay(size, s.AccountID)
		if err != nil {
			s.Logger.Warn("quickplay queue request failed", "account", s.AccountID, "size", size, "error", err)
			return err
		}
		defer s.Lobby.RemoveFromQueue(queueID)
		s.Logger.Info("waiting for room assignment", "account", s.AccountID, "size", size, "queue_id", queueID)
		// Wait until ProcessQueue assigns us to a room.
		select {
		case a := <-assigned:
			s.Room = a.Room
			s.Seat = a.Seat
			s.sessionToken = a.SessionToken
		case <-time.After(30 * time.Second):
			s.Logger.Warn("quickplay room assignment timeout", "account", s.AccountID, "size", size, "queue_id", queueID)
			return fmt.Errorf("queue timeout")
		}
		// Bind connection; the room auto-starts once every seat binds.
		s.Room.SetConnection(s.Seat, s.Conn)
		s.Logger = s.Logger.With("room_id", s.Room.ID, "seat", s.Seat)
		s.Logger.Info("player assigned from quickplay queue", "account", s.AccountID, "size", s.Room.Size, "code", s.Room.Code, "queue_id", queueID)
		return s.sendOK("joined", map[string]any{"room_id": s.Room.ID, "seat": s.Seat, "code": s.Room.Code, "size": s.Room.Size, "session_token": s.sessionToken})

	default:
		return fmt.Errorf("first intent must be join_room or queue_quickplay")
	}
}

func (s *ConnectionState) handleIntent(env *transport.Envelope) error {
	switch env.Kind {
	case transport.IntentConvertPoints:
		return s.handleConvertPoints(env)
	case transport.IntentReportMedia:
		return s.handleReportMedia(env)
	case transport.IntentRematch:
		if s.Room == nil {
			return fmt.Errorf("not joined")
		}
		mode, _ := env.Payload["mode"].(string)
		return s.Room.HandleRematch(s.Seat, mode)
	case transport.IntentDevGrantSpecialty:
		if s.Room == nil {
			return fmt.Errorf("not joined")
		}
		m := s.Room.Match()
		if m == nil {
			return fmt.Errorf("match not started")
		}
		specialty, _ := env.Payload["specialty"].(string)
		return m.GrantSpecialty(s.Seat, specialty)
	default:
		if s.Room == nil {
			return fmt.Errorf("not joined")
		}
		m := s.Room.Match()
		if m == nil {
			return fmt.Errorf("match not started")
		}
		return m.HandleIntent(s.Seat, env)
	}
}

func (s *ConnectionState) handleConvertPoints(env *transport.Envelope) error {
	if s.Economy == nil {
		return fmt.Errorf("convert_points unavailable")
	}
	pointsF, ok := env.Payload["points"].(float64)
	if !ok {
		return fmt.Errorf("points required")
	}
	points := int64(pointsF)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	noin, err := s.Economy.ConvertPoints(ctx, s.AccountID, points)
	if err != nil {
		return err
	}
	return s.send(transport.NewEvent(transport.EventPointsConverted, map[string]any{
		"points_converted": points,
		"noin_granted":     noin,
	}))
}

func (s *ConnectionState) handleReportMedia(env *transport.Envelope) error {
	if s.Audit == nil {
		return fmt.Errorf("report unavailable")
	}
	mediaID, _ := env.Payload["media_id"].(string)
	reason, _ := env.Payload["reason"].(string)
	if mediaID == "" {
		return fmt.Errorf("media_id required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.Audit.LogWithAccount(ctx, audit.EventIntentRejected, uuidToAccount(s.AccountID), s.RoomID(), "", map[string]any{
		"intent":   "report_media",
		"media_id": mediaID,
		"reason":   reason,
	})
	return s.sendOK("report_received", map[string]any{"media_id": mediaID})
}

func (s *ConnectionState) sendOK(key string, payload map[string]any) error {
	return s.send(transport.NewEvent("joined_"+key, payload))
}

func (s *ConnectionState) sendError(code, message string) error {
	return s.send(transport.NewErrorEnvelope(code, map[string]any{"message": message}, ""))
}

// sendErrorFor is sendError with the rejected intent's kind attached as
// params["reply_to"], so the client can correlate the rejection back to the
// action that caused it instead of reacting to it as if it were about
// whatever the client is currently doing.
func (s *ConnectionState) sendErrorFor(code, message, intentKind string) error {
	return s.send(transport.NewErrorEnvelope(code, map[string]any{"message": message}, intentKind))
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

func (s *ConnectionState) RoomID() string {
	if s.Room == nil {
		return ""
	}
	return s.Room.ID
}

func uuidToAccount(s string) uuid.UUID {
	if s == "" {
		return uuid.UUID{}
	}
	if id, err := uuid.Parse(s); err == nil {
		return id
	}
	return uuid.UUID{}
}
