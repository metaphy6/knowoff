package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/admin"
	authservice "github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/lib/pq"
)

func operatorHTTPDatabase(t *testing.T) (*sql.DB, *authservice.Manager) {
	t.Helper()
	// The shared fixture first verifies both runner token and actual DB identity.
	// Isolate ownership startup from earlier intentionally ownerless proof rows.
	_, economy, _, cleanup := setupEconomyHandlerTest(t)
	t.Cleanup(cleanup)
	parent := economy.DB()
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	var actual string
	if err = parent.QueryRow(`SELECT current_database()`).Scan(&actual); err != nil || actual != "knowoff_test_"+os.Getenv("KNOWOFF_TEST_DB_TOKEN") {
		t.Fatal("unexpected disposable parent", err)
	}
	name := actual + "_op_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err = parent.ExecContext(ctx, `CREATE DATABASE `+pq.QuoteIdentifier(name)); err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := parent.ExecContext(ctx, `DROP DATABASE `+pq.QuoteIdentifier(name)+` WITH (FORCE)`); err != nil {
			t.Error(err)
		}
	})
	if err = db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&actual); err != nil || actual != name {
		t.Fatal("unexpected disposable child", err)
	}
	if err = store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return db, authservice.NewManager(db, []byte("test-key-test-key-test-key-test"), "test", "test", time.Minute, time.Hour, authservice.OAuthProviders{})
}

// Both boundaries are real: an exact administrator cookie/CSRF HTTP request
// affects authenticated development-purpose WebSockets and the disposable DB.
// Synthetic content and zero prototype awards do not certify a live release.
func TestTextOperatorHTTPClosesAuthenticatedSocketsAllCells(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				db, auth := operatorHTTPDatabase(t)
				if err := auth.ConfigureDevelopment("test", true); err != nil {
					t.Fatal(err)
				}
				owner, err := store.AcquireTextOwner(t.Context(), db)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = owner.Release(context.Background()) })
				var values *store.TextValueStore
				srv, _, manager := textHTTPCustomFixture(t, auth, func(cfg *config.Config, d *lobby.TextDeps) {
					cfg.Text.Modes[mode] = config.TextModeConfig{Enabled: true, ContentLanguages: []string{"en"}}
					var e error
					values, e = store.NewTextValueStore(db, cfg.Tuning).WithOwner(owner)
					if e != nil {
						t.Fatal(e)
					}
					recovered, e := owner.RecoverLostOwners(t.Context(), values, 100)
					if e != nil || !recovered.Done {
						t.Fatal(recovered, e)
					}
					d.Owner = owner.Token().IncarnationID
					d.Authority = owner
					d.Values = values
					d.Operations = store.NewAdminOperationStore(db)
				})
				cfg := &config.Config{Security: config.SecurityConfig{BcryptCost: 4, AdminTOTPIssuer: "Fixture", AdminSessionTTLH: 1}}
				console := admin.NewManager(db, cfg, nil)
				administrator, err := auth.CreateDevelopmentAccount(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				email := administrator.AccountID + "@fixture.invalid"
				if err = console.CreateAdmin(t.Context(), administrator.AccountID, email, "fixture-password", "admin"); err != nil {
					t.Fatal(err)
				}
				actor, err := console.Authenticate(t.Context(), email, "fixture-password")
				if err != nil {
					t.Fatal(err)
				}
				sid, csrf, _, err := console.CreateSession(t.Context(), actor.ID)
				if err != nil {
					t.Fatal(err)
				}
				internal := httptest.NewServer(console.OperatorHandler(admin.OperatorHooks{Decide: manager.DecideRoomOperation}))
				t.Cleanup(internal.Close)
				client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
				peers := make([]*websocket.Conn, size)
				accounts, tokens := make([]string, size), make([]string, size)
				for i := range peers {
					pair, e := auth.CreateDevelopmentAccount(t.Context())
					if e != nil {
						t.Fatal(e)
					}
					accounts[i], tokens[i] = pair.AccountID, pair.AccessToken
					peers[i] = textDial(t, srv)
					textHello(t, peers[i], tokens[i])
				}
				settings := v2.LobbySettings{ModeID: mode, Size: size, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
				room := textLastLobby(t, textControl(t, peers[0], "room_create", "create", settings))
				for _, p := range peers[1:] {
					textControl(t, p, "room_join", "join", map[string]string{"code": room.Code})
				}
				for i, p := range peers {
					view := textLastLobby(t, textControl(t, p, "resync", fmt.Sprintf("sync-%d", i), struct{}{}))
					textControl(t, p, "room_ready", fmt.Sprintf("ready-%d", i), v2.ReadyAcknowledgement{SettingsRevision: view.Lobby.SettingsRevision, MembershipRevision: view.Lobby.MembershipRevision})
				}
				frames := textControl(t, peers[0], "room_start", "start", struct{}{})
				var first v2.Snapshot
				for _, f := range frames {
					if f.Type == "snapshot" {
						if err = json.Unmarshal(f.Payload, &first); err != nil {
							t.Fatal(err)
						}
					}
				}
				if first.Contract.MatchID == "" {
					t.Fatal("missing first snapshot")
				}
				for _, p := range peers[1:] {
					textNextSnapshot(t, p)
				}
				token := owner.Token()
				post := func(kind, target, key string) (string, int) {
					id := uuid.NewString()
					form := url.Values{"id": {id}, "kind": {kind}, "room_id": {first.Contract.RoomID}, "owner_id": {token.IncarnationID}, "owner_generation": {strconv.FormatInt(token.Generation, 10)}, "reason": {"Reviewed fixture intervention"}}
					if target != "" {
						form.Set("target_account_id", target)
					}
					request, e := http.NewRequest(http.MethodPost, internal.URL+"/admin/operators", strings.NewReader(form.Encode()))
					if e != nil {
						t.Fatal(e)
					}
					request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					request.Header.Set("X-CSRF-Token", key)
					request.AddCookie(&http.Cookie{Name: "knowoff_admin_session", Value: sid})
					response, e := client.Do(request)
					if e != nil {
						t.Fatal(e)
					}
					defer response.Body.Close()
					body, e := io.ReadAll(io.LimitReader(response.Body, 65537))
					if e != nil || len(body) > 65536 {
						t.Fatal("unbounded operator response", e)
					}
					for _, secret := range tokens {
						if strings.Contains(string(body), secret) {
							t.Fatal("credential in administrator response")
						}
					}
					return id, response.StatusCode
				}
				if _, status := post("room_kick", accounts[size-1], "invalid"); status != http.StatusForbidden {
					t.Fatal("CSRF refusal", status)
				}
				kick, status := post("room_kick", accounts[size-1], csrf)
				if status != http.StatusSeeOther {
					t.Fatal("kick HTTP", status)
				}
				kicked, err := store.NewAdminOperationStore(db).Get(t.Context(), kick)
				if err != nil || kicked.Status != "applied" {
					t.Fatal("kick not applied", err)
				}
				expectClosed := func(c *websocket.Conn) {
					c.SetReadDeadline(time.Now().Add(time.Second))
					for i := 0; i < 100; i++ {
						var frame lobby.TextEnvelope
						if e := c.ReadJSON(&frame); e != nil {
							if timeout, ok := e.(interface{ Timeout() bool }); ok && timeout.Timeout() {
								t.Fatal("socket never closed")
							}
							return
						}
					}
					t.Fatal("closed socket retained unbounded frames")
				}
				expectClosed(peers[size-1])
				closed, status := post("room_close", "", csrf)
				if status != http.StatusSeeOther {
					t.Fatal("close HTTP", status)
				}
				receipt, err := store.NewAdminOperationStore(db).Get(t.Context(), closed)
				if err != nil || receipt.Status != "applied" {
					t.Fatal("close not applied", err)
				}
				for _, p := range peers[:size-1] {
					expectClosed(p)
				}
				reconnect := textDial(t, srv)
				textHello(t, reconnect, tokens[0])
				if err = manager.PumpDeliveries(t.Context(), values); err != nil {
					t.Fatal(err)
				}
				delivery := textRead(t, reconnect)
				if delivery.Type != "settlement" {
					t.Fatal("missing private interruption delivery", delivery.Type)
				}
				var private struct {
					ID         int64                       `json:"id"`
					Match      string                      `json:"match_id"`
					Settlement store.TextPrivateSettlement `json:"settlement"`
				}
				if err = json.Unmarshal(delivery.Payload, &private); err != nil || private.Match != first.Contract.MatchID || !private.Settlement.Interrupted || private.Settlement.Points != 0 || private.Settlement.XP != 0 || private.Settlement.LeaderboardCounted {
					t.Fatal("invalid private interruption", err)
				}
				var recipient string
				if err = db.QueryRow(`SELECT account_id FROM text_outbox WHERE id=$1`, private.ID).Scan(&recipient); err != nil || recipient != accounts[0] {
					t.Fatal("cross-recipient outbox", err)
				}
				for _, account := range accounts {
					if strings.Contains(string(delivery.Payload), account) {
						t.Fatal("recipient list leaked")
					}
				}
			})
		}
	}
}
