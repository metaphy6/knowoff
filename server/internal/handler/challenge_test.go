package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/portal"
	"github.com/knowoff/knowoff/server/internal/profile"
)

type challengeTestScreen struct{}

func (challengeTestScreen) ScreenText(context.Context, string) error { return nil }

func TestChallengeHTTPJourney(t *testing.T) {
	_, econ, authMgr, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	db := econ.DB()
	ctx := context.Background()
	if _, err := db.Exec(`TRUNCATE challenge_votes,challenge_entries,challenge_winners,challenge_topics,portal_submissions,portal_terms CASCADE`); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Tuning: config.TuningConfig{Portal: config.PortalTuning{TermsVersion: "v1", MaxTextSubmissionLength: 100, SubmissionsPerContributorPerDay: 10}, LiveOps: config.LiveOpsTuning{ChallengeMaxEntries: 2}, Noin: config.NoinTuning{ChallengeWinner: 100}}}
	pm := portal.NewManager(portal.Deps{DB: db, Config: cfg, Auth: authMgr, Profile: profile.NewManager(db, cfg.Tuning.Progression), Economy: econ, Screener: challengeTestScreen{}})
	owner, ownerToken := seedAccountForEconomy(t, ctx, db, authMgr)
	viewer, viewerToken := seedAccountForEconomy(t, ctx, db, authMgr)
	_ = viewer
	admin := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$3,'test','test')`, admin, owner, admin+"@test.local"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO portal_terms(version,title,body) VALUES('v1','Contribution terms','Commercial use and modification permitted.')`); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterChallengeRoutes(mux, ChallengeDeps{Auth: authMgr, Portal: pm})
	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/api/challenge/active", "", ""); w.Code != 401 {
		t.Fatalf("anonymous %d", w.Code)
	}
	if w := call("GET", "/api/challenge/active", ownerToken, ""); w.Code != 204 {
		t.Fatalf("no topic %d %s", w.Code, w.Body.String())
	}
	if err := pm.GrantRole(ctx, admin, owner, portal.RoleContributor); err != nil {
		t.Fatal(err)
	}
	draft, err := pm.CreateDraft(ctx, owner, portal.MediaText, "An emotionally unavailable printer", portal.ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = pm.SubmitDraft(ctx, owner, draft.ID); err != nil {
		t.Fatal(err)
	}
	if err = pm.DecideSubmission(ctx, admin, draft.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Truncate(24 * time.Hour)
	monday := day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
	topic, err := pm.CreateChallengeTopic(ctx, admin, monday, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf(`{"topic_id":%q,"content":"It needs space","terms_version":"v1","terms_accepted":true}`, topic.ID)
	if w := call("POST", "/api/challenge/entry", ownerToken, `{"topic_id":"bad","content":"x"}`); w.Code != 400 || !strings.Contains(w.Body.String(), "terms_required") {
		t.Fatalf("consent %d %s", w.Code, w.Body.String())
	}
	w := call("POST", "/api/challenge/entry", ownerToken, payload)
	if w.Code != 201 {
		t.Fatalf("entry %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Entry struct {
			ID string `json:"id"`
		} `json:"entry"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	entry := body.Entry.ID
	w = call("GET", "/api/challenge/active", viewerToken, "")
	if strings.Contains(w.Body.String(), "It needs space") {
		t.Fatal("private pending content leaked")
	}
	w = call("GET", "/api/challenge/active", ownerToken, "")
	if !strings.Contains(w.Body.String(), "It needs space") || !strings.Contains(w.Body.String(), "An emotionally unavailable printer") {
		t.Fatalf("own/topic missing %s", w.Body.String())
	}
	if err = pm.ApproveChallengeEntry(ctx, admin, entry); err != nil {
		t.Fatal(err)
	}
	vote := fmt.Sprintf(`{"topic_id":%q,"entry_id":%q}`, topic.ID, entry)
	if w = call("POST", "/api/challenge/vote", ownerToken, vote); w.Code != 400 {
		t.Fatal("self vote accepted")
	}
	if w = call("POST", "/api/challenge/vote", viewerToken, vote); w.Code != 204 {
		t.Fatalf("vote %d %s", w.Code, w.Body.String())
	}
	if w = call("POST", "/api/challenge/vote", viewerToken, vote); w.Code != 400 || !strings.Contains(w.Body.String(), "challenge_already_voted") {
		t.Fatalf("repeat vote %d %s", w.Code, w.Body.String())
	}
	w = call("GET", "/api/challenge/active", viewerToken, "")
	var state map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state["voted_entry_id"] != entry || state["can_vote"] != false {
		t.Fatalf("vote state %s", w.Body.String())
	}
	if _, err = pm.CloseChallengeWeek(ctx, admin, topic.ID); err != nil {
		t.Fatal(err)
	}
	w = call("GET", "/api/challenge/active", viewerToken, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"can_submit":false`) {
		t.Fatalf("closed state %d %s", w.Code, w.Body.String())
	}
}

func TestChallengeRequestParsingRejectsTrailingOrUnknownJSON(t *testing.T) {
	for _, body := range []string{`{"content":"ok"} {"content":"again"}`, `{"unexpected":true}`, strings.Repeat("x", 65<<10)} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var req struct {
			Content string `json:"content"`
		}
		if decodeChallenge(w, r, &req) || w.Code != 400 {
			t.Fatalf("accepted malformed body")
		}
	}
}

func TestChallengeHTTPRechecksAccountEligibility(t *testing.T) {
	_, econ, authMgr, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	db := econ.DB()
	ctx := context.Background()
	adminAccount, _ := seedAccountForEconomy(t, ctx, db, authMgr)
	admin := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$3,'test','test')`, admin, adminAccount, admin+"@test.local"); err != nil {
		t.Fatal(err)
	}
	pm := portal.NewManager(portal.Deps{DB: db, Config: &config.Config{}, Auth: authMgr})
	mux := http.NewServeMux()
	RegisterChallengeRoutes(mux, ChallengeDeps{Auth: authMgr, Portal: pm})
	for _, tc := range []struct {
		name, update string
		denied       int
	}{
		{"banned after token issuance", `UPDATE accounts SET banned_at=now() WHERE id=$1`, 401},
		{"deleted after token issuance", `UPDATE accounts SET deleted_at=now() WHERE id=$1`, 403},
		{"active freeze", `INSERT INTO guard_freezes(account_id,frozen_by,reason,expires_at) VALUES($1,$2,'test',now()+interval '1 hour')`, 403},
		{"expired freeze", `INSERT INTO guard_freezes(account_id,frozen_by,reason,expires_at) VALUES($1,$2,'test',now()-interval '1 hour')`, 0},
		{"dismissed freeze", `INSERT INTO guard_freezes(account_id,frozen_by,reason,expires_at,dismissed_at) VALUES($1,$2,'test',now()+interval '1 hour',now())`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account, token := seedAccountForEconomy(t, ctx, db, authMgr)
			args := []any{account}
			if strings.Contains(tc.update, "$2") {
				args = append(args, admin)
			}
			if _, err := db.Exec(tc.update, args...); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/api/challenge/active", "/api/challenge/entry", "/api/challenge/vote"} {
				method := "POST"
				if strings.HasSuffix(path, "active") {
					method = "GET"
				}
				r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
				r.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if tc.denied != 0 && w.Code != tc.denied {
					t.Fatalf("%s: want %d got %d %s", path, tc.denied, w.Code, w.Body.String())
				}
				if tc.denied == 0 && (w.Code == 401 || w.Code == 403) {
					t.Fatalf("inactive freeze blocked %s: %s", path, w.Body.String())
				}
			}
		})
	}
}
