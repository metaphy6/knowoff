package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/admin"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/handler"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/store"
)

func TestTextRuntimeAdminSanctionClosesSocketAndExactLift(t *testing.T) {
	db, cfg := textRuntimeProofDB(t)
	cfg.Security.AdminSessionTTLH = 1
	cfg.Security.AdminTOTPIssuer = "disposable-sanctions"
	rt, err := newTextRuntime(t.Context(), db, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	am := auth.NewManager(db, []byte("disposable-sanction-runtime-signing"), "test", "test", time.Hour, 24*time.Hour, auth.OAuthProviders{})
	em := economy.NewManager(db, cfg)
	ad := admin.NewManager(db, cfg, nil)
	player, err := am.AuthenticateDevice(t.Context(), auth.HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	actor, err := am.AuthenticateDevice(t.Context(), auth.HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	if err = ad.CreateAdmin(t.Context(), actor.AccountID, "runtime-admin@test.invalid", "runtime-password", "admin"); err != nil {
		t.Fatal(err)
	}
	administrator, err := ad.Authenticate(t.Context(), "runtime-admin@test.invalid", "runtime-password")
	if err != nil {
		t.Fatal(err)
	}
	sid, csrf, _, err := ad.CreateSession(t.Context(), administrator.ID)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/admin/operators", ad.OperatorHandler(rt.operatorHooks(db, am, em)))
	handler.RegisterTextRealtimeRoutes(mux, handler.TextHandlerDeps{Config: cfg, Lobby: rt.Lobby, Auth: am})
	server := httptest.NewServer(mux)
	defer server.Close()
	open := func(token string) *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/v2", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ws.Close() })
		if err = ws.WriteJSON(map[string]any{"v": 2, "type": "hello", "payload": map[string]any{"client_generation": 2, "access_token": token}}); err != nil {
			t.Fatal(err)
		}
		ws.SetReadDeadline(time.Now().Add(3 * time.Second))
		for i := 0; i < 2; i++ {
			var f lobby.TextEnvelope
			if err = ws.ReadJSON(&f); err != nil || f.Type == "error" {
				t.Fatal("hello failed", err, f.Type)
			}
		}
		return ws
	}
	ws := open(player.AccessToken)
	post := func(id, kind, prior string) int {
		t.Helper()
		v := url.Values{"csrf_token": {csrf}, "id": {id}, "kind": {kind}, "target_account_id": {player.AccountID}, "reason": {"Runtime exact decision"}}
		if prior != "" {
			v.Set("prior_sanction_id", prior)
		}
		req, err := http.NewRequest("POST", server.URL+"/admin/operators", strings.NewReader(v.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "knowoff_admin_session", Value: sid})
		client := *server.Client()
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	decision := uuid.NewString()
	if got := post(decision, "account_sanction", ""); got != 303 {
		t.Fatal("sanction HTTP", got)
	}
	ws.SetReadDeadline(time.Now().Add(time.Second))
	var frame json.RawMessage
	if err = ws.ReadJSON(&frame); err == nil {
		t.Fatal("sanction retained live socket")
	}
	receipt, err := store.NewAdminOperationStore(db).Get(t.Context(), decision)
	if err != nil || receipt.Status != "applied" {
		t.Fatal(receipt.Status, err)
	}
	if got := post(uuid.NewString(), "sanction_lift", decision); got != 303 {
		t.Fatal("lift HTTP", got)
	}
	if _, err = am.ValidateAccessToken(t.Context(), player.AccessToken); err == nil {
		t.Fatal("lift revived revoked credential")
	}
	var hash string
	if err = db.QueryRow(`SELECT device_hash FROM device_tokens WHERE account_id=$1`, player.AccountID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	fresh, err := am.AuthenticateDevice(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	replacement := open(fresh.AccessToken)
	if err = rt.sanctionDelivery(store.NewAccountSanctionStore(db))(t.Context(), decision); err != nil {
		t.Fatal(err)
	}
	if err = replacement.WriteJSON(map[string]any{"v": 2, "type": "availability", "request_id": "live", "payload": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	var response lobby.TextEnvelope
	if err = replacement.ReadJSON(&response); err != nil {
		t.Fatal("stale receipt closed replacement socket", err)
	}
	var active bool
	if err = db.QueryRow(`SELECT direct_account_sanction_active($1)`, player.AccountID).Scan(&active); err != nil || active {
		t.Fatal("lift not effective", err)
	}
}
