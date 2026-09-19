package admin

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/store"
)

func TestNoinCorrectionBrowserGrantAndFullSourceRefund(t *testing.T) {
	db := operatorDB(t)
	m, _, sid, csrf, _ := operatorActor(t, db)
	account := newAccount(t, db)
	form := url.Values{"csrf_token": {csrf}, "id": {uuid.NewString()}, "kind": {"noin_grant"}, "target_account_id": {account}, "amount": {"100"}, "reason": {"Synthetic correction <script>"}}
	call := func(form url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/admin/economy/corrections", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
		w := httptest.NewRecorder()
		m.Handler(nil).ServeHTTP(w, r)
		return w
	}
	for range 2 {
		if w := call(form); w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "/admin/operators/") {
			t.Fatalf("grant route: %d %s", w.Code, w.Body.String())
		}
	}
	var balance, ledger, decisions, results, audits int64
	if err := db.QueryRow(`SELECT (SELECT balance FROM noin_wallets WHERE account_id=$1),(SELECT count(*) FROM noin_ledger),(SELECT count(*) FROM admin_operation_decisions),(SELECT count(*) FROM admin_operation_results),(SELECT count(*) FROM admin_audit_log WHERE action IN ('operator_decision','operator_applied'))`, account).Scan(&balance, &ledger, &decisions, &results, &audits); err != nil || balance != 100 || ledger != 1 || decisions != 1 || results != 1 || audits != 2 {
		t.Fatal("atomic grant/replay", balance, ledger, decisions, results, audits, err)
	}
	wallet := economy.NewWallet(db)
	if err := wallet.Debit(t.Context(), account, 40, "Synthetic spend"); err != nil {
		t.Fatal(err)
	}
	var source int64
	if err := db.QueryRow(`SELECT id FROM noin_ledger WHERE account_id=$1 AND event_type='spend'`, account).Scan(&source); err != nil {
		t.Fatal(err)
	}
	form.Set("id", uuid.NewString())
	form.Set("kind", "noin_refund")
	form.Del("amount")
	form.Set("source_ledger_id", strconv.FormatInt(source, 10))
	for range 2 {
		if w := call(form); w.Code != http.StatusSeeOther {
			t.Fatal("full refund", w.Code, w.Body.String())
		}
	}
	if balance, err := wallet.Balance(t.Context(), account); err != nil || balance != 100 {
		t.Fatal("refund/replay balance", balance, err)
	}
	if sum, err := wallet.LedgerSum(t.Context(), account); err != nil || sum != 100 {
		t.Fatal("refund ledger parity", sum, err)
	}
	r := httptest.NewRequest(http.MethodGet, "/admin/economy?account_id="+account, nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
	w := httptest.NewRecorder()
	m.Handler(nil).ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `action="/admin/economy/corrections"`) || strings.Contains(w.Body.String(), "Synthetic correction <script>") {
		t.Fatal("correction form or escaping", w.Code, w.Body.String())
	}

}

func correctionSnapshot(t *testing.T, db *sql.DB, tables ...string) []string {
	t.Helper()
	values := []string{}
	for _, table := range tables {
		var v string
		if err := db.QueryRow(`SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text),'[]')::text FROM ` + table + ` x`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		values = append(values, v)
	}
	return values
}

func correctionCommand(target string) store.AdminOperationCommand {
	return store.AdminOperationCommand{ID: uuid.NewString(), Kind: "noin_grant", TargetAccountID: target, Amount: 100, Reason: "Synthetic operator correction"}
}

func TestNoinCorrectionsRejectInputsAndRetainUnrelatedValue(t *testing.T) {
	db := operatorDB(t)
	_, actor, _, _, ctx := operatorActor(t, db)
	target, other := newAccount(t, db), newAccount(t, db)
	m := economy.NewManager(db, testConfig())
	mustMutationSQL(t, db, `UPDATE profiles SET overall_points=123,non_converted_points=23,xp=15 WHERE account_id=$1`, target)
	mustMutationSQL(t, db, `INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES($1,CURRENT_DATE,300)`, target)
	mustMutationSQL(t, db, `INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'custom_avatar','owned')`, target)
	mustMutationSQL(t, db, `INSERT INTO noin_wallets(account_id,balance) VALUES($1,100)`, target)
	var source, positive, foreign, minimum int64
	for _, row := range []struct {
		account, kind string
		amount        int
		id            *int64
	}{{target, "spend", -40, &source}, {target, "purchase", 10, &positive}, {other, "spend", -20, &foreign}, {target, "spend", math.MinInt32, &minimum}} {
		if err := db.QueryRow(`INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,$2,$3,'Synthetic source',CURRENT_DATE) RETURNING id`, row.account, row.kind, row.amount).Scan(row.id); err != nil {
			t.Fatal(err)
		}
	}
	original := correctionSnapshot(t, db, "noin_ledger", "noin_wallets", "profiles", "entitlements", "daily_noin_earned", "admin_operation_decisions", "admin_operation_results", "admin_audit_log")
	for _, name := range []string{"zero", "negative", "int_overflow", "foreign_source", "positive_source", "missing_source", "minimum_source", "partial_refund", "grant_with_source", "other_kind", "room_payload", "missing_reason", "malformed_id", "wrong_actor", "unbound"} {
		t.Run(name, func(t *testing.T) {
			c := correctionCommand(target)
			bound, named := ctx, actor
			switch name {
			case "zero":
				c.Amount = 0
			case "negative":
				c.Amount = -1
			case "int_overflow":
				c.Amount = math.MaxInt32 + 1
			case "foreign_source":
				c.Kind = "noin_refund"
				c.Amount = 0
				c.SourceLedgerID = foreign
			case "positive_source":
				c.Kind = "noin_refund"
				c.Amount = 0
				c.SourceLedgerID = positive
			case "missing_source":
				c.Kind = "noin_refund"
				c.Amount = 0
				c.SourceLedgerID = 9999999
			case "minimum_source":
				c.Kind = "noin_refund"
				c.Amount = 0
				c.SourceLedgerID = minimum
			case "partial_refund":
				c.Kind = "noin_refund"
				c.Amount = 20
				c.SourceLedgerID = source
			case "grant_with_source":
				c.SourceLedgerID = source
			case "other_kind":
				c.Kind = "account_sanction"
			case "room_payload":
				c.RoomID = uuid.NewString()
			case "missing_reason":
				c.Reason = " "
			case "malformed_id":
				c.ID = "bad"
			case "wrong_actor":
				named = uuid.NewString()
			case "unbound":
				bound = t.Context()
			}
			if receipt, err := m.CorrectNoin(bound, named, c); err == nil || receipt.Status != "" {
				t.Fatal("invalid correction accepted", receipt, err)
			}
			if got := correctionSnapshot(t, db, "noin_ledger", "noin_wallets", "profiles", "entitlements", "daily_noin_earned", "admin_operation_decisions", "admin_operation_results", "admin_audit_log"); !reflect.DeepEqual(original, got) {
				t.Fatal("refused correction changed data")
			}
		})
	}
	protected := correctionSnapshot(t, db, "profiles", "entitlements", "daily_noin_earned", "daily_quickplay_counts", "text_matches", "text_award_receipts", "billing_transactions", "store_purchases")
	if _, err := m.CorrectNoin(ctx, actor, correctionCommand(target)); err != nil {
		t.Fatal(err)
	}
	if got := correctionSnapshot(t, db, "profiles", "entitlements", "daily_noin_earned", "daily_quickplay_counts", "text_matches", "text_award_receipts", "billing_transactions", "store_purchases"); !reflect.DeepEqual(protected, got) {
		t.Fatal("correction changed game/provider value")
	}
	mustMutationSQL(t, db, `UPDATE noin_wallets SET balance=$2 WHERE account_id=$1`, target, int64(math.MaxInt64))
	before := correctionSnapshot(t, db, "noin_ledger", "noin_wallets", "admin_operation_decisions", "admin_operation_results", "admin_audit_log")
	if _, err := m.CorrectNoin(ctx, actor, correctionCommand(target)); err == nil {
		t.Fatal("wallet overflow accepted")
	}
	if got := correctionSnapshot(t, db, "noin_ledger", "noin_wallets", "admin_operation_decisions", "admin_operation_results", "admin_audit_log"); !reflect.DeepEqual(before, got) {
		t.Fatal("overflow left pending decision/effect")
	}
}

func TestNoinCorrectionsConcurrentReplayRefundAndSpend(t *testing.T) {
	db := operatorDB(t)
	_, actor, sid, _, ctx := operatorActor(t, db)
	target := newAccount(t, db)
	m := economy.NewManager(db, testConfig())
	c := correctionCommand(target)
	parallel := func(n int, fn func(int) error) []error {
		var wg sync.WaitGroup
		results := make([]error, n)
		for i := range n {
			wg.Add(1)
			go func() { defer wg.Done(); results[i] = fn(i) }()
		}
		wg.Wait()
		return results
	}
	for _, err := range parallel(12, func(int) error { _, err := m.CorrectNoin(ctx, actor, c); return err }) {
		if err != nil {
			t.Fatal(err)
		}
	}
	if balance, err := m.Wallet.Balance(ctx, target); err != nil || balance != 100 {
		t.Fatal("grant duplicated", balance, err)
	}
	changed := c
	changed.Amount++
	if _, err := m.CorrectNoin(ctx, actor, changed); err == nil {
		t.Fatal("changed request accepted")
	}
	if err := m.Wallet.Debit(ctx, target, 40, "Original spend"); err != nil {
		t.Fatal(err)
	}
	var source int64
	var original string
	if err := db.QueryRow(`SELECT id,to_jsonb(l)::text FROM noin_ledger l WHERE account_id=$1 AND event_type='spend'`, target).Scan(&source, &original); err != nil {
		t.Fatal(err)
	}
	results := parallel(8, func(i int) error {
		if i == 7 {
			return m.Wallet.Debit(ctx, target, 10, "Concurrent spend")
		}
		_, err := m.CorrectNoin(ctx, actor, store.AdminOperationCommand{ID: uuid.NewString(), Kind: "noin_refund", TargetAccountID: target, SourceLedgerID: source, Reason: "Full source refund"})
		return err
	})
	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 2 {
		t.Fatal("refund source reused or spend failed", successes, results)
	}
	if balance, err := m.Wallet.Balance(ctx, target); err != nil || balance != 90 {
		t.Fatal("concurrent refund/spend balance", balance, err)
	}
	if sum, err := m.Wallet.LedgerSum(ctx, target); err != nil || sum != 90 {
		t.Fatal("ledger parity", sum, err)
	}
	var retained string
	if err := db.QueryRow(`SELECT to_jsonb(l)::text FROM noin_ledger l WHERE id=$1`, source).Scan(&retained); err != nil || retained != original {
		t.Fatal("original spend changed", err)
	}
	mustMutationSQL(t, db, `DELETE FROM admin_sessions WHERE id=$1`, sid)
	if _, err := m.CorrectNoin(ctx, actor, c); err == nil {
		t.Fatal("revoked replay succeeded")
	}
}

func TestNoinCorrectionEveryWriteFailureRollsBack(t *testing.T) {
	for _, failure := range []string{"decision_audit", "ledger", "delivery_receipt", "delivery_audit"} {
		t.Run(failure, func(t *testing.T) {
			db := operatorDB(t)
			_, actor, _, _, ctx := operatorActor(t, db)
			target := newAccount(t, db)
			m := economy.NewManager(db, testConfig())
			table, when := "admin_audit_log", `NEW.action='operator_decision'`
			switch failure {
			case "ledger":
				table = "noin_ledger"
				when = "true"
			case "delivery_receipt":
				table = "admin_operation_results"
				when = "true"
			case "delivery_audit":
				when = `NEW.action='operator_applied'`
			}
			mustMutationSQL(t, db, fmt.Sprintf(`CREATE FUNCTION fail_correction() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF %s THEN RAISE EXCEPTION 'injected correction failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER injected_correction BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION fail_correction()`, when, table))
			before := correctionSnapshot(t, db, "noin_wallets", "noin_ledger", "admin_operation_decisions", "admin_operation_results", "admin_audit_log")
			if receipt, err := m.CorrectNoin(ctx, actor, correctionCommand(target)); err == nil || receipt.Status != "" {
				t.Fatal("failed correction claimed success", receipt, err)
			}
			if got := correctionSnapshot(t, db, "noin_wallets", "noin_ledger", "admin_operation_decisions", "admin_operation_results", "admin_audit_log"); !reflect.DeepEqual(before, got) {
				t.Fatal("failure committed pending decision/value")
			}
		})
	}
}

func TestNoinCorrectionSessionExpiryAfterAccountAndWalletWait(t *testing.T) {
	for _, lock := range []string{"account", "operation", "wallet"} {
		t.Run(lock, func(t *testing.T) {
			db := operatorDB(t)
			_, actor, sid, _, bound := operatorActor(t, db)
			target := newAccount(t, db)
			m := economy.NewManager(db, testConfig())
			c := correctionCommand(target)
			mustMutationSQL(t, db, `INSERT INTO noin_wallets(account_id,balance) VALUES($1,0)`, target)
			before := correctionSnapshot(t, db, "noin_wallets", "noin_ledger", "admin_operation_decisions", "admin_operation_results", "admin_audit_log")
			mustMutationSQL(t, db, `UPDATE admin_sessions SET expires_at=clock_timestamp()+interval '700 milliseconds' WHERE id=$1`, sid)
			blocker, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback()
			query, needle := `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, "SELECT id FROM accounts"
			var args = []any{target}
			if lock == "wallet" {
				query = `SELECT balance FROM noin_wallets WHERE account_id=$1 FOR UPDATE`
				needle = "SELECT balance FROM noin_wallets"
			}
			if lock == "operation" {
				query = `SELECT pg_advisory_xact_lock(hashtextextended($1,27))`
				args = []any{"admin_operation:" + c.ID}
				needle = "pg_advisory_xact_lock"
			}
			if _, err = blocker.Exec(query, args...); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := m.CorrectNoin(bound, actor, c); done <- err }()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			waitAdminSessionLock(t, ctx, db, needle)
			for {
				var expired bool
				if err := db.QueryRow(`SELECT clock_timestamp()>=expires_at FROM admin_sessions WHERE id=$1`, sid).Scan(&expired); err != nil {
					t.Fatal(err)
				}
				if expired {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(10 * time.Millisecond):
				}
			}
			if err = blocker.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err == nil {
				t.Fatal("expired after wait accepted")
			}
			if got := correctionSnapshot(t, db, "noin_wallets", "noin_ledger", "admin_operation_decisions", "admin_operation_results", "admin_audit_log"); !reflect.DeepEqual(before, got) {
				t.Fatal("expired operation wrote value/decision")
			}
		})
	}
}

func TestNoinCorrectionHTTPRefusals(t *testing.T) {
	db := operatorDB(t)
	m, actor, sid, csrf, _ := operatorActor(t, db)
	target := newAccount(t, db)
	for _, name := range []string{"csrf", "anonymous", "duplicate", "unknown", "partial", "oversize", "query", "role"} {
		t.Run(name, func(t *testing.T) {
			form := url.Values{"csrf_token": {csrf}, "id": {uuid.NewString()}, "kind": {"noin_grant"}, "target_account_id": {target}, "amount": {"1"}, "reason": {"Fixture"}}
			path := "/admin/economy/corrections"
			switch name {
			case "csrf":
				form.Set("csrf_token", "bad")
			case "duplicate":
				form.Add("amount", "2")
			case "unknown":
				form.Set("unreviewed", "1")
			case "partial":
				form.Set("kind", "noin_refund")
				form.Set("source_ledger_id", "1")
			case "oversize":
				form.Set("reason", strings.Repeat("x", 17<<10))
			case "query":
				path += "?amount=999"
			case "role":
				mustMutationSQL(t, db, `UPDATE admin_accounts SET role='curator' WHERE id=$1`, actor)
			}
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if name != "anonymous" {
				r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
			}
			w := httptest.NewRecorder()
			m.Handler(nil).ServeHTTP(w, r)
			if name == "anonymous" {
				if w.Code != 303 || w.Header().Get("Location") != "/admin/login" {
					t.Fatal("anonymous correction", w.Code)
				}
			} else if w.Code < 400 {
				t.Fatal("invalid HTTP correction", name, w.Code, w.Body.String())
			}
			var n int
			if err := db.QueryRow(`SELECT count(*) FROM admin_operation_decisions`).Scan(&n); err != nil || n != 0 {
				t.Fatal("refused request wrote decision", n, err)
			}
		})
	}
}
