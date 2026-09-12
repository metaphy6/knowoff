package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/lib/pq"
)

func operatorDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupTestDB(t)
	t.Cleanup(func() { db.Close() })
	return db
}

func operatorActor(t *testing.T, db *sql.DB) (*Manager, string, string, string, context.Context) {
	t.Helper()
	m := NewManager(db, testConfig(), nil)
	account := newAccount(t, db)
	if err := m.CreateAdmin(t.Context(), account, account+"@test.local", "operator-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(t.Context(), account+"@test.local", "operator-password")
	if err != nil {
		t.Fatal(err)
	}
	sid, csrf, _, err := m.CreateSession(t.Context(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := store.WithAdminAuthorization(t.Context(), a.ID, func(ctx context.Context, tx *sql.Tx) (string, error) {
		return m.AuthorizeSessionTx(ctx, tx, sid, csrf)
	})
	return m, a.ID, sid, csrf, ctx
}

func TestOperatorDecisionExactActorReplayAndNoPrematureValue(t *testing.T) {
	db := operatorDB(t)
	_, actor, sid, _, ctx := operatorActor(t, db)
	target := newAccount(t, db)
	s := store.NewAdminOperationStore(db)
	command := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "noin_grant", TargetAccountID: target, Amount: 17, Reason: "Reviewed correction"}
	var group sync.WaitGroup
	errorsSeen := make(chan error, 12)
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			receipt, err := s.Decide(ctx, actor, command, []string{target, target})
			if err == nil && (receipt.Status != "pending" || receipt.Command.ID != command.ID || len(receipt.AffectedAccounts) != 1) {
				err = errors.New("incorrect receipt")
			}
			errorsSeen <- err
		}()
	}
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Error(err)
		}
	}
	var decisions, audit, ledger int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM admin_operation_decisions),(SELECT count(*) FROM admin_audit_log WHERE action='operator_decision'),(SELECT count(*) FROM noin_ledger)`).Scan(&decisions, &audit, &ledger); err != nil || decisions != 1 || audit != 1 || ledger != 0 {
		t.Fatalf("decision/audit/value=%d/%d/%d error=%v", decisions, audit, ledger, err)
	}
	command.Amount++
	if _, err := s.Decide(ctx, actor, command, []string{target}); err == nil {
		t.Fatal("changed payload reused request identity")
	}
	command.Amount--
	if _, err := s.Decide(t.Context(), actor, command, []string{target}); err == nil {
		t.Fatal("new operator decision accepted unnamed browser authority")
	}
	if _, err := db.Exec(`DELETE FROM admin_sessions WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Decide(ctx, actor, command, []string{target}); err == nil {
		t.Fatal("revoked initiating session replayed success")
	}
}

func TestOperatorReceiptIsImmutableAndAuditFailureRollsBack(t *testing.T) {
	db := operatorDB(t)
	_, actor, _, _, ctx := operatorActor(t, db)
	target := newAccount(t, db)
	s := store.NewAdminOperationStore(db)
	command := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "noin_grant", TargetAccountID: target, Amount: 10, Reason: "Review"}
	if _, err := s.Decide(ctx, actor, command, []string{target}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE admin_operation_decisions SET reason=reason`,
		`DELETE FROM admin_operation_decisions`,
		`TRUNCATE admin_operation_decisions CASCADE`,
		`UPDATE admin_audit_log SET action=action`,
		`DELETE FROM admin_audit_log`,
		`TRUNCATE admin_audit_log CASCADE`,
	} {
		if _, err := db.Exec(query); err == nil {
			t.Fatalf("immutable history accepted %s", query)
		}
	}
	if _, err := db.Exec(`CREATE FUNCTION test_operator_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit failure'; END $$; CREATE TRIGGER test_operator_audit_failure BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION test_operator_audit_failure()`); err != nil {
		t.Fatal(err)
	}
	command.ID = uuid.NewString()
	if _, err := s.Decide(ctx, actor, command, []string{target}); err == nil {
		t.Fatal("audit failure accepted a decision")
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM admin_operation_decisions WHERE id=$1`, command.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("failed decision persisted %d %v", n, err)
	}
}

func TestOperatorReplayRechecksExpiryAfterDecisionIdentityWait(t *testing.T) {
	db := operatorDB(t)
	_, actor, sid, _, ctx := operatorActor(t, db)
	target := newAccount(t, db)
	s := store.NewAdminOperationStore(db)
	command := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "noin_grant", TargetAccountID: target, Amount: 10, Reason: "Review"}
	if _, err := s.Decide(ctx, actor, command, []string{target}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE admin_sessions SET expires_at=clock_timestamp()+interval '200 milliseconds' WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	blocker, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err = blocker.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,27))`, "admin_operation:"+command.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Decide(ctx, actor, command, []string{target}); done <- err }()
	select {
	case err := <-done:
		t.Fatalf("did not serialize on decision identity: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expired session returned replay success")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("decision did not resume")
	}
}

func TestOperatorSchemaRejectsMissingTypedAmount(t *testing.T) {
	db := operatorDB(t)
	_, actor, _, _, _ := operatorActor(t, db)
	target := newAccount(t, db)
	_, err := db.Exec(`INSERT INTO admin_operation_decisions(id,actor_admin_id,kind,target_account_id,reason,affected_accounts,request_hash) VALUES($1,$2,'noin_grant',NULLIF($3,'')::uuid,'Missing amount',jsonb_build_array($3::text),decode(repeat('00',32),'hex'))`, uuid.NewString(), actor, target)
	if err == nil {
		t.Fatal("typed grant CHECK accepted a NULL amount")
	}
	var pg *pq.Error
	if !errors.As(err, &pg) || pg.Code != "23514" {
		t.Fatalf("expected CHECK refusal, not fixture failure: %v", err)
	}
}

func TestOperatorSchemaRejectsCrossKindAndNullableShapes(t *testing.T) {
	db := operatorDB(t)
	_, actor, _, _, _ := operatorActor(t, db)
	target := newAccount(t, db)
	type shape struct {
		kind, target, room, owner  string
		generation, source, amount any
		prior                      string
		affected                   string
	}
	base := shape{kind: "noin_grant", target: target, amount: 1, affected: `["` + target + `"]`}
	for _, name := range []string{"unknown_kind", "zero_amount", "negative_amount", "refund_no_source", "grant_with_source", "room_no_generation", "kick_no_target", "lift_no_prior", "sanction_with_amount", "affected_null", "affected_object", "affected_empty", "affected_wrong_type", "affected_wrong_account"} {
		t.Run(name, func(t *testing.T) {
			s := base
			switch name {
			case "unknown_kind":
				s.kind = "unreviewed"
			case "zero_amount":
				s.amount = 0
			case "negative_amount":
				s.amount = -1
			case "refund_no_source":
				s.kind = "noin_refund"
			case "grant_with_source":
				s.source = 9999999
			case "room_no_generation":
				s.kind = "room_close"
				s.target = ""
				s.room = uuid.NewString()
				s.owner = uuid.NewString()
				s.amount = nil
			case "kick_no_target":
				s.kind = "room_kick"
				s.target = ""
				s.room = uuid.NewString()
				s.owner = uuid.NewString()
				s.generation = 1
				s.amount = nil
			case "lift_no_prior":
				s.kind = "sanction_lift"
				s.amount = nil
			case "sanction_with_amount":
				s.kind = "account_sanction"
			case "affected_null":
				s.affected = "null"
			case "affected_object":
				s.affected = "{}"
			case "affected_empty":
				s.affected = "[]"
			case "affected_wrong_type":
				s.affected = "[3]"
			case "affected_wrong_account":
				s.affected = `["` + uuid.NewString() + `"]`
			}
			_, err := db.Exec(`INSERT INTO admin_operation_decisions(id,actor_admin_id,kind,target_account_id,room_id,owner_id,owner_generation,source_ledger_id,amount,prior_sanction_id,reason,affected_accounts,request_hash) VALUES($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,$7,$8,$9,NULLIF($10,'')::uuid,'fixture',$11,decode(repeat('02',32),'hex'))`, uuid.NewString(), actor, s.kind, s.target, s.room, s.owner, s.generation, s.source, s.amount, s.prior, s.affected)
			var constraint *pq.Error
			if !errors.As(err, &constraint) || constraint.Code != "23514" {
				t.Fatalf("shape escaped typed CHECK or fixture failed elsewhere: %v", err)
			}
		})
	}
}

func TestOperatorCompletionIsAtomicAndRetainedAfterActorRevocation(t *testing.T) {
	db := operatorDB(t)
	_, actor, sid, _, ctx := operatorActor(t, db)
	target := newAccount(t, db)
	s := store.NewAdminOperationStore(db)
	command := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "noin_grant", TargetAccountID: target, Amount: 7, Reason: "Synthetic delivery boundary"}
	if _, err := s.Decide(ctx, actor, command, []string{target}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM admin_sessions WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	result := store.AdminOperationResult{Outcome: "applied", Detail: "fixture_receipt_only"}
	for i := 0; i < 2; i++ {
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.CompleteTx(t.Context(), tx, command.ID, result); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	receipt, err := s.Get(t.Context(), command.ID)
	if err != nil || receipt.Status != "applied" || receipt.CompletedAt == nil {
		t.Fatalf("completion=%+v %v", receipt, err)
	}
	for _, query := range []string{`UPDATE admin_operation_results SET outcome=outcome`, `DELETE FROM admin_operation_results`, `TRUNCATE admin_operation_results`} {
		if _, err = db.Exec(query); err == nil {
			t.Fatalf("rewrote completion: %s", query)
		}
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	result.Detail = "changed"
	if err = s.CompleteTx(t.Context(), tx, command.ID, result); err == nil {
		t.Fatal("changed delivery replay succeeded")
	}
	var n, ledger int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM admin_audit_log WHERE action='operator_applied'),(SELECT count(*) FROM noin_ledger)`).Scan(&n, &ledger); err != nil || n != 1 || ledger != 0 {
		t.Fatalf("delivery duplicated or fabricated value: %d/%d %v", n, ledger, err)
	}
}

func TestOperatorHTTPGuardsAndPendingStatus(t *testing.T) {
	db := operatorDB(t)
	m, actor, sid, csrf, _ := operatorActor(t, db)
	target := newAccount(t, db)
	s := store.NewAdminOperationStore(db)
	commandID := uuid.NewString()
	form := url.Values{"id": {commandID}, "kind": {"noin_grant"}, "target_account_id": {target}, "amount": {"9"}, "reason": {"<script>inert correction</script>"}}
	calls := 0
	handler := m.OperatorHandler(OperatorHooks{Decide: func(ctx context.Context, id string, c store.AdminOperationCommand) (store.AdminOperationReceipt, error) {
		calls++
		if id != actor {
			t.Fatal("browser forged actor")
		}
		return s.Decide(ctx, id, c, []string{target})
	}})
	call := func(h http.Handler, method, path, body, key string, authenticated bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if authenticated {
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
		}
		if key != "" {
			r.Header.Set("X-CSRF-Token", key)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call(handler, "POST", "/admin/operators", form.Encode(), csrf, false); w.Code != 303 {
		t.Fatalf("anonymous status=%d", w.Code)
	}
	if w := call(handler, "POST", "/admin/operators", form.Encode(), "", true); w.Code != 403 {
		t.Fatalf("missing CSRF=%d", w.Code)
	}
	if w := call(m.OperatorHandler(OperatorHooks{}), "POST", "/admin/operators", form.Encode(), csrf, true); w.Code != 503 {
		t.Fatalf("unimplemented product operation=%d", w.Code)
	}
	for _, extra := range []string{"&amount=10", "&affected_accounts=forged", "&actor_admin_id=" + target} {
		if w := call(handler, "POST", "/admin/operators", form.Encode()+extra, csrf, true); w.Code != 400 {
			t.Fatalf("ambiguous request=%d", w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("rejected request reached domain hook")
	}
	if w := call(handler, "POST", "/admin/operators", form.Encode(), csrf, true); w.Code != 303 || w.Header().Get("Location") != "/admin/operators/"+commandID {
		t.Fatalf("decision status=%d %s", w.Code, w.Body.String())
	}
	w := call(handler, "GET", "/admin/operators/"+commandID, "", "", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "pending") || strings.Contains(w.Body.String(), "<script>inert") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("private inert pending page=%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), target) || !strings.Contains(w.Body.String(), "<dt>Noin amount</dt><dd>9</dd>") {
		t.Fatal("receipt omitted the account and amount the operator actually authorized")
	}
	if w = call(handler, "GET", "/admin/operators", "", "", true); w.Code != 200 || !strings.Contains(w.Body.String(), commandID) {
		t.Fatalf("history=%d %s", w.Code, w.Body.String())
	}
}

func TestOperatorCompletionAuditFailureRollsBackDomainAndReceipt(t *testing.T) {
	db := operatorDB(t)
	_, actor, sid, _, ctx := operatorActor(t, db)
	target := newAccount(t, db)
	s := store.NewAdminOperationStore(db)
	c := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "noin_grant", TargetAccountID: target, Amount: 7, Reason: "Fixture transaction boundary"}
	if _, err := s.Decide(ctx, actor, c, []string{target}); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := db.QueryRow(`SELECT nickname FROM accounts WHERE id=$1`, target).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE FUNCTION refuse_operator_completion_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='operator_applied' THEN RAISE EXCEPTION 'fixture audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER refuse_operator_completion_audit BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION refuse_operator_completion_audit()`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.LockAdminTx(ctx, tx, actor, []string{"admin"}, target); err != nil {
		t.Fatal(err)
	}
	// This disposable marker stands for a domain write in the caller-owned TX;
	// the receipt primitive itself must never manufacture a wallet effect.
	if _, err = tx.Exec(`UPDATE accounts SET nickname='fixture uncommitted' WHERE id=$1`, target); err != nil {
		t.Fatal(err)
	}
	result := store.AdminOperationResult{Outcome: "applied", Detail: "fixture_receipt_only"}
	if err = s.CompleteTx(ctx, tx, c.ID, result); err == nil {
		t.Fatal("audit failure accepted completion")
	}
	tx.Rollback()
	var current string
	if err = db.QueryRow(`SELECT nickname FROM accounts WHERE id=$1`, target).Scan(&current); err != nil || current != original {
		t.Fatal("failed audit left domain effect", current, err)
	}
	if got, err := s.Get(t.Context(), c.ID); err != nil || got.Status != "pending" {
		t.Fatal("failed audit left completion", got, err)
	}
	if _, err = db.Exec(`DROP TRIGGER refuse_operator_completion_audit ON admin_audit_log; DROP FUNCTION refuse_operator_completion_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM admin_sessions WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	tx, err = db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteTx(ctx, tx, c.ID, result); err == nil {
		t.Fatal("stale browser session completed pending work")
	}
	tx.Rollback()
	var group sync.WaitGroup
	errorsSeen := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			tx, e := db.BeginTx(t.Context(), nil)
			if e == nil {
				defer tx.Rollback()
				e = s.CompleteTx(t.Context(), tx, c.ID, result)
				if e == nil {
					e = tx.Commit()
				}
			}
			errorsSeen <- e
		}()
	}
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Error(err)
		}
	}
	var completions, audits, ledger int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM admin_operation_results),(SELECT count(*) FROM admin_audit_log WHERE action='operator_applied'),(SELECT count(*) FROM noin_ledger)`).Scan(&completions, &audits, &ledger); err != nil || completions != 1 || audits != 1 || ledger != 0 {
		t.Fatalf("completion/audit/value=%d/%d/%d %v", completions, audits, ledger, err)
	}
	tx, err = db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = s.CompleteTx(ctx, tx, c.ID, result); err == nil {
		t.Fatal("stale browser replayed completed success")
	}
}

func TestOperatorHTTPRechecksAuthorityAfterMiddleware(t *testing.T) {
	for _, change := range []string{"session", "csrf", "role", "ban", "suspension"} {
		t.Run(change, func(t *testing.T) {
			db := operatorDB(t)
			m, actor, sid, csrf, _ := operatorActor(t, db)
			target := newAccount(t, db)
			s := store.NewAdminOperationStore(db)
			called := false
			h := m.OperatorHandler(OperatorHooks{Decide: func(ctx context.Context, id string, c store.AdminOperationCommand) (store.AdminOperationReceipt, error) {
				called = true
				var err error
				switch change {
				case "session":
					_, err = db.Exec(`DELETE FROM admin_sessions WHERE id=$1`, sid)
				case "csrf":
					_, err = db.Exec(`UPDATE admin_sessions SET csrf_token='replacement' WHERE id=$1`, sid)
				case "role":
					_, err = db.Exec(`UPDATE admin_accounts SET role='moderator' WHERE id=$1`, actor)
				case "ban":
					_, err = db.Exec(`UPDATE accounts SET banned_at=clock_timestamp() WHERE id=(SELECT account_id FROM admin_accounts WHERE id=$1)`, actor)
				case "suspension":
					_, err = db.Exec(`UPDATE accounts SET suspended_until=clock_timestamp()+interval '1 hour' WHERE id=(SELECT account_id FROM admin_accounts WHERE id=$1)`, actor)
				}
				if err != nil {
					t.Fatal(err)
				}
				return s.Decide(ctx, id, c, []string{target})
			}})
			form := url.Values{"id": {uuid.NewString()}, "kind": {"noin_grant"}, "target_account_id": {target}, "amount": {"5"}, "reason": {"Reviewed"}}
			r := httptest.NewRequest("POST", "/admin/operators", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("X-CSRF-Token", csrf)
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if !called || w.Code != 409 {
				t.Fatalf("initial authorization/transaction recheck=%v/%d %s", called, w.Code, w.Body.String())
			}
			var count int
			if err := db.QueryRow(`SELECT (SELECT count(*) FROM admin_operation_decisions)+(SELECT count(*) FROM admin_operation_results)+(SELECT count(*) FROM admin_audit_log WHERE action LIKE 'operator_%')+(SELECT count(*) FROM noin_ledger)`).Scan(&count); err != nil || count != 0 {
				t.Fatal("revoked actor wrote durable effects", count, err)
			}
		})
	}
}

func TestOperatorReceiptAPIWorksWithInsertOnlyPrivileges(t *testing.T) {
	db := operatorDB(t)
	_, actor, _, _, ctx := operatorActor(t, db)
	target := newAccount(t, db)
	role := "operator_receipt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pq.QuoteIdentifier(role)
	if _, err := db.Exec(`CREATE ROLE ` + quoted + ` NOLOGIN; GRANT USAGE ON SCHEMA public TO ` + quoted + `; GRANT SELECT ON ALL TABLES IN SCHEMA public TO ` + quoted + `; GRANT UPDATE ON accounts,admin_accounts,admin_sessions TO ` + quoted + `; GRANT INSERT ON admin_operation_decisions,admin_operation_results,admin_audit_log TO ` + quoted + `; GRANT USAGE ON SEQUENCE admin_audit_log_id_seq TO ` + quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DROP OWNED BY ` + quoted + `; DROP ROLE ` + quoted); err != nil {
			t.Error(err)
		}
	})
	// A dedicated single-connection pool holds SET ROLE for every tested API.
	// Authenticated runtime/capture role creation is separately exercised by the
	// complete cutover-role fixture; this proves receipt APIs need no UPDATE ACL.
	restricted, err := sql.Open("postgres", os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	restricted.SetMaxOpenConns(1)
	defer restricted.Close()
	if _, err = restricted.Exec(`SET ROLE ` + quoted); err != nil {
		t.Fatal(err)
	}
	var effective string
	if err = restricted.QueryRow(`SELECT current_user`).Scan(&effective); err != nil || effective != role {
		t.Fatal("fixture role missing", effective, err)
	}
	s := store.NewAdminOperationStore(restricted)
	c := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "noin_grant", TargetAccountID: target, Amount: 3, Reason: "Restricted receipt fixture"}
	if _, err = s.Decide(ctx, actor, c, []string{target}); err != nil {
		t.Fatal("insert-only decision failed", err)
	}
	if _, err = s.Decide(ctx, actor, c, []string{target}); err != nil {
		t.Fatal("insert-only replay failed", err)
	}
	tx, err := restricted.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteTx(t.Context(), tx, c.ID, store.AdminOperationResult{Outcome: "obsolete", Detail: "fixture_only"}); err != nil {
		tx.Rollback()
		t.Fatal("insert-only completion failed", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if receipt, err := s.Get(t.Context(), c.ID); err != nil || receipt.Status != "obsolete" {
		t.Fatal("insert-only read failed", receipt, err)
	}
	for _, table := range []string{"admin_operation_decisions", "admin_operation_results", "admin_audit_log"} {
		for _, verb := range []string{"UPDATE " + table + " SET " + map[string]string{"admin_operation_decisions": "reason=reason", "admin_operation_results": "outcome=outcome", "admin_audit_log": "action=action"}[table], "DELETE FROM " + table, "TRUNCATE " + table} {
			_, err := restricted.Exec(verb)
			var denied *pq.Error
			if !errors.As(err, &denied) || denied.Code != "42501" {
				t.Fatal("retained mutation must be refused by ACL before trigger", verb, err)
			}
		}
	}
}
