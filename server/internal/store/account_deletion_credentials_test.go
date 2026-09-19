package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const privacyCredentialsErase = `SELECT public.privacy_erase_credentials_batch($1,$2)`

type privacyCredentialFixture struct {
	account, admin, token string
	proof                 []any
	confirmation          []byte
}

func privacyCredentialsFixture(t *testing.T, db *sql.DB, bound bool) privacyCredentialFixture {
	t.Helper()
	f := privacyCredentialFixture{account: valueAccount(t, db), admin: uuid.NewString(), token: uuid.NewString()}
	f.proof = privacyContentProof(t, db, f.account)
	if err := db.QueryRow(privacyContentConfirm, f.proof...).Scan(&f.confirmation); err != nil {
		t.Fatal(err)
	}
	if bound {
		if _, err := db.Exec(`SELECT public.privacy_bind_suppression($1,$2,$3)`, f.proof[4], int64(1), bytes.Repeat([]byte{73}, 32)); err != nil {
			t.Fatal(err)
		}
	}
	// These synthetic leftover session rows deliberately exercise the executor's
	// postcondition independently of confirmation's earlier session revocation.
	marker := uuid.NewString()
	for _, stmt := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret,backup_codes) VALUES($1,$2,$3,'private-password','private-totp',ARRAY['private-backup'])`, []any{f.admin, f.account, marker + "@example.invalid"}},
		{`INSERT INTO admin_sessions(id,admin_id,csrf_token,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, []any{uuid.NewString(), f.admin, "private-csrf-" + marker}},
		{`INSERT INTO oauth_links(account_id,provider,provider_subject,provider_email) VALUES($1,'google',$2,$3)`, []any{f.account, "private-provider-" + marker, "private-email-" + marker}},
		{`INSERT INTO oauth_flows(id,provider,intent,state_hash,completion_hash,nonce_hash,code_verifier,requester_hash,account_id,initiating_token_id,session_epoch,expires_at) VALUES($1,'google','link',$2,$3,$3,$3,$3,$4,$5,0,clock_timestamp()+interval '5 minutes')`, []any{uuid.NewString(), marker + "1234567", strings.Repeat("x", 43), f.account, f.token}},
		{`INSERT INTO auth_revocations(token_id,expires_at) VALUES($1,clock_timestamp()+interval '1 hour')`, []any{f.token}},
		{`INSERT INTO portal_browser_sessions(token_hash,account_id,csrf_token,expires_at) VALUES($1,$2,'private-browser-csrf',clock_timestamp()+interval '1 hour')`, []any{"private-browser-" + marker, f.account}},
		{`INSERT INTO portal_login_requests(browser_hash,pairing_code,csrf_token,account_id,expires_at) VALUES($1,$2,'private-pair-csrf',$3,clock_timestamp()+interval '1 hour')`, []any{"private-pair-" + marker, marker, f.account}},
		{`INSERT INTO privacy_deletion_intents(id,kind,account_id,secret_sha256,expires_at,state,session_epoch,initiating_token_id,credential_until,installation) VALUES($1,'enroll',$2,$3,clock_timestamp()+interval '1 minute','pending',0,$4,clock_timestamp()+interval '1 minute','private-installation')`, []any{uuid.NewString(), f.account, bytes.Repeat([]byte{8}, 32), f.token}},
		{`INSERT INTO admin_audit_log(admin_id,action,before_state,after_state) VALUES($1,'survivor-test','{}','{}')`, []any{f.admin}},
	} {
		if _, err := db.Exec(stmt.q, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}
	return f
}
func privacyCredentialsOwnedCount(t *testing.T, db *sql.DB, f privacyCredentialFixture) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT
 (SELECT count(*) FROM oauth_links WHERE account_id=$1)+
 (SELECT count(*) FROM oauth_flows WHERE account_id=$1)+
 (SELECT count(*) FROM portal_browser_sessions WHERE account_id=$1)+
 (SELECT count(*) FROM portal_login_requests WHERE account_id=$1)+
 (SELECT count(*) FROM admin_sessions WHERE admin_id=$2)+
 (SELECT count(*) FROM admin_accounts WHERE id=$2 AND (email IS NOT NULL OR password_hash IS NOT NULL OR totp_secret IS NOT NULL OR cardinality(backup_codes)>0))+
 (SELECT count(*) FROM privacy_deletion_intents WHERE account_id=$1)+
 (SELECT count(*) FROM privacy_deletion_capabilities WHERE account_id=$1)+
 (SELECT count(*) FROM auth_revocations WHERE token_id=$3)`, f.account, f.admin, f.token).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
func privacyCredentialsRun(t *testing.T, db *sql.DB, request any, limit int) (int, bool) {
	t.Helper()
	var raw []byte
	if err := db.QueryRow(privacyCredentialsErase, request, limit).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Processed int  `json:"processed"`
		Complete  bool `json:"complete"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Processed < 0 || result.Processed > limit {
		t.Fatal("unbounded credential batch", string(raw))
	}
	return result.Processed, result.Complete
}
func TestPrivacyCredentialsBoundedErasurePreservesExactConfirmation(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, true)
	survivor := valueAccount(t, db)
	survivorMarker := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO oauth_links(account_id,provider,provider_subject,provider_email) VALUES($1,'google',$2,$2)`, survivor, survivorMarker); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auth_revocations(token_id,expires_at) VALUES($1,clock_timestamp()+interval '1 hour')`, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	var survivorBefore, requestBefore string
	if err := db.QueryRow(`SELECT to_jsonb(l)::text FROM oauth_links l WHERE account_id=$1`, survivor).Scan(&survivorBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT to_jsonb(r)::text FROM privacy_requests r WHERE id=$1`, f.proof[4]).Scan(&requestBefore); err != nil {
		t.Fatal(err)
	}
	complete := false
	for i := 0; i < 100 && !complete; i++ {
		before := privacyCredentialsOwnedCount(t, db, f)
		processed, done := privacyCredentialsRun(t, db, f.proof[4], 1)
		after := privacyCredentialsOwnedCount(t, db, f)
		if before-after != processed {
			t.Fatalf("batch reported %d but erased %d rows", processed, before-after)
		}
		complete = done
	}
	if !complete || privacyCredentialsOwnedCount(t, db, f) != 0 {
		t.Fatal("credential removal incomplete")
	}
	for range 2 {
		var got []byte
		if err := db.QueryRow(privacyContentConfirm, f.proof...).Scan(&got); err != nil || !bytes.Equal(got, f.confirmation) {
			t.Fatal("original confirmation no longer replayable", err)
		}
	}
	var survivorAfter, requestAfter string
	if err := db.QueryRow(`SELECT to_jsonb(l)::text FROM oauth_links l WHERE account_id=$1`, survivor).Scan(&survivorAfter); err != nil || survivorBefore != survivorAfter {
		t.Fatal("survivor credential changed", err)
	}
	if err := db.QueryRow(`SELECT to_jsonb(r)::text FROM privacy_requests r WHERE id=$1`, f.proof[4]).Scan(&requestAfter); err != nil || requestBefore != requestAfter {
		t.Fatal("credential cleanup/replay changed request obligations", err)
	}
	if valueCount(t, db, `SELECT count(*) FROM auth_revocations`) != 1 {
		t.Fatal("unknown revocation swept")
	}
	if valueCount(t, db, `SELECT count(*) FROM admin_audit_log WHERE admin_id=$1`, f.admin) != 1 {
		t.Fatal("survivor actor provenance erased")
	}
	if valueCount(t, db, `SELECT count(*) FROM privacy_step_receipts WHERE request_id=$1 AND step='credentials' AND result_code='credentials_removed'`, f.proof[4]) != 1 {
		t.Fatal("credential receipt absent")
	}
	if processed, done := privacyCredentialsRun(t, db, f.proof[4], 128); processed != 0 || !done {
		t.Fatal("completed batch not idempotent")
	}
	for i := range f.proof {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			changed := append([]any{}, f.proof...)
			switch i {
			case 1, 3, 5:
				changed[i] = bytes.Repeat([]byte{91}, 32)
			default:
				changed[i] = uuid.NewString()
			}
			if _, err := db.Exec(privacyContentConfirm, changed...); err == nil {
				t.Fatal("changed confirmation tuple accepted")
			}
		})
	}
	for _, i := range []int{2, 3} {
		changed := append([]any{}, f.proof...)
		changed[i] = nil
		if _, err := db.Exec(privacyContentConfirm, changed...); err == nil {
			t.Fatal("changed null tuple accepted")
		}
	}
}

func TestPrivacyCredentialsRefuseMissingSuppressionAndBadLimits(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, false)
	before := privacyCredentialsOwnedCount(t, db, f)
	for _, limit := range []any{nil, -1, 0, 1, 129} {
		if _, err := db.Exec(privacyCredentialsErase, f.proof[4], limit); err == nil {
			t.Fatal("invalid or unbound batch accepted", limit)
		}
	}
	if privacyCredentialsOwnedCount(t, db, f) != before || valueCount(t, db, `SELECT count(*) FROM privacy_step_receipts WHERE request_id=$1`, f.proof[4]) != 0 {
		t.Fatal("refusal changed credentials")
	}
	if _, err := db.Exec(`SELECT privacy_bind_suppression($1,1,$2)`, f.proof[4], bytes.Repeat([]byte{73}, 32)); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []any{nil, -1, 0, 129} {
		if _, err := db.Exec(privacyCredentialsErase, f.proof[4], limit); err == nil {
			t.Fatal("invalid bound accepted", limit)
		}
	}
	if _, err := db.Exec(privacyCredentialsErase, uuid.NewString(), 1); err == nil {
		t.Fatal("unknown request accepted")
	}
	if privacyCredentialsOwnedCount(t, db, f) != before {
		t.Fatal("bad limit changed credentials")
	}
}

func TestPrivacyCredentialsRefuseSourceDrift(t *testing.T) {
	for _, variant := range []string{"column", "rls", "rule"} {
		t.Run(variant, func(t *testing.T) {
			db, _ := textValueDB(t)
			f := privacyCredentialsFixture(t, db, true)
			before := privacyCredentialsOwnedCount(t, db, f)
			statement := map[string]string{"column": `ALTER TABLE oauth_flows ADD COLUMN private_unknown text DEFAULT 'do not silently erase'`, "rls": `ALTER TABLE oauth_flows ENABLE ROW LEVEL SECURITY`, "rule": `CREATE RULE privacy_test_hide AS ON DELETE TO oauth_flows DO INSTEAD NOTHING`}[variant]
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(privacyCredentialsErase, f.proof[4], 128); err == nil {
				t.Fatal("unreviewed source drift accepted")
			}
			if privacyCredentialsOwnedCount(t, db, f) != before || valueCount(t, db, `SELECT count(*) FROM privacy_step_receipts WHERE request_id=$1`, f.proof[4]) != 0 {
				t.Fatal("source drift left partial cleanup")
			}
		})
	}
}

func TestPrivacyCredentialsConfirmationAndAdminErasureAreImmutable(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, true)
	for range 100 {
		_, done := privacyCredentialsRun(t, db, f.proof[4], 128)
		if done {
			break
		}
	}
	for _, stmt := range []struct {
		q    string
		args []any
	}{
		{`UPDATE privacy_requests SET confirmation_sha256=$2 WHERE id=$1`, []any{f.proof[4], bytes.Repeat([]byte{92}, 32)}},
		{`UPDATE privacy_requests SET confirmation_kind='oauth' WHERE id=$1`, []any{f.proof[4]}},
		{`UPDATE admin_accounts SET role='admin',email='revived@example.invalid',password_hash='revived',totp_secret='revived',credentials_erased_at=NULL WHERE id=$1`, []any{f.admin}},
	} {
		if _, err := db.Exec(stmt.q, stmt.args...); err == nil {
			t.Fatal("immutable credential state changed")
		}
	}
	if valueCount(t, db, `SELECT count(*) FROM admin_accounts WHERE id=$1 AND role='erased' AND email IS NULL AND password_hash IS NULL AND totp_secret IS NULL AND cardinality(backup_codes)=0 AND credentials_erased_at IS NOT NULL`, f.admin) != 1 {
		t.Fatal("erased admin shape unavailable")
	}
	var active bool
	if err := db.QueryRow(`SELECT (account_deletion_status($1)->>'active_removed')::boolean`, f.proof[5]).Scan(&active); err != nil || active {
		t.Fatal("credential-only cleanup claims active removal", err)
	}
}

func TestPrivacyCredentialsMigrationBackfillAndRefusal(t *testing.T) {
	for _, variant := range []string{"valid", "missing", "ambiguous", "mismatch"} {
		t.Run(variant, func(t *testing.T) {
			db, _ := textValueDB(t)
			down, err := os.ReadFile("../../migrations/000037_account_deletion_credentials.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			up, err := os.ReadFile("../../migrations/000037_account_deletion_credentials.up.sql")
			if err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = tx.Exec(string(down)); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			f := privacyCredentialsFixture(t, db, true)
			switch variant {
			case "missing":
				if _, err = db.Exec(`DELETE FROM privacy_deletion_intents WHERE id=$1`, f.proof[0]); err != nil {
					t.Fatal(err)
				}
			case "ambiguous":
				if _, err = db.Exec(`INSERT INTO privacy_deletion_intents(id,kind,account_id,secret_sha256,expires_at,state,capability_id,capability_sha256,security_epoch,consumed_request,consumed_at) SELECT $2,kind,account_id,secret_sha256,expires_at,state,capability_id,capability_sha256,security_epoch,consumed_request,consumed_at FROM privacy_deletion_intents WHERE id=$1`, f.proof[0], uuid.NewString()); err != nil {
					t.Fatal(err)
				}
			case "mismatch":
				if _, err = db.Exec(`UPDATE privacy_deletion_intents SET secret_sha256=$2 WHERE id=$1`, f.proof[0], bytes.Repeat([]byte{95}, 32)); err != nil {
					t.Fatal(err)
				}
			}
			tx, err = db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			_, err = tx.Exec(string(up))
			if variant != "valid" {
				if err == nil {
					t.Fatal("unproven backfill accepted")
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			var before []byte
			if err = db.QueryRow(privacyContentConfirm, f.proof...).Scan(&before); err != nil || !bytes.Equal(before, f.confirmation) {
				t.Fatal("backfilled proof cannot replay", err)
			}
			_, complete := privacyCredentialsRun(t, db, f.proof[4], 128)
			if !complete {
				t.Fatal("backfilled account not erased")
			}
			var after []byte
			if err = db.QueryRow(privacyContentConfirm, f.proof...).Scan(&after); err != nil || !bytes.Equal(after, before) {
				t.Fatal("backfilled proof cannot replay after erasure", err)
			}
			tx, err = db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = tx.Exec(string(down)); err == nil {
				t.Fatal("retained erasure allowed down")
			}
		})
	}
}

func TestPrivacyCredentialsLateLockFailureRollsBackBatch(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, true)
	before := privacyCredentialsOwnedCount(t, db, f)
	blocker, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err = blocker.Exec(`SELECT id FROM oauth_links WHERE account_id=$1 FOR SHARE`, f.account); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.ExecContext(context.Background(), `SET statement_timeout='100ms'`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(context.Background(), privacyCredentialsErase, f.proof[4], 128); err == nil || !strings.Contains(err.Error(), "statement timeout") {
		t.Fatal("late lock failure not reached", err)
	}
	if privacyCredentialsOwnedCount(t, db, f) != before || valueCount(t, db, `SELECT count(*) FROM privacy_step_receipts WHERE request_id=$1`, f.proof[4]) != 0 {
		t.Fatal("partial credentials or receipt committed")
	}
	if err = blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(context.Background(), `SET statement_timeout=0`); err != nil {
		t.Fatal(err)
	}
	if _, done := privacyCredentialsRun(t, db, f.proof[4], 128); !done {
		t.Fatal("retry failed to converge")
	}
}

func TestPrivacyCredentialsConcurrentConfirmationReplaysAfterIntentRemoval(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, true)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var raw []byte
	if err = tx.QueryRow(privacyCredentialsErase, f.proof[4], 128).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var pid int
	if err = conn.QueryRowContext(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	var replay []byte
	go func() {
		done <- conn.QueryRowContext(context.Background(), privacyContentConfirm, f.proof...).Scan(&replay)
	}()
	privacyReleaseWaitLock(t, db, pid)
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil || !bytes.Equal(replay, f.confirmation) {
		t.Fatal("post-wait exact confirmation did not replay", err)
	}
}

func TestPrivacyCredentialsPreserveSharedRevocationAndUnboundFlow(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, true)
	other := valueAccount(t, db)
	shared, unbound := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(`INSERT INTO oauth_flows(id,provider,intent,state_hash,completion_hash,nonce_hash,code_verifier,requester_hash,account_id,initiating_token_id,session_epoch,expires_at) VALUES($1,'google','link',$2,$3,$3,$3,$3,$4,$5,0,clock_timestamp()+interval '5 minutes')`, shared, uuid.NewString()+"1234567", strings.Repeat("s", 43), other, f.token); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO oauth_flows(id,provider,intent,state_hash,completion_hash,nonce_hash,code_verifier,requester_hash,expires_at) VALUES($1,'google','restore',$2,$3,$3,$3,$3,clock_timestamp()+interval '5 minutes')`, unbound, uuid.NewString()+"1234567", strings.Repeat("u", 43)); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := db.QueryRow(`SELECT jsonb_agg(to_jsonb(f) ORDER BY id)::text FROM oauth_flows f WHERE id IN($1,$2)`, shared, unbound).Scan(&before); err != nil {
		t.Fatal(err)
	}
	done := false
	for i := 0; i < 100 && !done; i++ {
		_, done = privacyCredentialsRun(t, db, f.proof[4], 1)
	}
	if !done {
		t.Fatal("shared evidence prevented scoped completion")
	}
	var after string
	if err := db.QueryRow(`SELECT jsonb_agg(to_jsonb(f) ORDER BY id)::text FROM oauth_flows f WHERE id IN($1,$2)`, shared, unbound).Scan(&after); err != nil || before != after {
		t.Fatal("shared or unbound flow changed", err)
	}
	if valueCount(t, db, `SELECT count(*) FROM auth_revocations WHERE token_id=$1`, f.token) != 1 {
		t.Fatal("survivor revocation removed")
	}
}

func TestPrivacyCredentialsConfirmationShapeRejectsPartialPair(t *testing.T) {
	db, _ := textValueDB(t)
	account := valueAccount(t, db)
	request := uuid.NewString()
	if _, err := db.Exec(`SELECT privacy_prepare_verified_request($1,$2,$3,$4)`, request, account, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE privacy_requests SET confirmation_sha256=$2 WHERE id=$1`, request, bytes.Repeat([]byte{3}, 32)); err == nil {
		t.Fatal("digest without kind accepted")
	}
	if _, err := db.Exec(`UPDATE privacy_requests SET confirmation_kind='capability' WHERE id=$1`, request); err == nil {
		t.Fatal("kind without digest accepted")
	}
}
