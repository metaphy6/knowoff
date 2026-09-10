package portal

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestContributorBrowserDraftJourneyAndChallengePrivacy(t *testing.T) {
	db := setupBrowserDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	account := newAccount(t, db)
	admin := newAdmin(t, db)
	mux := http.NewServeMux()
	mux.Handle("/portal/", m.Handler())
	mux.Handle("POST /api/portal/connect", m.ConnectHandler())
	srv := httptest.NewServer(mux)
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	get := func(path string) string {
		t.Helper()
		res, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := portalBody(t, res)
		if res.StatusCode != 200 || res.Request.URL.Path != path {
			t.Fatalf("GET %s %d %s: %s", path, res.StatusCode, res.Request.URL.Path, body)
		}
		return body
	}
	body := get("/portal/login")
	csrf := portalField(t, body, "csrf_token")
	code := portalCode(t, body)
	raw, _ := json.Marshal(map[string]string{"code": code})
	req, _ := http.NewRequest("POST", srv.URL+"/api/portal/connect", strings.NewReader(string(raw)))
	req.Header.Set("Authorization", "Bearer "+account)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	portalBody(t, res)
	if res.StatusCode != 204 {
		t.Fatalf("connect %d", res.StatusCode)
	}
	res, err = client.PostForm(srv.URL+"/portal/session", url.Values{"csrf_token": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	body = portalBody(t, res)
	csrf = portalField(t, body, "csrf_token")
	post := func(path string, values url.Values, want int) string {
		t.Helper()
		values.Set("csrf_token", csrf)
		res, err := client.PostForm(srv.URL+path, values)
		if err != nil {
			t.Fatal(err)
		}
		body := portalBody(t, res)
		if res.StatusCode != want {
			t.Fatalf("POST %s %d: %s", path, res.StatusCode, body)
		}
		return body
	}
	get("/portal/apply")
	post("/portal/apply", url.Values{"role": {"contributor"}}, 200)
	if err = m.GrantRole(t.Context(), admin, account, RoleContributor); err != nil {
		t.Fatal(err)
	}
	body = get("/portal/submissions")
	if !strings.Contains(body, "Commercial") && !strings.Contains(body, "commercial use") {
		t.Fatal("consent missing")
	}
	post("/portal/submissions", url.Values{"content": {"private joke"}}, 400)
	body = post("/portal/submissions", url.Values{"content": {"<script>alert(1)</script>"}, "terms_version": {"v1"}, "terms_accepted": {"yes"}}, 200)
	if strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("draft not escaped")
	}
	subs, err := m.ListSubmissions(t.Context(), account, "")
	if err != nil || len(subs) != 1 {
		t.Fatalf("drafts=%d %v", len(subs), err)
	}
	id := subs[0].ID
	post("/portal/submissions/"+id+"/edit", url.Values{"content": {"The moon filed an expense report."}}, 200)
	post("/portal/submissions/"+id+"/submit", url.Values{}, 200)
	post("/portal/submissions/"+id+"/edit", url.Values{"content": {"forbidden edit"}}, 400)
	post("/portal/submissions/"+id+"/withdraw", url.Values{}, 200)
	post("/portal/submissions/"+id+"/submit", url.Values{}, 200)
	if err = m.DecideSubmission(t.Context(), admin, id, true, ""); err != nil {
		t.Fatal(err)
	}
	topic, err := m.CreateChallengeTopic(t.Context(), admin, weekMonday(time.Now().UTC()), id)
	if err != nil {
		t.Fatal(err)
	}
	other := newAccount(t, db)
	if _, err = m.SubmitChallengeEntry(t.Context(), other, topic.ID, MediaText, "private pending entry", ContributionConsent{Version: "v1", Accepted: true}); err != nil {
		t.Fatal(err)
	}
	body = get("/portal/challenge")
	if strings.Contains(body, "private pending entry") {
		t.Fatal("another player's unscreened content reached browser")
	}
	if !strings.Contains(body, "The moon filed an expense report.") {
		t.Fatal("topic content not rendered")
	}
	post("/portal/challenge/entry", url.Values{"topic_id": {topic.ID}, "content": {"my response"}, "terms_version": {"v1"}, "terms_accepted": {"yes"}}, 200)
	body = get("/portal/challenge")
	if !strings.Contains(body, "my response") || !strings.Contains(body, "submitted") {
		t.Fatal("own pending status missing")
	}
}

func TestChallengePageRendersAuthoritativeWinnerAndReadableAttribution(t *testing.T) {
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	closed := start.AddDate(0, 0, 7)
	winner := "second"
	snapshot := map[string]any{
		"topic": map[string]any{"week_start": start, "week_end": start.AddDate(0, 0, 6), "closed_at": &closed, "winner_entry_id": &winner, "nown": map[string]string{"content": "A meeting about meetings"}},
		// Equal tallies intentionally disagree with display order. The server decides.
		"entries":    []map[string]any{{"id": "first", "nickname": "First Player", "content": "First joke", "vote_count": 5}, {"id": "second", "nickname": "<script>Winner</script>", "content": "Second joke", "vote_count": 5}},
		"can_submit": false, "can_vote": false,
	}
	r := httptest.NewRequest("GET", "/portal/challenge", nil)
	w := httptest.NewRecorder()
	portalPage(w, r, "Weekly Nown Challenge", "", challengeBody, map[string]any{"Snapshot": snapshot})
	body := w.Body.String()
	if w.Code != 200 {
		t.Fatalf("render %d: %s", w.Code, body)
	}
	for _, want := range []string{"07 Sep 2026 — 13 Sep 2026", "First Player", "&lt;script&gt;Winner&lt;/script&gt;", "Week closed", "Week Winner"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "00:00:00 +0000") || strings.Contains(body, "<script>") {
		t.Fatal("raw timestamp or unescaped nickname rendered")
	}
	cards := strings.Split(body, `<article class="panel"`)
	if len(cards) != 3 {
		t.Fatalf("public cards=%d", len(cards)-1)
	}
	if strings.Contains(cards[1], "Week Winner") || !strings.Contains(cards[2], "Week Winner") {
		t.Fatal("winner chosen from list order/tally instead of authoritative ID")
	}
	if strings.Contains(body, "Vote for this entry") {
		t.Fatal("closed week still offers voting")
	}
}

func TestChallengePageClosedWithoutWinnerKeepsOwnPendingStatus(t *testing.T) {
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	closed := start.AddDate(0, 0, 7)
	for _, isClosed := range []bool{true, false} {
		t.Run(map[bool]string{true: "closed", false: "open"}[isClosed], func(t *testing.T) {
			var closedAt *time.Time
			if isClosed {
				closedAt = &closed
			}
			snapshot := map[string]any{"topic": map[string]any{"week_start": start, "week_end": start.AddDate(0, 0, 6), "closed_at": closedAt, "winner_entry_id": (*string)(nil), "nown": map[string]string{"content": "topic"}}, "entries": []map[string]any{}, "own_entry": map[string]any{"status": "submitted", "content": "My private pending response"}, "can_submit": false, "can_vote": !isClosed}
			r := httptest.NewRequest("GET", "/portal/challenge", nil)
			w := httptest.NewRecorder()
			portalPage(w, r, "Weekly Nown Challenge", "", challengeBody, map[string]any{"Snapshot": snapshot})
			body := w.Body.String()
			if w.Code != 200 {
				t.Fatalf("render %d: %s", w.Code, body)
			}
			if !strings.Contains(body, "My private pending response") || !strings.Contains(body, ">submitted<") {
				t.Fatal("own pending status disappeared")
			}
			hasResult := strings.Contains(body, "This week closed without a winner.")
			if hasResult != isClosed {
				t.Fatalf("closed result shown=%v for closed=%v", hasResult, isClosed)
			}
			if strings.Contains(body, "Week Winner") {
				t.Fatal("invented a winner")
			}
		})
	}
}
