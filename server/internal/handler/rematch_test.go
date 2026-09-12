package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func finishedTextQuickPlay(t *testing.T) (*httptest.Server, *lobby.TextManager, []*websocket.Conn, v2.Snapshot, v2.LobbySettings) {
	t.Helper()
	var clock atomic.Int64
	clock.Store(time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC).UnixMilli())
	srv, _, manager := textHTTPCustomFixture(t, textAuthStub{}, func(_ *config.Config, d *lobby.TextDeps) {
		d.Now = func() time.Time { return time.UnixMilli(clock.Load()) }
	})
	peers := make([]*websocket.Conn, 4)
	settings := v2.LobbySettings{ModeID: gamecontract.ModeTopThat, Size: 4, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
	for i := range peers {
		peers[i] = textDial(t, srv)
		textHello(t, peers[i], uuid.NewString())
		textControl(t, peers[i], "queue_join", fmt.Sprintf("queue-%d", i), settings)
	}
	for i, p := range peers {
		view := textLastLobby(t, textControl(t, p, "resync", fmt.Sprintf("sync-%d", i), struct{}{}))
		textControl(t, p, "room_ready", fmt.Sprintf("ready-%d", i), v2.ReadyAcknowledgement{SettingsRevision: view.Lobby.SettingsRevision, MembershipRevision: view.Lobby.MembershipRevision})
	}
	frames := textControl(t, peers[0], "room_start", "start", struct{}{})
	var state v2.Snapshot
	for _, f := range frames {
		if f.Type == "snapshot" {
			if err := json.Unmarshal(f.Payload, &state); err != nil {
				t.Fatal(err)
			}
		}
	}
	if state.Contract.MatchID == "" {
		t.Fatal("start snapshot missing")
	}
	for _, p := range peers[1:] {
		textNextSnapshot(t, p)
	}
	for step := 0; step < 80; step++ {
		if state.Phase == v2.PhaseVerdict {
			return srv, manager, peers, state, settings
		}
		clock.Store(state.DeadlineMS)
		if err := manager.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		for i, p := range peers {
			s := textNextSnapshot(t, p)
			if i == 0 {
				state = s
			}
		}
	}
	t.Fatal("match did not terminate within phase bound")
	return nil, nil, nil, v2.Snapshot{}, settings
}

func TestRematchSameTableRequiresFreshReadyAndMatchIdentity(t *testing.T) {
	_, _, peers, old, settings := finishedTextQuickPlay(t)
	view := textLastLobby(t, textControl(t, peers[2], "rematch", "return-2", struct{}{}))
	if old.Contract.Eligibility.EntryPath != "quick_play" || view.Lobby.HostSeat != 0 || view.Lobby.Settings != settings {
		t.Fatal("rematch lost original path/host/tuple", view)
	}
	for _, member := range view.Lobby.Seats {
		if member.Ready != nil {
			t.Fatal("Ready carried across match")
		}
	}
	if err := peers[0].WriteJSON(map[string]any{"v": 2, "type": "room_start", "request_id": "early-start", "payload": struct{}{}}); err != nil {
		t.Fatal(err)
	}
	for {
		f := textRead(t, peers[0])
		if f.Type == "snapshot" {
			t.Fatal("rematch started before fresh Ready")
		}
		if f.Type == "error" {
			if f.RequestID != "early-start" {
				t.Fatal(f)
			}
			break
		}
	}
	for i, p := range peers {
		v := textLastLobby(t, textControl(t, p, "resync", fmt.Sprintf("rematch-sync-%d", i), struct{}{}))
		textControl(t, p, "room_ready", fmt.Sprintf("rematch-ready-%d", i), v2.ReadyAcknowledgement{SettingsRevision: v.Lobby.SettingsRevision, MembershipRevision: v.Lobby.MembershipRevision})
	}
	frames := textControl(t, peers[0], "room_start", "new-start", struct{}{})
	var next v2.Snapshot
	for _, f := range frames {
		if f.Type == "snapshot" {
			if err := json.Unmarshal(f.Payload, &next); err != nil {
				t.Fatal(err)
			}
		}
	}
	if next.Contract.MatchID == "" || next.Contract.MatchID == old.Contract.MatchID || next.Cursor.StreamEpoch == old.Cursor.StreamEpoch || next.Contract.Eligibility.EntryPath != "quick_play" {
		t.Fatal("rematch reused previous identity/state")
	}
	if len(next.History) != 1 || next.History[0].Kind != "seed" || next.History[0].Round != 1 || len(next.Private.Hand) != 5 || next.Round != 1 {
		t.Fatal("rematch did not contain exactly its initial seed and fresh hand", next.History)
	}
}

func TestRematchReplacementUsesExactFIFOAndClearsReady(t *testing.T) {
	srv, _, peers, _, settings := finishedTextQuickPlay(t)
	textControl(t, peers[3], "room_leave", "leave", struct{}{})
	view := textLastLobby(t, textControl(t, peers[1], "rematch", "return", struct{}{}))
	if len(view.Lobby.Seats) != 3 {
		t.Fatal("departed seat or bot retained")
	}
	textControl(t, peers[0], "room_ready", "old-ready", v2.ReadyAcknowledgement{SettingsRevision: view.Lobby.SettingsRevision, MembershipRevision: view.Lobby.MembershipRevision})
	stranger := textDial(t, srv)
	textHello(t, stranger, uuid.NewString())
	different := settings
	different.ModeID = gamecontract.ModeMakeRoom
	textControl(t, stranger, "queue_join", "other-mode", different)
	current := textLastLobby(t, textControl(t, peers[0], "resync", "still-three", struct{}{}))
	if len(current.Lobby.Seats) != 3 {
		t.Fatal("wrong tuple refilled room")
	}
	replacement := textDial(t, srv)
	textHello(t, replacement, uuid.NewString())
	joined := textLastLobby(t, textControl(t, replacement, "queue_join", "matching", settings))
	if len(joined.Lobby.Seats) != 4 || joined.Lobby.MembershipRevision <= view.Lobby.MembershipRevision {
		t.Fatal("matching FIFO failed replacement")
	}
	for _, member := range joined.Lobby.Seats {
		if member.Ready != nil {
			t.Fatal("replacement preserved obsolete Ready")
		}
	}
}
