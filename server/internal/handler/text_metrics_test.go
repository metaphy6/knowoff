package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/ratelimit"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/prometheus/client_golang/prometheus"
)

func TestTextWebsocketMetricsFollowActualConnectionLifecycle(t *testing.T) {
	var cfg *config.Config
	_, _, manager := textHTTPCustomFixture(t, textAuthStub{}, func(c *config.Config, _ *lobby.TextDeps) { cfg = c })
	registry := prometheus.NewRegistry()
	connections := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_text_connections"})
	rejections := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_text_rejections_total"})
	registry.MustRegister(connections, rejections)
	limiter := ratelimit.NewConnLimiter(1)
	srv := httptest.NewServer(TextRealtimeHandler(TextHandlerDeps{Config: cfg, Lobby: manager, Auth: textAuthStub{}, ConnLimiter: limiter, Connections: connections, ConnectionRejections: rejections}))
	t.Cleanup(srv.Close)
	waitMetrics := func(wantConnections, wantRejections float64) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			families, err := registry.Gather()
			if err != nil {
				t.Fatal(err)
			}
			var active, rejected float64
			for _, family := range families {
				if len(family.Metric) != 1 || len(family.Metric[0].Label) != 0 {
					t.Fatal("connection metrics must have one unlabelled sample")
				}
				switch family.GetName() {
				case "test_text_connections":
					active = family.Metric[0].GetGauge().GetValue()
				case "test_text_rejections_total":
					rejected = family.Metric[0].GetCounter().GetValue()
				}
			}
			if active == wantConnections && rejected == wantRejections && int64(active) == limiter.Current() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("metrics active=%v rejected=%v limiter=%d; want %v/%v", active, rejected, limiter.Current(), wantConnections, wantRejections)
			}
			time.Sleep(time.Millisecond)
		}
	}
	response, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatal("invalid HTTP upgrade unexpectedly succeeded", response.StatusCode)
	}
	waitMetrics(0, 0)
	dial := func() *websocket.Conn {
		t.Helper()
		c, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		if err != nil {
			if response != nil {
				response.Body.Close()
			}
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		return c
	}
	first := dial() // The invalid HTTP upgrade must have released its reservation.
	waitMetrics(1, 0)
	blocked, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if blocked != nil {
		blocked.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("capacity did not refuse second socket", err)
	}
	response.Body.Close()
	waitMetrics(1, 1)
	first.Close()
	waitMetrics(0, 1)
	for _, tc := range []struct{ hello, wantType string }{
		{`{"v":1,"type":"hello","payload":{"client_generation":2,"access_token":"invalid"}}`, "error"},
		{`{"v":2,"type":"hello","payload":{"client_generation":2,"access_token":"invalid"}}`, "error"},
		{`{"v":2,"type":"hello","payload":{"client_generation":2,"access_token":"` + uuid.NewString() + `"}}`, "hello"},
	} {
		conn := dial()
		waitMetrics(1, 1)
		if err := conn.WriteMessage(websocket.TextMessage, []byte(tc.hello)); err != nil {
			t.Fatal(err)
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var reply struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil || reply.Type != tc.wantType {
			t.Fatalf("handshake got %s (%v); want %s", raw, err, tc.wantType)
		}
		conn.Close()
		waitMetrics(0, 1)
	}
}

type textTimedAuth struct {
	block       atomic.Bool
	enteredOnce sync.Once
	entered     chan struct{}
	release     chan struct{}
}

func (a *textTimedAuth) ValidateAccessToken(ctx context.Context, token string) (string, error) {
	if a.block.Load() {
		a.enteredOnce.Do(func() { close(a.entered) })
		minimumWait := time.NewTimer(60 * time.Millisecond)
		defer minimumWait.Stop()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-a.release:
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-minimumWait.C:
		}
	}
	return (textAuthStub{}).ValidateAccessToken(ctx, token)
}

func TestTextAcceptedActionTimingIncludesAuthorizationAndRejectsFailures(t *testing.T) {
	auth := &textTimedAuth{entered: make(chan struct{}), release: make(chan struct{})}
	var cfg *config.Config
	var now atomic.Int64
	now.Store(time.Now().UnixMilli())
	_, _, manager := textHTTPCustomFixture(t, auth, func(c *config.Config, d *lobby.TextDeps) {
		cfg = c
		d.Now = func() time.Time { return time.UnixMilli(now.Load()) }
	})
	type observation struct {
		mode    gamecontract.ModeID
		elapsed time.Duration
	}
	observed := make(chan observation, 16)
	srv := httptest.NewServer(TextRealtimeHandler(TextHandlerDeps{Config: cfg, Lobby: manager, Auth: auth, ObserveAcceptedAction: func(mode gamecontract.ModeID, elapsed time.Duration) {
		observed <- observation{mode, elapsed}
	}}))
	t.Cleanup(srv.Close)
	peers := make([]*websocket.Conn, 4)
	for i := range peers {
		peers[i] = textDial(t, srv)
		textHello(t, peers[i], uuid.NewString())
	}
	settings := v2.LobbySettings{ModeID: gamecontract.ModeMissedTheBriefing, Size: 4, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
	created := textLastLobby(t, textControl(t, peers[0], "room_create", "create", settings))
	for _, peer := range peers[1:] {
		textControl(t, peer, "room_join", "join", map[string]string{"code": created.Code})
	}
	for i, peer := range peers {
		state := textLastLobby(t, textControl(t, peer, "resync", fmt.Sprintf("sync-%d", i), struct{}{})).Lobby
		textControl(t, peer, "room_ready", fmt.Sprintf("ready-%d", i), v2.ReadyAcknowledgement{SettingsRevision: state.SettingsRevision, MembershipRevision: state.MembershipRevision})
	}
	textControl(t, peers[0], "room_start", "start", struct{}{})
	for _, peer := range peers[1:] {
		textNextSnapshot(t, peer)
	}
	now.Add(1100)
	if err := manager.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	snapshots := make([]v2.Snapshot, len(peers))
	for i, peer := range peers {
		snapshots[i] = textNextSnapshot(t, peer)
	}
	current := *snapshots[0].CurrentSeat
	before := snapshots[current]
	count := 1
	request := v2.ActionRequest{Version: 2, RequestID: "timed-draw", MatchID: before.Contract.MatchID, ModeID: settings.ModeID, Round: before.Round, Turn: before.Turn, Phase: before.Phase, PhaseID: before.PhaseID, ExpectedBoardRevision: before.Board.Revision, Action: v2.Action{Kind: v2.ActionDraw, Count: &count}}
	auth.block.Store(true)
	if err := textWriteJSON(peers[current], map[string]any{"v": 2, "type": "action", "payload": request}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-auth.entered:
	case <-time.After(time.Second):
		t.Fatal("request did not enter authorization")
	}
	blockedAt := time.Now()
	select {
	case <-observed:
		t.Fatal("unfinished intent counted as accepted")
	case <-time.After(60 * time.Millisecond):
	}
	close(auth.release)
	for i, peer := range peers {
		snapshots[i] = textNextSnapshot(t, peer)
	}
	if ack := textRead(t, peers[current]); ack.Type != "action_ack" || ack.RequestID != request.RequestID {
		t.Fatal("accepted intent has no matching acknowledgement")
	}
	select {
	case got := <-observed:
		if got.mode != settings.ModeID || got.elapsed < 60*time.Millisecond || got.elapsed > time.Since(blockedAt)+time.Second {
			t.Fatal("timing omitted server wait or wrong public mode", got)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted action was not observed")
	}
	auth.block.Store(false)
	// A successful retry is another accepted request, not another game mutation.
	if err := textWriteJSON(peers[current], map[string]any{"v": 2, "type": "action", "payload": request}); err != nil {
		t.Fatal(err)
	}
	after := textNextSnapshot(t, peers[current])
	ack := textRead(t, peers[current])
	if ack.Type != "action_ack" || !strings.Contains(string(ack.Payload), "true") || len(after.Private.Hand) != len(snapshots[current].Private.Hand) {
		t.Fatal("retry changed game state or lost receipt")
	}
	select {
	case got := <-observed:
		if got.mode != settings.ModeID || got.elapsed <= 0 {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted retry missing timing")
	}
	for _, kind := range []string{"stale", "unauthorized", "malformed"} {
		bad := request
		bad.RequestID = "timed-" + kind
		peer := peers[current]
		switch kind {
		case "stale":
			bad.ExpectedBoardRevision = after.Board.Revision + 100
		case "unauthorized":
			peer = peers[(current+1)%len(peers)]
		case "malformed":
			bad.Action.Count = nil
		}
		if err := textWriteJSON(peer, map[string]any{"v": 2, "type": "action", "payload": bad}); err != nil {
			t.Fatal(err)
		}
		if failure := textRead(t, peer); failure.Type != "error" {
			t.Fatal("invalid action accepted", kind, failure.Type)
		}
		textControl(t, peer, "resync", "barrier-"+kind, struct{}{})
		select {
		case <-observed:
			t.Fatal("rejected action counted as accepted", kind)
		default:
		}
	}
}
