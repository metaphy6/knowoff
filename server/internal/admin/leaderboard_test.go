package admin

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/leaderboard"
	"github.com/knowoff/knowoff/server/internal/store"
)

func TestLeaderboardAdminBrowserExclusion(t *testing.T) {
	db := operatorDB(t)
	m, _, sid, csrf, _ := operatorActor(t, db)
	target, other := newAccount(t, db), newAccount(t, db)
	mustMutationSQL(t, db, `INSERT INTO leaderboard_weeks(week_id,start_at,end_at) VALUES('2026-08-03','2026-08-03','2026-08-10')`)
	mustMutationSQL(t, db, `INSERT INTO leaderboard_entries(week_id,account_id,points,matches_counted) VALUES('2026-08-03',$1,100,1),('2026-08-03',$2,50,1)`, target, other)
	form := url.Values{"csrf_token": {csrf}, "id": {uuid.NewString()}, "kind": {"exclude"}, "week_id": {"2026-08-03"}, "target_account_id": {target}, "reason": {"Synthetic cheating review <script>"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/leaderboard/decisions", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
	w := httptest.NewRecorder()
	m.Handler(nil).ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("exclude route: %d %s", w.Code, w.Body.String())
	}
	top, own, err := leaderboard.NewManager(db).Get(t.Context(), "2026-08-03", 100, target)
	if err != nil || len(top) != 1 || top[0].AccountID != other || top[0].Rank != 1 || own != nil {
		t.Fatal("excluded projection", top, own, err)
	}
	var points int
	if err := db.QueryRow(`SELECT points FROM leaderboard_entries WHERE week_id='2026-08-03' AND account_id=$1`, target).Scan(&points); err != nil || points != 100 {
		t.Fatal("raw points changed", points, err)
	}
}

func leaderboardFixture(t *testing.T) (*sql.DB, *Manager, string, string, string, context.Context, []string) {
	t.Helper()
	db := operatorDB(t)
	m, actor, sid, csrf, ctx := operatorActor(t, db)
	ids := []string{newAccount(t, db), newAccount(t, db), newAccount(t, db)}
	mustMutationSQL(t, db, `INSERT INTO leaderboard_weeks(week_id,start_at,end_at) VALUES('2026-08-03','2026-08-03','2026-08-10')`)
	for i, id := range ids {
		points := 100
		if i == 2 {
			points = 50
		}
		mustMutationSQL(t, db, `INSERT INTO leaderboard_entries(week_id,account_id,points,matches_counted) VALUES('2026-08-03',$1,$2,1)`, id, points)
	}
	return db, m, actor, sid, csrf, ctx, ids
}
func boardCommand(kind, target, prior string) store.LeaderboardAdminCommand {
	return store.LeaderboardAdminCommand{ID: uuid.NewString(), Kind: kind, WeekID: "2026-08-03", TargetAccountID: target, PriorDecisionID: prior, Reason: "Synthetic leaderboard review <script>"}
}

func TestLeaderboardAdminLegacyClosedWeekRerun(t *testing.T) {
	db, _, actor, _, _, ctx, _ := leaderboardFixture(t)
	mustMutationSQL(t, db, `INSERT INTO leaderboard_history SELECT week_id,account_id,RANK() OVER(ORDER BY points DESC),points FROM leaderboard_entries WHERE week_id='2026-08-03'`)
	mustMutationSQL(t, db, `UPDATE leaderboard_weeks SET closed=true,closed_at='2026-08-10' WHERE week_id='2026-08-03'`)
	before := correctionSnapshot(t, db, "leaderboard_weeks", "leaderboard_history", "leaderboard_entries", "profiles", "noin_ledger")
	r, err := store.NewLeaderboardAdminStore(db).Decide(ctx, actor, boardCommand("close", "", ""))
	if err != nil || r.Status != "closed" {
		t.Fatal("legacy rerun", r, err)
	}
	if !reflect.DeepEqual(before, correctionSnapshot(t, db, "leaderboard_weeks", "leaderboard_history", "leaderboard_entries", "profiles", "noin_ledger")) {
		t.Fatal("legacy closure changed")
	}
}

func TestLeaderboardAdminReinstateCloseHistoryAndReplay(t *testing.T) {
	db, m, actor, sid, _, ctx, ids := leaderboardFixture(t)
	s := store.NewLeaderboardAdminStore(db)
	board := leaderboard.NewManager(db)
	originals := correctionSnapshot(t, db, "profiles", "noin_wallets", "noin_ledger", "entitlements", "text_award_receipts", "leaderboard_daily_counts")
	exclude := boardCommand("exclude", ids[0], "")
	for range 2 {
		r, err := s.Decide(ctx, actor, exclude)
		if err != nil || r.Status != "applied" || r.Revision != 1 {
			t.Fatal(r, err)
		}
	}
	// Eligibility doesn't erase accrual or reclaim a daily allowance.
	if counted, err := board.RecordPoints(ctx, "2026-08-03", ids[0], 25, time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC), 10); err != nil || !counted {
		t.Fatal(counted, err)
	}
	top, own, err := board.Get(ctx, "2026-08-03", 1, ids[2])
	if err != nil || len(top) != 1 || top[0].AccountID != ids[1] || own == nil || own.Rank != 2 {
		t.Fatal(top, own, err)
	}
	reinstate := boardCommand("reinstate", ids[0], exclude.ID)
	if r, err := s.Decide(ctx, actor, reinstate); err != nil || r.Status != "applied" || r.Revision != 2 {
		t.Fatal(r, err)
	}
	top, own, err = board.Get(ctx, "2026-08-03", 100, ids[0])
	if err != nil || len(top) != 3 || own == nil || own.Points != 125 || own.Rank != 1 {
		t.Fatal(top, own, err)
	}
	if _, err = s.Decide(ctx, actor, boardCommand("exclude", ids[0], exclude.ID)); err == nil {
		t.Fatal("stale exclusion source accepted")
	}
	exclude2 := boardCommand("exclude", ids[0], reinstate.ID)
	if _, err = s.Decide(ctx, actor, exclude2); err != nil {
		t.Fatal(err)
	}
	raw := correctionSnapshot(t, db, "leaderboard_entries")
	closeCommand := boardCommand("close", "", "")
	if r, err := s.Decide(ctx, actor, closeCommand); err != nil || r.Status != "pending" || r.ClosingAt == nil {
		t.Fatal(r, err)
	}
	if _, err = s.Decide(ctx, actor, boardCommand("reinstate", ids[0], exclude2.ID)); err == nil {
		t.Fatal("changed closing eligibility")
	}
	ranks, err := board.CloseWeek(ctx, "2026-08-03")
	if err != nil || len(ranks) != 2 || ranks[0].AccountID != ids[1] || ranks[1].Rank != 2 {
		t.Fatal(ranks, err)
	}
	history := correctionSnapshot(t, db, "leaderboard_history", "leaderboard_entries")
	for range 2 {
		if err = s.ResumePending(t.Context(), 5); err != nil {
			t.Fatal(err)
		}
		r, err := s.Decide(ctx, actor, closeCommand)
		if err != nil || r.Status != "closed" {
			t.Fatal(r, err)
		}
	}
	if _, err = s.Decide(ctx, actor, boardCommand("close", "", "")); err != nil {
		t.Fatal("closed rerun", err)
	}
	if !reflect.DeepEqual(history, correctionSnapshot(t, db, "leaderboard_history", "leaderboard_entries")) || !reflect.DeepEqual(raw, correctionSnapshot(t, db, "leaderboard_entries")) {
		t.Fatal("history or raw totals changed")
	}
	top, own, err = board.Get(ctx, "2026-08-03", 100, ids[0])
	if err != nil || len(top) != 2 || own != nil {
		t.Fatal(top, own, err)
	}
	// Daily count changed only once from RecordPoints; other account value is exact.
	after := correctionSnapshot(t, db, "profiles", "noin_wallets", "noin_ledger", "entitlements", "text_award_receipts")
	if !reflect.DeepEqual(originals[:5], after) {
		t.Fatal("admin eligibility changed unrelated value")
	}
	for _, path := range []string{"/admin/leaderboard?week_id=2026-08-03", "/admin/leaderboard/decisions/" + closeCommand.ID} {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
		w := httptest.NewRecorder()
		m.Handler(nil).ServeHTTP(w, req)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "review <script>") {
			t.Fatal("history page", w.Code, w.Body.String())
		}
	}
}

func TestLeaderboardAdminStrictAuthorityAndConflictRefusals(t *testing.T) {
	db, m, actor, sid, csrf, ctx, ids := leaderboardFixture(t)
	s := store.NewLeaderboardAdminStore(db)
	c := boardCommand("exclude", ids[0], "")
	if _, err := s.Decide(ctx, actor, c); err != nil {
		t.Fatal(err)
	}
	before := correctionSnapshot(t, db, "leaderboard_admin_decisions", "leaderboard_admin_results", "leaderboard_entries", "leaderboard_weeks", "admin_audit_log", "profiles", "noin_ledger")
	for _, mutate := range []func(*store.LeaderboardAdminCommand){func(c *store.LeaderboardAdminCommand) { c.Reason = "different" }, func(c *store.LeaderboardAdminCommand) { c.ID = uuid.NewString() }, func(c *store.LeaderboardAdminCommand) { c.WeekID = "2026-08-04" }, func(c *store.LeaderboardAdminCommand) { c.Kind = "delete" }, func(c *store.LeaderboardAdminCommand) { c.PriorDecisionID = uuid.NewString() }, func(c *store.LeaderboardAdminCommand) { c.Reason = " " }, func(c *store.LeaderboardAdminCommand) { c.Reason = strings.Repeat("x", 501) }, func(c *store.LeaderboardAdminCommand) { c.TargetAccountID = uuid.NewString() }} {
		altered := c
		mutate(&altered)
		if _, err := s.Decide(ctx, actor, altered); err == nil {
			t.Fatal("invalid command accepted", altered)
		}
	}
	if _, err := s.Decide(t.Context(), actor, c); err == nil {
		t.Fatal("unbound authority accepted")
	}
	if _, err := s.Decide(ctx, uuid.NewString(), c); err == nil {
		t.Fatal("different actor accepted")
	}
	for _, test := range []struct {
		name          string
		form          url.Values
		cookie, query string
		want          int
	}{
		{"csrf", url.Values{"csrf_token": {"wrong"}}, sid, "", 403},
		{"anonymous", url.Values{}, "", "", 303},
		{"duplicate", url.Values{"csrf_token": {csrf}, "id": {uuid.NewString(), uuid.NewString()}}, sid, "", 400},
		{"unknown", url.Values{"csrf_token": {csrf}, "unknown": {"x"}}, sid, "", 400},
		{"query", url.Values{"csrf_token": {csrf}}, sid, "?kind=close", 400},
		{"oversize", url.Values{"csrf_token": {csrf}, "reason": {strings.Repeat("x", 17000)}}, sid, "", 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/admin/leaderboard/decisions"+test.query, strings.NewReader(test.form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if test.cookie != "" {
				r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: test.cookie})
			}
			w := httptest.NewRecorder()
			m.Handler(nil).ServeHTTP(w, r)
			if w.Code != test.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	mustMutationSQL(t, db, `UPDATE admin_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, sid)
	if _, err := s.Decide(ctx, actor, c); err == nil {
		t.Fatal("expired replay accepted")
	}
	if !reflect.DeepEqual(before, correctionSnapshot(t, db, "leaderboard_admin_decisions", "leaderboard_admin_results", "leaderboard_entries", "leaderboard_weeks", "admin_audit_log", "profiles", "noin_ledger")) {
		t.Fatal("refusal changed durable state")
	}
}

func TestLeaderboardAdminSessionExpiryAfterEveryWait(t *testing.T) {
	for _, lock := range []string{"week", "account", "identity"} {
		t.Run(lock, func(t *testing.T) {
			db, _, actor, sid, _, ctx, ids := leaderboardFixture(t)
			s := store.NewLeaderboardAdminStore(db)
			c := boardCommand("exclude", ids[0], "")
			before := correctionSnapshot(t, db, "leaderboard_weeks", "leaderboard_entries", "leaderboard_admin_decisions", "leaderboard_admin_results", "admin_audit_log")
			mustMutationSQL(t, db, `UPDATE admin_sessions SET expires_at=clock_timestamp()+interval '700 milliseconds' WHERE id=$1`, sid)
			blocker, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback()
			query, needle, arg := `SELECT closed FROM leaderboard_weeks WHERE week_id=$1 FOR UPDATE`, "SELECT closed,closing_at,end_at", "2026-08-03"
			if lock == "account" {
				query, needle, arg = `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, "SELECT id FROM accounts", ids[0]
			}
			if lock == "identity" {
				query, needle, arg = `SELECT pg_advisory_xact_lock(hashtextextended($1,30))`, "pg_advisory_xact_lock", c.ID
			}
			if _, err = blocker.Exec(query, arg); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := s.Decide(ctx, actor, c); done <- err }()
			waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			waitAdminSessionLock(t, waitCtx, db, needle)
			for {
				var expired bool
				if err = db.QueryRow(`SELECT clock_timestamp()>=expires_at FROM admin_sessions WHERE id=$1`, sid).Scan(&expired); err != nil {
					t.Fatal(err)
				}
				if expired {
					break
				}
				select {
				case <-waitCtx.Done():
					t.Fatal(waitCtx.Err())
				case <-time.After(10 * time.Millisecond):
				}
			}
			if err = blocker.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err == nil {
				t.Fatal("expired session accepted")
			}
			if !reflect.DeepEqual(before, correctionSnapshot(t, db, "leaderboard_weeks", "leaderboard_entries", "leaderboard_admin_decisions", "leaderboard_admin_results", "admin_audit_log")) {
				t.Fatal("expired operation left effects")
			}
		})
	}
}

func TestLeaderboardAdminConcurrentReplayAndAwardLockOrders(t *testing.T) {
	for _, first := range []string{"exclude", "award"} {
		t.Run(first, func(t *testing.T) {
			db, _, actor, _, _, ctx, ids := leaderboardFixture(t)
			s := store.NewLeaderboardAdminStore(db)
			board := leaderboard.NewManager(db)
			c := boardCommand("exclude", ids[0], "")
			blocker, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback()
			if _, err = blocker.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, ids[0]); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 2)
			exclusion := func() { _, err := s.Decide(ctx, actor, c); result <- err }
			award := func() {
				_, err := board.RecordPoints(ctx, c.WeekID, ids[0], 25, time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC), 10)
				result <- err
			}
			if first == "exclude" {
				go exclusion()
			} else {
				go award()
			}
			waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			waitAdminSessionLock(t, waitCtx, db, "SELECT id FROM accounts")
			if first == "exclude" {
				go award()
			} else {
				go exclusion()
			}
			waitAdminSessionLock(t, waitCtx, db, "FROM leaderboard_weeks WHERE week_id")
			if err = blocker.Commit(); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err = <-result; err != nil {
					t.Fatal(first, err)
				}
			}
			var group sync.WaitGroup
			results := make(chan error, 12)
			for range 12 {
				group.Add(1)
				go func() { defer group.Done(); _, err := s.Decide(ctx, actor, c); results <- err }()
			}
			group.Wait()
			close(results)
			for err := range results {
				if err != nil {
					t.Fatal(err)
				}
			}
			var count, points int
			if err = db.QueryRow(`SELECT (SELECT count(*) FROM leaderboard_admin_decisions),(SELECT points FROM leaderboard_entries WHERE week_id=$1 AND account_id=$2)`, c.WeekID, ids[0]).Scan(&count, &points); err != nil || count != 1 || points != 125 {
				t.Fatal(count, points, err)
			}
			if _, own, err := board.Get(ctx, c.WeekID, 100, ids[0]); err != nil || own != nil {
				t.Fatal(own, err)
			}
		})
	}
}

func TestLeaderboardAdminExclusionCloseOrderAndConcurrentReruns(t *testing.T) {
	for _, first := range []string{"exclude", "close"} {
		t.Run(first, func(t *testing.T) {
			db, _, actor, _, _, ctx, ids := leaderboardFixture(t)
			s := store.NewLeaderboardAdminStore(db)
			exclude, closeCommand := boardCommand("exclude", ids[0], ""), boardCommand("close", "", "")
			var actorAccount string
			if err := db.QueryRow(`SELECT account_id FROM admin_accounts WHERE id=$1`, actor).Scan(&actorAccount); err != nil {
				t.Fatal(err)
			}
			blocker, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback()
			if _, err = blocker.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, actorAccount); err != nil {
				t.Fatal(err)
			}
			firstDone, secondDone := make(chan error, 1), make(chan error, 1)
			firstCommand, secondCommand := exclude, closeCommand
			if first == "close" {
				firstCommand, secondCommand = closeCommand, exclude
			}
			go func() { _, err := s.Decide(ctx, actor, firstCommand); firstDone <- err }()
			waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			waitAdminSessionLock(t, waitCtx, db, "SELECT id FROM accounts")
			go func() { _, err := s.Decide(ctx, actor, secondCommand); secondDone <- err }()
			waitAdminSessionLock(t, waitCtx, db, "FROM leaderboard_weeks WHERE week_id")
			if err = blocker.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-firstDone; err != nil {
				t.Fatal(err)
			}
			err = <-secondDone
			if first == "close" && err == nil || first == "exclude" && err != nil {
				t.Fatal("wrong close ordering", first, err)
			}
			var group sync.WaitGroup
			done := make(chan error, 6)
			for range 6 {
				group.Add(1)
				go func() {
					defer group.Done()
					_, err := s.Decide(ctx, actor, boardCommand("close", "", ""))
					if err == nil {
						err = store.NewLeaderboardAdminStore(db).ResumePending(t.Context(), 5)
					}
					done <- err
				}()
			}
			group.Wait()
			close(done)
			for err := range done {
				if err != nil {
					t.Fatal(err)
				}
			}
			if err = s.ResumePending(t.Context(), 5); err != nil {
				t.Fatal(err)
			}
			var rows, receipts, audits int
			if err = db.QueryRow(`SELECT (SELECT count(*) FROM leaderboard_history),(SELECT count(*) FROM leaderboard_admin_results WHERE outcome='closed'),(SELECT count(*) FROM admin_audit_log WHERE action='leaderboard_closed')`).Scan(&rows, &receipts, &audits); err != nil {
				t.Fatal(err)
			}
			expectedRows := 3
			if first == "exclude" {
				expectedRows = 2
			}
			if rows != expectedRows || receipts != 7 || audits != 7 {
				t.Fatal("duplicated or inconsistent close", rows, receipts, audits)
			}
		})
	}
}

func TestLeaderboardAdminFailuresRollbackAndClosedCompletionRetries(t *testing.T) {
	for _, where := range []string{"leaderboard_admin_decisions", "leaderboard_admin_results", "admin_audit_log"} {
		t.Run(where, func(t *testing.T) {
			db, _, actor, _, _, ctx, ids := leaderboardFixture(t)
			before := correctionSnapshot(t, db, "leaderboard_admin_decisions", "leaderboard_admin_results", "leaderboard_weeks", "admin_audit_log")
			mustMutationSQL(t, db, `CREATE FUNCTION refuse_leaderboard_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic leaderboard failure'; END $$`)
			mustMutationSQL(t, db, `CREATE TRIGGER refuse_leaderboard_write BEFORE INSERT ON `+where+` FOR EACH ROW EXECUTE FUNCTION refuse_leaderboard_write()`)
			if r, err := store.NewLeaderboardAdminStore(db).Decide(ctx, actor, boardCommand("exclude", ids[0], "")); err == nil || r.Command.ID != "" {
				t.Fatal("partial decision returned", r, err)
			}
			if !reflect.DeepEqual(before, correctionSnapshot(t, db, "leaderboard_admin_decisions", "leaderboard_admin_results", "leaderboard_weeks", "admin_audit_log")) {
				t.Fatal("failed eligibility left writes")
			}
		})
	}
	db, _, actor, _, _, ctx, _ := leaderboardFixture(t)
	s := store.NewLeaderboardAdminStore(db)
	c := boardCommand("close", "", "")
	// Failure accepting a cutoff leaves neither decision nor closing_at.
	mustMutationSQL(t, db, `CREATE FUNCTION refuse_close_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='leaderboard_decision' THEN RAISE EXCEPTION 'synthetic close acceptance'; END IF; RETURN NEW; END $$; CREATE TRIGGER refuse_close_audit BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION refuse_close_audit()`)
	before := correctionSnapshot(t, db, "leaderboard_weeks", "leaderboard_admin_decisions", "admin_audit_log")
	if _, err := s.Decide(ctx, actor, c); err == nil {
		t.Fatal("audit failure accepted cutoff")
	}
	if !reflect.DeepEqual(before, correctionSnapshot(t, db, "leaderboard_weeks", "leaderboard_admin_decisions", "admin_audit_log")) {
		t.Fatal("orphan cutoff")
	}
	mustMutationSQL(t, db, `DROP TRIGGER refuse_close_audit ON admin_audit_log; DROP FUNCTION refuse_close_audit()`)
	if _, err := s.Decide(ctx, actor, c); err != nil {
		t.Fatal(err)
	}
	mustMutationSQL(t, db, `CREATE FUNCTION refuse_close_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='leaderboard_closed' THEN RAISE EXCEPTION 'synthetic close completion'; END IF; RETURN NEW; END $$; CREATE TRIGGER refuse_close_audit BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION refuse_close_audit()`)
	before = correctionSnapshot(t, db, "leaderboard_weeks", "leaderboard_history", "leaderboard_admin_results", "admin_audit_log")
	if err := s.ResumePending(t.Context(), 5); err == nil {
		t.Fatal("completion audit failure ignored")
	}
	if !reflect.DeepEqual(before, correctionSnapshot(t, db, "leaderboard_weeks", "leaderboard_history", "leaderboard_admin_results", "admin_audit_log")) {
		t.Fatal("partial immutable closure")
	}
	mustMutationSQL(t, db, `DROP TRIGGER refuse_close_audit ON admin_audit_log; DROP FUNCTION refuse_close_audit()`)
	if err := store.NewLeaderboardAdminStore(db).ResumePending(t.Context(), 5); err != nil {
		t.Fatal(err)
	}
	r, err := s.Get(ctx, c.ID)
	if err != nil || r.Status != "closed" {
		t.Fatal(r, err)
	}
}
