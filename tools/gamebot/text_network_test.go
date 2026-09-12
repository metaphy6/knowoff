package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/handler"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

func TestTextNetworkRefusesWithoutServerPrototypeBeforeAdmission(t *testing.T) {
	for _, stage := range []string{"hello", "availability"} {
		t.Run(stage, func(t *testing.T) {
			var admission atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				var hello lobby.TextEnvelope
				if err := conn.ReadJSON(&hello); err != nil || hello.Type != "hello" {
					return
				}
				limits := lobby.TextLimits{MaxFrameBytes: 8192, MaxHistoryEvents: 100, MaxHistoryPageEvents: 10, MaxTextBytes: 1000, MaxRequestsPerSeat: 100}
				payload, _ := json.Marshal(map[string]any{"client_generation": 2, "account_id": "fixture", "limits": limits, "prototype": stage != "hello"})
				if err := conn.WriteJSON(lobby.TextEnvelope{Version: 2, Type: "hello", Payload: payload}); err != nil {
					return
				}
				if stage == "availability" {
					payload, _ = json.Marshal(lobby.TextAvailability{ProtocolVersion: 2, ClientGeneration: 2, Limits: limits})
					if err := conn.WriteJSON(lobby.TextEnvelope{Version: 2, Type: "availability", Payload: payload}); err != nil {
						return
					}
				}
				conn.SetReadDeadline(time.Now().Add(time.Second))
				if _, _, err := conn.ReadMessage(); err == nil {
					admission.Add(1)
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			peer, err := connectTextNetwork(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/v2", "fixture-only-token", 1)
			if peer != nil {
				peer.close()
			}
			if err == nil {
				t.Fatal("nonprototype server accepted")
			}
			if admission.Load() != 0 {
				t.Fatal("admission sent before prototype authorization")
			}
		})
	}
}

func TestTextNetworkEndpointRefusesRemoteAndCredentialURLs(t *testing.T) {
	for _, endpoint := range []string{"ws://example.com/ws/v2", "ws://localhost/ws/v2", "wss://127.0.0.1/ws/v2", "ws://u:p@127.0.0.1/ws/v2", "ws://127.0.0.1/ws/v2?token=x", "ws://127.0.0.1/ws/v2#fragment", "ws://127.0.0.1/ws"} {
		if textNetworkEndpoint(endpoint) == nil {
			t.Errorf("accepted %s", endpoint)
		}
	}
}

type networkFixture struct {
	server      *httptest.Server
	db          *sql.DB
	manager     *lobby.TextManager
	values      *store.TextValueStore
	now         atomic.Int64
	drawPenalty int64
	connections networkConnectionGauge
}

type networkConnectionGauge struct{ atomic.Int64 }

func (g *networkConnectionGauge) Inc() { g.Add(1) }
func (g *networkConnectionGauge) Dec() { g.Add(-1) }

func TestTextNetworkCommandAuthenticatesAndSavesInterruptedTrace(t *testing.T) {
	f := newNetworkFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "interrupted-network.json")
	result := make(chan error, 1)
	go func() {
		result <- runTextNetwork(ctx, "ws"+strings.TrimPrefix(f.server.URL, "http")+"/ws/v2", gamecontract.ModeMissedTheBriefing, 4, 42, path, "disposable-network-fixture-key-for-local-tests")
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-result:
			t.Fatalf("command never admitted an authenticated prototype match: %v", err)
		case <-ctx.Done():
			t.Fatal("command did not reach an authenticated match before its deadline")
		case <-ticker.C:
			var started int
			if err := f.db.QueryRowContext(ctx, `SELECT count(*) FROM text_matches WHERE state='started'`).Scan(&started); err != nil {
				t.Fatal(err)
			}
			if started == 0 {
				continue
			}
			if started != 1 {
				t.Fatal("command created duplicate matches")
			}
			cancel()
			if err := <-result; !errors.Is(err, context.Canceled) {
				t.Fatalf("command cancellation did not preserve its cause: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal("failed command did not save its private trace", err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("interrupted trace permissions", err)
			}
			if strings.Contains(string(raw), "access_token") || strings.Contains(string(raw), "disposable-network-fixture-key-for-local-tests") {
				t.Fatal("interrupted trace exposed credentials")
			}
			var accounts int
			if err := f.db.QueryRow(`SELECT count(*) FROM accounts WHERE auth_purpose='development'`).Scan(&accounts); err != nil || accounts != 4 {
				t.Fatal("first authenticated account was not reused for its seat", accounts, err)
			}
			return
		}
	}
}

func TestTextNetworkDevelopmentAuthenticationAndAvailabilityGate(t *testing.T) {
	var authCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/text/availability":
			if r.Header.Get("Authorization") != "Bearer disposable-token" {
				t.Error("availability omitted authenticated development identity")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(lobby.TextAvailability{ProtocolVersion: 2, ClientGeneration: 2})
		case "/api/auth/development":
			authCalls.Add(1)
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer disposable-unit-test-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"access_token": "disposable-token", "refresh_token": "unused-refresh"})
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/v2"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runTextNetwork(ctx, endpoint, gamecontract.ModeMissedTheBriefing, 4, 1, filepath.Join(t.TempDir(), "private.json"), "disposable-unit-test-key"); err == nil {
		t.Fatal("entrypoint accepted nonprototype availability")
	}
	if authCalls.Load() != 1 {
		t.Fatal("entrypoint must authenticate once before availability and refuse remaining seats")
	}
	if _, err := networkDevelopmentAuth(ctx, endpoint, ""); err == nil {
		t.Fatal("development secret optional")
	}
	token, err := networkDevelopmentAuth(ctx, endpoint, "disposable-unit-test-key")
	if err != nil || token != "disposable-token" || authCalls.Load() != 2 {
		t.Fatal("authenticated development endpoint failed", err)
	}
}

func TestTextNetworkTraceRejectsTokenAndExistingArtifacts(t *testing.T) {
	peer := &textNetworkBot{token: "never-persist-access-token"}
	for _, frame := range []lobby.TextEnvelope{
		{Version: 2, Type: "error", Payload: json.RawMessage(`{"code":"never-persist-access-token"}`)},
		{Version: 2, Type: "error", Payload: json.RawMessage(`{"code":"prefix-\u006eever-persist-access-token-suffix"}`)},
		{Version: 2, Type: "error", Payload: json.RawMessage(`{"code":"\u006eever-persist-access-token","code":"safe"}`)},
		{Version: 2, Type: "prefix-never-persist-access-token-suffix", Payload: json.RawMessage(`{}`)},
		{Version: 2, Type: "error", RequestID: "prefix-never-persist-access-token-suffix", Payload: json.RawMessage(`{}`)},
	} {
		if err := peer.record("server", frame); err == nil {
			t.Error("trace accepted a plain, escaped or embedded credential")
		}
	}
	if len(peer.trace) != 0 || peer.traceBytes != 0 {
		t.Fatal("credential refusal retained sensitive frames")
	}
	path := filepath.Join(t.TempDir(), "trace.json")
	if err := writeTextNetworkTrace(path, []*textNetworkBot{peer}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTextNetworkTrace(path, []*textNetworkBot{peer}); err == nil {
		t.Fatal("trace overwrote existing artifact")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("existing artifact changed", err)
	}
}

func TestTextNetworkTraceRejectsCredentialsAcrossRecipients(t *testing.T) {
	for _, frame := range []lobby.TextEnvelope{
		{Version: 2, Type: "error", Payload: json.RawMessage(`{"code":"peer-one-private-token"}`)},
		{Version: 2, Type: "error", Payload: json.RawMessage(`{"code":"prefix-\u0070eer-one-private-token-suffix","code":"safe"}`)},
		{Version: 2, Type: "prefix-peer-one-private-token-suffix", Payload: json.RawMessage(`{}`)},
		{Version: 2, Type: "error", RequestID: "prefix-peer-one-private-token-suffix", Payload: json.RawMessage(`{}`)},
	} {
		first := &textNetworkBot{token: "peer-one-private-token"}
		second := &textNetworkBot{token: "peer-two-private-token"}
		if err := second.record("server", frame); err != nil {
			t.Fatal("recipient cannot know the other credential", err)
		}
		path := filepath.Join(t.TempDir(), "rejected-trace.json")
		if err := writeTextNetworkTrace(path, []*textNetworkBot{first, second}); err == nil {
			t.Fatal("aggregate trace retained another recipient's credential")
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("credential-bearing output was created", err)
		}
	}
}

type networkValues struct {
	*store.TextValueStore
	t *testing.T
}

func (v networkValues) Award(ctx context.Context, a store.TextAward) (int, error) {
	n, err := v.TextValueStore.Award(ctx, a)
	if err != nil {
		v.t.Logf("durable network award failed: kind=%s ordinal=%d amount=%d: %v", a.Kind, a.Ordinal, a.Amount, err)
	}
	return n, err
}

func (v networkValues) Finish(ctx context.Context, o store.TextOutcome) error {
	err := v.TextValueStore.Finish(ctx, o)
	if err != nil {
		v.t.Logf("durable network finish failed: kind=%s players=%d: %v", o.Kind, len(o.Players), err)
	}
	return err
}

func newNetworkFixture(t *testing.T) *networkFixture {
	return newNetworkFixtureWithObserver(t, nil)
}

func newNetworkFixtureWithObserver(t *testing.T, observe func(gamecontract.ModeID, time.Duration)) *networkFixture {
	t.Helper()
	ctx := context.Background()
	dsn, token := os.Getenv("KNOWOFF_TEST_DSN"), os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	u, err := url.Parse(dsn)
	if err != nil || len(token) != 12 || strings.Trim(token, "0123456789abcdef") != "" || u.Path != "/knowoff_test_"+token || (u.Hostname() != "postgres" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.RawQuery != "sslmode=disable" || u.Fragment != "" {
		t.Fatal("use xops/test/tests-lints.py uniquely named disposable PostgreSQL")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var actual string
	if err = db.QueryRow(`SELECT current_database()`).Scan(&actual); err != nil || actual != "knowoff_test_"+token {
		t.Fatal("disposable database mismatch", err)
	}
	if _, err = db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = store.MigrateUp(db, "../../server/migrations"); err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(16)
	redisAddress := os.Getenv("KNOWOFF_TEST_REDIS_ADDR")
	if redisAddress == "" {
		t.Fatal("runner disposable Redis required")
	}
	redis := store.NewRedisClient(redisAddress, "", 0)
	t.Cleanup(func() { redis.Close() })
	if err = redis.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	raw, err := os.ReadFile("../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = yaml.Unmarshal(raw, &cfg.Tuning); err != nil {
		t.Fatal(err)
	}
	cfg.App.Env = "test"
	cfg.WebSocket = config.WebSocketConfig{MaxMessageBytes: 8192, PongWaitS: 60, PingPeriodS: 5, WriteWaitS: 5}
	cfg.Tuning.Contract.MaxHistoryPageEvents = 8
	cfg.Text = &config.TextConfig{Version: 2, RulesVersion: "text-v1", Compatibility: config.TextCompatibility{ProtocolVersion: 2, MinClientGeneration: 2}, Modes: map[gamecontract.ModeID]config.TextModeConfig{}}
	pack, err := media.LoadTextPack("../../server/pkg/media/testdata/text-en", media.TextLimits{MaxTextBytes: cfg.Tuning.Contract.MaxTextBytes, MaxRecords: cfg.Tuning.TextCatalog.MaxRecords, MaxFileBytes: cfg.Tuning.TextCatalog.MaxFileBytes, MaxBundleBytes: cfg.Tuning.TextCatalog.MaxBundleBytes})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Release(ctx) })
	releases := store.NewTextReleaseStore(db, cfg.Tuning, nil)
	values, err := store.NewTextValueStore(db, cfg.Tuning).WithStartGuard(releases.ValidateStart).WithOwner(owner)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := owner.RecoverLostOwners(ctx, values, 100)
	if err != nil || !recovery.Done {
		t.Fatal(recovery, err)
	}
	f := &networkFixture{db: db, values: values, drawPenalty: int64(cfg.Tuning.Points.DrawPenalty)}
	// Deliberately separate the gameplay clock from PostgreSQL's wall clock so
	// delivery tests cannot pass only because an uninstrumented match runs fast.
	f.now.Store(time.Now().UTC().Add(-time.Hour).UnixMilli())
	manager, err := lobby.NewTextManager(lobby.TextDeps{Owner: owner.Token().IncarnationID, Authority: owner, Config: cfg, Values: networkValues{values, t}, Prototype: pack, Now: func() time.Time { return time.UnixMilli(f.now.Load()) }})
	if err != nil {
		t.Fatal(err)
	}
	f.manager = manager
	authManager := auth.NewManager(db, []byte("disposable-network-test-signing-key"), "network-test", "network-client", time.Hour, 24*time.Hour, auth.OAuthProviders{})
	if err := authManager.ConfigureDevelopment("local", true); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterAuthRoutes(mux, handler.AuthDeps{Auth: authManager, DevBotKey: "disposable-network-fixture-key-for-local-tests"})
	deps := handler.TextHandlerDeps{Config: cfg, Lobby: manager, Auth: authManager, Deliveries: values, DeliveryWorker: manager.Owner(), Connections: &f.connections, ObserveAcceptedAction: observe}
	mux.HandleFunc("/ws/v2", handler.TextRealtimeHandler(deps))
	mux.HandleFunc("/api/text/availability", handler.TextAvailabilityHandler(deps))
	f.server = httptest.NewServer(mux)
	t.Cleanup(func() { manager.Close(ctx); f.server.Close() })
	return f
}
func (f *networkFixture) advance(ctx context.Context, at int64) error {
	if at <= f.now.Load() {
		return fmt.Errorf("clock did not advance")
	}
	f.now.Store(at)
	return f.manager.Tick(ctx)
}
func networkZero(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	var n int64
	if err := db.QueryRow(query).Scan(&n); err != nil || n != 0 {
		t.Fatalf("live value changed (%d): %v", n, err)
	}
}

func TestTextNetworkAuthenticatedFiveModesAndSizes(t *testing.T) {
	f := newNetworkFixture(t)
	// The complete ten-match matrix also runs with race instrumentation and
	// validates every full recipient history; retain a bounded CI work budget.
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	pages := 0
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				peers := make([]*textNetworkBot, size)
				for i := range peers {
					token, err := networkDevelopmentAuth(ctx, "ws"+strings.TrimPrefix(f.server.URL, "http")+"/ws/v2", "disposable-network-fixture-key-for-local-tests")
					if err != nil {
						t.Fatal(err)
					}
					peer, err := connectTextNetwork(ctx, "ws"+strings.TrimPrefix(f.server.URL, "http")+"/ws/v2", token, int64(42+i))
					if err != nil {
						t.Fatal(err)
					}
					peers[i] = peer
					defer peer.close()
				}
				settings := v2.LobbySettings{ModeID: mode, Size: size, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
				if err := peers[0].control(ctx, "room_create", settings); err != nil {
					t.Fatal(err)
				}
				code := peers[0].lobby.Code
				for _, peer := range peers[1:] {
					if err := peer.control(ctx, "room_join", map[string]any{"code": code}); err != nil {
						t.Fatal(err)
					}
				}
				for _, peer := range peers {
					if err := peer.control(ctx, "resync", struct{}{}); err != nil {
						t.Fatal(err)
					}
					l := peer.lobby.Lobby
					if err := peer.control(ctx, "room_ready", v2.ReadyAcknowledgement{SettingsRevision: l.SettingsRevision, MembershipRevision: l.MembershipRevision}); err != nil {
						t.Fatal(err)
					}
				}
				if err := peers[0].control(ctx, "room_start", struct{}{}); err != nil {
					t.Fatal(err)
				}
				reconnect, drawn := false, false
				actions := 0
				for step := 0; step < 500; step++ {
					for _, peer := range peers {
						if err := peer.control(ctx, "resync", struct{}{}); err != nil {
							t.Fatal(err)
						}
					}
					first := peers[0].snapshot
					if first.Phase == v2.PhaseVerdict {
						break
					}
					if !reconnect && actions >= 3 && first.PendingOffer == nil {
						old := peers[0].snapshot
						if err := peers[0].reconnect(ctx, code); err != nil {
							t.Fatal(err)
						}
						fresh := peers[0].snapshot
						if fresh.Private.Role != old.Private.Role || fresh.Private.Points != old.Private.Points || fresh.Cursor.StreamEpoch == old.Cursor.StreamEpoch || len(fresh.History) < len(old.History) {
							t.Fatal("reconnect lost private facts/history/epoch")
						}
						reconnect = true
						continue
					}
					acted := false
					for _, peer := range peers {
						action := textPolicy(peer.snapshot, peer.rng)
						if action == nil {
							continue
						}
						if !drawn && peer.snapshot.CurrentSeat != nil && *peer.snapshot.CurrentSeat == peer.snapshot.Private.Seat && peer.snapshot.Phase == v2.PhasePlay && peer.snapshot.Private.ReserveCount > 0 {
							count := 1
							before := peer.snapshot.Private
							if err := peer.act(ctx, v2.Action{Kind: v2.ActionDraw, Count: &count}); err != nil {
								t.Fatal(err)
							}
							after := peer.snapshot.Private
							if after.Points != max(before.Points-f.drawPenalty, 0) || len(after.Hand) != len(before.Hand)+1 || after.ReserveCount != before.ReserveCount-1 {
								t.Fatal("draw did not preserve private card/point accounting")
							}
							for _, other := range peers {
								if other == peer {
									continue
								}
								points := other.snapshot.Private.Points
								if err := other.control(ctx, "resync", struct{}{}); err != nil {
									t.Fatal(err)
								}
								if other.snapshot.Private.Points != points {
									t.Fatal("draw changed another recipient's private points")
								}
								for _, card := range other.snapshot.Private.Hand {
									if card.CopyID == after.Hand[len(after.Hand)-1].CopyID {
										t.Fatal("draw leaked another hand copy")
									}
								}
							}
							drawn, acted = true, true
							actions++
							break
						}
						if err := peer.act(ctx, *action); err != nil {
							t.Fatalf("action %s at phase %s round %d turn %d: %v", action.Kind, peer.snapshot.Phase, peer.snapshot.Round, peer.snapshot.Turn, err)
						}
						actions++
						acted = true
						break
					}
					if !acted {
						clockInput, _ := json.Marshal(map[string]int64{"at_ms": first.DeadlineMS})
						if err := peers[0].record("fixture_clock", lobby.TextEnvelope{Version: 2, Type: "advance", Payload: clockInput}); err != nil {
							t.Fatal(err)
						}
						if err := f.advance(ctx, first.DeadlineMS); err != nil {
							t.Fatal(err)
						}
					}
				}
				// Gameplay uses a manually advanced clock, but PostgreSQL creates
				// outbox availability with wall time. Race instrumentation can let
				// wall time overtake the fixture. Synchronize only after the verdict,
				// using the persisted availability rather than sleeping or changing
				// any gameplay deadline.
				var available time.Time
				if err := f.db.QueryRowContext(ctx, `SELECT max(available_at) FROM text_outbox WHERE match_id=$1`, peers[0].snapshot.Contract.MatchID).Scan(&available); err != nil {
					t.Fatal(err)
				}
				if at := available.UnixMilli() + 1; at > f.now.Load() {
					clockInput, _ := json.Marshal(map[string]int64{"at_ms": at})
					if err := peers[0].record("fixture_clock", lobby.TextEnvelope{Version: 2, Type: "advance", Payload: clockInput}); err != nil {
						t.Fatal(err)
					}
					if err := f.advance(ctx, at); err != nil {
						t.Fatal(err)
					}
				}
				if err := f.manager.PumpDeliveries(ctx, f.values); err != nil {
					t.Fatal(err)
				}
				for _, peer := range peers {
					if err := peer.control(ctx, "resync", struct{}{}); err != nil {
						t.Fatal(err)
					}
					s := peer.snapshot
					if s.Phase != v2.PhaseVerdict || s.Verdict == nil || s.Verdict.Outcome != "completed" || s.Contract.Eligibility.Rewards || s.Contract.Eligibility.Leaderboard || len(s.VerdictNowns) != s.Round || s.PendingOffer != nil || len(peer.deliveries) != 1 {
						t.Fatalf("incomplete prototype verdict phase=%s deliveries=%d", s.Phase, len(peer.deliveries))
					}
					for _, d := range peer.deliveries {
						if d.Points != 0 || d.XP != 0 || d.LeaderboardCounted {
							t.Fatal("prototype granted result value")
						}
						for _, a := range d.Awards {
							if a.Credited != 0 {
								t.Fatal("prototype granted Noin")
							}
						}
					}
					pages += peer.pageCount
				}
				if !reconnect || !drawn || actions == 0 {
					t.Fatal("missing reconnect/actions")
				}
				// Delivery IDs and payloads are bound to the authenticated recipient.
				for _, peer := range peers {
					for id := range peer.deliveries {
						var account string
						if err := f.db.QueryRow(`SELECT account_id FROM text_outbox WHERE id=$1`, id).Scan(&account); err != nil || account != peer.account {
							t.Fatal("private outbox crossed recipient boundary", err)
						}
					}
				}
				var firstDelivery int64
				for id := range peers[0].deliveries {
					firstDelivery = id
				}
				if err := peers[1].control(ctx, "settlement_ack", map[string]int64{"id": firstDelivery}); err == nil {
					t.Fatal("foreign recipient acknowledged another private delivery")
				}
				// A terminal disconnect releases room membership. Durable private
				// delivery remains recoverable from a fresh authenticated connection.
				peers[0].close()
				if err := peers[0].connect(ctx); err != nil {
					t.Fatal(err)
				}
				if err := f.advance(ctx, f.now.Load()+31000); err != nil {
					t.Fatal(err)
				}
				if err := f.manager.PumpDeliveries(ctx, f.values); err != nil {
					t.Fatal(err)
				}
				for i, peer := range peers {
					if i == 0 {
						frame, err := peer.read(ctx)
						if err != nil || frame.Type != "settlement" {
							t.Fatal("fresh authenticated connection did not recover private delivery", err)
						}
						if err := peer.receive(frame); err != nil {
							t.Fatal(err)
						}
					} else if err := peer.control(ctx, "resync", struct{}{}); err != nil {
						t.Fatal(err)
					}
					if len(peer.deliveries) != 1 {
						t.Fatal("terminal reconnect/replay duplicated private settlement")
					}
					for id := range peer.deliveries {
						if err := peer.control(ctx, "settlement_ack", map[string]int64{"id": id}); err != nil {
							t.Fatal(err)
						}
					}
				}
				var attempts int
				if err := f.db.QueryRow(`SELECT attempts FROM text_outbox WHERE id=$1 AND acknowledged_at IS NOT NULL`, firstDelivery).Scan(&attempts); err != nil || attempts != 2 {
					t.Fatal("private settlement was not replayed and acknowledged exactly", attempts, err)
				}
				path := filepath.Join(t.TempDir(), "private-network-trace.json")
				if err := writeTextNetworkTrace(path, peers); err != nil {
					t.Fatal(err)
				}
				info, _ := os.Stat(path)
				if info.Mode().Perm() != 0600 {
					t.Fatal("private trace permissions")
				}
				raw, _ := os.ReadFile(path)
				for _, peer := range peers {
					if strings.Contains(string(raw), peer.token) {
						t.Fatal("trace leaked access token")
					}
				}
				t.Logf("authenticated prototype completed: actions=%d rounds=%d history=%d", actions, peers[0].snapshot.Round, len(peers[0].snapshot.History))
			})
		}
	}
	if pages == 0 {
		t.Fatal("network proof never assembled paged history")
	}
	networkZero(t, f.db, `SELECT count(*) FROM noin_ledger`)
	networkZero(t, f.db, `SELECT COALESCE(sum(overall_points+non_converted_points+xp+matches_played),0) FROM profiles`)
	networkZero(t, f.db, `SELECT count(*) FROM leaderboard_entries`)
	networkZero(t, f.db, `SELECT COALESCE(sum(count),0) FROM daily_quickplay_counts`)
}
