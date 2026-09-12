package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/ratelimit"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
)

type TextAuth interface {
	ValidateAccessToken(context.Context, string) (string, error)
}
type TextDeliveryAcker interface {
	AcknowledgeDelivery(context.Context, int64, string, string, time.Time) error
}
type TextHandlerDeps struct {
	Config         *config.Config
	Lobby          *lobby.TextManager
	Auth           TextAuth
	ConnLimiter    *ratelimit.ConnLimiter
	Deliveries     TextDeliveryAcker
	DeliveryWorker string
}
type textFrame[T any] struct {
	Version   int    `json:"v"`
	Type      string `json:"type"`
	RequestID string `json:"request_id,omitempty"`
	Payload   T      `json:"payload"`
}

func (f textFrame[T]) Validate(_ v2.Limits) error {
	if f.Version != 2 {
		return &v2.ContractError{Code: v2.ErrVersion, Field: "v"}
	}
	if f.Type == "" || len(f.Type) > 64 || len(f.RequestID) > 128 {
		return &v2.ContractError{Code: v2.ErrMalformed, Field: "envelope"}
	}
	for _, r := range f.RequestID {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return &v2.ContractError{Code: v2.ErrMalformed, Field: "request_id"}
		}
	}
	return nil
}

type textHelloPayload struct {
	Generation int    `json:"client_generation"`
	Token      string `json:"access_token"`
}
type textCodePayload struct {
	Code string `json:"code"`
}
type textSettingsPayload struct {
	Revision uint64           `json:"settings_revision"`
	Settings v2.LobbySettings `json:"settings"`
}
type textDeliveryAck struct {
	ID int64 `json:"id"`
}
type textErrorPayload struct {
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
}

func textErrorCode(err error) string {
	var contract *v2.ContractError
	if errors.As(err, &contract) {
		return string(contract.Code)
	}
	for _, known := range []error{lobby.ErrTextUnavailable, lobby.ErrTextMembership, lobby.ErrTextReady, lobby.ErrTextHost, lobby.ErrTextRevision, lobby.ErrTextSlowConsumer, store.ErrQuotaExhausted, store.ErrValueConflict, store.ErrValueFence} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return "request.unavailable"
}
func textWireLimits(d TextHandlerDeps) v2.Limits {
	l := d.Lobby.Limits()
	return v2.Limits{MaxFrameBytes: l.MaxFrameBytes, MaxHistoryEvents: l.MaxHistoryEvents, MaxHistoryPageEvents: l.MaxHistoryPageEvents, MaxTextBytes: l.MaxTextBytes, MaxRequestsPerSeat: l.MaxRequestsPerSeat}
}
func textDecode[T any](raw []byte, d TextHandlerDeps) (textFrame[T], error) {
	var f textFrame[T]
	e := v2.Decode(raw, &f, textWireLimits(d))
	return f, e
}
func textOriginAllowed(r *http.Request, allowed []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, e := url.Parse(origin)
	if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	for _, candidate := range allowed {
		if candidate == origin {
			return true
		}
	}
	return u.Host == r.Host
}
func TextAvailabilityHandler(d TextHandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if d.Auth == nil || d.Lobby == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if _, e := d.Auth.ValidateAccessToken(ctx, token); e != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(d.Lobby.Availability(ctx))
	}
}
func TextRealtimeHandler(d TextHandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Config == nil || d.Lobby == nil || d.Auth == nil {
			http.Error(w, "service.unavailable", http.StatusServiceUnavailable)
			return
		}
		if d.ConnLimiter != nil {
			if !d.ConnLimiter.TryAcquire() {
				http.Error(w, "connection.capacity", http.StatusServiceUnavailable)
				return
			}
			defer d.ConnLimiter.Release()
		}
		upgrade := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return textOriginAllowed(r, d.Config.Server.AllowedOrigins) }}
		conn, e := upgrade.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer conn.Close()
		conn.SetReadLimit(int64(d.Lobby.Limits().MaxFrameBytes))
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		reject := func(code string) {
			conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			_ = conn.WriteJSON(lobby.TextEnvelope{Version: 2, Type: "error", Payload: textMarshal(textErrorPayload{Code: code})})
		}
		messageType, raw, e := conn.ReadMessage()
		if e != nil {
			return
		}
		if messageType != websocket.TextMessage {
			reject("protocol.malformed")
			return
		}
		hello, e := textDecode[textHelloPayload](raw, d)
		if e != nil {
			reject(textErrorCode(e))
			return
		}
		if hello.Type != "hello" || hello.Payload.Generation != 2 || hello.Payload.Generation < d.Config.Text.Compatibility.MinClientGeneration {
			reject("protocol.upgrade_required")
			return
		}
		if hello.Payload.Token == "" {
			reject("auth.required")
			return
		}
		authCtx, authCancel := context.WithTimeout(r.Context(), 5*time.Second)
		account, e := d.Auth.ValidateAccessToken(authCtx, hello.Payload.Token)
		authCancel()
		if e != nil {
			reject("auth.required")
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		peer, e := d.Lobby.Open(ctx, account)
		if e != nil {
			reject(textErrorCode(e))
			return
		}
		writerDone := make(chan struct{})
		go textWriteLoop(ctx, conn, d, peer, hello.Payload.Token, writerDone, cancel)
		defer func() {
			cancel()
			conn.Close()
			<-writerDone
			cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			_ = d.Lobby.Disconnect(cleanup, peer)
		}()
		if e = d.Lobby.Send(peer, "hello", "", map[string]any{"prototype": d.Lobby.Prototype(), "client_generation": 2, "account_id": account, "limits": d.Lobby.Limits()}); e != nil {
			return
		}
		if e = d.Lobby.Send(peer, "availability", "", d.Lobby.Availability(ctx)); e != nil {
			return
		}
		wait := time.Duration(d.Config.WebSocket.PongWaitS) * time.Second
		if wait <= 0 {
			wait = 30 * time.Second
		}
		conn.SetReadDeadline(time.Now().Add(wait))
		conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(wait)) })
		receipts := map[string][32]byte{}
		for {
			messageType, raw, e = conn.ReadMessage()
			if e != nil {
				return
			}
			if messageType != websocket.TextMessage {
				_ = d.Lobby.Send(peer, "error", "", textErrorPayload{Code: "protocol.malformed"})
				continue
			}
			allowed := d.Lobby.AllowRequest(peer)
			frame, e := textDecode[map[string]json.RawMessage](raw, d)
			if e != nil {
				_ = d.Lobby.Send(peer, "error", "", textErrorPayload{Code: textErrorCode(e)})
				continue
			}
			work, done := context.WithTimeout(ctx, 5*time.Second)
			current, authErr := d.Auth.ValidateAccessToken(work, hello.Payload.Token)
			if authErr != nil || current != account {
				done()
				_ = d.Lobby.Send(peer, "error", "", textErrorPayload{Code: "auth.required"})
				return
			}
			if frame.Type == "action" {
				outerID := frame.RequestID
				var nestedID string
				if json.Unmarshal(frame.Payload["request_id"], &nestedID) == nil {
					frame.RequestID = nestedID
				}
				action, decodeErr := textDecode[v2.ActionRequest](raw, d)
				e = decodeErr
				if e == nil {
					frame.RequestID = action.Payload.RequestID
					if outerID != "" && outerID != frame.RequestID {
						e = &v2.ContractError{Code: v2.ErrMalformed, Field: "request_id"}
					} else {
						e = action.Payload.Validate(textWireLimits(d))
					}
					if e == nil && !allowed {
						e = &v2.ContractError{Code: v2.ErrRateLimited, Field: "rate"}
					}
					if e == nil {
						e = d.Lobby.Action(work, peer, action.Payload)
					}
				}
			} else if frame.RequestID == "" {
				e = &v2.ContractError{Code: v2.ErrMalformed, Field: "request_id"}
			} else if !allowed {
				e = &v2.ContractError{Code: v2.ErrRateLimited, Field: "rate"}
			} else {
				canonical, _ := json.Marshal(frame)
				hash := sha256.Sum256(canonical)
				if old, ok := receipts[frame.RequestID]; ok {
					if old != hash {
						e = &v2.ContractError{Code: v2.ErrRequestConflict, Field: "request_id"}
					} else {
						e = d.Lobby.Send(peer, "control_ack", frame.RequestID, map[string]any{"request_id": frame.RequestID, "duplicate": true})
					}
				} else if len(receipts) >= d.Lobby.Limits().MaxRequestsPerSeat {
					e = &v2.ContractError{Code: v2.ErrRequestLimit, Field: "controls"}
				} else {
					e = textHandleControl(work, d, peer, raw, frame.Type)
					if e == nil {
						receipts[frame.RequestID] = hash
						e = d.Lobby.Send(peer, "control_ack", frame.RequestID, map[string]any{"request_id": frame.RequestID, "duplicate": false})
					}
				}
			}
			done()
			if e != nil {
				var sendErr error
				if frame.Type == "action" {
					sendErr = d.Lobby.RejectAction(peer, frame.RequestID, textActionErrorCode(e))
				} else {
					sendErr = d.Lobby.Send(peer, "error", frame.RequestID, textErrorPayload{Code: textErrorCode(e), RequestID: frame.RequestID})
				}
				if sendErr != nil {
					return
				}
			}
		}
	}
}

func textActionErrorCode(err error) v2.ErrorCode {
	var contract *v2.ContractError
	if errors.As(err, &contract) {
		return contract.Code
	}
	if errors.Is(err, lobby.ErrTextMembership) || strings.HasPrefix(err.Error(), "terms.") || strings.HasPrefix(err.Error(), "chat.") {
		return v2.ErrUnauthorized
	}
	return v2.ErrPersistencePending
}
func textMarshal(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func textWriteLoop(ctx context.Context, conn *websocket.Conn, d TextHandlerDeps, p *lobby.TextPeer, token string, done chan<- struct{}, cancel context.CancelFunc) {
	defer close(done)
	defer cancel()
	defer conn.Close()
	period := time.Duration(d.Config.WebSocket.PingPeriodS) * time.Second
	if period <= 0 {
		period = 10 * time.Second
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	authPeriod := min(period, 5*time.Second)
	authTicker := time.NewTicker(authPeriod)
	defer authTicker.Stop()
	wait := time.Duration(d.Config.WebSocket.WriteWaitS) * time.Second
	if wait <= 0 {
		wait = 5 * time.Second
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.Done:
			return
		case <-authTicker.C:
			check, finish := context.WithTimeout(ctx, 5*time.Second)
			account, err := d.Auth.ValidateAccessToken(check, token)
			finish()
			if err != nil || account != p.AccountID {
				conn.SetWriteDeadline(time.Now().Add(wait))
				_ = conn.WriteJSON(lobby.TextEnvelope{Version: 2, Type: "error", Payload: textMarshal(textErrorPayload{Code: "auth.required"})})
				return
			}
		case env := <-p.Frames:
			select {
			case <-p.Done:
				return
			case <-ctx.Done():
				return
			default:
			}
			conn.SetWriteDeadline(time.Now().Add(wait))
			if e := conn.WriteJSON(env); e != nil {
				return
			}
		case <-ticker.C:
			if e := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(wait)); e != nil {
				return
			}
		}
	}
}
func textHandleControl(ctx context.Context, d TextHandlerDeps, p *lobby.TextPeer, raw []byte, kind string) error {
	switch kind {
	case "queue_join", "room_create":
		f, e := textDecode[v2.LobbySettings](raw, d)
		if e != nil {
			return e
		}
		if kind == "queue_join" {
			return d.Lobby.QueueJoin(ctx, p, f.Payload)
		}
		_, e = d.Lobby.Create(ctx, p, f.Payload)
		return e
	case "room_join":
		f, e := textDecode[textCodePayload](raw, d)
		if e != nil {
			return e
		}
		if len(f.Payload.Code) != 6 {
			return lobby.ErrTextMembership
		}
		return d.Lobby.Join(ctx, p, f.Payload.Code)
	case "room_settings":
		f, e := textDecode[textSettingsPayload](raw, d)
		if e != nil {
			return e
		}
		return d.Lobby.Settings(ctx, p, f.Payload.Revision, f.Payload.Settings)
	case "room_ready":
		f, e := textDecode[v2.ReadyAcknowledgement](raw, d)
		if e != nil {
			return e
		}
		return d.Lobby.Ready(ctx, p, f.Payload)
	case "action":
		f, e := textDecode[v2.ActionRequest](raw, d)
		if e != nil {
			return e
		}
		if e = f.Payload.Validate(textWireLimits(d)); e != nil {
			return e
		}
		return d.Lobby.Action(ctx, p, f.Payload)
	case "settlement_ack":
		f, e := textDecode[textDeliveryAck](raw, d)
		if e != nil {
			return e
		}
		if d.Deliveries == nil {
			return lobby.ErrTextUnavailable
		}
		return d.Deliveries.AcknowledgeDelivery(ctx, f.Payload.ID, d.DeliveryWorker, p.AccountID, time.Now())
	case "queue_leave", "queue_keep_waiting", "room_leave", "room_start", "rematch", "resync", "availability":
		if _, e := textDecode[struct{}](raw, d); e != nil {
			return e
		}
		switch kind {
		case "queue_leave":
			return d.Lobby.QueueLeave(ctx, p)
		case "queue_keep_waiting":
			return d.Lobby.KeepWaiting(ctx, p)
		case "room_leave":
			return d.Lobby.Leave(ctx, p)
		case "room_start":
			return d.Lobby.Start(ctx, p)
		case "rematch":
			return d.Lobby.Rematch(ctx, p)
		case "resync":
			return d.Lobby.Resync(ctx, p)
		case "availability":
			return d.Lobby.Send(p, "availability", "", d.Lobby.Availability(ctx))
		}
	}
	return &v2.ContractError{Code: v2.ErrMalformed, Field: "type"}
}
