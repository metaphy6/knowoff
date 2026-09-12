package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"testing"
	"time"
)

func ownerReady(t *testing.T, db *sql.DB, values *TextValueStore) (*TextOwner, *TextValueStore) {
	t.Helper()
	owner, err := AcquireTextOwner(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Release(context.Background()); err != nil && !errors.Is(err, ErrTextOwnerLost) {
			t.Error(err)
		}
	})
	bound, err := values.WithOwner(owner)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := owner.RecoverLostOwners(context.Background(), bound, 100)
	if err != nil || !batch.Done {
		t.Fatalf("ready: %+v %v", batch, err)
	}
	return owner, bound
}
func ownerMatch(t *testing.T, owner *TextOwner, values *TextValueStore, db *sql.DB, at time.Time, start bool) (TextMatchRecord, []string) {
	t.Helper()
	m, accounts := valueUnpreparedMatch(t, values, db, at, false)
	m.Owner = owner.Token().IncarnationID
	if err := values.Prepare(context.Background(), m, at); err != nil {
		t.Fatal(err)
	}
	if start {
		if err := values.Start(context.Background(), m.Contract.MatchID, m.Owner, 1, at); err != nil {
			t.Fatal(err)
		}
	}
	return m, accounts
}
func TestTextOwnerExclusionReleaseAndUnboundFence(t *testing.T) {
	db, values := textValueDB(t)
	ctx := context.Background()
	owner, bound := ownerReady(t, db, values)
	token := owner.Token()
	if token.Generation != 1 || !valueUUID(token.IncarnationID) {
		t.Fatal(token)
	}
	if other, err := AcquireTextOwner(ctx, db); !errors.Is(err, ErrTextOwnerBusy) || other != nil {
		t.Fatalf("second owner: %v %v", other, err)
	}
	a := TextReservation{ID: uuid.NewString(), AccountID: valueAccount(t, db), EntryPath: "quick_play", At: time.Now()}
	if err := values.Reserve(ctx, a); !errors.Is(err, ErrValueFence) {
		t.Fatal("unbound", err)
	}
	if err := bound.Reserve(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := owner.Release(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-owner.Done():
	default:
		t.Fatal("release did not close Done")
	}
	if err := bound.Reserve(ctx, a); !errors.Is(err, ErrValueFence) {
		t.Fatal("released replay", err)
	}
	next, err := AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release(ctx)
	if next.Token().Generation != 2 {
		t.Fatal(next.Token())
	}
	nextValues, _ := values.WithOwner(next)
	r, err := next.RecoverLostOwners(ctx, nextValues, 1)
	if err != nil || r.Released != 1 {
		t.Fatalf("recover reserved: %+v %v", r, err)
	}
	if err := bound.CancelReservation(ctx, a.ID, a.AccountID); !errors.Is(err, ErrValueFence) {
		t.Fatal("old cancellation", err)
	}
}
func TestTextOwnerPhysicalLossPreservesAwardsAndTerminal(t *testing.T) {
	db, values := textValueDB(t)
	ctx := context.Background()
	owner, bound := ownerReady(t, db, values)
	at := time.Date(2026, 9, 12, 23, 59, 0, 0, time.UTC)
	active, ids := ownerMatch(t, owner, bound, db, at, true)
	prepared, _ := ownerMatch(t, owner, bound, db, at, false)
	terminal, terminalIDs := ownerMatch(t, owner, bound, db, at, true)
	award := TextAward{MatchID: active.Contract.MatchID, Owner: active.Owner, Epoch: 1, AccountID: ids[1], Kind: "correct_vote", Ordinal: 1, Amount: values.tuning.Noin.CorrectVote, At: at}
	credited, err := bound.Award(ctx, award)
	if err != nil {
		t.Fatal(err)
	}
	outcome := TextOutcome{MatchID: terminal.Contract.MatchID, Owner: terminal.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: at}
	for seat, id := range terminalIDs {
		role := "nower"
		if seat == 0 {
			role = "donower"
		}
		outcome.Players = append(outcome.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 10})
	}
	if err := bound.Finish(ctx, outcome); err != nil {
		t.Fatal(err)
	}
	var killed bool
	if err := db.QueryRow(`SELECT pg_terminate_backend(backend_pid) FROM text_process_owners WHERE incarnation_id=$1`, owner.Token().IncarnationID).Scan(&killed); err != nil || !killed {
		t.Fatal(killed, err)
	}
	select {
	case <-owner.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("backend loss did not close Done")
	}
	if err := owner.Check(ctx); !errors.Is(err, ErrTextOwnerLost) {
		t.Fatal("physical loss", err)
	}
	next, err := AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release(ctx)
	defer next.Release(ctx)
	replacement, _ := values.WithOwner(next)
	total := TextOwnerRecovery{}
	for i := 0; i < 10; i++ {
		r, err := next.RecoverLostOwners(ctx, replacement, 1)
		if err != nil {
			t.Fatal(err)
		}
		total.Cancelled += r.Cancelled
		total.Interrupted += r.Interrupted
		if r.Done {
			break
		}
	}
	if total.Cancelled != 1 || total.Interrupted != 1 {
		t.Fatal(total)
	}
	if n := valueCount(t, db, `SELECT count FROM daily_quickplay_counts WHERE account_id=$1 AND server_day=$2`, ids[0], valueDay(at)); n != 0 {
		t.Fatal("quota not compensated", n)
	}
	if n := valueCount(t, db, `SELECT credited FROM text_award_receipts WHERE match_id=$1`, active.Contract.MatchID); n != int64(credited) {
		t.Fatal("award changed", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE state='pending' AND match_id IN ($1,$2)`, active.Contract.MatchID, prepared.Contract.MatchID); n != 0 {
		t.Fatal("fabricated outcome", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE match_id=$1 AND state='applied'`, active.Contract.MatchID); n != 4 {
		t.Fatal("interruption delivery missing", n)
	}
	if n := valueCount(t, db, `SELECT sum(overall_points+xp) FROM profiles WHERE account_id=ANY($1)`, pq.Array(ids)); n != 0 {
		t.Fatal("interruption granted result value", n)
	}
	if _, err := bound.Award(ctx, award); !errors.Is(err, ErrValueFence) {
		t.Fatal("old award receipt replay", err)
	}
	if err := bound.Finish(ctx, outcome); !errors.Is(err, ErrValueFence) {
		t.Fatal("old terminal replay", err)
	}
	if err := values.SettlePending(ctx, terminal.Contract.MatchID); err != nil {
		t.Fatal("independent committed settlement", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE state='applied' AND match_id=$1`, terminal.Contract.MatchID); n != 4 {
		t.Fatal(n)
	}
	r, err := next.RecoverLostOwners(ctx, replacement, 1)
	if err != nil || !r.Done || r.Interrupted != 0 || r.Cancelled != 0 {
		t.Fatal(r, err)
	}
}

func TestTextOwnerPausedStartSerializesTakeover(t *testing.T) {
	db, values := textValueDB(t)
	ctx := context.Background()
	owner, bound := ownerReady(t, db, values)
	at := time.Now().UTC()
	m, ids := ownerMatch(t, owner, bound, db, at, false)
	entered, release := make(chan struct{}), make(chan struct{})
	guarded := bound.WithStartGuard(func(context.Context, *sql.Tx, TextMatchRecord, []string, time.Time) error {
		close(entered)
		<-release
		return nil
	})
	finished := make(chan error, 1)
	go func() { finished <- guarded.Start(ctx, m.Contract.MatchID, m.Owner, 1, at) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("start never entered guard")
	}
	var killed bool
	if err := db.QueryRow(`SELECT pg_terminate_backend(backend_pid) FROM text_process_owners WHERE incarnation_id=$1`, owner.Token().IncarnationID).Scan(&killed); err != nil || !killed {
		close(release)
		t.Fatal(killed, err)
	}
	deadline, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	failed, err := AcquireTextOwner(deadline, db)
	cancel()
	if err == nil || failed != nil {
		close(release)
		t.Fatal("takeover passed guarded start", err)
	}
	if n := valueCount(t, db, `SELECT generation FROM text_process_current`); n != 1 {
		close(release)
		t.Fatal("generation advanced around start", n)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal("previously accepted start", err)
	}
	next, err := AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal("failed acquisition leaked lock", err)
	}
	defer next.Release(ctx)
	defer next.Release(ctx)
	replacement, _ := values.WithOwner(next)
	if err := replacement.Reserve(ctx, TextReservation{ID: uuid.NewString(), AccountID: valueAccount(t, db), EntryPath: "local", At: at}); !errors.Is(err, ErrValueFence) {
		t.Fatal("admission before recovery", err)
	}
	result, err := next.RecoverLostOwners(ctx, replacement, 1)
	if err != nil || result.Interrupted != 1 || !result.Done {
		t.Fatal(result, err)
	}
	if n := valueCount(t, db, `SELECT count FROM daily_quickplay_counts WHERE account_id=$1`, ids[0]); n != 0 {
		t.Fatal(n)
	}
}

func TestTextOwnerRecoveryRollbackAndSuccessorResume(t *testing.T) {
	db, values := textValueDB(t)
	ctx := context.Background()
	owner, bound := ownerReady(t, db, values)
	at := time.Date(2026, 9, 12, 23, 59, 0, 0, time.UTC)
	m, ids := ownerMatch(t, owner, bound, db, at, true)
	_, _ = ownerMatch(t, owner, bound, db, at, false)
	if err := owner.Release(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release(ctx)
	replacement, _ := values.WithOwner(next)
	// Fail after each participant compensation, including after earlier wallet-day
	// writes. The entire unit must roll back, retaining all consumed counts.
	for _, id := range ids {
		if _, err := db.Exec(`CREATE OR REPLACE FUNCTION text_owner_test_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='compensated' AND NEW.account_id::text='` + id + `' THEN RAISE EXCEPTION 'injected compensation failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER text_owner_test_failure AFTER UPDATE ON text_admissions FOR EACH ROW EXECUTE FUNCTION text_owner_test_failure()`); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			r, err := next.RecoverLostOwners(ctx, replacement, 1)
			if err != nil {
				break
			}
			if r.Done {
				t.Fatal("failure injection did not run")
			}
			if i == 2 {
				t.Fatal("interruption not reached")
			}
		}
		for _, account := range ids {
			if n := valueCount(t, db, `SELECT count FROM daily_quickplay_counts WHERE account_id=$1`, account); n != 1 {
				t.Fatal("partial compensation committed", n)
			}
		}
		if n := valueCount(t, db, `SELECT count(*) FROM text_admissions WHERE match_id=$1 AND state='started'`, m.Contract.MatchID); n != 4 {
			t.Fatal(n)
		}
		if _, err := db.Exec(`DROP TRIGGER text_owner_test_failure ON text_admissions`); err != nil {
			t.Fatal(err)
		}
	}
	if err := next.Release(ctx); err != nil {
		t.Fatal(err)
	}
	successor, err := AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer successor.Release(ctx)
	resumed, _ := values.WithOwner(successor)
	r, err := successor.RecoverLostOwners(ctx, resumed, 1)
	if err != nil || r.Interrupted != 1 {
		t.Fatal(r, err)
	}
	if !r.Done {
		r, err = successor.RecoverLostOwners(ctx, resumed, 1)
		if err != nil || !r.Done || r.Interrupted != 0 {
			t.Fatal(r, err)
		}
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_award_receipts`); n != 0 {
		t.Fatal("fabricated value", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_process_owners WHERE lost_at IS NOT NULL AND recovered_at IS NOT NULL`); n != 2 {
		t.Fatal("lost generations not recovered", n)
	}
}

func TestTextOwnerRefusesHistoricalActiveWork(t *testing.T) {
	db, values := textValueDB(t)
	ctx := context.Background()
	_, _ = valuePreparedMatch(t, values, db, time.Now(), false)
	owner, err := AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release(ctx)
	bound, _ := values.WithOwner(owner)
	if _, err := owner.RecoverLostOwners(ctx, bound, 100); !errors.Is(err, ErrTextOwnerLegacyActive) {
		t.Fatal(err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_matches WHERE state='prepared'`); n != 1 {
		t.Fatal("historical work changed", n)
	}
	if err := values.Reserve(ctx, TextReservation{ID: uuid.NewString(), AccountID: valueAccount(t, db), EntryPath: "local", At: time.Now()}); !errors.Is(err, ErrValueFence) {
		t.Fatal("fallback survived first acquisition", err)
	}
}

func TestTextOwnerMigrationParityEmptyDownAndConstraints(t *testing.T) {
	db := transitionDesignDB(t, 12, true)
	db.SetMaxOpenConns(12)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	legacyID, legacyAccount := uuid.NewString(), valueAccount(t, db)
	if _, err := db.ExecContext(ctx, `INSERT INTO text_admissions(id,account_id,entry_path,prototype,access_kind,quota_day,reserved_at,state) VALUES($1,$2,'local',false,'local',CURRENT_DATE,now(),'reserved')`, legacyID, legacyAccount); err != nil {
		t.Fatal(err)
	}
	var legacyBefore, legacyAfter string
	if err := db.QueryRowContext(ctx, `SELECT to_jsonb(a)::text FROM text_admissions a WHERE id=$1`, legacyID).Scan(&legacyBefore); err != nil {
		t.Fatal(err)
	}
	before := transitionLegacySnapshot(t, ctx, db)
	path := transitionMigrationPath(t, 13)
	if err := MigrateUp(db, path); err != nil {
		t.Fatal(err)
	}
	after := transitionLegacySnapshot(t, ctx, db)
	if err := db.QueryRowContext(ctx, `SELECT (to_jsonb(a)-'process_owner_id'-'process_generation')::text FROM text_admissions a WHERE id=$1 AND process_owner_id IS NULL AND process_generation IS NULL`, legacyID).Scan(&legacyAfter); err != nil {
		t.Fatal(err)
	}
	if legacyAfter != legacyBefore {
		t.Fatal("migration rewrote historical text admission")
	}

	for _, table := range before.Tables {
		if table.Name == "schema_migrations" {
			continue
		}
		found := false
		for _, newTable := range after.Tables {
			if table.Name == newTable.Name {
				found = true
				if table.Rows != newTable.Rows || (table.Name != "text_admissions" && table.SHA256 != newTable.SHA256) {
					t.Fatalf("migration changed %s", table.Name)
				}
			}
		}
		if !found {
			t.Fatal("missing table", table.Name)
		}
	}
	if err := runMigration(db, path, "ownership empty down", func(m *migrate.Migrate) error { return m.Steps(-1) }); err != nil {
		t.Fatal(err)
	}
	assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
	if err := MigrateUp(db, path); err != nil {
		t.Fatal(err)
	}
	account := valueAccount(t, db)
	if _, err := db.Exec(`INSERT INTO text_admissions(id,account_id,entry_path,prototype,access_kind,quota_day,reserved_at,state,process_owner_id) VALUES($1,$2,'local',false,'local',CURRENT_DATE,now(),'reserved',$3)`, uuid.NewString(), account, uuid.NewString()); err == nil {
		t.Fatal("partial process identity accepted")
	}
	db.SetMaxOpenConns(12)
	owner, err := AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release(ctx)
	if _, err := db.Exec(`UPDATE text_process_owners SET backend_pid=backend_pid+1`); err == nil {
		t.Fatal("owner identity mutable")
	}
	if _, err := db.Exec(`INSERT INTO text_process_current(singleton,incarnation_id,generation) VALUES(2,$1,1)`, owner.Token().IncarnationID); err == nil {
		t.Fatal("second singleton accepted")
	}
	if err := runMigration(db, path, "ownership retained down refusal", func(m *migrate.Migrate) error { return m.Steps(-1) }); err == nil {
		t.Fatal("retained ownership discarded")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_process_owners`); n != 1 {
		t.Fatal(n)
	}
}

func TestTextOwnerRejectsSingleConnectionPool(t *testing.T) {
	db, _ := textValueDB(t)
	db.SetMaxOpenConns(1)
	owner, err := AcquireTextOwner(context.Background(), db)
	if owner != nil {
		defer owner.Release(context.Background())
	}
	if !errors.Is(err, ErrValueConflict) {
		t.Fatal("a single pinned connection would starve all value writers", err)
	}
}

func TestTextOwnerRLSCannotHidePersistedFence(t *testing.T) {
	db, values := textValueDB(t)
	_, _ = ownerReady(t, db, values)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// This disposable role can read the table but its RLS policy hides all rows.
	// It must fail closed, not reinterpret the hidden singleton as absent.
	if _, err := tx.Exec(`CREATE ROLE text_owner_test_restricted; GRANT USAGE ON SCHEMA public TO text_owner_test_restricted; GRANT ALL PRIVILEGES ON text_process_current TO text_owner_test_restricted; ALTER TABLE text_process_current ENABLE ROW LEVEL SECURITY; CREATE POLICY hidden_owner ON text_process_current USING(false); SET LOCAL ROLE text_owner_test_restricted`); err != nil {
		t.Fatal(err)
	}
	if err := values.ownerFence(context.Background(), tx, false); err == nil {
		t.Fatal("RLS-hidden singleton enabled the unbound fallback")
	}
}
