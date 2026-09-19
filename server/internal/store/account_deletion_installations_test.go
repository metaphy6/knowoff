package store

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"github.com/lib/pq"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPrivacyInstallationsBoundedSharedBootstrapErasure(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, true)
	if _, complete := privacyCredentialsRun(t, db, f.proof[4], 128); !complete {
		t.Fatal("credential prerequisite incomplete")
	}
	survivor := valueAccount(t, db)
	hashes := []string{uuid.NewString(), uuid.NewString()}
	for _, hash := range hashes {
		if _, err := db.Exec(`INSERT INTO auth_installations(device_hash) VALUES($1)`, hash); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO auth_installation_bootstrap(device_hash,account_id) VALUES($1,$2)`, hash, f.account); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2)`, f.account, hash); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2)`, survivor, hashes[0]); err != nil {
		t.Fatal(err)
	}
	var original []byte
	if err := db.QueryRow(`SELECT to_jsonb(d)::text FROM device_tokens d WHERE account_id=$1`, survivor).Scan(&original); err != nil {
		t.Fatal(err)
	}
	for _, hash := range hashes {
		sanction, bootstrap := sha256.Sum256([]byte("sanction:"+hash)), sha256.Sum256([]byte("bootstrap:"+hash))
		if _, err := db.Exec(`SELECT privacy_register_installation_keys($1,ARRAY['fixture-key-1'],ARRAY[$2::bytea],ARRAY[$3::bytea])`, hash, sanction[:], bootstrap[:]); err != nil {
			t.Fatal("trusted selector registration", err)
		}
	}
	complete := false
	for i := 0; i < 12 && !complete; i++ {
		var raw []byte
		if err := db.QueryRow(`SELECT privacy_erase_installations_batch($1,1,ARRAY['fixture-key-1'])`, f.proof[4]).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Processed int  `json:"processed"`
			Complete  bool `json:"complete"`
		}
		if err := json.Unmarshal(raw, &result); err != nil || result.Processed < 0 || result.Processed > 1 || !result.Complete && result.Processed == 0 {
			t.Fatal("bounded batch failed to progress", string(raw), err)
		}
		complete = result.Complete
	}
	if !complete {
		t.Fatal("installation cleanup did not converge")
	}
	var remaining, evidence int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM device_tokens WHERE account_id=$1)+(SELECT count(*) FROM auth_installation_bootstrap WHERE account_id=$1)`, f.account).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("owned identity retained", remaining, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM privacy_installation_evidence WHERE request_id=$1 AND kind='erased_bootstrap'`, f.proof[4]).Scan(&evidence); err != nil || evidence != 2 {
		t.Fatal("distinct bootstrap evidence lost", evidence, err)
	}
	var after []byte
	if err := db.QueryRow(`SELECT to_jsonb(d)::text FROM device_tokens d WHERE account_id=$1`, survivor).Scan(&after); err != nil || !bytes.Equal(original, after) {
		t.Fatal("surviving association changed", err)
	}
}

func TestPrivacyInstallationsPurgeAndErasureLockBothOrders(t *testing.T) {
	for _, first := range []string{"erase", "purge"} {
		t.Run(first, func(t *testing.T) {
			db, _ := textValueDB(t)
			f := privacyCredentialsFixture(t, db, true)
			privacyCredentialsRun(t, db, f.proof[4], 128)
			if _, err := db.Exec(`INSERT INTO privacy_installation_evidence(request_id,kind,source_id,key_id,selector_sha256,source_sha256,occurred_at,match_until,purge_after) VALUES($1,'erased_bootstrap',$1,'fixture-key-1',$2,$2,clock_timestamp()-interval '2 days',clock_timestamp()-interval '1 day',clock_timestamp()-interval '1 day')`, f.proof[4], bytes.Repeat([]byte{7}, 32)); err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = tx.Exec(`SET LOCAL statement_timeout='4s'`); err != nil {
				t.Fatal(err)
			}
			if first == "erase" {
				if _, err = tx.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, f.account); err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(`SELECT id FROM privacy_requests WHERE id=$1 FOR UPDATE`, f.proof[4]); err != nil {
					t.Fatal(err)
				}
			} else if _, err = tx.Exec(`SELECT privacy_purge_installation_evidence(1)`); err != nil {
				t.Fatal(err)
			}
			conn, err := db.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(); _ = conn.Close() }()
			var pid int
			if err = conn.QueryRowContext(t.Context(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				if first == "erase" {
					_, e := conn.ExecContext(t.Context(), `SELECT privacy_purge_installation_evidence(1)`)
					done <- e
				} else {
					_, e := conn.ExecContext(t.Context(), `SELECT privacy_erase_installations_batch($1,1,ARRAY['fixture-key-1'])`, f.proof[4])
					done <- e
				}
			}()
			privacyReleaseWaitLock(t, db, pid)
			if first == "erase" {
				if _, err = tx.Exec(`SELECT privacy_erase_installations_batch($1,1,ARRAY['fixture-key-1'])`, f.proof[4]); err != nil {
					t.Fatal("erasure/global keyset inverted", err)
				}
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("purge/erasure did not converge")
			}
		})
	}
}

func TestPrivacyInstallationsRefuseUnreviewedCascade(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, true)
	privacyCredentialsRun(t, db, f.proof[4], 128)
	hash := uuid.NewString()
	var link string
	if err := db.QueryRow(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2) RETURNING id`, f.account, hash).Scan(&link); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT privacy_register_installation_keys($1,ARRAY['fixture-key-1'],ARRAY[$2::bytea],ARRAY[$2::bytea])`, hash, bytes.Repeat([]byte{9}, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE privacy_cascade_regression(id uuid PRIMARY KEY,link_id uuid REFERENCES device_tokens(id) ON DELETE CASCADE); INSERT INTO privacy_cascade_regression VALUES(gen_random_uuid(),'` + link + `')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT privacy_erase_installations_batch($1,128,ARRAY['fixture-key-1'])`, f.proof[4]); err == nil {
		t.Fatal("unreviewed cascade erased dependent source")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM privacy_cascade_regression`); n != 1 {
		t.Fatal("dependent source changed", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM device_tokens WHERE id=$1`, link); n != 1 {
		t.Fatal("original source changed", n)
	}
}

func TestPrivacyInstallationsMatchingRefusesPrivateRLSDrift(t *testing.T) {
	for _, table := range []string{"privacy_installation_keys", "privacy_installation_evidence"} {
		t.Run(table, func(t *testing.T) {
			db, _ := textValueDB(t)
			if _, err := db.Exec(`ALTER TABLE ` + table + ` ENABLE ROW LEVEL SECURITY; ALTER TABLE ` + table + ` FORCE ROW LEVEL SECURITY`); err != nil {
				t.Fatal(err)
			}
			for _, predicate := range []string{"installation_sanction_active", "privacy_erased_bootstrap_active"} {
				if _, err := db.Exec(`SELECT ` + predicate + `('unregistered-installation')`); err == nil {
					t.Fatal("private RLS could conceal finite evidence", predicate)
				}
			}
		})
	}
}

func TestPrivacyInstallationsCompletedReceiptRejectsReappearingSource(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, true)
	privacyCredentialsRun(t, db, f.proof[4], 128)
	var result struct {
		Processed, Mutations int
		Complete             bool
	}
	var raw []byte
	if err := db.QueryRow(`SELECT privacy_erase_installations_batch($1,1,ARRAY['k1'])`, f.proof[4]).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &result); err != nil || !result.Complete || result.Processed != 0 || result.Mutations != 1 {
		t.Fatal("terminal receipt mutation was not counted", string(raw), err)
	}
	hash := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2)`, f.account, hash); err != nil {
		t.Fatal(err)
	}
	privacyInstallationRegister(t, db, hash, []string{"k1"})
	if _, err := db.Exec(`SELECT privacy_erase_installations_batch($1,1,ARRAY['k1'])`, f.proof[4]); err == nil {
		t.Fatal("completed receipt silently accepted new source")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM device_tokens WHERE account_id=$1`, f.account); n != 1 {
		t.Fatal("conflicting source changed", n)
	}
}

func privacyInstallationRegister(t *testing.T, db *sql.DB, hash string, keys []string) {
	t.Helper()
	sanctions, bootstraps := pq.ByteaArray{}, pq.ByteaArray{}
	for _, key := range keys {
		s := sha256.Sum256([]byte("sanction:" + key + ":" + hash))
		b := sha256.Sum256([]byte("bootstrap:" + key + ":" + hash))
		sanctions = append(sanctions, s[:])
		bootstraps = append(bootstraps, b[:])
	}
	if _, err := db.Exec(`SELECT privacy_register_installation_keys($1,$2,$3,$4)`, hash, pq.Array(keys), sanctions, bootstraps); err != nil {
		t.Fatal(err)
	}
}

func TestPrivacyInstallationsSourceDriftAndPrerequisites(t *testing.T) {
	for _, kind := range []string{"column", "rls", "rule", "missing_guard", "missing_suppression", "missing_credentials", "zero_limit"} {
		t.Run(kind, func(t *testing.T) {
			db, _ := textValueDB(t)
			f := privacyCredentialsFixture(t, db, kind != "missing_suppression")
			if kind != "missing_suppression" && kind != "missing_credentials" {
				privacyCredentialsRun(t, db, f.proof[4], 128)
			}
			hash := uuid.NewString()
			privacyInstallationRegister(t, db, hash, []string{"k1"})
			if _, err := db.Exec(`INSERT INTO auth_installation_bootstrap(device_hash,account_id) VALUES($1,$2)`, hash, f.account); err != nil {
				t.Fatal(err)
			}
			var original string
			if err := db.QueryRow(`SELECT to_jsonb(b)::text FROM auth_installation_bootstrap b WHERE device_hash=$1`, hash).Scan(&original); err != nil {
				t.Fatal(err)
			}
			ddl := ""
			switch kind {
			case "column":
				ddl = `ALTER TABLE device_tokens ADD COLUMN marked_secret text`
			case "rls":
				ddl = `ALTER TABLE device_tokens ENABLE ROW LEVEL SECURITY`
			case "rule":
				ddl = `CREATE RULE refuse_bootstrap_delete AS ON DELETE TO auth_installation_bootstrap DO INSTEAD NOTHING`
			case "missing_guard":
				ddl = `DROP TRIGGER sanction_immutable ON auth_installation_bootstrap`
			}
			if ddl != "" {
				if _, err := db.Exec(ddl); err != nil {
					t.Fatal(err)
				}
			}
			limit := 1
			if kind == "zero_limit" {
				limit = 0
			}
			if _, err := db.Exec(`SELECT privacy_erase_installations_batch($1,$2,ARRAY['k1'])`, f.proof[4], limit); err == nil {
				t.Fatal("unsupported erasure accepted", kind)
			}
			var after string
			if err := db.QueryRow(`SELECT to_jsonb(b)::text FROM auth_installation_bootstrap b WHERE device_hash=$1`, hash).Scan(&after); err != nil || after != original {
				t.Fatal("source mutated", err)
			}
			if n := valueCount(t, db, `SELECT (SELECT count(*) FROM privacy_installation_evidence)+(SELECT count(*) FROM privacy_installation_erasure_authorizations)+(SELECT count(*) FROM privacy_step_receipts WHERE step='installations')`); n != 0 {
				t.Fatal("failed batch left effects", n)
			}
		})
	}
}

func TestPrivacyInstallationsRequiredKeysPurposeExpiryAndRotation(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, true)
	hash := uuid.NewString()
	privacyInstallationRegister(t, db, hash, []string{"k1"})
	tag := sha256.Sum256([]byte("bootstrap:k1:" + hash))
	if _, err := db.Exec(`INSERT INTO privacy_installation_evidence(request_id,kind,source_id,key_id,selector_sha256,source_sha256,occurred_at,match_until,purge_after) VALUES($1,'erased_bootstrap',$1,'k1',$2,$2,clock_timestamp(),clock_timestamp()+interval '1 hour',clock_timestamp()+interval '1 hour')`, f.proof[4], tag[:]); err != nil {
		t.Fatal(err)
	}
	var active bool
	if err := db.QueryRow(`SELECT privacy_erased_bootstrap_active($1)`, hash).Scan(&active); err != nil || !active {
		t.Fatal("matching selector lost", active, err)
	}
	if err := db.QueryRow(`SELECT installation_sanction_active($1)`, hash).Scan(&active); err != nil || active {
		t.Fatal("bootstrap tag crossed purpose", active, err)
	}
	if _, err := db.Exec(`SELECT privacy_erased_bootstrap_active('unregistered')`); err == nil {
		t.Fatal("direct SQL bypassed required coverage")
	}
	bad := bytes.Repeat([]byte{99}, 32)
	if _, err := db.Exec(`SELECT privacy_register_installation_keys($1,ARRAY['k2'],ARRAY[$2::bytea],ARRAY[$2::bytea])`, hash, bad); err == nil {
		t.Fatal("rotation omitted live key")
	}
	if _, err := db.Exec(`SELECT privacy_register_installation_keys($1,ARRAY['k1'],ARRAY[$2::bytea],ARRAY[$2::bytea])`, hash, bad); err == nil {
		t.Fatal("same key accepted changed derivation")
	}
	privacyInstallationRegister(t, db, hash, []string{"k1", "k2"})
	privacyInstallationRegister(t, db, hash, []string{"k1", "k3"})
	if n := valueCount(t, db, `SELECT count(*) FROM privacy_installation_keys WHERE device_hash=$1`, hash); n != 2 {
		t.Fatal("retired registrations accumulated", n)
	}
	for _, key := range []string{"k2", "k3", "k4"} {
		if _, err := db.Exec(`INSERT INTO privacy_installation_evidence(request_id,kind,source_id,key_id,selector_sha256,source_sha256,occurred_at,match_until,purge_after) VALUES($1,'erased_bootstrap',$1,$2,$3,$3,clock_timestamp(),clock_timestamp()+interval '1 hour',clock_timestamp()+interval '1 hour')`, f.proof[4], key, bad); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`SELECT privacy_erased_bootstrap_active($1)`, hash); err == nil {
		t.Fatal("partial required key coverage accepted")
	}
	privacyInstallationRegister(t, db, hash, []string{"k1", "k2", "k3", "k4"})
	tags := pq.ByteaArray{bad, bad, bad, bad, bad}
	if _, err := db.Exec(`SELECT privacy_register_installation_keys($1,$2,$3,$3)`, hash, pq.Array([]string{"k1", "k2", "k3", "k4", "k5"}), tags); err == nil {
		t.Fatal("fifth key accepted")
	}
	if _, err := db.Exec(`UPDATE privacy_installation_evidence SET match_until=clock_timestamp(),purge_after=clock_timestamp()`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT privacy_erased_bootstrap_active('unregistered')`).Scan(&active); err != nil || active {
		t.Fatal("expired selector still required", active, err)
	}
	for range 4 {
		if _, err := db.Exec(`SELECT privacy_purge_installation_evidence(1)`); err != nil {
			t.Fatal(err)
		}
	}
	if n := valueCount(t, db, `SELECT count(*) FROM privacy_installation_evidence`); n != 0 {
		t.Fatal("expired evidence retained", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM privacy_installation_keys WHERE device_hash=$1`, hash); n != 4 {
		t.Fatal("purge deleted live mappings", n)
	}
}

func TestPrivacyInstallationsMigrationBackfillAndRetainedDown(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_backfill", true: "retained"}[retained], func(t *testing.T) {
			db, _ := textValueDB(t)
			down, err := os.ReadFile("../../migrations/000039_account_deletion_installations.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			up, err := os.ReadFile("../../migrations/000039_account_deletion_installations.up.sql")
			if err != nil {
				t.Fatal(err)
			}
			if retained {
				f := privacyCredentialsFixture(t, db, true)
				var before string
				if err := db.QueryRow(`SELECT to_jsonb(r)::text FROM privacy_requests r WHERE id=$1`, f.proof[4]).Scan(&before); err != nil {
					t.Fatal(err)
				}
				tx, err := db.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if _, err = tx.Exec(string(down)); err == nil {
					t.Fatal("retained down accepted")
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
				var after string
				if err = db.QueryRow(`SELECT to_jsonb(r)::text FROM privacy_requests r WHERE id=$1`, f.proof[4]).Scan(&after); err != nil || before != after {
					t.Fatal("down changed retained request", err)
				}
				return
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
			a, b := valueAccount(t, db), valueAccount(t, db)
			single, multi, canonical := uuid.NewString(), uuid.NewString(), uuid.NewString()
			for _, pair := range [][2]string{{a, single}, {a, multi}, {b, multi}, {b, canonical}} {
				if _, err = db.Exec(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2)`, pair[0], pair[1]); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = db.Exec(`INSERT INTO auth_installations(device_hash) VALUES($1)`, canonical); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO auth_installation_bootstrap(device_hash,account_id) VALUES($1,$2)`, canonical, a); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(string(up)); err != nil {
				t.Fatal(err)
			}
			for _, want := range []struct{ hash, account, state string }{{single, a, "bound"}, {multi, "", "ambiguous"}, {canonical, a, "bound"}} {
				var account, state string
				if err = db.QueryRow(`SELECT coalesce(account_id::text,''),state FROM auth_installation_bootstrap WHERE device_hash=$1`, want.hash).Scan(&account, &state); err != nil || account != want.account || state != want.state {
					t.Fatal("canonical backfill changed authority", want, account, state, err)
				}
			}
			var body string
			if err = db.QueryRow(`SELECT prosrc FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure`).Scan(&body); err != nil || body != strings.Split(string(up), "$registration$")[1] {
				t.Fatal("re-up function mismatch", err)
			}
		})
	}
}

func TestPrivacyInstallationsSanctionMultipleSourcesAndRotationWitness(t *testing.T) {
	db, _ := textValueDB(t)
	f := privacyCredentialsFixture(t, db, true)
	privacyCredentialsRun(t, db, f.proof[4], 128)
	survivor := valueAccount(t, db)
	operation := uuid.NewString()
	hashes := []string{uuid.NewString(), uuid.NewString()}
	if _, err := db.Exec(`INSERT INTO admin_operation_decisions(id,actor_admin_id,kind,target_account_id,reason,affected_accounts,request_hash,sanction_until) VALUES($1,$2,'account_sanction',$3,'fixture',jsonb_build_array($3::uuid::text),$4,clock_timestamp()+interval '1 hour')`, operation, f.admin, f.account, bytes.Repeat([]byte{1}, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO account_sanctions(operation_id,account_id,until_at) SELECT id,target_account_id,sanction_until FROM admin_operation_decisions WHERE id=$1`, operation); err != nil {
		t.Fatal(err)
	}
	for _, hash := range hashes {
		privacyInstallationRegister(t, db, hash, []string{"k1"})
		if _, err := db.Exec(`INSERT INTO account_sanction_installations(operation_id,device_hash) VALUES($1,$2)`, operation, hash); err != nil {
			t.Fatal(err)
		}
	}
	old, access, refresh, shared := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(`INSERT INTO auth_installation_rotations(old_refresh_id,account_id,device_hash,session_epoch,issued_at,access_id,refresh_id,issuance_config_hash) VALUES($1,$2,$3,0,clock_timestamp(),$4,$5,repeat('a',43)),($6,$7,$3,0,clock_timestamp(),$4,$8,repeat('b',43))`, old, f.account, hashes[0], access, refresh, shared, survivor, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{old, access, refresh, "unowned-revocation"} {
		if _, err := db.Exec(`INSERT INTO auth_revocations(token_id,expires_at) VALUES($1,clock_timestamp()+interval '1 hour')`, token); err != nil {
			t.Fatal(err)
		}
	}
	var original string
	if err := db.QueryRow(`SELECT to_jsonb(r)::text FROM auth_installation_rotations r WHERE old_refresh_id=$1`, shared).Scan(&original); err != nil {
		t.Fatal(err)
	}
	complete := false
	for i := 0; i < 20 && !complete; i++ {
		var raw []byte
		if err := db.QueryRow(`SELECT privacy_erase_installations_batch($1,1,ARRAY['k1'])`, f.proof[4]).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var r struct {
			Processed, Mutations int
			Complete             bool
		}
		if err := json.Unmarshal(raw, &r); err != nil || r.Processed > 1 || r.Mutations > 13 {
			t.Fatal("unbounded result", string(raw), err)
		}
		complete = r.Complete
	}
	if !complete {
		t.Fatal("limit1 failed to converge")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM privacy_installation_evidence WHERE source_id=$1 AND kind='sanction'`, operation); n != 2 {
		t.Fatal("multi-installation sanction lost", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM privacy_installation_evidence e JOIN account_sanctions s ON s.operation_id=e.source_id WHERE e.match_until<>s.until_at OR e.purge_after<>s.until_at`); n != 0 {
		t.Fatal("actual sanction deadline changed", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM auth_revocations WHERE token_id IN($1,$2)`, old, refresh); n != 0 {
		t.Fatal("owned revocations retained", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM auth_revocations WHERE token_id IN($1,'unowned-revocation')`, access); n != 2 {
		t.Fatal("survivor/unknown revocation erased", n)
	}
	var after string
	if err := db.QueryRow(`SELECT to_jsonb(r)::text FROM auth_installation_rotations r WHERE old_refresh_id=$1`, shared).Scan(&after); err != nil || original != after {
		t.Fatal("survivor rotation changed", err)
	}
	for _, hash := range hashes {
		privacyInstallationRegister(t, db, hash, []string{"k1"})
		var active bool
		if err := db.QueryRow(`SELECT installation_sanction_active($1)`, hash).Scan(&active); err != nil || !active {
			t.Fatal("erased sanction stopped matching", err)
		}
	}
	lift := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO admin_operation_decisions(id,actor_admin_id,kind,target_account_id,reason,affected_accounts,request_hash,prior_sanction_id) VALUES($1,$2,'sanction_lift',$3,'fixture lift',jsonb_build_array($3::uuid::text),$4,$5)`, lift, f.admin, f.account, bytes.Repeat([]byte{2}, 32), operation); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO account_sanction_lifts(operation_id,sanction_id) VALUES($1,$2)`, lift, operation); err != nil {
		t.Fatal(err)
	}
	for _, hash := range append(hashes, "unregistered") {
		var active bool
		if err := db.QueryRow(`SELECT installation_sanction_active($1)`, hash).Scan(&active); err != nil || active {
			t.Fatal("lift did not apply immediately", hash, err)
		}
	}
	for _, q := range []string{`DELETE FROM auth_installation_rotations WHERE old_refresh_id=$1`, `UPDATE auth_installation_rotations SET issuance_config_hash='forged' WHERE old_refresh_id=$1`} {
		if _, err := db.Exec(q, shared); err == nil {
			t.Fatal("ordinary source rewrite bypassed guard")
		}
	}
	if n := valueCount(t, db, `SELECT count(*) FROM privacy_installation_erasure_authorizations`); n != 0 {
		t.Fatal("typed permits escaped transaction", n)
	}
}

func TestPrivacyInstallationsPrivateDependencyAndPrimaryKeyDrift(t *testing.T) {
	for _, kind := range []string{"evidence_cascade", "keys_cascade", "keys_primary"} {
		t.Run(kind, func(t *testing.T) {
			db, _ := textValueDB(t)
			f := privacyCredentialsFixture(t, db, true)
			hash := uuid.NewString()
			privacyInstallationRegister(t, db, hash, []string{"k1"})
			tag := bytes.Repeat([]byte{7}, 32)
			if _, err := db.Exec(`INSERT INTO privacy_installation_evidence(request_id,kind,source_id,key_id,selector_sha256,source_sha256,occurred_at,match_until,purge_after) VALUES($1,'erased_bootstrap',$1,'k1',$2,$2,clock_timestamp()-interval '2 days',clock_timestamp()-interval '1 day',clock_timestamp()-interval '1 day')`, f.proof[4], tag); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "evidence_cascade":
				if _, err := db.Exec(`CREATE TABLE privacy_evidence_sentinel(request_id uuid,kind text,source_id uuid,key_id text,selector_sha256 bytea,FOREIGN KEY(request_id,kind,source_id,key_id,selector_sha256) REFERENCES privacy_installation_evidence ON DELETE CASCADE); INSERT INTO privacy_evidence_sentinel SELECT request_id,kind,source_id,key_id,selector_sha256 FROM privacy_installation_evidence`); err != nil {
					t.Fatal(err)
				}
			case "keys_cascade":
				if _, err := db.Exec(`CREATE TABLE privacy_keys_sentinel(device_hash text,key_id text,FOREIGN KEY(device_hash,key_id) REFERENCES privacy_installation_keys ON DELETE CASCADE); INSERT INTO privacy_keys_sentinel SELECT device_hash,key_id FROM privacy_installation_keys`); err != nil {
					t.Fatal(err)
				}
			case "keys_primary":
				if _, err := db.Exec(`ALTER TABLE privacy_installation_keys DROP CONSTRAINT privacy_installation_keys_pkey`); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "evidence_cascade" {
				if _, err := db.Exec(`SELECT privacy_purge_installation_evidence(1)`); err == nil {
					t.Fatal("purge crossed private dependency boundary")
				}
				if n := valueCount(t, db, `SELECT count(*) FROM privacy_evidence_sentinel`); n != 1 {
					t.Fatal("dependent evidence erased", n)
				}
			} else {
				if _, err := db.Exec(`SELECT privacy_register_installation_keys($1,ARRAY['k2'],ARRAY[$2::bytea],ARRAY[$2::bytea])`, hash, tag); err == nil {
					t.Fatal("registration accepted private dependency/identity drift")
				}
				if kind == "keys_cascade" && valueCount(t, db, `SELECT count(*) FROM privacy_keys_sentinel`) != 1 {
					t.Fatal("dependent key erased")
				}
			}
			if n := valueCount(t, db, `SELECT count(*) FROM privacy_installation_evidence`); n != 1 {
				t.Fatal("evidence changed", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM privacy_installation_keys WHERE key_id='k1'`); n != 1 {
				t.Fatal("key changed", n)
			}
		})
	}
}

func TestPrivacyInstallationsPrivateDependencyDDLBothOrders(t *testing.T) {
	for _, first := range []string{"ddl", "purge"} {
		t.Run(first, func(t *testing.T) {
			db, _ := textValueDB(t)
			f := privacyCredentialsFixture(t, db, true)
			tag := bytes.Repeat([]byte{7}, 32)
			if _, err := db.Exec(`INSERT INTO privacy_installation_evidence(request_id,kind,source_id,key_id,selector_sha256,source_sha256,occurred_at,match_until,purge_after) VALUES($1,'erased_bootstrap',$1,'k1',$2,$2,clock_timestamp()-interval '2 days',clock_timestamp()-interval '1 day',clock_timestamp()-interval '1 day')`, f.proof[4], tag); err != nil {
				t.Fatal(err)
			}
			ddl := `CREATE TABLE privacy_ddl_sentinel(request_id uuid,kind text,source_id uuid,key_id text,selector_sha256 bytea,FOREIGN KEY(request_id,kind,source_id,key_id,selector_sha256) REFERENCES privacy_installation_evidence ON DELETE CASCADE)`
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if first == "ddl" {
				if _, err = tx.Exec(ddl); err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(`INSERT INTO privacy_ddl_sentinel SELECT request_id,kind,source_id,key_id,selector_sha256 FROM privacy_installation_evidence`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err = tx.Exec(`SELECT privacy_purge_installation_evidence(1)`); err != nil {
					t.Fatal(err)
				}
				var locked bool
				if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=pg_backend_pid() AND relation='privacy_installation_evidence'::regclass AND mode='RowExclusiveLock' AND granted)`).Scan(&locked); err != nil || !locked {
					t.Fatal("private evidence DDL lock absent", err)
				}
			}
			conn, err := db.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(); _ = conn.Close() }()
			var pid int
			if err = conn.QueryRowContext(t.Context(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				q := ddl
				if first == "ddl" {
					q = `SELECT privacy_purge_installation_evidence(1)`
				}
				_, e := conn.ExecContext(t.Context(), q)
				done <- e
			}()
			privacyReleaseWaitLock(t, db, pid)
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if first == "ddl" && err == nil {
					t.Fatal("post-wait private dependency bypassed")
				}
				if first == "purge" && err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("private DDL wait failed to converge")
			}
			want := int64(0)
			if first == "ddl" {
				want = 1
			}
			if n := valueCount(t, db, `SELECT count(*) FROM privacy_installation_evidence`); n != want {
				t.Fatal("unexpected evidence disposition", n, want)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM privacy_ddl_sentinel`); n != want {
				t.Fatal("DDL dependent changed", n, want)
			}
		})
	}
}
