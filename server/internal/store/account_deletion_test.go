package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// This closed foundation fixture asserts privileged service authority explicitly;
// it is not evidence of end-user reauthentication or complete account erasure.
func TestPrivacyFoundationProfileReceipt(t *testing.T) {
	db, _ := textValueDB(t)
	account := valueAccount(t, db)
	other := valueAccount(t, db)
	request := uuid.NewString()
	proof, token, receipt := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{3}, 32)
	var got string
	if err := db.QueryRow(`SELECT public.privacy_prepare_verified_request($1,$2,$3,$4)`, request, account, proof, token).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != request {
		t.Fatal("request identity", got)
	}
	// Binding alone never advertises full active-data removal.
	if _, err := db.Exec(`SELECT public.privacy_bind_suppression($1,1,$2)`, request, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		var result []byte
		if err := db.QueryRow(`SELECT public.privacy_erase_profile_batch($1,1)`, request).Scan(&result); err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(result, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["result"] != "profile_removed" {
			t.Fatal("wrong step result", string(result))
		}
	}
	if valueCount(t, db, `SELECT count(*) FROM profiles WHERE account_id=$1`, account) != 0 || valueCount(t, db, `SELECT count(*) FROM profiles WHERE account_id=$1`, other) != 1 {
		t.Fatal("profile scope")
	}
	if valueCount(t, db, `SELECT count(*) FROM privacy_step_receipts WHERE request_id=$1`, request) != 1 {
		t.Fatal("receipt replay duplicated")
	}
	var removed sql.NullTime
	if err := db.QueryRow(`SELECT active_removed_at FROM privacy_requests WHERE id=$1`, request).Scan(&removed); err != nil || removed.Valid {
		t.Fatal("partial step advertised complete", removed, err)
	}
}

func privacyPrepare(t *testing.T, db *sql.DB, account string) (string, []byte) {
	t.Helper()
	request, token := uuid.NewString(), []byte(uuid.NewString()[:32])
	var got string
	if err := db.QueryRow(`SELECT public.privacy_prepare_verified_request($1,$2,$3,$4)`, request, account, bytes.Repeat([]byte{1}, 32), token).Scan(&got); err != nil {
		t.Fatal(err)
	}
	return request, token
}
func privacyRefuse(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err == nil {
		t.Fatal("expected privacy refusal", query)
	}
}
func TestPrivacyFoundationAuthorityAndStatus(t *testing.T) {
	db, _ := textValueDB(t)
	account := valueAccount(t, db)
	request, token := privacyPrepare(t, db, account)
	for _, q := range []string{
		`CREATE ROLE privacy_fixture_owner NOLOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`,
		`CREATE ROLE privacy_fixture_executor LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`,
		`CREATE ROLE privacy_fixture_runtime LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`,
		`CREATE ROLE privacy_fixture_capture LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`,
		`GRANT USAGE ON SCHEMA public TO privacy_fixture_owner,privacy_fixture_executor,privacy_fixture_runtime,privacy_fixture_capture`,
		`GRANT SELECT(id,deleted_at),UPDATE(id) ON accounts TO privacy_fixture_owner`,
		`GRANT SELECT,DELETE,UPDATE(account_id) ON profiles TO privacy_fixture_owner`,
		`GRANT SELECT(account_id) ON text_admissions,noin_ledger TO privacy_fixture_owner`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"privacy_requests", "privacy_step_receipts", "account_deletion_fences"} {
		if _, err := db.Exec(`ALTER TABLE public.` + table + ` OWNER TO privacy_fixture_owner; GRANT SELECT ON public.` + table + ` TO privacy_fixture_capture`); err != nil {
			t.Fatal(err)
		}
	}
	funcs := []string{"privacy_prepare_verified_request(uuid,uuid,bytea,bytea)", "privacy_bind_suppression(uuid,bigint,bytea)", "privacy_erase_profile_batch(uuid,integer)", "account_deletion_status(bytea)"}
	for i, f := range funcs {
		grantee := "privacy_fixture_executor"
		if i == 3 {
			grantee = "privacy_fixture_runtime"
		}
		if _, err := db.Exec(`ALTER FUNCTION public.` + f + ` OWNER TO privacy_fixture_owner; GRANT EXECUTE ON FUNCTION public.` + f + ` TO ` + grantee); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`GRANT SELECT ON account_deletion_fences TO privacy_fixture_runtime`); err != nil {
		t.Fatal(err)
	}
	// SESSION AUTHORIZATION, rather than SET ROLE as the superuser, proves that
	// the application principal itself cannot adopt the privileged owner.
	for _, role := range []string{"privacy_fixture_runtime", "privacy_fixture_executor", "privacy_fixture_capture"} {
		forbidden := []string{`SET ROLE privacy_fixture_owner`, `UPDATE privacy_requests SET phase='profile_removed'`, `INSERT INTO privacy_step_receipts(request_id) VALUES(NULL)`, `ALTER TABLE privacy_requests DISABLE TRIGGER ALL`, `TRUNCATE privacy_requests CASCADE`, `CREATE FUNCTION public.privacy_forgery() RETURNS int LANGUAGE SQL AS 'SELECT 1'`}
		if role != "privacy_fixture_runtime" {
			forbidden = append(forbidden, `DELETE FROM profiles`)
		}
		if role != "privacy_fixture_capture" {
			forbidden = append(forbidden, `SELECT * FROM privacy_requests`)
		}
		if role != "privacy_fixture_executor" {
			forbidden = append(forbidden, `SELECT privacy_erase_profile_batch('00000000-0000-0000-0000-000000000001',1)`, `SELECT privacy_prepare_verified_request(NULL,NULL,NULL,NULL)`, `SELECT privacy_bind_suppression(NULL,1,NULL)`)
		}
		if role != "privacy_fixture_runtime" {
			forbidden = append(forbidden, `SELECT account_deletion_status(NULL)`)
		}
		for _, q := range forbidden {
			t.Run(role+"/"+q, func(t *testing.T) {
				tx, err := db.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if _, err = tx.Exec(`SET LOCAL SESSION AUTHORIZATION ` + role); err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(`SELECT set_config('app.privacy_authorized','true',true)`); err != nil {
					t.Fatal(err)
				}
				_, err = tx.Exec(q)
				var pg *pq.Error
				if !errors.As(err, &pg) || pg.Code != "42501" {
					t.Fatalf("want permission refusal: %v", err)
				}
			})
		}
	}
	var raw []byte
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL SESSION AUTHORIZATION privacy_fixture_runtime`); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(`SELECT account_deletion_status($1)`, token).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(account)) || bytes.Contains(raw, []byte(request)) || bytes.Contains(raw, []byte("proof")) {
		t.Fatal("status exposed private identity", string(raw))
	}
	if err = tx.QueryRow(`SELECT account_deletion_status($1)`, bytes.Repeat([]byte{99}, 32)).Scan(&raw); err != nil || raw != nil {
		t.Fatal("unknown capability", string(raw), err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL SESSION AUTHORIZATION privacy_fixture_executor`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`SELECT privacy_bind_suppression($1,1,$2)`, request, bytes.Repeat([]byte{3}, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`SELECT privacy_erase_profile_batch($1,1)`, request); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestPrivacyFoundationRefusalAtomicityAndReplay(t *testing.T) {
	db, _ := textValueDB(t)
	account := valueAccount(t, db)
	request, token := privacyPrepare(t, db, account)
	for _, args := range [][]any{{request, valueAccount(t, db), bytes.Repeat([]byte{1}, 32), token}, {request, account, bytes.Repeat([]byte{2}, 32), token}, {request, account, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{4}, 32)}} {
		privacyRefuse(t, db, `SELECT privacy_prepare_verified_request($1,$2,$3,$4)`, args...)
	}
	privacyRefuse(t, db, `SELECT privacy_erase_profile_batch($1,1)`, request)
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	privacyRefuse(t, db, `SELECT privacy_erase_profile_batch($1,1)`, request)
	privacyRefuse(t, db, `SELECT privacy_bind_suppression($1,0,$2)`, request, bytes.Repeat([]byte{3}, 32))
	privacyRefuse(t, db, `SELECT privacy_bind_suppression($1,1,$2)`, uuid.NewString(), bytes.Repeat([]byte{3}, 32))
	for range 2 {
		if _, err := db.Exec(`SELECT privacy_bind_suppression($1,1,$2)`, request, bytes.Repeat([]byte{3}, 32)); err != nil {
			t.Fatal(err)
		}
	}
	privacyRefuse(t, db, `SELECT privacy_bind_suppression($1,2,$2)`, request, bytes.Repeat([]byte{3}, 32))
	privacyRefuse(t, db, `SELECT privacy_bind_suppression($1,1,$2)`, request, bytes.Repeat([]byte{4}, 32))
	for _, limit := range []any{nil, -1, 0, 2, 2147483647} {
		privacyRefuse(t, db, `SELECT privacy_erase_profile_batch($1,$2)`, request, limit)
	}
	for _, table := range []string{"privacy_step_receipts", "privacy_requests"} {
		if _, err := db.Exec(`CREATE TRIGGER fail_privacy BEFORE INSERT OR UPDATE ON ` + table + ` FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite()`); err != nil {
			t.Fatal(err)
		}
		snapshot := consentSnapshot(t, db)
		privacyRefuse(t, db, `SELECT privacy_erase_profile_batch($1,1)`, request)
		assertTransitionLegacyUnchanged(t, snapshot, consentSnapshot(t, db))
		if _, err := db.Exec(`DROP TRIGGER fail_privacy ON ` + table); err != nil {
			t.Fatal(err)
		}
	}
	var expected []byte
	if err := db.QueryRow(`SELECT sha256(convert_to(to_jsonb(p)::text,'UTF8')) FROM profiles p WHERE account_id=$1`, account).Scan(&expected); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT privacy_erase_profile_batch($1,1)`, request); err != nil {
		t.Fatal(err)
	}
	var original []byte
	if err := db.QueryRow(`SELECT original_sha256 FROM privacy_step_receipts WHERE request_id=$1`, request).Scan(&original); err != nil || !bytes.Equal(expected, original) {
		t.Fatal("profile witness mismatch", err)
	}
	// An unexpected profile resurrection is an error, never accepted replay.
	if _, err := db.Exec(`INSERT INTO profiles(account_id) VALUES($1)`, account); err != nil {
		t.Fatal(err)
	}
	privacyRefuse(t, db, `SELECT privacy_erase_profile_batch($1,1)`, request)
}

func TestPrivacyFoundationRejectsAcceptedValueAndConcurrentReplay(t *testing.T) {
	db, _ := textValueDB(t)
	account := valueAccount(t, db)
	request, token := uuid.NewString(), bytes.Repeat([]byte{2}, 32)
	errs := make(chan error, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var got string
			err := db.QueryRow(`SELECT privacy_prepare_verified_request($1,$2,$3,$4)`, request, account, bytes.Repeat([]byte{1}, 32), token).Scan(&got)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if valueCount(t, db, `SELECT count(*) FROM privacy_requests`) != 1 || valueCount(t, db, `SELECT count(*) FROM account_deletion_fences`) != 1 {
		t.Fatal("concurrent request duplication")
	}
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT privacy_bind_suppression($1,1,$2)`, request, bytes.Repeat([]byte{3}, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,'fixture',1,'Synthetic accepted value','2026-09-19')`, account); err != nil {
		t.Fatal(err)
	}
	before := consentSnapshot(t, db)
	privacyRefuse(t, db, `SELECT privacy_erase_profile_batch($1,1)`, request)
	assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
}

func TestPrivacyFoundationMigration31To32ParityAndDown(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty_down", true: "retained_refusal"}[retained], func(t *testing.T) {
			db := transitionDesignDB(t, 31, true)
			before := consentSnapshot(t, db)
			path := transitionMigrationPath(t, 32)
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			after := consentSnapshot(t, db)
			for _, old := range before.Tables {
				if old.Name == "schema_migrations" {
					continue
				}
				found := false
				for _, current := range after.Tables {
					if current.Name == old.Name {
						found = true
						if !reflect.DeepEqual(old, current) {
							t.Fatal("legacy data changed", old.Name)
						}
					}
				}
				if !found {
					t.Fatal("legacy table lost", old.Name)
				}
			}
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, after, consentSnapshot(t, db))
			if retained {
				privacyPrepare(t, db, valueAccount(t, db))
			}
			before = consentSnapshot(t, db)
			down, err := os.ReadFile("../../migrations/000032_account_deletion_foundation.down.sql")
			if err != nil {
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
					t.Fatal("retained request rollback allowed")
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
				up, err := os.ReadFile("../../migrations/000032_account_deletion_foundation.up.sql")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec(string(up)); err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, after, consentSnapshot(t, db))
			}
		})
	}
}

func TestPrivacyFoundationPendingAdmissionAndPreparationRollback(t *testing.T) {
	db, _ := textValueDB(t)
	account := valueAccount(t, db)
	// Failure on the last preparation write must roll back request and fence.
	if _, err := db.Exec(`CREATE TRIGGER fail_privacy BEFORE INSERT ON account_deletion_fences FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite()`); err != nil {
		t.Fatal(err)
	}
	before := consentSnapshot(t, db)
	privacyRefuse(t, db, `SELECT privacy_prepare_verified_request($1,$2,$3,$4)`, uuid.NewString(), account, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
	if _, err := db.Exec(`DROP TRIGGER fail_privacy ON account_deletion_fences`); err != nil {
		t.Fatal(err)
	}
	request, _ := privacyPrepare(t, db, account)
	if _, err := db.Exec(`INSERT INTO text_admissions(id,account_id,entry_path,prototype,access_kind,quota_day,reserved_at,state) VALUES($1,$2,'quick_play',false,'free','2026-09-19',now(),'reserved')`, uuid.NewString(), account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT privacy_bind_suppression($1,1,$2)`, request, bytes.Repeat([]byte{3}, 32)); err != nil {
		t.Fatal(err)
	}
	before = consentSnapshot(t, db)
	privacyRefuse(t, db, `SELECT privacy_erase_profile_batch($1,1)`, request)
	assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
}

func TestPrivacyFoundationRefusesProfileSchemaDrift(t *testing.T) {
	db, _ := textValueDB(t)
	account := valueAccount(t, db)
	request, _ := privacyPrepare(t, db, account)
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT privacy_bind_suppression($1,1,$2)`, request, bytes.Repeat([]byte{3}, 32)); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ setup, cleanup string }{
		{`ALTER TABLE profiles ADD COLUMN marked_secret TEXT DEFAULT 'unreviewed personal marker'`, `ALTER TABLE profiles DROP COLUMN marked_secret`},
		{`CREATE RULE privacy_profile_no_delete AS ON DELETE TO profiles DO INSTEAD NOTHING`, `DROP RULE privacy_profile_no_delete ON profiles`},
		{`ALTER TABLE profiles ENABLE ROW LEVEL SECURITY`, `ALTER TABLE profiles DISABLE ROW LEVEL SECURITY`},
		{`ALTER TABLE text_admissions ENABLE ROW LEVEL SECURITY`, `ALTER TABLE text_admissions DISABLE ROW LEVEL SECURITY`},
		{`ALTER TABLE noin_ledger ENABLE ROW LEVEL SECURITY`, `ALTER TABLE noin_ledger DISABLE ROW LEVEL SECURITY`},
		{`CREATE TABLE privacy_unreviewed_dependency(account_id UUID REFERENCES profiles(account_id) ON DELETE CASCADE); INSERT INTO privacy_unreviewed_dependency SELECT account_id FROM profiles`, `DROP TABLE privacy_unreviewed_dependency`},
	} {
		if _, err := db.Exec(fixture.setup); err != nil {
			t.Fatal(err)
		}
		before := consentSnapshot(t, db)
		privacyRefuse(t, db, `SELECT privacy_erase_profile_batch($1,1)`, request)
		assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
		if _, err := db.Exec(fixture.cleanup); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPrivacyFoundationManifestLocksConcurrentDDL(t *testing.T) {
	db, _ := textValueDB(t)
	account := valueAccount(t, db)
	request, _ := privacyPrepare(t, db, account)
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT privacy_bind_suppression($1,1,$2)`, request, bytes.Repeat([]byte{3}, 32)); err != nil {
		t.Fatal(err)
	}
	blocker, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err = blocker.Exec(`SELECT account_id FROM profiles WHERE account_id=$1 FOR UPDATE`, account); err != nil {
		t.Fatal(err)
	}
	worker, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = worker.Close() })
	var pid int
	if err = worker.QueryRowContext(t.Context(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := worker.ExecContext(t.Context(), `SELECT privacy_erase_profile_batch($1,1)`, request)
		done <- err
	}()
	waiting := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event_type='Lock')`, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("privacy worker never reached profile row wait")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	_, err = db.ExecContext(ctx, `CREATE TABLE privacy_race_dependency(account_id UUID REFERENCES profiles(account_id) ON DELETE CASCADE)`)
	if err == nil {
		t.Fatal("concurrent incoming FK crossed manifest guard")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("DDL failed for another reason", err)
	}
	if err = blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if valueCount(t, db, `SELECT count(*) FROM profiles WHERE account_id=$1`, account) != 0 {
		t.Fatal("profile step did not finish")
	}
}
