package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/admin"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/avatar"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/handler"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/notices"
	"github.com/knowoff/knowoff/server/internal/portal"
	"github.com/knowoff/knowoff/server/internal/reports"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/internal/transport"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
	"gopkg.in/yaml.v3"
)

func TestTextRuntimeHealthRecoveryPreservesMaintenance(t *testing.T) {
	db, cfg := textRuntimeProofDB(t)
	ctx := context.Background()
	runtime, err := newTextRuntime(ctx, db, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtime.Close(ctx) })
	var redisErr, maintenanceErr error
	maintenance := false
	deps := transport.Deps{DB: db, RuntimeReady: runtime.Lobby.RuntimeReady, RedisPing: func(context.Context) error { return redisErr }}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	markMaintenance := func(context.Context) error {
		if maintenanceErr != nil {
			return maintenanceErr
		}
		runtime.Lobby.SetReady(!maintenance)
		return nil
	}
	transport.SetReady(true)
	t.Cleanup(func() { transport.SetReady(false) })
	check := func(want int) {
		t.Helper()
		refreshRuntimeHealth(ctx, deps, runtime.Lobby, markMaintenance, logger)
		rec := httptest.NewRecorder()
		transport.ReadyzHandler(deps).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != want {
			t.Fatalf("runtime readiness %d, want %d", rec.Code, want)
		}
	}
	check(http.StatusOK)
	maintenance = true
	check(http.StatusServiceUnavailable)
	redisErr = errors.New("Redis outage")
	check(http.StatusServiceUnavailable)
	redisErr = nil
	check(http.StatusServiceUnavailable)
	maintenance = false
	check(http.StatusOK)
	maintenanceErr = errors.New("notice lookup outage")
	check(http.StatusServiceUnavailable)
	maintenanceErr = nil
	check(http.StatusOK)
	transport.SetReady(false)
	check(http.StatusServiceUnavailable)
	runtime.Lobby.Drain()
	transport.SetReady(true)
	check(http.StatusServiceUnavailable)
}

// Exercise the actual executable's assembly, including its owner, release guard
// and trust adapters. This fixture is intentionally restricted to the runner's
// disposable database; it never points development identities at a deployment.
func textRuntimeProofDB(t *testing.T) (*sql.DB, *config.Config) {
	t.Helper()
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
	if err = store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(16)
	cfg := &config.Config{}
	raw, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = yaml.Unmarshal(raw, &cfg.Tuning); err != nil {
		t.Fatal(err)
	}
	cfg.App.Env = "test"
	cfg.Trust.UserTermsVersion = "runtime-proof-v1"
	cfg.WebSocket = config.WebSocketConfig{MaxMessageBytes: 8192, PongWaitS: 30, PingPeriodS: 5, WriteWaitS: 5}
	cfg.Text = &config.TextConfig{Version: 2, RulesVersion: "text-v1", Compatibility: config.TextCompatibility{ProtocolVersion: 2, MinClientGeneration: 2}, Modes: map[gamecontract.ModeID]config.TextModeConfig{}}
	return db, cfg
}

type runtimeProofPeer struct {
	account  string
	token    string
	conn     *websocket.Conn
	serial   int
	lobby    lobby.TextLobbyView
	snapshot v2.Snapshot
}

func (p *runtimeProofPeer) read(t *testing.T) lobby.TextEnvelope {
	t.Helper()
	if err := p.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var frame lobby.TextEnvelope
	if err := p.conn.ReadJSON(&frame); err != nil {
		t.Fatal(err)
	}
	if frame.Version != 2 || frame.Type == "error" {
		t.Fatalf("unexpected runtime frame type %q", frame.Type)
	}
	switch frame.Type {
	case "lobby":
		if err := json.Unmarshal(frame.Payload, &p.lobby); err != nil {
			t.Fatal(err)
		}
	case "snapshot":
		if err := json.Unmarshal(frame.Payload, &p.snapshot); err != nil {
			t.Fatal(err)
		}
	}
	return frame
}

func (p *runtimeProofPeer) control(t *testing.T, kind string, payload any) {
	t.Helper()
	p.serial++
	id := fmt.Sprintf("runtime-proof-%d", p.serial)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.conn.WriteJSON(lobby.TextEnvelope{Version: 2, Type: kind, RequestID: id, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	for {
		frame := p.read(t)
		if frame.Type == "control_ack" {
			var ack struct {
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(frame.Payload, &ack); err != nil || frame.RequestID != id || ack.RequestID != id {
				t.Fatal("mismatched control acknowledgement")
			}
			return
		}
	}
}

func startTextRuntimeProofMatch(t *testing.T, ctx context.Context, rt *textRuntime, db *sql.DB, cfg *config.Config, gates ...*runtimeLifecycle) string {
	t.Helper()
	authManager := auth.NewManager(db, []byte("disposable-runtime-proof-signing-key"), "runtime-proof", "runtime-client", time.Hour, 24*time.Hour, auth.OAuthProviders{})
	if err := authManager.ConfigureDevelopment(cfg.App.Env, rt.Lobby.Prototype()); err != nil {
		t.Fatal(err)
	}
	const developmentKey = "disposable-runtime-proof-development-key"
	mux := http.NewServeMux()
	handler.RegisterAuthRoutes(mux, handler.AuthDeps{Auth: authManager, DevBotKey: developmentKey})
	deps := handler.TextHandlerDeps{Config: cfg, Lobby: rt.Lobby, Auth: authManager, Deliveries: rt.Values, DeliveryWorker: rt.Lobby.Owner()}
	mux.HandleFunc("/api/text/availability", handler.TextAvailabilityHandler(deps))
	mux.HandleFunc("/ws/v2", handler.TextRealtimeHandler(deps))
	var routes http.Handler = mux
	if len(gates) == 1 {
		routes = gates[0].Handler(mux, transport.Deps{})
	} else if len(gates) != 0 {
		t.Fatal("expected one lifecycle gate")
	}
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	peers := make([]*runtimeProofPeer, 4)
	for i := range peers {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/auth/development", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+developmentKey)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var pair auth.TokenPair
		err = json.NewDecoder(response.Body).Decode(&pair)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || pair.AccessToken == "" {
			t.Fatal("runtime development authentication failed")
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/text/availability", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
		response, err = server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var availability lobby.TextAvailability
		err = json.NewDecoder(response.Body).Decode(&availability)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || !availability.Prototype || len(availability.Modes) != 5 {
			t.Fatal("runtime prototype discovery failed")
		}
		for _, mode := range availability.Modes {
			if !mode.Available || len(mode.Languages) != 1 || mode.Languages[0].PackReleaseID != "synthetic-text-en" {
				t.Fatal("runtime prototype advertised an unavailable or substituted mode")
			}
		}
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/v2", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		peers[i] = &runtimeProofPeer{conn: conn}
		if err = conn.WriteJSON(map[string]any{"v": 2, "type": "hello", "payload": map[string]any{"client_generation": 2, "access_token": pair.AccessToken}}); err != nil {
			t.Fatal(err)
		}
		hello := peers[i].read(t)
		var identity struct {
			AccountID string `json:"account_id"`
			Prototype bool   `json:"prototype"`
		}
		if err = json.Unmarshal(hello.Payload, &identity); err != nil || hello.Type != "hello" || identity.AccountID != pair.AccountID || !identity.Prototype {
			t.Fatal("runtime handshake did not bind the development identity")
		}
		if frame := peers[i].read(t); frame.Type != "availability" {
			t.Fatal("runtime handshake omitted pre-admission discovery")
		}
	}
	settings := v2.LobbySettings{ModeID: gamecontract.ModeMissedTheBriefing, Size: 4, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
	peers[0].control(t, "room_create", settings)
	code := peers[0].lobby.Code
	for _, peer := range peers[1:] {
		peer.control(t, "room_join", map[string]string{"code": code})
	}
	for _, peer := range peers {
		peer.control(t, "resync", struct{}{})
		state := peer.lobby.Lobby
		peer.control(t, "room_ready", v2.ReadyAcknowledgement{SettingsRevision: state.SettingsRevision, MembershipRevision: state.MembershipRevision})
	}
	peers[0].control(t, "room_start", struct{}{})
	match := peers[0].snapshot.Contract.MatchID
	for _, peer := range peers {
		peer.control(t, "resync", struct{}{})
		s := peer.snapshot
		if s.Contract.MatchID != match || s.Contract.Eligibility.Rewards || s.Contract.Eligibility.Leaderboard || len(s.Private.Hand) == 0 {
			t.Fatal("runtime did not start a pinned zero-value private match")
		}
	}
	if match == "" || rt.Lobby.ActiveMatches() != 1 {
		t.Fatal("runtime did not own exactly one active match")
	}
	return match
}

func TestTextRuntimeLifecycleJoinsActualSocketsAndOwnerHeartbeat(t *testing.T) {
	db, cfg := textRuntimeProofDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	rt, err := newTextRuntime(ctx, db, cfg, "../../pkg/media/testdata/text-en")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithCancel(context.Background())
		cancel()
		rt.Close(stop)
	})
	lifecycle := newRuntimeLifecycle()
	match := startTextRuntimeProofMatch(t, ctx, rt, db, cfg, lifecycle)
	if got := lifecycle.Requests.Active(); got != 4 {
		t.Fatalf("actual four live sockets not tracked: %d", got)
	}
	workerDone := make(chan struct{})
	if !lifecycle.Workers.Go(ctx, func(ctx context.Context) {
		defer close(workerDone)
		rt.Lobby.Run(ctx, nil)
	}) {
		t.Fatal("game clock not registered")
	}
	if err := lifecycle.Shutdown(100*time.Millisecond, rt.Close); err != nil {
		t.Fatal("socket/owner shutdown incomplete", err)
	}
	waitRuntimeSignal(t, workerDone)
	if lifecycle.Requests.Active() != 0 || lifecycle.Workers.Active() != 0 {
		t.Fatal("claimed shutdown before actual handlers and workers returned")
	}
	if err := rt.Owner.Wait(ctx); err != nil {
		t.Fatal("owner heartbeat remains live", err)
	}
	var state string
	var receipts, value int
	if err := db.QueryRowContext(ctx, `SELECT state FROM text_matches WHERE id=$1`, match).Scan(&state); err != nil || state != "interrupted" {
		t.Fatal("grace expiry omitted authoritative interruption", state, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM text_outbox WHERE match_id=$1 AND payload->>'interrupted'='true' AND payload->>'points'='0' AND payload->>'xp'='0' AND payload->>'leaderboard_counted'='false' AND payload->'awards'='[]'::jsonb`, match).Scan(&receipts); err != nil || receipts != 4 {
		t.Fatal("socket closure lost durable private interruption receipts", receipts, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM noin_ledger)+(SELECT count(*) FROM daily_quickplay_counts)+(SELECT COALESCE(sum(overall_points+non_converted_points+xp+matches_played),0) FROM profiles)`).Scan(&value); err != nil || value != 0 {
		t.Fatal("prototype shutdown fabricated value", value, err)
	}
}

func TestTextRuntimeRealAssemblyDrainAndLostOwnerRecovery(t *testing.T) {
	for _, loss := range []bool{false, true} {
		t.Run(fmt.Sprintf("owner_loss_%t", loss), func(t *testing.T) {
			db, cfg := textRuntimeProofDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			runtime, err := newTextRuntime(ctx, db, cfg, "../../pkg/media/testdata/text-en")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				stop, done := context.WithCancel(context.Background())
				done()
				runtime.Close(stop)
			})
			if second, err := newTextRuntime(ctx, db, cfg, "../../pkg/media/testdata/text-en"); err == nil || second != nil {
				t.Fatal("second executable runtime acquired live ownership")
			}
			match := startTextRuntimeProofMatch(t, ctx, runtime, db, cfg)
			if loss {
				var terminated bool
				if err := db.QueryRowContext(ctx, `SELECT pg_terminate_backend(backend_pid) FROM text_process_owners WHERE incarnation_id=$1`, runtime.Owner.Token().IncarnationID).Scan(&terminated); err != nil || !terminated {
					t.Fatal("could not simulate physical owner session loss", err)
				}
			} else {
				stop, done := context.WithCancel(ctx)
				done()
				if err := runtime.Close(stop); err != nil {
					t.Fatal("expired drain did not complete independent compensation", err)
				}
			}
			// Successor construction must itself finish confirmed-loss recovery;
			// no external worker or manual store recovery runs before these checks.
			next, err := newTextRuntime(ctx, db, cfg, "../../pkg/media/testdata/text-en")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { next.Close(context.Background()) })
			if next.Owner.Token().Generation <= runtime.Owner.Token().Generation || next.Lobby.ActiveMatches() != 0 {
				t.Fatal("successor reused ownership or reconstructed live gameplay")
			}
			var state string
			if err := db.QueryRowContext(ctx, `SELECT state FROM text_matches WHERE id=$1`, match).Scan(&state); err != nil || state != "interrupted" {
				t.Fatal("runtime reopened admission before interrupted recovery", state, err)
			}
			var interrupted, ledger, quota, value int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM text_outbox WHERE match_id=$1 AND payload->>'interrupted'='true' AND payload->>'points'='0' AND payload->>'xp'='0' AND payload->>'leaderboard_counted'='false' AND payload->'awards'='[]'::jsonb`, match).Scan(&interrupted); err != nil || interrupted != 4 {
				t.Fatal("interruption did not retain exactly four private zero-value receipts", interrupted, err)
			}
			if err := db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM noin_ledger), (SELECT count(*) FROM daily_quickplay_counts), (SELECT COALESCE(sum(overall_points+non_converted_points+xp+matches_played),0) FROM profiles)`).Scan(&ledger, &quota, &value); err != nil || ledger != 0 || quota != 0 || value != 0 {
				t.Fatal("runtime prototype manufactured live value", err)
			}
			if err := next.Close(ctx); err != nil {
				t.Fatal(err)
			}
			cfg.App.Env = "production"
			closed, err := newTextRuntime(ctx, db, cfg, "")
			if err != nil {
				t.Fatal(err)
			}
			defer closed.Close(ctx)
			if closed.Lobby.Prototype() {
				t.Fatal("ordinary startup inferred prototype mode")
			}
			for _, mode := range closed.Lobby.Availability(ctx).Modes {
				if mode.Available || len(mode.Languages) != 0 {
					t.Fatal("ordinary startup advertised an unreleased mode")
				}
			}
		})
	}
}

func TestTextRuntimeFinalModerationClosesSocketAndRevokesRefresh(t *testing.T) {
	db, cfg := textRuntimeProofDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rt, err := newTextRuntime(ctx, db, cfg, "../../pkg/media/testdata/text-en")
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)
	am := auth.NewManager(db, []byte("disposable-runtime-proof-signing-key"), "runtime-proof", "runtime-client", time.Hour, 24*time.Hour, auth.OAuthProviders{})
	if err = am.ConfigureDevelopment(cfg.App.Env, true); err != nil {
		t.Fatal(err)
	}
	target, err := am.CreateDevelopmentAccount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := am.CreateAnonymousAccount(ctx, "runtime-guard")
	if err != nil {
		t.Fatal(err)
	}
	adminAccount, err := am.CreateAnonymousAccount(ctx, "runtime-admin")
	if err != nil {
		t.Fatal(err)
	}
	adminID := uuid.NewString()
	if _, err = db.ExecContext(ctx, `INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret,role) VALUES($1,$2,'runtime@example.invalid','hash','secret','admin')`, adminID, adminAccount.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO portal_roles(account_id,role,granted_by) VALUES($1,'guard',$2)`, guard.AccountID, adminID); err != nil {
		t.Fatal(err)
	}
	pm := portal.NewManager(portal.Deps{DB: db, Config: cfg, Auth: am})
	rt.bindModeration(pm, am)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/v2", handler.TextRealtimeHandler(handler.TextHandlerDeps{Config: cfg, Lobby: rt.Lobby, Auth: am}))
	server := httptest.NewServer(mux)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/v2", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.WriteJSON(map[string]any{"v": 2, "type": "hello", "payload": map[string]any{"client_generation": 2, "access_token": target.AccessToken}}); err != nil {
		t.Fatal(err)
	}
	peer := runtimeProofPeer{conn: conn}
	peer.read(t)
	peer.read(t)
	peer.control(t, "room_create", v2.LobbySettings{ModeID: gamecontract.ModeMissedTheBriefing, Size: 4, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"})
	if err = pm.FreezeAccount(ctx, guard.AccountID, target.AccountID, "conduct review"); err != nil {
		t.Fatal(err)
	}
	// A Guard freeze cannot end a current session or revoke identity credentials.
	if _, err = am.ValidateAccessToken(ctx, target.AccessToken); err != nil {
		t.Fatal("Guard revoked token", err)
	}
	peer.control(t, "resync", struct{}{})
	freezes, err := pm.ListActiveFreezes(ctx)
	if err != nil || len(freezes) != 1 {
		t.Fatal(freezes, err)
	}
	freeze := freezes[0]["id"].(string)
	if err = pm.ConvertFreezeToBan(ctx, adminID, freeze, "confirmed conduct"); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err = conn.ReadMessage(); err == nil {
		t.Fatal("admin-final action left socket open")
	}
	var delivered bool
	var epoch int
	if err = db.QueryRowContext(ctx, `SELECT disconnect_delivered_at IS NOT NULL FROM guard_freezes WHERE id=$1`, freeze).Scan(&delivered); err != nil || !delivered {
		t.Fatal("missing delivery receipt", err)
	}
	if err = db.QueryRowContext(ctx, `SELECT session_epoch FROM accounts WHERE id=$1`, target.AccountID).Scan(&epoch); err != nil || epoch != 1 {
		t.Fatal("missing session revocation", epoch, err)
	}
	if err = pm.ConvertFreezeToBan(ctx, adminID, freeze, "confirmed conduct"); err != nil {
		t.Fatal(err)
	}
	// Lifting a sanction does not resurrect credentials issued before it.
	if _, err = db.ExecContext(ctx, `UPDATE accounts SET banned_at=NULL WHERE id=$1`, target.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = am.Refresh(ctx, target.RefreshToken); err == nil {
		t.Fatal("ban lift resurrected revoked refresh")
	}
	if err = db.QueryRowContext(ctx, `SELECT session_epoch FROM accounts WHERE id=$1`, target.AccountID).Scan(&epoch); err != nil || epoch != 1 {
		t.Fatal("duplicate final action repeated revocation", epoch, err)
	}
}

// This exercises production-shaped persistence with explicitly simulated review
// evidence. It is not a publishable pack or proof of human editorial acceptance.
func runtimeReviewedFixture(t *testing.T, db *sql.DB, cfg *config.Config) (string, *media.TextSnapshot) {
	t.Helper()
	ctx := context.Background()
	account, adminID := uuid.NewString(), uuid.NewString()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO accounts(id,nickname) VALUES($1,'Fixture editor')`, []any{account}},
		{`INSERT INTO profiles(account_id) VALUES($1)`, []any{account}},
		{`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,'fixture@example.invalid','test','test')`, []any{adminID, account}},
		{`INSERT INTO portal_terms(version,title,body) VALUES('runtime-contribution-fixture','Test only','Simulated fixture consent')`, nil},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	limits := media.TextLimits{MaxTextBytes: cfg.Tuning.Contract.MaxTextBytes, MaxRecords: cfg.Tuning.TextCatalog.MaxRecords, MaxFileBytes: cfg.Tuning.TextCatalog.MaxFileBytes, MaxBundleBytes: cfg.Tuning.TextCatalog.MaxBundleBytes}
	dealing := media.TextDealTuning{HandSize: cfg.Tuning.Hand.Size, ReserveSize: cfg.Tuning.Hand.DrawPile, MinHigh: cfg.Tuning.Dealing.MinHighPerNown, MinDistant: cfg.Tuning.Dealing.MinDistantPerNown, MaxSearchNodes: cfg.Tuning.TextCatalog.MaxSearchNodes}
	releases := store.NewTextReleaseStore(db, cfg.Tuning, func(context.Context, string) error { return nil })
	snapshot, err := media.LoadTextPack("../../pkg/media/testdata/text-en", limits)
	if err != nil {
		t.Fatal(err)
	}
	bundle := snapshot.Bundle()
	bundle.Manifest.Synthetic = false
	bundle.Manifest.ReleaseID = "runtime-persisted-test-only"
	accept := func(content string) media.TextProvenance {
		id := uuid.NewString()
		if _, err := db.Exec(`INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at,decided_at,decided_by) VALUES($1,$2,'text',$3,'approved','runtime-contribution-fixture',now()-interval '1 day',now(),$4)`, id, account, content, adminID); err != nil {
			t.Fatal(err)
		}
		provenance, err := releases.CaptureAccepted(ctx, adminID, "portal_submission", id)
		if err != nil {
			t.Fatal(err)
		}
		return provenance
	}
	for i := range bundle.Nowns {
		bundle.Nowns[i].Provenance = accept(bundle.Nowns[i].Text)
	}
	for i := range bundle.Cards {
		bundle.Cards[i].Provenance = accept(bundle.Cards[i].Text)
	}
	bundle.Manifest.CertificationArtifacts = nil
	bundle.Artifacts = map[string][]byte{}
	sealed, err := media.SealTextBundle(bundle, limits)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = media.NewTextSnapshot(sealed, limits)
	if err != nil {
		t.Fatal(err)
	}
	sealed.Manifest.CertificationArtifacts = map[string]string{}
	sealed.Artifacts["technical.json"], err = json.Marshal(media.CertifyText(snapshot, dealing, 20, 71))
	if err != nil {
		t.Fatal(err)
	}
	sealed.Artifacts["replay.json"], err = json.Marshal(media.NewTextReplay(snapshot, dealing, 20, 71))
	if err != nil {
		t.Fatal(err)
	}
	for _, gate := range []string{"editorial", "actions", "screening", "release"} {
		evidence := media.TextGateEvidence{SchemaVersion: 2, SnapshotSHA256: snapshot.SHA256(), RulesVersion: bundle.Manifest.RulesVersion, Language: bundle.Manifest.Language, Gate: gate, ActorReference: "simulated-runtime-test-only", TuningSHA256: media.TextTuningSHA256(dealing)}
		for _, mode := range bundle.Manifest.Modes {
			for _, size := range []int{4, 6} {
				evidence.Cells = append(evidence.Cells, media.TextGateCell{Mode: mode, TableSize: size, Passed: true, RecordReference: "simulated-test-only", EvidenceSHA256: media.ContentHash([]byte("simulated runtime fixture"))})
			}
		}
		sealed.Artifacts[gate+".json"], err = json.Marshal(evidence)
		if err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range sealed.Artifacts {
		sealed.Manifest.CertificationArtifacts[name] = media.ContentHash(data)
	}
	sealed, err = media.SealTextBundle(sealed, limits)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = media.NewTextSnapshot(sealed, limits)
	if err != nil {
		t.Fatal(err)
	}
	if err = releases.Publish(ctx, adminID, snapshot, store.TextPackAccess{Class: "core"}); err != nil {
		t.Fatal(err)
	}
	if err = releases.Activate(ctx, adminID, snapshot.Manifest().ReleaseID); err != nil {
		t.Fatal(err)
	}
	return adminID, snapshot
}

func TestTextRuntimePersistedReleaseAdmissionAllCellsAndWithdrawal(t *testing.T) {
	db, cfg := textRuntimeProofDB(t)
	adminID, pack := runtimeReviewedFixture(t, db, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cfg.App.Env = "prod"
	for _, mode := range gamecontract.AllModes() {
		cfg.Text.Modes[mode] = config.TextModeConfig{Enabled: true, ContentLanguages: []string{"en"}}
	}
	rt, err := newTextRuntime(ctx, db, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		stop, done := context.WithCancel(context.Background())
		done()
		if err := rt.Close(stop); err != nil {
			t.Error(err)
		}
	}()
	am := auth.NewManager(db, []byte("disposable-runtime-proof-signing-key"), "runtime-proof", "runtime-client", time.Hour, 24*time.Hour, auth.OAuthProviders{})
	deps := handler.TextHandlerDeps{Config: cfg, Lobby: rt.Lobby, Auth: am, Deliveries: rt.Values, DeliveryWorker: rt.Lobby.Owner()}
	mux := http.NewServeMux()
	handler.RegisterTextRealtimeRoutes(mux, deps)
	handler.RegisterPublicRoutes(mux, handler.PublicRouteDeps{Auth: am, Reports: reports.NewManager(db), VisibleText: rt.Lobby.VisibleText, Notices: notices.NewManager(db, cfg, rt.Lobby), Avatar: avatar.NewManager(db, cfg, nil)})
	econ := economy.NewManager(db, cfg)
	handler.RegisterEconomyRoutes(mux, handler.EconomyDeps{Config: cfg, Auth: am, Economy: econ, WalletAccess: rt.Lobby.WithWalletAccess})
	server := httptest.NewServer(mux)
	defer server.Close()
	var hosts []*runtimeProofPeer
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			peers := make([]*runtimeProofPeer, size)
			for i := range peers {
				pair, err := am.CreateAnonymousAccount(ctx, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
				conn, _, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/v2", nil)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				peer := &runtimeProofPeer{conn: conn, token: pair.AccessToken, account: pair.AccountID}
				peers[i] = peer
				if err = conn.WriteJSON(map[string]any{"v": 2, "type": "hello", "payload": map[string]any{"client_generation": 2, "access_token": pair.AccessToken}}); err != nil {
					t.Fatal(err)
				}
				hello := peer.read(t)
				var identity struct {
					Prototype bool `json:"prototype"`
				}
				if err = json.Unmarshal(hello.Payload, &identity); err != nil || identity.Prototype {
					t.Fatal("ordinary account became prototype", err)
				}
				frame := peer.read(t)
				var available lobby.TextAvailability
				if err = json.Unmarshal(frame.Payload, &available); err != nil || available.Prototype || len(available.Modes) != 5 {
					t.Fatal("persisted discovery", err)
				}
				for _, cell := range available.Modes {
					if !cell.Available || len(cell.Languages) != 1 || cell.Languages[0].PackReleaseID != pack.Manifest().ReleaseID {
						t.Fatal("missing certified persisted cell", cell.ModeID)
					}
				}
			}
			settings := v2.LobbySettings{ModeID: mode, Size: size, ContentLanguage: "en", PackReleaseID: pack.Manifest().ReleaseID, RulesVersion: "text-v1"}
			peers[0].control(t, "room_create", settings)
			for _, peer := range peers[1:] {
				peer.control(t, "room_join", map[string]string{"code": peers[0].lobby.Code})
			}
			for _, peer := range peers {
				peer.control(t, "resync", struct{}{})
				state := peer.lobby.Lobby
				peer.control(t, "room_ready", v2.ReadyAcknowledgement{SettingsRevision: state.SettingsRevision, MembershipRevision: state.MembershipRevision})
			}
			peers[0].control(t, "room_start", struct{}{})
			for _, peer := range peers {
				peer.control(t, "resync", struct{}{})
				s := peer.snapshot
				if s.Contract.ModeID != mode || s.Contract.OriginalSize != size || s.Contract.PackSHA256 != pack.SHA256() || !s.Contract.Eligibility.Rewards || len(s.Private.Hand) != 5 {
					t.Fatal("persisted runtime contract/hand", s.Contract)
				}
			}
			hosts = append(hosts, peers[0])
		}
	}
	if rt.Lobby.ActiveMatches() != 10 {
		t.Fatal("missing started cells", rt.Lobby.ActiveMatches())
	}
	postReport := func(token string, host *runtimeProofPeer, ref v2.ContentRef, want int) string {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"report_type": "media", "target_text": reports.TextTargetRequest{MatchID: host.snapshot.Contract.MatchID, ContentRef: ref}, "reason": "test-only content case"})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/reports", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != want {
			t.Fatalf("content report %d want %d: %s (%v)", resp.StatusCode, want, body, err)
		}
		return string(body)
	}
	outsider, err := am.CreateAnonymousAccount(ctx, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		ref := host.snapshot.Private.Hand[0].Content.ContentRef
		// Hidden-but-existing release members and invented revisions produce the
		// same response as an outsider's guess, without storing any report.
		denied := postReport(outsider.AccessToken, host, ref, http.StatusBadRequest)
		wrong := ref
		wrong.Revision++
		if got := postReport(host.token, host, wrong, http.StatusBadRequest); got != denied {
			t.Fatal("revision existence oracle")
		}
		for _, nown := range pack.Bundle().Nowns {
			hidden := v2.ContentRef{ContentID: v2.ContentID(nown.ID), Revision: nown.Revision}
			if host.snapshot.Private.Nown != nil && host.snapshot.Private.Nown.ContentRef == hidden {
				continue
			}
			if got := postReport(host.token, host, hidden, http.StatusBadRequest); got != denied {
				t.Fatal("hidden Nown existence oracle")
			}
			break
		}
		postReport(host.token, host, ref, http.StatusNoContent)
	}
	if err = rt.Releases.Takedown(ctx, adminID, pack.Manifest().ReleaseID, "test-only withdrawal"); err != nil {
		t.Fatal(err)
	}
	for _, cell := range rt.Lobby.Availability(ctx).Modes {
		if cell.Available || len(cell.Languages) != 0 {
			t.Fatal("withdrawn cell advertised")
		}
	}
	for _, host := range hosts {
		prior := host.snapshot.Contract
		host.control(t, "resync", struct{}{})
		if host.snapshot.Contract != prior || len(host.snapshot.Private.Hand) != 5 {
			t.Fatal("withdrawal rewrote begun match")
		}
		// Exact retry still resolves the immutable pinned content after takedown.
		postReport(host.token, host, host.snapshot.Private.Hand[0].Content.ContentRef, http.StatusNoContent)
	}
	var reportCount int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM reports WHERE text_target IS NOT NULL`).Scan(&reportCount); err != nil || reportCount != 10 {
		t.Fatal("content reports duplicated or hidden target stored", reportCount, err)
	}
	var quota, ledger int
	if err = db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM daily_quickplay_counts),(SELECT count(*) FROM noin_ledger)`).Scan(&quota, &ledger); err != nil || quota != 0 || ledger != 0 {
		t.Fatal("local admission or publication produced value", quota, ledger, err)
	}
	wallet := func(routes http.Handler, token string, want int) map[string]any {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/economy/wallet", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("wallet status", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	for _, host := range hosts {
		if body := wallet(mux, host.token, http.StatusConflict); len(body) != 1 || body["code"] != "wallet.match_in_progress" {
			t.Fatal("live wallet revealed amounts", body)
		}
	}
	if body := wallet(mux, outsider.AccessToken, http.StatusOK); body["balance"] != float64(0) {
		t.Fatal("outsider wallet", body)
	}
	// Exercise the real admin callback assembly while all ten certified-fixture
	// matches still run. A planned drain cannot turn them into interruptions.
	cfg.Security.AdminSessionTTLH = 1
	adminManager := admin.NewManager(db, cfg, nil)
	session, csrf, _, err := adminManager.CreateSession(ctx, adminID)
	if err != nil {
		t.Fatal(err)
	}
	adminRoutes := adminManager.RuntimeHandler(rt.adminRuntimeHooks())
	drainBody, err := json.Marshal(map[string]any{"owner_id": rt.Owner.Token().IncarnationID, "generation": rt.Owner.Token().Generation, "request_id": uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	operate := func(routes http.Handler, method, path string, body []byte, want int) admin.RuntimeStatus {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "knowoff_admin_session", Value: session})
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatal("actual runtime operation", w.Code, w.Body.String())
		}
		var status admin.RuntimeStatus
		if want == http.StatusOK || want == http.StatusAccepted {
			if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
		}
		return status
	}
	for i := 0; i < 2; i++ {
		status := operate(adminRoutes, "POST", "/admin/runtime/drain", drainBody, http.StatusOK)
		if !status.Process.AdmissionClosed || status.Process.ActiveMatches != 10 || status.Durable.StartedMatches != 10 || status.MatchesDrained || status.WritersQuiescent {
			t.Fatal("actual drain counts", status)
		}
	}
	if status := operate(adminRoutes, "GET", "/admin/runtime/wait?timeout_ms=30", nil, http.StatusAccepted); !status.TimedOut || status.Process.ActiveMatches != 10 {
		t.Fatal("false/interrupting drain wait", status)
	}
	for _, p := range hosts {
		p.control(t, "resync", struct{}{})
		if p.snapshot.Phase == v2.PhaseVerdict {
			t.Fatal("planned wait interrupted match")
		}
	}
	// Inject one real durable occurrence through the production value boundary.
	// This fixture proves HTTP privacy/recovery, not an additional engine vote.
	host := hosts[0]
	credited, err := rt.Values.Award(ctx, store.TextAward{MatchID: host.snapshot.Contract.MatchID, Owner: rt.Lobby.Owner(), Epoch: 1, AccountID: host.account, Kind: "correct_vote", Ordinal: 1, Amount: cfg.Tuning.Noin.CorrectVote, At: time.Now().UTC()})
	if err != nil || credited != cfg.Tuning.Noin.CorrectVote {
		t.Fatal("durable occurrence", credited, err)
	}
	if body := wallet(mux, host.token, http.StatusConflict); len(body) != 1 {
		t.Fatal("occurrence wallet oracle", body)
	}
	host.control(t, "room_leave", struct{}{})
	if body := wallet(mux, host.token, http.StatusConflict); len(body) != 1 {
		t.Fatal("leave wallet oracle", body)
	}
	stop, stopped := context.WithCancel(ctx)
	stopped()
	if err = rt.Close(stop); err != nil {
		t.Fatal(err)
	}
	next, err := newTextRuntime(ctx, db, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close(ctx)
	nextAdmin := adminManager.RuntimeHandler(next.adminRuntimeHooks())
	operate(nextAdmin, "POST", "/admin/runtime/drain", drainBody, http.StatusConflict)
	status := operate(nextAdmin, "GET", "/admin/runtime/status", nil, http.StatusOK)
	if status.MatchesDrained || status.WritersQuiescent || status.Durable.StartedMatches != 0 || status.Durable.PendingSettlements != 0 || status.Durable.Undelivered != 50 {
		t.Fatal("successor drain/durable delivery", status)
	}
	recovered := http.NewServeMux()
	handler.RegisterEconomyRoutes(recovered, handler.EconomyDeps{Config: cfg, Auth: am, Economy: econ, WalletAccess: next.Lobby.WithWalletAccess})
	if body := wallet(recovered, host.token, http.StatusOK); body["balance"] != float64(cfg.Tuning.Noin.CorrectVote) || body["daily_earned"] != float64(cfg.Tuning.Noin.CorrectVote) {
		t.Fatal("interruption lost private occurrence", body)
	}
}

func TestTextRuntimeRejectsIncompatibleSchemaBeforeOwnership(t *testing.T) {
	for _, state := range []string{"dirty", "older", "future", "empty", "ambiguous", "missing"} {
		t.Run(state, func(t *testing.T) {
			db, cfg := textRuntimeProofDB(t)
			var version int64
			if err := db.QueryRow(`SELECT version FROM schema_migrations`).Scan(&version); err != nil {
				t.Fatal(err)
			}
			query := ""
			switch state {
			case "dirty":
				query = `UPDATE schema_migrations SET dirty=true`
			case "older":
				query = `UPDATE schema_migrations SET version=version-1`
			case "future":
				query = `UPDATE schema_migrations SET version=version+1`
			case "empty":
				query = `DELETE FROM schema_migrations`
			case "ambiguous":
				query = `INSERT INTO schema_migrations(version,dirty) SELECT version+1,false FROM schema_migrations`
			case "missing":
				query = `ALTER TABLE schema_migrations RENAME TO fixture_hidden_migrations`
			}
			if _, err := db.Exec(query); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if state == "missing" {
					if _, err := db.Exec(`ALTER TABLE fixture_hidden_migrations RENAME TO schema_migrations`); err != nil {
						t.Error(err)
					}
				}
			})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			rt, err := newTextRuntime(ctx, db, cfg, "")
			if rt != nil {
				if closeErr := rt.Close(ctx); closeErr != nil {
					t.Error(closeErr)
				}
			}
			if err == nil || rt != nil {
				t.Errorf("%s schema started runtime", state)
			}
			var owners int
			if err := db.QueryRow(`SELECT count(*) FROM text_process_owners`).Scan(&owners); err != nil || owners != 0 {
				t.Errorf("schema refusal wrote ownership: %d %v", owners, err)
			}
		})
	}
}
