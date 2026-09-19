package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
	"gopkg.in/yaml.v3"
)

type textAuthStub struct{}

type textCountingAuth struct{ calls atomic.Int32 }

func (a *textCountingAuth) ValidateAccessToken(ctx context.Context, token string) (string, error) {
	a.calls.Add(1)
	return (textAuthStub{}).ValidateAccessToken(ctx, token)
}

func TestTextRoutesRejectLegacyGameplayBeforeAuthentication(t *testing.T) {
	auth := &textCountingAuth{}
	srv, values, manager := textHTTPAuthFixture(t, auth)
	for _, path := range []string{"/ws", "/rooms/create", "/join/ABCDEF"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(`{"size":4}`))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+uuid.NewString())
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				Code string `json:"code"`
			}
			err = json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusUpgradeRequired || body.Code != "protocol.upgrade_required" || resp.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("legacy %s %s = %d %+v %v", method, path, resp.StatusCode, body, err)
			}
		}
	}
	if auth.calls.Load() != 0 || values.reserved.Load() != 0 || manager.ActiveMatches() != 0 {
		t.Fatal("legacy route crossed auth/admission boundary")
	}
}

func (textAuthStub) ValidateAccessToken(_ context.Context, token string) (string, error) {
	if _, e := uuid.Parse(token); e != nil {
		return "", errors.New("unauthorized")
	}
	return token, nil
}

type textValueStub struct{ reserved atomic.Int32 }

func TestTextCooldownControlErrorKeepsStableCode(t *testing.T) {
	if got := textErrorCode(fmt.Errorf("queue reservation: %w", store.ErrTextCooldown)); got != "admission.cooldown" {
		t.Fatal("cooldown lost stable refusal code", got)
	}
}

func (v *textValueStub) Reserve(context.Context, store.TextReservation) error {
	v.reserved.Add(1)
	return nil
}
func (*textValueStub) CancelReservation(context.Context, string, string) error         { return nil }
func (*textValueStub) Prepare(context.Context, store.TextMatchRecord, time.Time) error { return nil }
func (*textValueStub) Start(context.Context, string, string, int64, time.Time) error   { return nil }
func (*textValueStub) CancelPrepared(context.Context, string, string, int64, time.Time) error {
	return nil
}
func (*textValueStub) Award(context.Context, store.TextAward) (int, error)               { return 0, nil }
func (*textValueStub) Abandon(context.Context, store.TextAbandon) error                  { return nil }
func (*textValueStub) Finish(context.Context, store.TextOutcome) error                   { return nil }
func (*textValueStub) SettlePending(context.Context, string) error                       { return nil }
func (*textValueStub) Interrupt(context.Context, string, string, int64, time.Time) error { return nil }
func textHTTPFixture(t *testing.T) (*httptest.Server, *textValueStub, *lobby.TextManager) {
	return textHTTPAuthFixture(t, textAuthStub{})
}
func textHTTPAuthFixture(t *testing.T, auth TextAuth) (*httptest.Server, *textValueStub, *lobby.TextManager) {
	return textHTTPCustomFixture(t, auth, nil)
}
func textHTTPCustomFixture(t *testing.T, auth TextAuth, configure func(*config.Config, *lobby.TextDeps)) (*httptest.Server, *textValueStub, *lobby.TextManager) {
	t.Helper()
	raw, e := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if e != nil {
		t.Fatal(e)
	}
	cfg := &config.Config{}
	if e = yaml.Unmarshal(raw, &cfg.Tuning); e != nil {
		t.Fatal(e)
	}
	cfg.App.Env = "test"
	cfg.Tuning.Timers.RoundStartCountdown = 1
	cfg.WebSocket = config.WebSocketConfig{MaxMessageBytes: 65536, PongWaitS: 10, PingPeriodS: 2, WriteWaitS: 2}
	cfg.Text = &config.TextConfig{Version: 1, RulesVersion: "text-v1", Compatibility: config.TextCompatibility{ProtocolVersion: 2, MinClientGeneration: 2}, Modes: map[gamecontract.ModeID]config.TextModeConfig{}}
	pack, e := media.LoadTextPack("../../pkg/media/testdata/text-en", media.TextLimits{MaxTextBytes: cfg.Tuning.Contract.MaxTextBytes, MaxRecords: cfg.Tuning.TextCatalog.MaxRecords, MaxFileBytes: cfg.Tuning.TextCatalog.MaxFileBytes, MaxBundleBytes: cfg.Tuning.TextCatalog.MaxBundleBytes})
	if e != nil {
		t.Fatal(e)
	}
	values := &textValueStub{}
	managerDeps := lobby.TextDeps{Config: cfg, Values: values, Prototype: pack}
	if configure != nil {
		configure(cfg, &managerDeps)
	}
	manager, e := lobby.NewTextManager(managerDeps)
	if e != nil {
		t.Fatal(e)
	}
	deps := TextHandlerDeps{Config: cfg, Lobby: manager, Auth: auth}
	mux := http.NewServeMux()
	RegisterTextRealtimeRoutes(mux, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if e := manager.Close(ctx); e != nil {
			t.Error(e)
		}
		srv.Close()
	})
	return srv, values, manager
}
func textDial(t *testing.T, s *httptest.Server) *websocket.Conn {
	t.Helper()
	c, _, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http")+"/ws/v2", nil)
	if e != nil {
		t.Fatal(e)
	}
	textTraces.Store(c, &textTrace{})
	t.Cleanup(func() { c.Close(); textTraces.Delete(c) })
	return c
}
func textRead(t *testing.T, c *websocket.Conn) lobby.TextEnvelope {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var e lobby.TextEnvelope
	if err := c.ReadJSON(&e); err != nil {
		t.Fatal(err)
	}
	textCapture(c, "server", e)
	return e
}
func textHello(t *testing.T, c *websocket.Conn, token string) {
	t.Helper()
	if e := textWriteJSON(c, map[string]any{"v": 2, "type": "hello", "payload": map[string]any{"client_generation": 2, "access_token": token}}); e != nil {
		t.Fatal(e)
	}
	if e := textRead(t, c); e.Type != "hello" {
		t.Fatalf("expected hello: %s", e.Payload)
	}
	if e := textRead(t, c); e.Type != "availability" {
		t.Fatalf("expected discovery: %s", e.Payload)
	}
}
func TestTextWebsocketRejectsBeforeAdmission(t *testing.T) {
	srv, values, _ := textHTTPFixture(t)
	for _, raw := range []string{`{"v":1,"type":"hello","payload":{"client_generation":1,"access_token":"x"}}`, `{"v":2,"type":"hello","payload":{"client_generation":2,"access_token":"x"}}`, `{"v":2,"v":2,"type":"hello","payload":{"client_generation":2,"access_token":"x"}}`, `{"v":2,"type":"room_create","payload":{"dev":true}}`} {
		c := textDial(t, srv)
		if e := c.WriteMessage(websocket.TextMessage, []byte(raw)); e != nil {
			t.Fatal(e)
		}
		reply := textRead(t, c)
		if reply.Type != "error" {
			t.Fatal("invalid handshake accepted")
		}
		c.Close()
	}
	if values.reserved.Load() != 0 {
		t.Fatal("rejection reserved admission")
	}
	response, e := http.Get(srv.URL + "/api/text/availability")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("availability requires auth")
	}
}
func TestTextWebsocketControlsAreStrictAndDuplicateSafe(t *testing.T) {
	srv, values, _ := textHTTPFixture(t)
	c := textDial(t, srv)
	textHello(t, c, uuid.NewString())
	request := map[string]any{"v": 2, "type": "room_create", "request_id": "create-1", "payload": map[string]any{"mode_id": "missed_the_briefing", "size": 4, "content_language": "en", "pack_release_id": "synthetic-text-en", "rules_version": "text-v1"}}
	if e := textWriteJSON(c, request); e != nil {
		t.Fatal(e)
	}
	if r := textRead(t, c); r.Type != "lobby" {
		t.Fatalf("create: %s", r.Payload)
	}
	if r := textRead(t, c); r.Type != "dev_role" || string(r.Payload) != `{"role":"random"}` {
		t.Fatalf("initial dev role: %s", r.Payload)
	}
	if r := textRead(t, c); r.Type != "control_ack" {
		t.Fatalf("create ack: %s", r.Payload)
	}
	if e := textWriteJSON(c, request); e != nil {
		t.Fatal(e)
	}
	if r := textRead(t, c); r.Type != "control_ack" {
		t.Fatal("duplicate reexecuted")
	}
	if values.reserved.Load() != 1 {
		t.Fatal("duplicate reserved again")
	}
	request["payload"].(map[string]any)["size"] = 6
	textWriteJSON(c, request)
	if r := textRead(t, c); r.Type != "error" || !strings.Contains(string(r.Payload), "request.conflict") {
		t.Fatal("conflicting id accepted")
	}
	raw, _ := json.Marshal(request)
	_ = raw
}

func textControl(t *testing.T, c *websocket.Conn, kind, id string, payload any) []lobby.TextEnvelope {
	t.Helper()
	if e := textWriteJSON(c, map[string]any{"v": 2, "type": kind, "request_id": id, "payload": payload}); e != nil {
		t.Fatal(e)
	}
	out := []lobby.TextEnvelope{}
	for i := 0; i < 100; i++ {
		frame := textRead(t, c)
		out = append(out, frame)
		if frame.Type == "error" {
			t.Fatalf("%s failed: %s", kind, frame.Payload)
		}
		if frame.Type == "control_ack" && frame.RequestID == id {
			return out
		}
	}
	t.Fatal("missing control ack")
	return nil
}
func textLastLobby(t *testing.T, frames []lobby.TextEnvelope) lobby.TextLobbyView {
	t.Helper()
	var out lobby.TextLobbyView
	found := false
	for _, f := range frames {
		if f.Type == "lobby" {
			if e := json.Unmarshal(f.Payload, &out); e != nil {
				t.Fatal(e)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no lobby")
	}
	return out
}
func textNextSnapshot(t *testing.T, c *websocket.Conn) v2.Snapshot {
	t.Helper()
	for i := 0; i < 100; i++ {
		f := textRead(t, c)
		if f.Type == "error" {
			t.Fatalf("snapshot rejected: %s", f.Payload)
		}
		if f.Type == "snapshot" {
			var s v2.Snapshot
			if e := json.Unmarshal(f.Payload, &s); e != nil {
				t.Fatal(e)
			}
			return s
		}
	}
	t.Fatal("no snapshot")
	return v2.Snapshot{}
}
func TestTextWebsocketAllModesFourSixPrivateDrawAndResync(t *testing.T) {
	traces := []textSessionTrace{}
	defer func() { writeTextSessionTraces(t, traces) }()
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				srv, _, manager := textHTTPFixture(t)
				peers := make([]*websocket.Conn, size)
				for i := range peers {
					peers[i] = textDial(t, srv)
					textHello(t, peers[i], uuid.NewString())
				}
				settings := v2.LobbySettings{ModeID: mode, Size: size, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
				created := textLastLobby(t, textControl(t, peers[0], "room_create", "create", settings))
				for _, p := range peers[1:] {
					textControl(t, p, "room_join", "join", map[string]any{"code": created.Code})
				}
				for i, p := range peers {
					view := textLastLobby(t, textControl(t, p, "resync", fmt.Sprintf("sync-%d", i), struct{}{}))
					textControl(t, p, "room_ready", fmt.Sprintf("ready-%d", i), v2.ReadyAcknowledgement{SettingsRevision: view.Lobby.SettingsRevision, MembershipRevision: view.Lobby.MembershipRevision})
				}
				frames := textControl(t, peers[0], "room_start", "start", struct{}{})
				snapshots := make([]v2.Snapshot, size)
				for _, f := range frames {
					if f.Type == "snapshot" {
						if e := json.Unmarshal(f.Payload, &snapshots[0]); e != nil {
							t.Fatal(e)
						}
					}
				}
				for i := 1; i < size; i++ {
					snapshots[i] = textNextSnapshot(t, peers[i])
				}
				allCopies := map[v2.CopyID]int{}
				for seat, s := range snapshots {
					if s.Cursor.RecipientSeq != 1 || s.Private.Seat != seat || s.Contract.ModeID != mode || s.Contract.OriginalSize != size || s.Contract.Eligibility.Rewards {
						t.Fatal("first stream/seat/contract mismatch")
					}
					if (s.Private.Role == "donower") != (s.Private.Nown == nil) {
						t.Fatal("role-scoped prompt mismatch")
					}
					for _, card := range s.Private.Hand {
						if _, seen := allCopies[card.CopyID]; seen {
							t.Fatal("private copy shared across seats")
						}
						allCopies[card.CopyID] = seat
					}
				}
				time.Sleep(1050 * time.Millisecond)
				if e := manager.Tick(context.Background()); e != nil {
					t.Fatal(e)
				}
				for i, p := range peers {
					snapshots[i] = textNextSnapshot(t, p)
				}
				current := *snapshots[0].CurrentSeat
				before := snapshots[current]
				count := 1
				request := v2.ActionRequest{Version: 2, RequestID: "draw-one", MatchID: before.Contract.MatchID, ModeID: mode, Round: before.Round, Turn: before.Turn, Phase: before.Phase, PhaseID: before.PhaseID, ExpectedBoardRevision: before.Board.Revision, Action: v2.Action{Kind: v2.ActionDraw, Count: &count}}
				if e := textWriteJSON(peers[current], map[string]any{"v": 2, "type": "action", "payload": request}); e != nil {
					t.Fatal(e)
				}
				for i, p := range peers {
					snapshots[i] = textNextSnapshot(t, p)
				}
				ack := textRead(t, peers[current])
				if ack.Type != "action_ack" || ack.RequestID != request.RequestID {
					t.Fatal("ack before complete snapshot or wrong identity")
				}
				after := snapshots[current]
				if len(after.Private.Hand) != len(before.Private.Hand)+1 {
					t.Fatal("draw missing")
				}
				drawn := after.Private.Hand[len(after.Private.Hand)-1]
				for seat, s := range snapshots {
					if seat == current {
						continue
					}
					raw, _ := json.Marshal(s)
					if strings.Contains(string(raw), string(drawn.CopyID)) {
						t.Fatal("draw copy leaked to other recipient")
					}
				}
				replay := textControl(t, peers[current], "resync", "resync-after-draw", struct{}{})
				var refreshed v2.Snapshot
				for _, f := range replay {
					if f.Type == "snapshot" {
						json.Unmarshal(f.Payload, &refreshed)
					}
				}
				if refreshed.Cursor.RecipientSeq != 1 || refreshed.Cursor.StreamEpoch == after.Cursor.StreamEpoch || len(refreshed.Private.Hand) != len(after.Private.Hand) || refreshed.DeadlineMS != after.DeadlineMS {
					t.Fatal("resync changed hand/clock or failed epoch fence")
				}
				if e := textWriteJSON(peers[current], map[string]any{"v": 2, "type": "action", "payload": request}); e != nil {
					t.Fatal(e)
				}
				duplicateState := textNextSnapshot(t, peers[current])
				duplicateAck := textRead(t, peers[current])
				if len(duplicateState.Private.Hand) != len(after.Private.Hand) || duplicateAck.Type != "action_ack" || !strings.Contains(string(duplicateAck.Payload), "true") {
					t.Fatal("duplicate draw mutated or missing ack")
				}
				rejected := request
				rejected.RequestID = "stale-draw"
				rejected.ExpectedBoardRevision = duplicateState.Board.Revision + 1
				if e := textWriteJSON(peers[current], map[string]any{"v": 2, "type": "action", "payload": rejected}); e != nil {
					t.Fatal(e)
				}
				failure := textRead(t, peers[current])
				var typedError v2.ErrorEvent
				if failure.Type != "error" || json.Unmarshal(failure.Payload, &typedError) != nil || typedError.Validate(v2.Limits{}) != nil {
					t.Fatal("match error is not a typed cursor event")
				}
				if typedError.Code != v2.ErrStaleRevision || typedError.RequestID != rejected.RequestID || typedError.Cursor.RecipientSeq != duplicateState.Cursor.RecipientSeq+1 || typedError.Cursor.StreamEpoch != duplicateState.Cursor.StreamEpoch || typedError.Cursor.EvidenceSeq != duplicateState.Cursor.EvidenceSeq || typedError.CurrentBoardRevision == nil || *typedError.CurrentBoardRevision != duplicateState.Board.Revision {
					t.Fatal("match rejection changed state or lost stream ordering")
				}
				if e := textWriteJSON(peers[current], map[string]any{"v": 2, "type": "action", "payload": request}); e != nil {
					t.Fatal(e)
				}
				afterError := textNextSnapshot(t, peers[current])
				if afterError.Cursor.RecipientSeq != typedError.Cursor.RecipientSeq+1 || afterError.Board.Revision != duplicateState.Board.Revision || len(afterError.Private.Hand) != len(duplicateState.Private.Hand) {
					t.Fatal("action after error lost sequence or mutated")
				}
				if ack := textRead(t, peers[current]); ack.Type != "action_ack" {
					t.Fatal("duplicate acknowledgement missing after error")
				}
				traces = append(traces, textSessionTrace{string(mode), size, textCaptured(peers[current])})
			})
		}
	}
}

func TestTextActionErrorCorrelatesNestedRequestIdentity(t *testing.T) {
	srv, _, _ := textHTTPFixture(t)
	c := textDial(t, srv)
	textHello(t, c, uuid.NewString())
	request := v2.ActionRequest{Version: 2, RequestID: "rejected-action-1", MatchID: uuid.NewString(), ModeID: gamecontract.ModeMissedTheBriefing, Round: 1, Turn: 1, Phase: v2.PhasePlay, PhaseID: "phase-one", ExpectedBoardRevision: 0, Action: v2.Action{Kind: v2.ActionReady}}
	if e := textWriteJSON(c, map[string]any{"v": 2, "type": "action", "payload": request}); e != nil {
		t.Fatal(e)
	}
	reply := textRead(t, c)
	if reply.Type != "error" || reply.RequestID != request.RequestID || !strings.Contains(string(reply.Payload), request.RequestID) {
		t.Fatal("action error lost nested request identity")
	}
}

type textRevocableAuth struct{ revoked atomic.Bool }

type textOpenRevokedAuth struct{ calls atomic.Int32 }

func (a *textOpenRevokedAuth) ValidateAccessToken(ctx context.Context, token string) (string, error) {
	if a.calls.Add(1) > 1 {
		return "", errors.New("revoked during Open")
	}
	return (textAuthStub{}).ValidateAccessToken(ctx, token)
}

func TestTextRechecksTokenAfterRegisteringConnection(t *testing.T) {
	auth := &textOpenRevokedAuth{}
	srv, values, _ := textHTTPAuthFixture(t, auth)
	c := textDial(t, srv)
	if err := textWriteJSON(c, map[string]any{"v": 2, "type": "hello", "payload": map[string]any{"client_generation": 2, "access_token": uuid.NewString()}}); err != nil {
		t.Fatal(err)
	}
	frame := textRead(t, c)
	if frame.Type != "error" || !strings.Contains(string(frame.Payload), "auth.required") {
		t.Fatal("token revoked before Open admitted a connection", frame.Type)
	}
	if values.reserved.Load() != 0 {
		t.Fatal("post-open refusal reserved value")
	}
	c.SetReadDeadline(time.Now().Add(time.Second))
	if err := c.ReadJSON(&frame); err == nil {
		t.Fatal("revoked connection received later discovery or game data")
	}
}

func (a *textRevocableAuth) ValidateAccessToken(ctx context.Context, token string) (string, error) {
	if a.revoked.Load() {
		return "", errors.New("revoked")
	}
	return (textAuthStub{}).ValidateAccessToken(ctx, token)
}
func TestTextIdleConnectionRechecksAuthorization(t *testing.T) {
	auth := &textRevocableAuth{}
	srv, _, _ := textHTTPAuthFixture(t, auth)
	c := textDial(t, srv)
	textHello(t, c, uuid.NewString())
	auth.revoked.Store(true)
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var frame lobby.TextEnvelope
	if e := c.ReadJSON(&frame); e != nil {
		t.Fatalf("idle revocation never delivered: %v", e)
	}
	if frame.Type != "error" || !strings.Contains(string(frame.Payload), "auth.required") {
		t.Fatal("idle authorization retained")
	}
}
func TestTextWebsocketPagedHistoryPrecedesAcknowledgement(t *testing.T) {
	srv, _, manager := textHTTPCustomFixture(t, textAuthStub{}, func(cfg *config.Config, deps *lobby.TextDeps) {
		cfg.WebSocket.MaxMessageBytes = 4096
		cfg.Tuning.Contract.MaxHistoryPageEvents = 3
		deps.ModerateChat = func(_ context.Context, _ string, a v2.Action) (v2.Action, error) { return a, nil }
	})
	peers := make([]*websocket.Conn, 4)
	for i := range peers {
		peers[i] = textDial(t, srv)
		textHello(t, peers[i], uuid.NewString())
	}
	settings := v2.LobbySettings{ModeID: gamecontract.ModeMissedTheBriefing, Size: 4, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
	view := textLastLobby(t, textControl(t, peers[0], "room_create", "create", settings))
	for _, p := range peers[1:] {
		textControl(t, p, "room_join", "join", map[string]any{"code": view.Code})
	}
	for i, p := range peers {
		l := textLastLobby(t, textControl(t, p, "resync", fmt.Sprintf("sync-%d", i), struct{}{}))
		textControl(t, p, "room_ready", fmt.Sprintf("ready-%d", i), v2.ReadyAcknowledgement{SettingsRevision: l.Lobby.SettingsRevision, MembershipRevision: l.Lobby.MembershipRevision})
	}
	textControl(t, peers[0], "room_start", "start", struct{}{})
	for _, p := range peers[1:] {
		textNextSnapshot(t, p)
	}
	time.Sleep(1050 * time.Millisecond)
	if e := manager.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	snapshot := textNextSnapshot(t, peers[0])
	for _, p := range peers[1:] {
		textNextSnapshot(t, p)
	}
	paged := false
	for i := 0; i < 8; i++ {
		request := v2.ActionRequest{Version: 2, RequestID: fmt.Sprintf("chat-%d", i), MatchID: snapshot.Contract.MatchID, ModeID: settings.ModeID, Round: snapshot.Round, Turn: snapshot.Turn, Phase: snapshot.Phase, PhaseID: snapshot.PhaseID, ExpectedBoardRevision: snapshot.Board.Revision, Action: v2.Action{Kind: v2.ActionChat, Text: strings.Repeat("synthetic echo ", 28), UILocale: "en"}}
		if e := textWriteJSON(peers[0], map[string]any{"v": 2, "type": "action", "payload": request}); e != nil {
			t.Fatal(e)
		}
		snapshot = textNextSnapshot(t, peers[0])
		pages := []v2.HistoryPage{}
		for {
			frame := textRead(t, peers[0])
			raw, _ := json.Marshal(frame)
			if len(raw) > 4096 {
				t.Fatal("envelope exceeded advertised frame limit")
			}
			if frame.Type == "history_page" {
				var page v2.HistoryPage
				if e := json.Unmarshal(frame.Payload, &page); e != nil {
					t.Fatal(e)
				}
				pages = append(pages, page)
				continue
			}
			if frame.Type != "action_ack" || frame.RequestID != request.RequestID {
				t.Fatalf("unexpected frame before ack: %s", frame.Type)
			}
			break
		}
		if snapshot.HistoryPages != nil {
			paged = true
			l := manager.Limits()
			history, e := v2.AssembleHistory(*snapshot.HistoryPages, pages, snapshot.Contract.MatchID, snapshot.SnapshotID, snapshot.Cursor.StreamEpoch, v2.Limits{MaxFrameBytes: l.MaxFrameBytes, MaxHistoryEvents: l.MaxHistoryEvents, MaxHistoryPageEvents: l.MaxHistoryPageEvents, MaxTextBytes: l.MaxTextBytes, MaxRequestsPerSeat: l.MaxRequestsPerSeat})
			if e != nil || len(history) != i+1 {
				t.Fatalf("incomplete history before ack: %d %v", len(history), e)
			}
		}
	}
	if !paged {
		t.Fatal("fixture failed to exercise pages")
	}
	writeTextSessionTraces(t, []textSessionTrace{{string(settings.ModeID), 4, textCaptured(peers[0])}})
}

func TestTextPrototypeIsServerMetadataBeforeAnyAdmission(t *testing.T) {
	for _, prototype := range []bool{false, true} {
		t.Run(fmt.Sprint(prototype), func(t *testing.T) {
			srv, values, _ := textHTTPCustomFixture(t, textAuthStub{}, func(_ *config.Config, d *lobby.TextDeps) {
				if !prototype {
					d.Prototype = nil
				}
			})
			c := textDial(t, srv)
			if e := textWriteJSON(c, map[string]any{"v": 2, "type": "hello", "payload": map[string]any{"client_generation": 2, "access_token": uuid.NewString()}}); e != nil {
				t.Fatal(e)
			}
			for _, kind := range []string{"hello", "availability"} {
				f := textRead(t, c)
				var payload struct {
					Prototype *bool `json:"prototype"`
				}
				if f.Type != kind || json.Unmarshal(f.Payload, &payload) != nil || payload.Prototype == nil || *payload.Prototype != prototype {
					t.Fatal("server prototype metadata missing or client-inferred", kind)
				}
			}
			if values.reserved.Load() != 0 {
				t.Fatal("metadata discovery consumed admission")
			}
			forged := textDial(t, srv)
			if e := textWriteJSON(forged, map[string]any{"v": 2, "type": "hello", "payload": map[string]any{"client_generation": 2, "access_token": uuid.NewString(), "prototype": true}}); e != nil {
				t.Fatal(e)
			}
			if f := textRead(t, forged); f.Type != "error" {
				t.Fatal("client injected prototype override")
			}
			if values.reserved.Load() != 0 {
				t.Fatal("forged prototype consumed admission")
			}
		})
	}
}

// Real websocket projections exercise the production client through role reveal,
// recipient chat redaction/resync and the terminal turn reset. Auth/value hooks
// remain synthetic; the separate gamebot suite owns real account/Postgres proof.
func TestTextWebsocketTerminalRevealAndChatProjectionTrace(t *testing.T) {
	textTerminalTrace(t, gamecontract.ModeMissedTheBriefing, 4)
}

func TestTextWebsocketSixPlayerTerminalTrace(t *testing.T) {
	textTerminalTrace(t, gamecontract.ModeSecretScale, 6)
}

func textTerminalTrace(t *testing.T, mode gamecontract.ModeID, size int) {
	var clockMS atomic.Int64
	clockMS.Store(time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC).UnixMilli())
	var blocked atomic.Bool
	srv, _, manager := textHTTPCustomFixture(t, textAuthStub{}, func(_ *config.Config, deps *lobby.TextDeps) {
		deps.Now = func() time.Time { return time.UnixMilli(clockMS.Load()) }
		deps.ModerateChat = func(_ context.Context, _ string, a v2.Action) (v2.Action, error) { return a, nil }
		deps.HideChat = func(context.Context, string, string) (bool, error) { return blocked.Load(), nil }
	})
	peers := make([]*websocket.Conn, size)
	for i := range peers {
		peers[i] = textDial(t, srv)
		textHello(t, peers[i], uuid.NewString())
	}
	settings := v2.LobbySettings{ModeID: mode, Size: size, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
	view := textLastLobby(t, textControl(t, peers[0], "room_create", "create", settings))
	for _, p := range peers[1:] {
		textControl(t, p, "room_join", "join", map[string]any{"code": view.Code})
	}
	for i, p := range peers {
		l := textLastLobby(t, textControl(t, p, "resync", fmt.Sprintf("sync-%d", i), struct{}{}))
		textControl(t, p, "room_ready", fmt.Sprintf("ready-%d", i), v2.ReadyAcknowledgement{SettingsRevision: l.Lobby.SettingsRevision, MembershipRevision: l.Lobby.MembershipRevision})
	}
	frames := textControl(t, peers[0], "room_start", "start", struct{}{})
	snapshots := make([]v2.Snapshot, size)
	for _, f := range frames {
		if f.Type == "snapshot" {
			if err := json.Unmarshal(f.Payload, &snapshots[0]); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := 1; i < size; i++ {
		snapshots[i] = textNextSnapshot(t, peers[i])
	}
	readAll := func() {
		for i, p := range peers {
			snapshots[i] = textNextSnapshot(t, p)
		}
	}
	action := func(seat int, id string, a v2.Action) {
		s := snapshots[seat]
		r := v2.ActionRequest{Version: 2, RequestID: id, MatchID: s.Contract.MatchID, ModeID: s.Contract.ModeID, Round: s.Round, Turn: s.Turn, Phase: s.Phase, PhaseID: s.PhaseID, ExpectedBoardRevision: s.Board.Revision, Action: a}
		if err := textWriteJSON(peers[seat], map[string]any{"v": 2, "type": "action", "payload": r}); err != nil {
			t.Fatal(err)
		}
		readAll()
		if ack := textRead(t, peers[seat]); ack.Type != "action_ack" || ack.RequestID != id {
			t.Fatal("missing action acknowledgement")
		}
	}
	tick := func(at int64) {
		clockMS.Store(at)
		if err := manager.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		readAll()
	}
	tick(snapshots[0].DeadlineMS)
	action(1, "authored-chat", v2.Action{Kind: v2.ActionChat, Text: "immutable synthetic wording", UILocale: "en"})
	for i, hide := range []bool{true, false} {
		blocked.Store(hide)
		frames := textControl(t, peers[0], "resync", fmt.Sprintf("chat-visibility-%d", i), struct{}{})
		for _, f := range frames {
			if f.Type == "snapshot" {
				snapshots[0] = v2.Snapshot{}
				if err := json.Unmarshal(f.Payload, &snapshots[0]); err != nil {
					t.Fatal(err)
				}
			}
		}
		var chat v2.PublicAction
		for _, event := range snapshots[0].History {
			if event.Kind == "chat" {
				chat = event
				break
			}
		}
		if hide && (chat.Text != "" || chat.PhraseID != "chat.hidden") || !hide && (chat.Text != "immutable synthetic wording" || chat.PhraseID != "") {
			t.Fatalf("invalid recipient chat projection: hide=%v kind=%s hasText=%v phrase=%s", hide, chat.Kind, chat.Text != "", chat.PhraseID)
		}
	}
	resultWindows := 0
	for step := 0; step < 40; step++ {
		s := snapshots[0]
		if s.Phase == v2.PhaseVerdict {
			if s.Turn != 0 || s.Verdict == nil || s.Verdict.Outcome != "completed" || len(s.Scores) != size || resultWindows == 0 {
				t.Fatal("terminal projection incomplete")
			}
			writeTextSessionTraces(t, []textSessionTrace{{string(settings.ModeID), size, textCaptured(peers[0])}})
			return
		}
		if s.Phase == v2.PhaseKnowoff {
			candidates := s.Ballot.Candidates
			target := candidates[len(candidates)-1]
			if size == 6 {
				for _, candidate := range candidates {
					if snapshots[candidate].Private.Role == "donower" {
						target = candidate
						break
					}
				}
			}
			for seat, p := range s.Seats {
				if p.Eliminated || !p.Connected {
					continue
				}
				vote := target
				if seat == target {
					for _, candidate := range candidates {
						if candidate != seat {
							vote = candidate
							break
						}
					}
				}
				action(seat, fmt.Sprintf("vote-%d-%d", s.Round, seat), v2.Action{Kind: v2.ActionVote, TargetSeat: &vote})
			}
		}
		s = snapshots[0]
		if s.Phase == v2.PhaseResult {
			resultWindows++
			if s.ResultRevealAtMS == nil || s.Ballot.Result.RevealedRole != "" {
				t.Fatal("role revealed during falling")
			}
			tick(*s.ResultRevealAtMS)
			if snapshots[0].Ballot.Result.RevealedRole == "" {
				t.Fatal("role absent at poster boundary")
			}
		}
		tick(snapshots[0].DeadlineMS)
	}
	t.Fatal("terminal trace exceeded phase bound")
}

// The private launcher serves ordinary browser clients on six separate origins.
// A REST wildcard does not authorize WebSocket upgrades; exercise the actual
// checked-in overlay against the real handler instead of an origin-free bot.
func TestTextPlaytestBrowserOrigins(t *testing.T) {
	raw, err := os.ReadFile("../../../configs/playtest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var overlay config.Config
	if err := yaml.Unmarshal(raw, &overlay); err != nil {
		t.Fatal(err)
	}
	if overlay.App.Env != "local" || overlay.Text != nil || len(overlay.Server.AllowedOrigins) != 6 {
		t.Fatal("prototype overlay must remain local, explicit-origin, and without production mode activation")
	}
	srv, _, _ := textHTTPCustomFixture(t, textAuthStub{}, func(cfg *config.Config, _ *lobby.TextDeps) {
		cfg.Server.AllowedOrigins = overlay.Server.AllowedOrigins
	})
	for port := 8001; port <= 8006; port++ {
		origin := fmt.Sprintf("http://localhost:%d", port)
		t.Run(origin, func(t *testing.T) {
			connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/v2", http.Header{"Origin": {origin}})
			if err != nil {
				t.Fatalf("browser upgrade refused: %v", err)
			}
			defer connection.Close()
			if response.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("upgrade = %d", response.StatusCode)
			}
			textHello(t, connection, uuid.NewString())
		})
	}
	for _, origin := range []string{"http://localhost:8007", "https://untrusted.invalid"} {
		connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/v2", http.Header{"Origin": {origin}})
		if connection != nil {
			connection.Close()
		}
		if response != nil {
			response.Body.Close()
		}
		if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
			t.Fatal("unlisted browser origin admitted")
		}
	}
}

// make web.run uses port 8000; its origins must pass the actual v2 upgrade
// policy, whose exact matching deliberately does not interpret a REST wildcard.
func TestTextManualDebugBrowserOrigins(t *testing.T) {
	raw, err := os.ReadFile("../../../configs/local.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var local config.Config
	if err := yaml.Unmarshal(raw, &local); err != nil {
		t.Fatal(err)
	}
	srv, _, _ := textHTTPCustomFixture(t, textAuthStub{}, func(cfg *config.Config, _ *lobby.TextDeps) { cfg.Server.AllowedOrigins = local.Server.AllowedOrigins })
	for _, origin := range []string{"http://localhost:8000", "http://127.0.0.1:8000", "http://0.0.0.0:8000", "https://app.knowoff.local"} {
		t.Run(origin, func(t *testing.T) {
			connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/v2", http.Header{"Origin": {origin}})
			if response != nil {
				defer response.Body.Close()
			}
			if err != nil {
				t.Fatalf("manual browser upgrade refused: %v", err)
			}
			defer connection.Close()
			if response.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("upgrade=%d", response.StatusCode)
			}
			textHello(t, connection, uuid.NewString())
		})
	}
	for _, origin := range []string{"http://localhost:8007", "https://untrusted.invalid", "http://localhost:8000.untrusted.invalid"} {
		connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/v2", http.Header{"Origin": {origin}})
		if connection != nil {
			connection.Close()
		}
		if response != nil {
			response.Body.Close()
		}
		if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
			t.Fatalf("unlisted origin admitted: %s", origin)
		}
	}
}

func TestTextWebsocketDevelopmentRoleAndSpecialties(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				srv, _, _ := textHTTPFixture(t)
				peers := make([]*websocket.Conn, size)
				for i := range peers {
					peers[i] = textDial(t, srv)
					textHello(t, peers[i], uuid.NewString())
				}
				settings := v2.LobbySettings{ModeID: mode, Size: size, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
				created := textLastLobby(t, textControl(t, peers[0], "room_create", "create-dev", settings))
				for _, peer := range peers[1:] {
					textControl(t, peer, "room_join", "join-dev", map[string]string{"code": created.Code})
				}
				textControl(t, peers[0], "dev_role", "role-dev", map[string]string{"role": "donower"})
				for i, peer := range peers {
					view := textLastLobby(t, textControl(t, peer, "resync", fmt.Sprintf("sync-dev-%d", i), struct{}{}))
					textControl(t, peer, "room_ready", fmt.Sprintf("ready-dev-%d", i), v2.ReadyAcknowledgement{SettingsRevision: view.Lobby.SettingsRevision, MembershipRevision: view.Lobby.MembershipRevision})
				}
				frames := textControl(t, peers[0], "room_start", "start-dev", struct{}{})
				var before v2.Snapshot
				for _, f := range frames {
					if f.Type == "snapshot" {
						if err := json.Unmarshal(f.Payload, &before); err != nil {
							t.Fatal(err)
						}
					}
				}
				if before.Private.Role != "donower" || before.Contract.Eligibility.Rewards || before.Contract.Eligibility.Leaderboard {
					t.Fatal("role/prototype admission mismatch")
				}
				for i, name := range []string{"pass", "reveal", "one_more_free_card", "shuffle", "revote", ""} {
					frames = textControl(t, peers[0], "dev_specialty", fmt.Sprintf("grant-%d", i), map[string]string{"specialty": name})
					found := false
					for _, f := range frames {
						if f.Type == "snapshot" {
							var snapshot v2.Snapshot
							if err := json.Unmarshal(f.Payload, &snapshot); err != nil {
								t.Fatal(err)
							}
							if snapshot.Private.Specialty != name || snapshot.Private.Seat != 0 || snapshot.Private.Role != "donower" {
								t.Fatal("wrong grant projection")
							}
							found = true
						}
					}
					if !found {
						t.Fatal("grant acknowledgement without authoritative snapshot")
					}
				}
			})
		}
	}
}
