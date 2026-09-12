package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
)

func TestSafetyHTTPExplicitConsentPrivateBlocksAndMatchIdentity(t *testing.T) {
	if os.Getenv("KNOWOFF_TEST_DSN") == "" {
		t.Fatal("disposable PostgreSQL required")
	}
	_, econ, authMgr, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	db := econ.DB()
	ctx := context.Background()
	actor, token := seedAccountForEconomy(t, ctx, db, authMgr)
	target, otherToken := seedAccountForEconomy(t, ctx, db, authMgr)
	version := "safety-" + uuid.NewString()
	cfg := config.TrustConfig{UserTermsVersion: version, SupportURL: "https://example.invalid/support", PrivacyURL: "https://example.invalid/privacy"}
	mux := http.NewServeMux()
	roomID := uuid.NewString()
	RegisterSafetyRoutes(mux, SafetyDeps{Auth: authMgr, Trust: store.NewTextTrustStore(db), Config: cfg, RoomAccount: func(_ context.Context, viewer, room string, seat int) (string, error) {
		if viewer == actor && room == roomID && seat == 1 {
			return target, nil
		}
		return "", errors.New("membership unavailable")
	}})
	request := func(method, path, body, access string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if access != "" {
			r.Header.Set("Authorization", "Bearer "+access)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private safety response cacheable", w.Code)
		}
		return w
	}
	for _, path := range []string{"/api/safety", "/api/safety/blocks"} {
		if r := request("GET", path, "", ""); r.Code != 401 {
			t.Fatal("unauthenticated safety", r.Code)
		}
	}
	if r := request("GET", "/api/safety/help", "", ""); r.Code != 200 || !strings.Contains(r.Body.String(), cfg.SupportURL) || !strings.Contains(r.Body.String(), cfg.PrivacyURL) || strings.Contains(r.Body.String(), "terms") || strings.Contains(r.Body.String(), actor) {
		t.Fatal("signed-out help unavailable or discloses account state", r.Code, r.Body.String())
	}
	read := func() map[string]any {
		r := request("GET", "/api/safety", "", token)
		if r.Code != 200 {
			t.Fatal(r.Code, r.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if v := read()["terms"].(map[string]any); v["available"] != false || v["accepted"] != false || v["body"] != "" {
		t.Fatal(v)
	}
	if r := request("POST", "/api/safety/terms", `{"version":"`+version+`"}`, token); r.Code != 409 {
		t.Fatal("unpublished consent", r.Code)
	}
	const terms = "Test only user terms: <script>inert</script> — İstanbul العربية"
	if _, err := db.Exec(`INSERT INTO user_terms_versions(version,body,active_from) VALUES($1,$2,now()-interval '1 minute')`, version, terms); err != nil {
		t.Fatal(err)
	}
	if v := read()["terms"].(map[string]any); v["available"] != true || v["accepted"] != false || v["body"] != terms {
		t.Fatal(v)
	}
	for _, body := range []string{`{"version":"old"}`, `{"version":"` + version + `","accepted":true}`, `{"version":"` + version + `"} {}`, strings.Repeat("x", 2049)} {
		if r := request("POST", "/api/safety/terms", body, token); r.Code < 400 {
			t.Fatal("invalid consent", r.Code)
		}
	}
	for i := 0; i < 2; i++ {
		if r := request("POST", "/api/safety/terms", `{"version":"`+version+`"}`, token); r.Code != 204 {
			t.Fatal(r.Code, r.Body.String())
		}
	}
	if v := read()["terms"].(map[string]any); v["accepted"] != true {
		t.Fatal("explicit consent missing", v)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM user_terms_acceptances WHERE account_id=$1`, actor).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	for i := 0; i < 2; i++ {
		if r := request("POST", "/api/safety/blocks", `{"account_id":"`+target+`"}`, token); r.Code != 204 {
			t.Fatal(r.Code, r.Body.String())
		}
	}
	if r := request("POST", "/api/safety/blocks", `{"account_id":"`+actor+`"}`, token); r.Code != 400 {
		t.Fatal("self block", r.Code)
	}
	r := request("GET", "/api/safety/blocks", "", token)
	if r.Code != 200 || !strings.Contains(r.Body.String(), target) {
		t.Fatal(r.Code, r.Body.String())
	}
	r = request("GET", "/api/safety/blocks", "", otherToken)
	if r.Code != 200 || strings.Contains(r.Body.String(), actor) || strings.Contains(r.Body.String(), target) {
		t.Fatal("incoming block relation disclosed", r.Body.String())
	}
	if r := request("GET", "/api/safety/blocks?after=bad", "", token); r.Code != 400 {
		t.Fatal(r.Code)
	}
	match := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO text_matches(id,room_id,contract,contract_hash,owner_id,fence,state,prototype,created_at,started_at) VALUES($1,'test-room','{}',repeat('a',64),$2,1,'started',true,now(),now())`, match, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	for seat, id := range []string{actor, target} {
		if _, err := db.Exec(`INSERT INTO text_admissions(id,account_id,match_id,seat,entry_path,prototype,access_kind,quota_day,reserved_at,state) VALUES($1,$2,$3,$4,'local',true,'prototype',CURRENT_DATE,now(),'released')`, uuid.NewString(), id, match, seat); err != nil {
			t.Fatal(err)
		}
	}
	path := "/api/safety/matches/" + match + "/seats/1"
	r = request("GET", path, "", token)
	var identity map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &identity); err != nil || r.Code != 200 || identity["account_id"] != target || identity["current_week_winner"] != false || len(identity) != 3 {
		t.Fatal("identity authorization/shape", r.Code, identity, err)
	}
	_, outsider := seedAccountForEconomy(t, ctx, db, authMgr)
	if r := request("GET", path, "", outsider); r.Code != 404 {
		t.Fatal("nonparticipant identity disclosed", r.Code)
	}
	roomPath := "/api/safety/rooms/" + roomID + "/seats/1"
	if r := request("GET", roomPath, "", token); r.Code != 200 || !strings.Contains(r.Body.String(), target) {
		t.Fatal("room identity", r.Code, r.Body.String())
	}
	if r := request("GET", roomPath, "", outsider); r.Code != 404 {
		t.Fatal("outsider room lookup", r.Code)
	}
	if r := request("GET", roomPath, "", ""); r.Code != 401 {
		t.Fatal("unsigned room lookup", r.Code)
	}
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=$2 WHERE id=$1`, target, time.Now()); err != nil {
		t.Fatal(err)
	}
	if r := request("GET", path, "", token); r.Code != 404 {
		t.Fatal("deleted profile visible", r.Code)
	}
	if r := request("GET", roomPath, "", token); r.Code != 404 {
		t.Fatal("deleted room target", r.Code)
	}
	if r := request("DELETE", "/api/safety/blocks/"+target, "", token); r.Code != 204 {
		t.Fatal("deleted target cannot be unblocked", r.Code)
	}
	if err := authMgr.RevokeAccount(ctx, actor); err != nil {
		t.Fatal(err)
	}
	if r := request("GET", "/api/safety", "", token); r.Code != 401 {
		t.Fatal("revoked access", r.Code)
	}
}
