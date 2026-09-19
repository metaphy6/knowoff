package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/migrations"
	"github.com/lib/pq"
)

func controllerFixture(t *testing.T) (*sql.DB, *sql.DB, *CutoverController, CutoverControlRoleSpec, CutoverBegin) {
	t.Helper()
	db, capture, control, spec := cutoverControlFixture(t)
	db.SetMaxIdleConns(0)
	capture.SetMaxIdleConns(0)
	c, err := OpenCutoverController(t.Context(), control, spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	id, err := c.Identity(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	id.InstanceID = uuid.NewString()
	manifest, err := migrations.Compiled()
	if err != nil {
		t.Fatal(err)
	}
	b := CutoverBegin{Identity: id, RequestID: uuid.NewString(), Pins: CutoverPins{SchemaSHA256: manifest.SHA256, ImageSHA256: strings.Repeat("b", 64), ConfigSHA256: strings.Repeat("c", 64), ContentSHA256: strings.Repeat("d", 64)}, Lease: bytes.Repeat([]byte{42}, 32), LeaseSeconds: 60}
	return db, control, c, spec, b
}
func proofFor(b CutoverBegin, r CutoverReceipt) CutoverProof {
	return CutoverProof{InstanceID: b.Identity.InstanceID, RequestID: b.RequestID, Generation: r.Generation, Lease: b.Lease}
}
func controllerWriter(t *testing.T, db *sql.DB, role string) *sql.DB {
	t.Helper()
	secret := uuid.NewString()
	if _, err := db.Exec(`ALTER ROLE ` + pq.QuoteIdentifier(role) + ` PASSWORD ` + pq.QuoteLiteral(secret)); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(role, secret)
	writer, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writer.Close() })
	return writer
}
func TestCutoverControllerFencesExistingAndFutureWriters(t *testing.T) {
	db, _, c, spec, b := controllerFixture(t)
	writer := controllerWriter(t, db, spec.Roles.Runtime)
	held, err := writer.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback()
	account := uuid.NewString()
	if _, err := held.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,'uncommitted')`, account); err != nil {
		t.Fatal(err)
	}
	migrator := controllerWriter(t, db, spec.Roles.Migrator)
	migration, err := migrator.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer migration.Close()
	if _, err := migration.ExecContext(t.Context(), `SET ROLE `+pq.QuoteIdentifier(spec.Roles.Owner)); err != nil {
		t.Fatal(err)
	}
	privacy := controllerWriter(t, db, spec.Roles.PrivacyExecutor)
	privacySession, err := privacy.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer privacySession.Close()
	idle := controllerWriter(t, db, spec.Roles.Runtime)
	var idlePID int
	if err := idle.QueryRow(`SELECT pg_backend_pid()`).Scan(&idlePID); err != nil {
		t.Fatal(err)
	}
	r, err := c.Begin(t.Context(), b)
	if n := valueCount(t, db, `SELECT count(*) FROM pg_stat_activity WHERE pid=$1`, idlePID); n != 0 {
		t.Fatal("idle pooled writer survived fence")
	}
	if err != nil || r.Phase != "closing" || r.Generation != 1 {
		t.Fatal(r, err)
	}
	if err := held.Commit(); err == nil {
		t.Fatal("pre-fence transaction committed")
	}
	if _, err := migration.ExecContext(t.Context(), `SELECT 1`); err == nil {
		t.Fatal("SET ROLE migration connection survived")
	}
	if _, err := privacySession.ExecContext(t.Context(), `SELECT 1`); err == nil {
		t.Fatal("privacy executor survived fence")
	}
	privacy.Close()
	freshPrivacy := controllerWriter(t, db, spec.Roles.PrivacyExecutor)
	if err := freshPrivacy.PingContext(t.Context()); err == nil {
		t.Fatal("NOLOGIN privacy executor reconnected")
	}
	writer.Close()
	fresh := controllerWriter(t, db, spec.Roles.Runtime)
	if err := fresh.PingContext(t.Context()); err == nil {
		t.Fatal("NOLOGIN writer reconnected")
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM accounts WHERE id=$1`, account).Scan(&n); err != nil || n != 0 {
		t.Fatal("uncommitted write survived", n, err)
	}
	again, err := c.Begin(t.Context(), b)
	if err != nil || again.Generation != r.Generation {
		t.Fatal("uncertain begin replay", again, err)
	}
	conflict := b
	conflict.Pins.ImageSHA256 = strings.Repeat("e", 64)
	if _, err = c.Begin(t.Context(), conflict); err == nil {
		t.Fatal("conflicting pins accepted")
	}
}
func TestCutoverControllerSealReplayAndTamperRefusal(t *testing.T) {
	db, _, c, _, b := controllerFixture(t)
	account := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,'private-fixture')`, account); err != nil {
		t.Fatal(err)
	}
	r, err := c.Begin(t.Context(), b)
	if err != nil {
		t.Fatal(err)
	}
	seal := CutoverSeal{Proof: proofFor(b, r), WatermarkID: uuid.NewString()}
	r, err = c.Seal(t.Context(), seal)
	if err != nil || r.Phase != "sealed" || len(r.EvidenceSHA256) != 64 {
		t.Fatal(r, err)
	}
	replay, err := c.Seal(t.Context(), seal)
	if err != nil || replay.EvidenceSHA256 != r.EvidenceSHA256 {
		t.Fatal("seal replay", replay, err)
	}
	var evidence string
	if err := db.QueryRow(`SELECT evidence::text FROM cutover_watermarks`).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(evidence, account) || strings.Contains(evidence, "private-fixture") {
		t.Fatal("private row exported")
	}
	if _, err := db.Exec(`UPDATE accounts SET nickname='tampered' WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Seal(t.Context(), seal); err == nil {
		t.Fatal("changed retained data accepted")
	}
}
func TestCutoverControllerPendingAndUnknownSessionsRefuseSeal(t *testing.T) {
	db, _, c, _, b := controllerFixture(t)
	r, err := c.Begin(t.Context(), b)
	if err != nil {
		t.Fatal(err)
	}
	seal := CutoverSeal{Proof: proofFor(b, r), WatermarkID: uuid.NewString()}
	unknown, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Seal(t.Context(), seal); err == nil {
		t.Fatal("unknown privileged backend accepted")
	}
	unknown.Close()
	if _, err := db.Exec(`INSERT INTO oauth_flows(id,provider,intent,state_hash,completion_hash,nonce_hash,code_verifier,requester_hash,status,expires_at) VALUES(gen_random_uuid(),'google','restore',repeat('a',43),repeat('b',43),repeat('c',43),'',repeat('d',43),'exchanging',clock_timestamp()+interval '1 minute')`); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Seal(t.Context(), seal); !errors.Is(err, ErrCutoverPending) {
		t.Fatal("pending provider work accepted", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM cutover_watermarks`).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}
func TestCutoverControllerLaneLeaseRecoveryAndHandoff(t *testing.T) {
	db, control, c, spec, b := controllerFixture(t)
	other, err := OpenCutoverController(t.Context(), control, spec)
	if err == nil {
		other.Close()
		t.Fatal("competing controller acquired lane")
	}
	b.LeaseSeconds = 1
	r, err := c.Begin(t.Context(), b)
	if err != nil {
		t.Fatal(err)
	}
	seal := CutoverSeal{Proof: proofFor(b, r), WatermarkID: uuid.NewString()}
	bad := seal
	bad.Proof.Lease = bytes.Repeat([]byte{9}, 32)
	if _, err = c.Seal(t.Context(), bad); err == nil {
		t.Fatal("wrong secret accepted")
	}
	if _, err := db.Exec(`SELECT pg_sleep(1.05)`); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Seal(t.Context(), seal); !errors.Is(err, ErrCutoverLease) {
		t.Fatal("expired lease accepted", err)
	}
	next := b
	next.RequestID = uuid.NewString()
	next.PredecessorID = b.RequestID
	next.LeaseSeconds = 60
	r, err = c.Begin(t.Context(), next)
	if err != nil || r.Generation != 2 {
		t.Fatal("closed recovery", r, err)
	}
	if _, err = c.Seal(t.Context(), seal); !errors.Is(err, ErrCutoverConflict) {
		t.Fatal("old generation proof accepted", err)
	}
	seal = CutoverSeal{Proof: proofFor(next, r), WatermarkID: uuid.NewString()}
	r, err = c.Seal(t.Context(), seal)
	if err != nil {
		t.Fatal(err)
	}
	target := b.Identity
	target.InstanceID = uuid.NewString()
	target.DatabaseOID++
	h := CutoverHandoff{Proof: seal.Proof, WatermarkID: seal.WatermarkID, HandoffID: uuid.NewString(), Target: target}
	if _, err = c.Handoff(t.Context(), h); err == nil {
		t.Fatal("same cluster activation accepted")
	}
	target.ClusterSystemIdentifier = "1"
	h.Target = target
	receipt, err := c.Handoff(t.Context(), h)
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.Handoff(t.Context(), h)
	if err != nil || again.HandoffID != receipt.HandoffID {
		t.Fatal("handoff replay", again, err)
	}
	h.Target.InstanceID = uuid.NewString()
	if _, err = c.Handoff(t.Context(), h); err == nil {
		t.Fatal("retarget accepted")
	}
	next.PredecessorID = next.RequestID
	next.RequestID = uuid.NewString()
	if _, err = c.Begin(t.Context(), next); err == nil {
		t.Fatal("handed-off source recovered")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	spec.WritersEnabled = false
	resumed, err := OpenCutoverController(t.Context(), control, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	var enabled bool
	if err := db.QueryRow(`SELECT bool_or(rolcanlogin) FROM pg_roles WHERE rolname=ANY($1)`, pq.Array([]string{spec.Roles.Runtime, spec.Roles.Migrator})).Scan(&enabled); err != nil || enabled {
		t.Fatal("close reopened source", enabled, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = resumed.Identity(ctx); err == nil {
		t.Fatal("canceled context succeeded")
	}
}

func TestCutoverControllerSealedSchemaDriftRefusesReplay(t *testing.T) {
	for name, sql := range map[string]string{
		"empty_column":   `ALTER TABLE accounts ADD COLUMN unexpected_cutover_column text`,
		"insert_rule":    `CREATE RULE silently_drop_wallet_inserts AS ON INSERT TO noin_wallets DO INSTEAD NOTHING`,
		"sequence_owner": `ALTER SEQUENCE noin_ledger_id_seq OWNED BY NONE`,
	} {
		t.Run(name, func(t *testing.T) {
			db, _, c, spec, b := controllerFixture(t)
			r, err := c.Begin(t.Context(), b)
			if err != nil {
				t.Fatal(err)
			}
			seal := CutoverSeal{Proof: proofFor(b, r), WatermarkID: uuid.NewString()}
			if _, err = c.Seal(t.Context(), seal); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`SET ROLE ` + pq.QuoteIdentifier(spec.Roles.Owner) + `;` + sql + `;RESET ROLE`); err != nil {
				t.Fatal(err)
			}
			if _, err = c.Seal(t.Context(), seal); !errors.Is(err, ErrCutoverParity) {
				t.Fatal("schema drift accepted", err)
			}
		})
	}
}

func TestCutoverControllerTerminalHandoffReplaySurvivesLeaseExpiry(t *testing.T) {
	db, _, c, _, b := controllerFixture(t)
	b.LeaseSeconds = 2
	r, err := c.Begin(t.Context(), b)
	if err != nil {
		t.Fatal(err)
	}
	seal := CutoverSeal{Proof: proofFor(b, r), WatermarkID: uuid.NewString()}
	if _, err = c.Seal(t.Context(), seal); err != nil {
		t.Fatal(err)
	}
	target := b.Identity
	target.InstanceID = uuid.NewString()
	target.ClusterSystemIdentifier = "1"
	h := CutoverHandoff{Proof: seal.Proof, WatermarkID: seal.WatermarkID, HandoffID: uuid.NewString(), Target: target}
	if _, err = c.Handoff(t.Context(), h); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`SELECT pg_sleep(2.05)`); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Handoff(t.Context(), h); err != nil {
		t.Fatal("terminal authenticated receipt became unavailable", err)
	}
	h.Proof.Lease = bytes.Repeat([]byte{99}, 32)
	if _, err = c.Handoff(t.Context(), h); !errors.Is(err, ErrCutoverLease) {
		t.Fatal("terminal replay bypassed secret", err)
	}
}

func TestCutoverControllerInvalidBeginCannotCloseWriters(t *testing.T) {
	db, _, c, spec, b := controllerFixture(t)
	for name, change := range map[string]func(*CutoverBegin){
		"physical_tuple":      func(b *CutoverBegin) { b.Identity.DatabaseOID++ },
		"compiled_schema_pin": func(b *CutoverBegin) { b.Pins.SchemaSHA256 = strings.Repeat("f", 64) },
		"lease_shape":         func(b *CutoverBegin) { b.Lease = []byte("private-secret") },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := b
			change(&candidate)
			if _, err := c.Begin(t.Context(), candidate); err == nil {
				t.Fatal("invalid begin succeeded")
			}
			if n := valueCount(t, db, `SELECT count(*) FROM cutover_instances`); n != 0 {
				t.Fatal("invalid begin created authority")
			}
			if n := valueCount(t, db, `SELECT count(*) FROM pg_roles WHERE rolname=ANY($1::text[]) AND rolcanlogin`, pq.Array(spec.Roles.writers())); n != 3 {
				t.Fatal("invalid begin closed writers")
			}
		})
	}
}
func TestCutoverControllerActualDurablePendingWork(t *testing.T) {
	for _, kind := range []string{"missing_settlement", "pending_settlement", "reserved_admission", "prepared_match", "started_match", "operator_decision", "completed_settlement"} {
		t.Run(kind, func(t *testing.T) {
			db, _, c, _, b := controllerFixture(t)
			account := valueAccount(t, db)
			match := uuid.NewString()
			operation := uuid.NewString()
			if kind == "operator_decision" {
				admin := uuid.NewString()
				if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret,role) VALUES($1,$2,'cutover-admin@example.invalid','fixture','fixture','admin')`, admin, account); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO admin_operation_decisions(id,actor_admin_id,kind,target_account_id,amount,reason,affected_accounts,request_hash) VALUES($1,$2,'noin_grant',$3::uuid,1,'fixture',jsonb_build_array($3::uuid::text),decode(repeat('a',64),'hex'))`, operation, admin, account); err != nil {
					t.Fatal(err)
				}
			} else {
				state := "completed"
				if kind == "prepared_match" {
					state = "prepared"
				}
				if kind == "started_match" {
					state = "started"
				}
				if _, err := db.Exec(`INSERT INTO text_matches(id,room_id,contract,contract_hash,owner_id,fence,state,prototype,created_at) VALUES($1,'cutover-fixture','{}',repeat('a',64),$2,1,$3,true,clock_timestamp())`, match, uuid.NewString(), state); err != nil {
					t.Fatal(err)
				}
				admissionState := "released"
				if kind == "reserved_admission" {
					admissionState = "reserved"
				}
				if _, err := db.Exec(`INSERT INTO text_admissions(id,account_id,match_id,seat,entry_path,prototype,access_kind,quota_day,reserved_at,state) VALUES($1,$2,$3,0,'local',true,'prototype',CURRENT_DATE,clock_timestamp(),$4)`, uuid.NewString(), account, match, admissionState); err != nil {
					t.Fatal(err)
				}
				if kind == "pending_settlement" || kind == "completed_settlement" || kind == "reserved_admission" {
					settlementState := "pending"
					if kind != "pending_settlement" {
						settlementState = "applied"
					}
					if _, err := db.Exec(`INSERT INTO text_settlements(match_id,account_id,outcome_hash,state) VALUES($1,$2,repeat('a',64),$3)`, match, account, settlementState); err != nil {
						t.Fatal(err)
					}
				}
			}
			r, err := c.Begin(t.Context(), b)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Seal(t.Context(), CutoverSeal{Proof: proofFor(b, r), WatermarkID: uuid.NewString()})
			if kind == "completed_settlement" {
				if err != nil {
					t.Fatal("completed settlement blocked capture", err)
				}
			} else {
				if !errors.Is(err, ErrCutoverPending) {
					t.Fatal("pending work accepted", err)
				}
				if n := valueCount(t, db, `SELECT count(*) FROM cutover_watermarks`); n != 0 {
					t.Fatal("pending work minted watermark")
				}
			}
		})
	}
}

func TestCutoverControllerUnpublishedAuthenticatedStartupBlocksSeal(t *testing.T) {
	db, _, c, spec, b := controllerFixture(t)
	secret := uuid.NewString()
	if _, err := db.Exec(`ALTER ROLE ` + pq.QuoteIdentifier(spec.Roles.Runtime) + ` PASSWORD ` + pq.QuoteLiteral(secret)); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(spec.Roles.Runtime, secret)
	q := u.Query()
	q.Set("options", "-c post_auth_delay=3")
	u.RawQuery = q.Encode()
	delayed, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer delayed.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	account := uuid.NewString()
	done := make(chan error, 1)
	go func() {
		_, err := delayed.ExecContext(ctx, `INSERT INTO accounts(id,nickname) VALUES($1,'late-authenticated')`, account)
		done <- err
	}()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		n := valueCount(t, db, `SELECT count(*) FROM pg_locks l WHERE l.locktype='object' AND l.classid='pg_database'::regclass AND l.objid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND l.mode='RowExclusiveLock' AND NOT EXISTS(SELECT 1 FROM pg_stat_activity a WHERE a.pid=l.pid)`)
		if n > 0 {
			break
		}
		select {
		case <-ticker.C:
		case err := <-done:
			t.Fatal("startup did not stay unpublished", err)
		case <-ctx.Done():
			t.Fatal("startup was not observed")
		}
	}
	r, err := c.Begin(ctx, b)
	if !errors.Is(err, ErrCutoverBusy) {
		t.Fatal("unpublished authenticated writer passed fence", err)
	}
	seal := CutoverSeal{Proof: proofFor(b, r), WatermarkID: uuid.NewString()}
	if _, err = c.Seal(ctx, seal); !errors.Is(err, ErrCutoverBusy) {
		t.Fatal("unpublished authenticated writer passed seal", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM cutover_watermarks`); n != 0 {
		t.Fatal("startup race minted watermark")
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal("already authorized startup did not finish", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// The authenticated startup was allowed to finish only while capture refused.
	// Repeating the same request now removes its pooled physical session, and the
	// final watermark covers the committed late row. No source LOGIN is reopened.
	if _, err = c.Begin(ctx, b); err != nil {
		t.Fatal("physical cleanup did not resume", err)
	}
	if _, err = c.Seal(ctx, seal); err != nil {
		t.Fatal("sealed after actual startup completion", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM accounts WHERE id=$1`, account); n != 1 {
		t.Fatal("late durable write not retained")
	}
}

func TestCutoverControllerSnapshotAfterPhysicalFence(t *testing.T) {
	db, _, c, spec, b := controllerFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	migrator := controllerWriter(t, db, spec.Roles.Migrator)
	migrator.SetMaxIdleConns(0)
	writer, err := migrator.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	blocker, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Begin(ctx, b)
	if !errors.Is(err, ErrCutoverBusy) {
		t.Fatal("expected durable closing with unknown session", err)
	}
	blocker.Close()
	held, err := writer.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback()
	account := uuid.NewString()
	if _, err = held.ExecContext(ctx, `SET ROLE `+pq.QuoteIdentifier(spec.Roles.Owner)+`; LOCK TABLE schema_migrations IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	if _, err = held.ExecContext(ctx, `INSERT INTO accounts(id,nickname) VALUES($1,'snapshot-late')`, account); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/postgres"
	if spec.Roles.Database == "postgres" {
		u.Path = "/template1"
	}
	observer, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	seal := CutoverSeal{Proof: proofFor(b, r), WatermarkID: uuid.NewString()}
	done := make(chan error, 1)
	go func() { _, e := c.Seal(ctx, seal); done <- e }()
	until := time.Now().Add(2 * time.Second)
	for {
		var n int
		err = observer.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=$1 AND usename=$2 AND wait_event_type='Lock' AND query LIKE '%schema_migrations%'`, spec.Roles.Database, spec.Control).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("Seal did not enter forced snapshot window")
		}
		time.Sleep(time.Millisecond)
	}
	if err = held.Commit(); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	migrator.Close()
	if err = <-done; err != nil {
		t.Fatal("Seal result", err)
	}
	var actual, captured int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE id=$1`, account).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT (evidence->'tables'->'accounts'->>'count')::int FROM cutover_watermarks WHERE id=$1`, seal.WatermarkID).Scan(&captured); err != nil {
		t.Fatal(err)
	}
	t.Logf("Seal returned success: actual committed rows=%d, captured rows=%d", actual, captured)
	if actual != 1 || captured != 1 {
		t.Fatal("watermark omitted committed pre-fence row")
	}
	target := b.Identity
	target.InstanceID = uuid.NewString()
	target.ClusterSystemIdentifier = "1"
	_, err = c.Handoff(ctx, CutoverHandoff{Proof: seal.Proof, WatermarkID: seal.WatermarkID, HandoffID: uuid.NewString(), Target: target})
	if err != nil {
		t.Fatal("fresh handoff rejected coherent watermark", err)
	}
	t.Log("Fresh handoff preserves the complete committed source snapshot")
}

func TestCutoverControllerRequiresPrivacySuppressionPublication(t *testing.T) {
	db, _, c, _, b := controllerFixture(t)
	account := valueAccount(t, db)
	request := uuid.NewString()
	if _, err := db.Exec(`SELECT privacy_prepare_verified_request($1,$2,decode(repeat('a',64),'hex'),decode(repeat('b',64),'hex'))`, request, account); err != nil {
		t.Fatal(err)
	}
	r, err := c.Begin(t.Context(), b)
	if err != nil {
		t.Fatal(err)
	}
	seal := CutoverSeal{Proof: proofFor(b, r), WatermarkID: uuid.NewString()}
	if _, err := c.Seal(t.Context(), seal); !errors.Is(err, ErrCutoverPending) {
		t.Fatal("unpublished deletion suppression sealed", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM cutover_watermarks`); n != 0 {
		t.Fatal("pending deletion minted watermark")
	}
	// Only a fixture authority binds this synthetic receipt; no external journal is claimed.
	if _, err := db.Exec(`SELECT privacy_bind_suppression($1,1,decode(repeat('c',64),'hex'))`, request); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Seal(t.Context(), seal); err != nil {
		t.Fatal("published deletion blocked resumable snapshot", err)
	}
}
