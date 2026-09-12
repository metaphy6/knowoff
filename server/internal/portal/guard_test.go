package portal

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/store"
)

type guardRevocation struct {
	fakeAuth
	fail  bool
	calls int
}

func (a *guardRevocation) RevokeAccount(context.Context, string) error {
	a.calls++
	if a.fail {
		return errors.New("synthetic revocation offline")
	}
	return nil
}

func TestGuardLateTimedDeliveryDoesNotRevokeFreshSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	guard := guardAccount(t, m, db, admin)
	target := newAccount(t, db)
	calls := 0
	m.SetAccountDisconnect(func(context.Context, string) error { calls++; return errors.New("synthetic runtime down") })
	if err := m.FreezeAccount(ctx, guard, target, "review"); err != nil {
		t.Fatal(err)
	}
	freezes, _ := m.ListActiveFreezes(ctx)
	id := freezes[0]["id"].(string)
	until := m.now().Add(time.Hour)
	if err := m.ConvertFreezeToTimedBan(ctx, admin, id, "confirmed", until); err == nil {
		t.Fatal("failed delivery reported success")
	}
	future := until.Add(time.Hour)
	m.nowFn = func() time.Time { return future }
	if err := m.retryGuardDeliveries(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.ConvertFreezeToTimedBan(ctx, admin, id, "confirmed", until); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("late delivery touched fresh session, calls=%d", calls)
	}
	var obsolete, delivered bool
	if err := db.QueryRow("SELECT disconnect_obsolete_at IS NOT NULL,disconnect_delivered_at IS NOT NULL FROM guard_freezes WHERE id=$1", id).Scan(&obsolete, &delivered); err != nil || !obsolete || delivered {
		t.Fatalf("late disposition %v %v %v", obsolete, delivered, err)
	}
}

func TestGuardCommittedBanRetriesFailedDisconnectWithoutNewDecision(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	a := &guardRevocation{fail: true}
	m.SetAccountDisconnect(a.RevokeAccount)
	guard := guardAccount(t, m, db, admin)
	target := newAccount(t, db)
	if err := m.FreezeAccount(ctx, guard, target, "review"); err != nil {
		t.Fatal(err)
	}
	if a.calls != 0 {
		t.Fatal("Guard freeze revoked active match sessions")
	}
	freezes, _ := m.ListActiveFreezes(ctx)
	id := freezes[0]["id"].(string)
	if err := m.ConvertFreezeToBan(ctx, admin, id, "confirmed"); err == nil {
		t.Fatal("failed postcommit delivery reported success")
	}
	a.fail = false
	if err := m.ConvertFreezeToBan(ctx, admin, id, "confirmed"); err != nil {
		t.Fatalf("committed decision could not retry delivery: %v", err)
	}
	var audits int
	if err := db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE target_id=$1 AND action='guard_freeze_convert_ban'`, target).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("duplicate final audit %d %v", audits, err)
	}
	if a.calls != 2 {
		t.Fatalf("revocation retries=%d", a.calls)
	}
}

func guardAccount(t *testing.T, m *Manager, db *sql.DB, admin string) string {
	t.Helper()
	id := newAccount(t, db)
	if err := m.GrantRole(context.Background(), admin, id, RoleGuard); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGuardExpiryRetainsDeletedTargetEvidence(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	guard := guardAccount(t, m, db, admin)
	target := newAccount(t, db)
	if err := m.FreezeAccount(ctx, guard, target, "retained safety evidence"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, target); err != nil {
		t.Fatal(err)
	}
	future := m.now().Add(49 * time.Hour)
	m.nowFn = func() time.Time { return future }
	if err := m.ExpireFreezes(ctx); err != nil {
		t.Fatal(err)
	}
	var resolved bool
	if err := db.QueryRow(`SELECT expired_at IS NOT NULL FROM guard_freezes WHERE account_id=$1`, target).Scan(&resolved); err != nil || !resolved {
		t.Fatalf("deleted target blocked expiry %v %v", resolved, err)
	}
}

func TestGuardPortalRouteCSRFAndRevokedRole(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	guard := guardAccount(t, m, db, admin)
	target := newAccount(t, db)
	reporter := newAccount(t, db)
	if _, err := db.Exec(`INSERT INTO reports(report_type,reporter_id,target_account_id,reason,description,status) SELECT 'conduct',$1,$2,'<script>case</script>','private conduct description','new' FROM generate_series(1,105)`, reporter, target); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO reports(report_type,reporter_id,target_account_id,reason,status) VALUES('conduct',$1,$2,'resolved-only','resolved'),('media',$1,$2,'media-only','new')`, reporter, target); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO portal_browser_sessions(token_hash,account_id,csrf_token,expires_at) VALUES($1,$2,'synthetic-csrf',now()+interval '1 hour')`, portalHash("synthetic-session"), guard); err != nil {
		t.Fatal(err)
	}
	call := func(method, path, csrf string) *httptest.ResponseRecorder {
		values := url.Values{"account_id": {target}, "reason": {"reported conduct"}, "csrf_token": {csrf}}
		r := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: portalSessionCookie, Value: "synthetic-session"})
		w := httptest.NewRecorder()
		m.Handler().ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/portal/guard", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `action="/portal/guard/freeze"`) {
		t.Fatalf("Guard form absent %d %s", w.Code, w.Body.String())
	}
	w := call("GET", "/portal/guard", "")
	body := w.Body.String()
	if strings.Count(body, `data-guard-case`) != 100 || !strings.Contains(body, "&lt;script&gt;case&lt;/script&gt;") || !strings.Contains(body, "private conduct description") {
		t.Fatal("bounded escaped open conduct queue unavailable")
	}
	if strings.Contains(body, reporter) || strings.Contains(body, "<script>case</script>") || strings.Contains(body, "resolved-only") || strings.Contains(body, "media-only") {
		t.Fatal("Guard queue leaked reporter identity, active markup or unrelated cases")
	}
	if w := call("POST", "/portal/guard/freeze", ""); w.Code != 403 {
		t.Fatalf("missing CSRF=%d", w.Code)
	}
	if w := call("POST", "/portal/guard/freeze", "synthetic-csrf"); w.Code != 303 {
		t.Fatalf("freeze=%d %s", w.Code, w.Body.String())
	}
	if err := m.RevokeRole(ctx, admin, guard, RoleGuard); err != nil {
		t.Fatal(err)
	}
	if w := call("GET", "/portal/guard", ""); w.Code != 403 {
		t.Fatalf("revoked Guard=%d", w.Code)
	}
}

func TestGuardTimedDecisionDoesNotClearPermanentBan(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	target := newAccount(t, db)
	guard := guardAccount(t, m, db, admin)
	if err := m.FreezeAccount(ctx, guard, target, "review needed"); err != nil {
		t.Fatal(err)
	}
	freezes, _ := m.ListActiveFreezes(ctx)
	id := freezes[0]["id"].(string)
	if err := m.ConvertFreezeToTimedBan(ctx, guard, id, "Guard is not an admin", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("Guard converted own freeze")
	}
	until := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Microsecond)
	if err := m.ConvertFreezeToTimedBan(ctx, admin, id, "confirmed", until); err != nil {
		t.Fatal(err)
	}
	var got time.Time
	var ban sql.NullTime
	if err := db.QueryRow(`SELECT suspended_until,banned_at FROM accounts WHERE id=$1`, target).Scan(&got, &ban); err != nil || !got.Equal(until) || ban.Valid {
		t.Fatalf("timed outcome %v %v %v", got, ban, err)
	}
	if m.portalAccountAllowed(ctx, target) {
		t.Fatal("timed ban allowed portal")
	}
	if err := db.QueryRow(`UPDATE accounts SET banned_at=now(),suspended_until=now()-interval '1 second' WHERE id=$1 RETURNING banned_at`, target).Scan(&ban); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpireFreezes(ctx); err != nil {
		t.Fatal(err)
	}
	if m.portalAccountAllowed(ctx, target) {
		t.Fatal("expired timed decision cleared permanent ban")
	}
}

func TestGuardUsesPlayerRoleAndIndependentFreezeIdentity(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	target := newAccount(t, db)
	g1 := guardAccount(t, m, db, admin)
	g2 := guardAccount(t, m, db, admin)
	if err := m.FreezeAccount(ctx, admin, target, "admin UUID is not a player Guard"); err == nil {
		t.Fatal("accepted admin namespace as Guard identity")
	}
	if err := m.FreezeAccount(ctx, g1, target, "first report"); err != nil {
		t.Fatal(err)
	}
	if err := m.FreezeAccount(ctx, g2, target, "independent report"); err != nil {
		t.Fatal(err)
	}
	var banned sql.NullTime
	if err := db.QueryRow(`SELECT banned_at FROM accounts WHERE id=$1`, target).Scan(&banned); err != nil || banned.Valid {
		t.Fatalf("Guard changed admin ban: %v %v", banned, err)
	}
	if m.portalAccountAllowed(ctx, target) {
		t.Fatal("frozen target retained portal access")
	}
	if err := store.NewTextTrustStore(db).CanMatch(ctx, []string{target}, time.Now()); err == nil {
		t.Fatal("frozen target retained admission")
	}
	freezes, err := m.ListActiveFreezes(ctx)
	if err != nil || len(freezes) != 2 {
		t.Fatalf("freezes=%v %v", freezes, err)
	}
	if err = m.DismissFreeze(ctx, g1, freezes[0]["id"].(string)); err == nil {
		t.Fatal("Guard may not make final decision")
	}
	if err = m.DismissFreeze(ctx, admin, freezes[0]["id"].(string)); err != nil {
		t.Fatal(err)
	}
	if m.portalAccountAllowed(ctx, target) {
		t.Fatal("dismissing one cleared overlapping freeze")
	}
	if err = m.DismissFreeze(ctx, admin, freezes[1]["id"].(string)); err != nil {
		t.Fatal(err)
	}
	if !m.portalAccountAllowed(ctx, target) {
		t.Fatal("all dismissed still frozen")
	}
	var audits int
	if err = db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='guard_freeze' AND target_id=$1 AND admin_id IS NULL AND after_state->>'guard_account_id' IN ($2,$3)`, target, g1, g2).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("attributed audit=%d %v", audits, err)
	}
}

func TestGuardConcurrentDuplicateAndExpiryPreserveAdminBan(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	target := newAccount(t, db)
	guard := guardAccount(t, m, db, admin)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- m.FreezeAccount(ctx, guard, target, "same Guard report") }()
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("duplicate successful writes=%d", success)
	}
	if _, err := db.Exec(`UPDATE accounts SET banned_at=now() WHERE id=$1`, target); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(49 * time.Hour)
	m.nowFn = func() time.Time { return future }
	if err := m.ExpireFreezes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpireFreezes(ctx); err != nil {
		t.Fatal(err)
	}
	var banned bool
	if err := db.QueryRow(`SELECT banned_at IS NOT NULL FROM accounts WHERE id=$1`, target).Scan(&banned); err != nil || !banned {
		t.Fatalf("expiry erased admin ban %v %v", banned, err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='guard_freeze_expire' AND target_id=$1`, target).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expiry audit not exactly once %d %v", n, err)
	}
}
