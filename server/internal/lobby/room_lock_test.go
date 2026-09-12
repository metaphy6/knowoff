package lobby

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/transport"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func TestRoomCurrentTurnDisconnectBroadcast(t *testing.T) {
	for _, size := range []int{4, 6} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			deps := testDeps()
			deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			deps.Config.Tuning.Timers = config.TimersTuning{PlayTurn: 20, DiscussionPerPlayer: 5, KnowoffBallot: 20, KnowoffRunoff: 15, VoteResultWindow: 8}
			deps.Config.Tuning.Hand = config.HandTuning{Size: 5, DrawPile: 3}
			deps.Config.Tuning.Dealing = config.DealingTuning{BandHigh: .55, BandLow: .30, MinHighPerNown: 2, MinDistantPerNown: 2}
			deps.Config.Tuning.Game.ReconnectGraceS = 20
			pack, err := media.LoadPack("../../pkg/media/testdata/golden-pack", media.DefaultDealingTuning())
			if err != nil {
				t.Fatal(err)
			}
			manager := media.NewManager(pack)
			room := NewRoom("lock-regression", "LOCK", size, 0, false, deps)
			accepted := make(chan *websocket.Conn, size)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err == nil {
					accepted <- conn
				}
			}))
			t.Cleanup(server.Close)
			clients := make([]*websocket.Conn, size)
			for seat := 0; seat < size; seat++ {
				room.ClaimSeat("", false)
				client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
				if err != nil {
					t.Fatal(err)
				}
				conn := <-accepted
				t.Cleanup(func() { client.Close(); conn.Close() })
				clients[seat] = client
				room.SetConnection(seat, conn)
			}
			renderer := game.NewPayloadRenderer(manager, media.NewSignedURLIssuer([]byte("test-only"), time.Minute), "")
			if err := room.StartMatch(game.Dependencies{Config: deps.Config, Pack: pack, Renderer: renderer}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				room.Match().Stop()
				room.mu.Lock()
				for _, binding := range room.bindings {
					if binding.GraceTimer != nil {
						binding.GraceTimer.Stop()
					}
				}
				room.mu.Unlock()
			})
			clients[0].SetReadDeadline(time.Now().Add(2 * time.Second))
			current := -1
			for current < 0 {
				var event transport.Envelope
				if err := clients[0].ReadJSON(&event); err != nil {
					t.Fatal(err)
				}
				if event.Kind == transport.EventTurnStarted {
					value, ok := event.Payload["turn_seat"].(float64)
					if !ok || value < 0 || value >= float64(size) {
						t.Fatal("invalid current seat")
					}
					current = int(value)
				}
			}
			done := make(chan struct{})
			go func() { room.SetConnection(current, nil); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("current-turn disconnect deadlocked during actual room broadcast")
			}
			observer := (current + 1) % size
			clients[observer].SetReadDeadline(time.Now().Add(time.Second))
			for {
				var event transport.Envelope
				if err := clients[observer].ReadJSON(&event); err != nil {
					t.Fatal(err)
				}
				if event.Kind == transport.EventTurnStarted && int(event.Payload["turn_seat"].(float64)) != current {
					break
				}
			}
		})
	}
}

func roomLockFixture(t *testing.T) (*Room, game.Dependencies) {
	t.Helper()
	deps := testDeps()
	deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	deps.Config.Tuning.Timers = config.TimersTuning{PlayTurn: 20, DiscussionPerPlayer: 5, KnowoffBallot: 20, KnowoffRunoff: 15, VoteResultWindow: 8, PrefetchCountdown: 60}
	deps.Config.Tuning.Hand = config.HandTuning{Size: 5, DrawPile: 3}
	deps.Config.Tuning.Dealing = config.DealingTuning{BandHigh: .55, BandLow: .30, MinHighPerNown: 2, MinDistantPerNown: 2}
	deps.Config.Tuning.Game.ReconnectGraceS = 20
	pack, e := media.LoadPack("../../pkg/media/testdata/golden-pack", media.DefaultDealingTuning())
	if e != nil {
		t.Fatal(e)
	}
	r := NewRoom("room-lock", "LOCK", 4, 0, false, deps)
	for seat := 0; seat < 4; seat++ {
		r.ClaimSeat("", false)
	}
	t.Cleanup(func() {
		if m := r.Match(); m != nil {
			m.Stop()
		}
		r.mu.Lock()
		for _, b := range r.bindings {
			if b.GraceTimer != nil {
				b.GraceTimer.Stop()
			}
		}
		r.mu.Unlock()
	})
	return r, game.Dependencies{Config: deps.Config, Pack: pack, Renderer: game.NewPayloadRenderer(media.NewManager(pack), media.NewSignedURLIssuer([]byte("test"), time.Minute), "")}
}
func TestRoomGraceCallbackMayReenterConnection(t *testing.T) {
	r, deps := roomLockFixture(t)
	seat := 0
	called := false
	deps.OnFinish = func(_ game.Role, _ game.MatchResult) {
		_ = r.Match().Phase()
		_ = r.SeatIdentity(seat)
		r.SetConnection(seat, nil)
		called = true
	}
	m := game.NewMatch(4, deps, &roomBcast{room: r}, game.WithReplay(true), game.WithSeed(42))
	if e := m.Start(); e != nil {
		t.Fatal(e)
	}
	r.match = m
	for i, role := range m.Roles() {
		if role == game.RoleDonower {
			seat = i
		}
	}
	done := make(chan struct{})
	r.mu.Lock()
	r.bindings[seat].GraceDeadline = time.Now().Add(-time.Second)
	r.mu.Unlock()
	go func() { r.onGraceExpired(seat); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("finish callback deadlocked reentering room connection")
	}
	if !called {
		t.Fatal("finish callback omitted")
	}
}

type roomStartBarrier struct {
	slog.Handler
	count   atomic.Int32
	entered chan struct{}
	second  chan struct{}
	release chan struct{}
}

func (h *roomStartBarrier) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "creating match" {
		n := h.count.Add(1)
		if n == 1 {
			close(h.entered)
			<-h.release
		} else if n == 2 {
			close(h.second)
		}
	}
	return h.Handler.Handle(ctx, r)
}
func TestRoomConcurrentStartInitializesOnce(t *testing.T) {
	r, deps := roomLockFixture(t)
	h := &roomStartBarrier{Handler: slog.NewTextHandler(io.Discard, nil), entered: make(chan struct{}), second: make(chan struct{}), release: make(chan struct{})}
	r.deps.Logger = slog.New(h)
	done := make(chan error, 2)
	go func() { done <- r.StartMatch(deps) }()
	<-h.entered
	go func() { done <- r.StartMatch(deps) }()
	select {
	case <-h.second:
	case <-time.After(100 * time.Millisecond):
	}
	close(h.release)
	a, b := <-done, <-done
	if (a == nil) == (b == nil) || h.count.Load() != 1 {
		t.Fatal("concurrent starts initialized more than one engine")
	}
}

func TestRoomStaleGraceDoesNotExpireNewBinding(t *testing.T) {
	for _, kind := range []string{"reconnect", "reuse"} {
		t.Run(kind, func(t *testing.T) {
			r, _ := roomLockFixture(t)
			r.mu.Lock()
			r.scheduleGraceLocked(0)
			old := r.bindings[0]
			epoch, deadline := old.GraceEpoch, old.GraceDeadline
			r.mu.Unlock()
			if kind == "reconnect" {
				r.ReclaimSeat(old.SessionToken)
			} else {
				r.mu.Lock()
				r.releaseSeatLocked(0)
				r.bindings[0] = &SeatBinding{Seat: 0, SessionToken: "replacement"}
				r.mu.Unlock()
			}
			r.mu.Lock()
			r.scheduleGraceLocked(0)
			current := r.bindings[0]
			timer := current.GraceTimer
			r.mu.Unlock()
			r.expireGrace(0, old, epoch, deadline)
			r.mu.RLock()
			defer r.mu.RUnlock()
			if current.GraceTimer != timer {
				t.Fatal("stale callback expired new grace generation")
			}
		})
	}
}
