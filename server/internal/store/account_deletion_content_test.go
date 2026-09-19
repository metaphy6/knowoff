package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Deliberately relational fixtures, not certified/publication-ready content.
// Both source kinds are present in the same release for real executor ACL tests.
func privacyContentFixture(t *testing.T, db *sql.DB, account string) string {
	t.Helper()
	return privacyContentFixtureKinds(t, db, account, true, true)
}
func privacyContentFixtureKinds(t *testing.T, db *sql.DB, account string, portal, challenge bool) string {
	t.Helper()
	adminAccount := valueAccount(t, db)
	admin, terms, submission, entry, topic := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, statement := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$3,'fixture','fixture')`, []any{admin, adminAccount, admin + "@example.invalid"}},
		{`INSERT INTO portal_terms(version,title,body) VALUES($1,'fixture','fixture')`, []any{terms}},
		{`INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at,decided_at,decided_by) VALUES($1,$2,'text','private portal marker','approved',$3,now(),now(),$4)`, []any{submission, account, terms, admin}},
		{`INSERT INTO challenge_topics(id,week_start,week_end,nown_media_id) SELECT $1,COALESCE(max(week_start),DATE '2050-01-03')+7,COALESCE(max(week_start),DATE '2050-01-03')+14,$2 FROM challenge_topics`, []any{topic, submission}},
		{`INSERT INTO challenge_entries(id,account_id,topic_id,entry_type,content,terms_version,terms_accepted_at,status,screen_decided_at,screen_decided_by) VALUES($1,$2,$3,'text','private challenge marker',$4,now(),'approved',now(),$5)`, []any{entry, account, topic, terms, admin}},
	} {
		if _, err := db.Exec(statement.q, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	inputs := []string{uuid.NewString(), uuid.NewString()}
	for i, kind := range []string{"portal_submission", "challenge_entry"} {
		source := submission
		var sub, en any = submission, nil
		if i == 1 {
			source = entry
			sub, en = nil, entry
		}
		if _, err := db.Exec(`INSERT INTO text_accepted_inputs(id,source_kind,source_id,text_content,provenance,submission_id,entry_id) VALUES($1,$2,$3,'accepted private marker',$4,$5,$6)`, inputs[i], kind, source, `{"fixture":"retained"}`, sub, en); err != nil {
			t.Fatal(err)
		}
	}
	release := uuid.NewString()
	nowns, cards := []any{}, []any{}
	if portal {
		nowns = append(nowns, map[string]any{"provenance": map[string]string{"source_id": inputs[0]}})
	}
	if challenge {
		cards = append(cards, map[string]any{"provenance": map[string]string{"source_id": inputs[1]}})
	}
	bundle, _ := json.Marshal(map[string]any{"nowns": nowns, "cards": cards, "artifacts": map[string]string{"fixture": "original bytes"}})
	if _, err := db.Exec(`INSERT INTO text_releases(release_id,language,rules_version,manifest_sha256,snapshot_sha256,bundle,access_class,published_by) VALUES($1,'en','text-v1',$2,$3,$4,'core',$5)`, release, strings.Repeat("a", 64), strings.Repeat("b", 64), string(bundle), admin); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO text_active_releases(language,rules_version,access_key,release_id) VALUES('en','text-v1',$1,$2)`, release, release); err != nil {
		t.Fatal(err)
	}
	return release
}
func privacyContentProof(t *testing.T, db *sql.DB, account string) []any {
	t.Helper()
	intent, capID, request := uuid.NewString(), uuid.NewString(), uuid.NewString()
	nonce, capHash, status := []byte(uuid.NewString()[:32]), []byte(uuid.NewString()[:32]), []byte(uuid.NewString()[:32])
	if _, err := db.Exec(`INSERT INTO privacy_deletion_capabilities(account_id,capability_id,secret_sha256) VALUES($1,$2,$3)`, account, capID, capHash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO privacy_deletion_intents(id,kind,account_id,secret_sha256,expires_at,state,capability_id,capability_sha256,security_epoch) VALUES($1,'capability',$2,$3,clock_timestamp()+interval '1 minute','pending',$4,$5,0)`, intent, account, nonce, capID, capHash); err != nil {
		t.Fatal(err)
	}
	return []any{intent, nonce, capID, capHash, request, status, account}
}

const privacyContentConfirm = `SELECT public.privacy_confirm_deletion($1,$2,$3,$4,$5,$6,$7)`

func privacyContentSnapshot(t *testing.T, db *sql.DB, release string) []byte {
	t.Helper()
	var raw []byte
	if err := db.QueryRow(`SELECT (to_jsonb(r)-'withdrawn_at') FROM text_releases r WHERE release_id=$1`, release).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestPrivacyContentConfirmationWithdrawsExactAuthoredReleases(t *testing.T) {
	db, _ := textValueDB(t)
	account, other := valueAccount(t, db), valueAccount(t, db)
	release, unrelated := privacyContentFixture(t, db, account), privacyContentFixture(t, db, other)
	original, otherOriginal := privacyContentSnapshot(t, db, release), privacyContentSnapshot(t, db, unrelated)
	proof := privacyContentProof(t, db, account)
	var first []byte
	var at time.Time
	for i := range 2 {
		var raw []byte
		if err := db.QueryRow(privacyContentConfirm, proof...).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = raw
		} else if !bytes.Equal(first, raw) {
			t.Fatal("confirmation replay changed original receipt")
		}
		var withdrawn sql.NullTime
		if err := db.QueryRow(`SELECT withdrawn_at FROM text_releases WHERE release_id=$1`, release).Scan(&withdrawn); err != nil || !withdrawn.Valid {
			t.Fatal("authored release remains publishable", err)
		}
		if i == 0 {
			at = withdrawn.Time
		} else if !at.Equal(withdrawn.Time) {
			t.Fatal("replay changed first withdrawal time")
		}
	}
	if valueCount(t, db, `SELECT count(*) FROM text_active_releases WHERE release_id=$1`, release) != 0 {
		t.Fatal("authored active pointer survived")
	}
	if valueCount(t, db, `SELECT count(*) FROM text_releases WHERE release_id=$1 AND withdrawn_at IS NULL`, unrelated) != 1 || valueCount(t, db, `SELECT count(*) FROM text_active_releases WHERE release_id=$1`, unrelated) != 1 {
		t.Fatal("unrelated release withdrawn")
	}
	if !bytes.Equal(original, privacyContentSnapshot(t, db, release)) || !bytes.Equal(otherOriginal, privacyContentSnapshot(t, db, unrelated)) {
		t.Fatal("immutable release bytes changed")
	}
	if valueCount(t, db, `SELECT count(*) FROM text_accepted_inputs WHERE source_id IN (SELECT id FROM portal_submissions WHERE account_id=$1 UNION ALL SELECT id FROM challenge_entries WHERE account_id=$1)`, account) != 2 {
		t.Fatal("accepted source erased by withdrawal")
	}
}

func TestPrivacyContentEachSourceKindAndExistingWithdrawal(t *testing.T) {
	for _, kind := range []string{"portal", "challenge", "already_withdrawn"} {
		t.Run(kind, func(t *testing.T) {
			db, _ := textValueDB(t)
			account := valueAccount(t, db)
			release := privacyContentFixtureKinds(t, db, account, kind != "challenge", kind != "portal")
			original := privacyContentSnapshot(t, db, release)
			var originalAt time.Time
			if kind == "already_withdrawn" {
				if err := db.QueryRow(`UPDATE text_releases SET withdrawn_at=clock_timestamp()-interval '1 day' WHERE release_id=$1 RETURNING withdrawn_at`, release).Scan(&originalAt); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(privacyContentConfirm, privacyContentProof(t, db, account)...); err != nil {
				t.Fatal(err)
			}
			var at time.Time
			if err := db.QueryRow(`SELECT withdrawn_at FROM text_releases WHERE release_id=$1`, release).Scan(&at); err != nil {
				t.Fatal(err)
			}
			if kind == "already_withdrawn" && !at.Equal(originalAt) {
				t.Fatal("first withdrawal changed")
			}
			if valueCount(t, db, `SELECT count(*) FROM text_active_releases WHERE release_id=$1`, release) != 0 {
				t.Fatal("active pointer survived")
			}
			if _, err := db.Exec(`DELETE FROM text_releases WHERE release_id=$1`, release); err == nil {
				t.Fatal("immutable release DELETE succeeded")
			}
			if !bytes.Equal(original, privacyContentSnapshot(t, db, release)) {
				t.Fatal("immutable bytes changed")
			}
		})
	}
}
func privacyContentAssertUnconfirmed(t *testing.T, db *sql.DB, account, release string, proof []any) {
	t.Helper()
	if valueCount(t, db, `SELECT count(*) FROM accounts WHERE id=$1 AND deleted_at IS NULL AND session_epoch=0`, account) != 1 || valueCount(t, db, `SELECT count(*) FROM privacy_requests WHERE account_id=$1`, account) != 0 || valueCount(t, db, `SELECT count(*) FROM account_deletion_fences WHERE account_id=$1`, account) != 0 || valueCount(t, db, `SELECT count(*) FROM privacy_deletion_intents WHERE id=$1 AND state='pending'`, proof[0]) != 1 || valueCount(t, db, `SELECT count(*) FROM text_releases WHERE release_id=$1 AND withdrawn_at IS NULL`, release) != 1 || valueCount(t, db, `SELECT count(*) FROM text_active_releases WHERE release_id=$1`, release) != 1 {
		t.Fatal("failed confirmation left partial state")
	}
}
func TestPrivacyContentRollbackAfterWithdrawal(t *testing.T) {
	db, _ := textValueDB(t)
	account := valueAccount(t, db)
	release := privacyContentFixture(t, db, account)
	proof := privacyContentProof(t, db, account)
	original := privacyContentSnapshot(t, db, release)
	if _, err := db.Exec(`CREATE FUNCTION privacy_content_test_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected withdrawal failure'; END $$; CREATE TRIGGER privacy_content_test_fault BEFORE DELETE ON text_active_releases FOR EACH STATEMENT EXECUTE FUNCTION privacy_content_test_fault()`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(privacyContentConfirm, proof...); err == nil || !strings.Contains(err.Error(), "injected withdrawal failure") {
		t.Fatal("fault not reached", err)
	}
	privacyContentAssertUnconfirmed(t, db, account, release, proof)
	if !bytes.Equal(original, privacyContentSnapshot(t, db, release)) {
		t.Fatal("rollback changed source")
	}
	if _, err := db.Exec(`DROP TRIGGER privacy_content_test_fault ON text_active_releases; DROP FUNCTION privacy_content_test_fault()`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(privacyContentConfirm, proof...); err != nil {
		t.Fatal("exact retry failed", err)
	}
}
func TestPrivacyContentReleaseLockWaitCannotExtendProof(t *testing.T) {
	for _, lockKind := range []string{"release", "portal_submissions", "challenge_entries"} {
		t.Run(lockKind, func(t *testing.T) {
			db, _ := textValueDB(t)
			account := valueAccount(t, db)
			release := privacyContentFixture(t, db, account)
			proof := privacyContentProof(t, db, account)
			if _, err := db.Exec(`UPDATE privacy_deletion_intents SET expires_at=clock_timestamp()+interval '500 milliseconds' WHERE id=$1`, proof[0]); err != nil {
				t.Fatal(err)
			}
			blocker, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback()
			lockSQL := `LOCK TABLE text_releases IN SHARE ROW EXCLUSIVE MODE`
			if lockKind != "release" {
				lockSQL = `SELECT id FROM ` + lockKind + ` WHERE account_id='` + account + `' FOR SHARE`
			}
			if _, err = blocker.Exec(lockSQL); err != nil {
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
			go func() { _, e := conn.ExecContext(context.Background(), privacyContentConfirm, proof...); done <- e }()
			deadline := time.Now().Add(3 * time.Second)
			waiting := false
			for time.Now().Before(deadline) {
				var lock bool
				if err = db.QueryRow(`SELECT wait_event_type='Lock' FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&lock); err == nil && lock {
					waiting = true
					break
				}
				time.Sleep(time.Millisecond * 5)
			}
			if !waiting {
				t.Fatal("confirmation never waited on release authority")
			}
			var expired bool
			for !expired {
				if err = db.QueryRow(`SELECT expires_at<=clock_timestamp() FROM privacy_deletion_intents WHERE id=$1`, proof[0]).Scan(&expired); err != nil {
					t.Fatal(err)
				}
				if !expired {
					time.Sleep(time.Millisecond * 5)
				}
			}
			if err = blocker.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if err == nil {
					t.Fatal("expired confirmation succeeded")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("confirmation stuck")
			}
			privacyContentAssertUnconfirmed(t, db, account, release, proof)
		})
	}
}
func TestPrivacyContentMigrationRoundTripAndRetainedRefusal(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "retained"}[retained], func(t *testing.T) {
			db, _ := textValueDB(t)
			account := valueAccount(t, db)
			release := privacyContentFixture(t, db, account)
			original := privacyContentSnapshot(t, db, release)
			if retained {
				if _, err := db.Exec(privacyContentConfirm, privacyContentProof(t, db, account)...); err != nil {
					t.Fatal(err)
				}
			}
			down, err := os.ReadFile("../../migrations/000035_account_deletion_content.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			up, err := os.ReadFile("../../migrations/000035_account_deletion_content.up.sql")
			if err != nil {
				t.Fatal(err)
			}
			body := func(raw []byte) string { return strings.Split(string(raw), "$deletion$")[1] }
			var originalBody string
			if err = db.QueryRow(`SELECT prosrc FROM pg_proc WHERE oid='public.privacy_confirm_deletion(uuid,bytea,uuid,bytea,uuid,bytea,uuid)'::regprocedure`).Scan(&originalBody); err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			_, err = tx.Exec(string(down))
			if retained {
				if err == nil {
					t.Fatal("retained requests allowed down")
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				var got string
				if err = tx.QueryRow(`SELECT prosrc FROM pg_proc WHERE oid='public.privacy_confirm_deletion(uuid,bytea,uuid,bytea,uuid,bytea,uuid)'::regprocedure`).Scan(&got); err != nil || got != body(down) {
					t.Fatal("down did not restore exact prior function", err)
				}
				if _, err = tx.Exec(string(up)); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			var got string
			wantBody := body(up)
			if retained {
				// A refused rollback preserves the installed head, which may be
				// newer than migration35's original confirmation implementation.
				wantBody = originalBody
			}
			if err = db.QueryRow(`SELECT prosrc FROM pg_proc WHERE oid='public.privacy_confirm_deletion(uuid,bytea,uuid,bytea,uuid,bytea,uuid)'::regprocedure`).Scan(&got); err != nil || got != wantBody {
				t.Fatal("up/current function mismatch", err)
			}
			if !bytes.Equal(original, privacyContentSnapshot(t, db, release)) {
				t.Fatal("migration changed immutable source")
			}
		})
	}
}

func TestPrivacyContentSourceReadersSerializeBothOrders(t *testing.T) {
	for _, kind := range []string{"portal_submission", "challenge_entry"} {
		for _, first := range []string{"reader", "confirmation"} {
			t.Run(kind+"/"+first, func(t *testing.T) {
				db, _ := textValueDB(t)
				account := valueAccount(t, db)
				release := privacyContentFixture(t, db, account)
				proof := privacyContentProof(t, db, account)
				table := "portal_submissions"
				if kind == "challenge_entry" {
					table = "challenge_entries"
				}
				var source string
				if err := db.QueryRow(`SELECT id FROM `+table+` WHERE account_id=$1`, account).Scan(&source); err != nil {
					t.Fatal(err)
				}
				holder, err := db.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer holder.Rollback()
				if first == "reader" {
					if _, err = readAcceptedSource(context.Background(), holder, kind, source, true); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err = holder.Exec(privacyContentConfirm, proof...); err != nil {
						t.Fatal(err)
					}
				}
				waiter, err := db.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer waiter.Rollback()
				var pid int
				if err = waiter.QueryRow(`SELECT pg_backend_pid()`).Scan(&pid); err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() {
					if first == "reader" {
						_, e := waiter.Exec(privacyContentConfirm, proof...)
						done <- e
					} else {
						_, e := readAcceptedSource(context.Background(), waiter, kind, source, true)
						done <- e
					}
				}()
				waiting := false
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					var lock bool
					if err = db.QueryRow(`SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&lock); err != nil {
						t.Fatal(err)
					}
					if lock {
						waiting = true
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
				if !waiting {
					t.Fatal("expected source lock was not held")
				}
				// Both paths now hold the publication lock: a publisher cannot slip between
				// reading lineage and committing the account/source fence.
				publisher, e := db.Begin()
				if e != nil {
					t.Fatal(e)
				}
				if _, e = publisher.Exec(`SET LOCAL lock_timeout='50ms'`); e != nil {
					t.Fatal(e)
				}
				_, e = publisher.Exec(`LOCK TABLE text_releases IN SHARE ROW EXCLUSIVE MODE`)
				if e == nil {
					t.Fatal("publication serialization lock absent")
				}
				if e = publisher.Rollback(); e != nil {
					t.Fatal(e)
				}
				if err = holder.Commit(); err != nil {
					t.Fatal(err)
				}
				select {
				case err = <-done:
				case <-time.After(3 * time.Second):
					t.Fatal("source wait did not finish")
				}
				if first == "reader" {
					if err != nil {
						t.Fatal(err)
					}
					if err = waiter.Commit(); err != nil {
						t.Fatal(err)
					}
				} else {
					if err != ErrTextReleaseUnavailable {
						t.Fatal("post-wait reader admitted deleted creator", err)
					}
					if err = waiter.Rollback(); err != nil {
						t.Fatal(err)
					}
				}
				if valueCount(t, db, `SELECT count(*) FROM text_active_releases WHERE release_id=$1`, release) != 0 {
					t.Fatal("active publication survived")
				}
			})
		}
	}
}
