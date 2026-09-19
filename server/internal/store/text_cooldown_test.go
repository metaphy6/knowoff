package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/knowoff/knowoff/server/internal/config"
	"gopkg.in/yaml.v3"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func TestTextAbandonTruncateCannotHideReceiptsWithRLS(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	m, ids := valueMatch(t, s, db, at, false)
	if err := s.Abandon(ctx, TextAbandon{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Seat: 0, At: at.Add(20 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	role := pq.QuoteIdentifier("abandon_fixture_" + uuid.NewString())
	for _, q := range []string{"CREATE ROLE " + role, "GRANT USAGE ON SCHEMA public TO " + role, "GRANT SELECT,TRUNCATE ON text_abandons TO " + role, "ALTER TABLE text_abandons ENABLE ROW LEVEL SECURITY", "SET LOCAL ROLE " + role} {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	var visible int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM text_abandons`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("RLS fixture did not hide rows", visible, err)
	}
	if _, err = tx.ExecContext(ctx, `TRUNCATE text_abandons`); err == nil {
		t.Fatal("RLS-hidden retained incidents could be truncated")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_abandons`); n != 1 {
		t.Fatal("retained receipt lost", n)
	}
}

func TestTextAdmissionHonorsRetainedCooldown(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 12, 23, 59, 50, 0, time.UTC)
	account := valueAccount(t, db)
	until := at.Add(time.Minute)
	if _, err := db.Exec(`INSERT INTO queue_cooldowns(account_id,abandon_count,cooldown_until) VALUES($1,2,$2)`, account, until); err != nil {
		t.Fatal(err)
	}
	attempt := TextReservation{ID: uuid.NewString(), AccountID: account, EntryPath: "quick_play", At: at}
	if err := s.Reserve(ctx, attempt); err == nil {
		t.Fatal("active retained cooldown admitted Quick Play")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_admissions`); n != 0 {
		t.Fatal("refusal wrote admission", n)
	}
	attempt.At = until
	if err := s.Reserve(ctx, attempt); err != nil {
		t.Fatal("exact expiry must admit", err)
	}
	if err := s.CancelReservation(ctx, attempt.ID, account); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"local", "quick_play"} {
		attempt.ID = uuid.NewString()
		attempt.At = at
		attempt.EntryPath = path
		attempt.Prototype = path == "quick_play"
		if err := s.Reserve(ctx, attempt); err != nil {
			t.Fatal("exempt entry refused", path, err)
		}
		if err := s.CancelReservation(ctx, attempt.ID, account); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTextStartRechecksRetainedCooldown(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	at := valueTime(time.Now())
	m, accounts := valuePreparedMatch(t, s, db, at, false)
	until := at.Add(time.Minute)
	if _, err := db.Exec(`INSERT INTO queue_cooldowns(account_id,abandon_count,cooldown_until) VALUES($1,1,$2)`, accounts[0], until); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, at); err == nil {
		t.Fatal("cooldown arising after reservation did not block start")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM daily_quickplay_counts`); n != 0 {
		t.Fatal("denied start consumed quota", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_matches WHERE state='prepared'`); n != 1 {
		t.Fatal("denied start changed state", n)
	}
	if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, until); err != nil {
		t.Fatal("expiry start", err)
	}
	if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, until); err != nil {
		t.Fatal("start replay", err)
	}
	if n := valueCount(t, db, `SELECT sum(count) FROM daily_quickplay_counts`); n != 4 {
		t.Fatal("quota replay", n)
	}
}

func TestTextAbandonReceiptExactlyOncePinnedAndRetained(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 12, 23, 59, 55, 0, time.UTC)
	m, accounts := valueMatch(t, s, db, at, false)
	event := TextAbandon{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: accounts[0], Seat: 0, At: at.Add(20 * time.Second)}
	retained := at.Add(2 * time.Hour)
	if _, err := db.Exec(`INSERT INTO queue_cooldowns(account_id,abandon_count,cooldown_until) VALUES($1,1,$2)`, accounts[0], retained); err != nil {
		t.Fatal(err)
	}
	// Restarted configuration must not change an already pinned match's policy.
	changed := s.tuning.Clone()
	changed.Game.AbandonCooldownsS = []int{99999}
	s = NewTextValueStore(db, changed)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.Abandon(ctx, event) }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_abandons`); n != 1 {
		t.Fatal("duplicate incident receipts", n)
	}
	if n := valueCount(t, db, `SELECT abandon_count FROM queue_cooldowns WHERE account_id=$1`, accounts[0]); n != 2 {
		t.Fatal("count changed on retry", n)
	}
	var until, occurred time.Time
	var duration int
	if err := db.QueryRow(`SELECT cooldown_until,occurred_at,duration_seconds FROM text_abandons`).Scan(&until, &occurred, &duration); err != nil {
		t.Fatal(err)
	}
	if !until.Equal(retained) || !occurred.Equal(event.At) || duration != 300 {
		t.Fatal("retained expiry/occurrence/pinned escalation changed", until, occurred, duration)
	}
	changedEvent := event
	changedEvent.At = changedEvent.At.Add(time.Microsecond)
	if err := s.Abandon(ctx, changedEvent); !errors.Is(err, ErrValueConflict) {
		t.Fatal("changed receipt accepted", err)
	}
	changedEvent = event
	changedEvent.Seat = 1
	if err := s.Abandon(ctx, changedEvent); !errors.Is(err, ErrValueConflict) {
		t.Fatal("wrong admitted seat accepted", err)
	}
	if _, err := db.Exec(`UPDATE text_abandons SET duration_seconds=1`); err == nil {
		t.Fatal("receipt mutable")
	}
	if _, err := db.Exec(`DELETE FROM text_abandons`); err == nil {
		t.Fatal("receipt deletable")
	}
	if _, err := db.Exec(`TRUNCATE text_abandons`); err == nil {
		t.Fatal("receipt truncatable")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger`); n != 0 {
		t.Fatal("abandon mutated earned value", n)
	}
}

func TestTextAbandonFreshSaturatedAndAtomic(t *testing.T) {
	for _, prior := range []int{0, 2, 100} {
		t.Run(fmt.Sprint(prior), func(t *testing.T) {
			db, s := textValueDB(t)
			ctx := context.Background()
			at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
			m, ids := valueMatch(t, s, db, at, false)
			if prior > 0 {
				if _, err := db.Exec(`INSERT INTO queue_cooldowns(account_id,abandon_count) VALUES($1,$2)`, ids[0], prior); err != nil {
					t.Fatal(err)
				}
			}
			a := TextAbandon{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Seat: 0, At: at.Add(20 * time.Second)}
			if _, err := db.Exec(`CREATE FUNCTION reject_abandon_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture failure'; END $$; CREATE TRIGGER reject_abandon_fixture BEFORE INSERT ON text_abandons FOR EACH ROW EXECUTE FUNCTION reject_abandon_fixture()`); err != nil {
				t.Fatal(err)
			}
			if err := s.Abandon(ctx, a); err == nil {
				t.Fatal("injected receipt failure accepted")
			}
			if n := valueCount(t, db, `SELECT COALESCE((SELECT abandon_count FROM queue_cooldowns WHERE account_id=$1),0)`, ids[0]); n != int64(prior) {
				t.Fatal("receipt failure partially incremented", n)
			}
			if _, err := db.Exec(`DROP TRIGGER reject_abandon_fixture ON text_abandons`); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := s.Abandon(ctx, a); err != nil {
					t.Fatal(err)
				}
			}
			seconds := 60
			if prior >= 2 {
				seconds = 900
			}
			var until time.Time
			if err := db.QueryRow(`SELECT cooldown_until FROM queue_cooldowns WHERE account_id=$1`, ids[0]).Scan(&until); err != nil {
				t.Fatal(err)
			}
			if !until.Equal(a.At.Add(time.Duration(seconds) * time.Second)) {
				t.Fatal("wrong frozen cooldown", until)
			}
			if n := valueCount(t, db, `SELECT abandon_count FROM queue_cooldowns WHERE account_id=$1`, ids[0]); n != int64(prior+1) {
				t.Fatal("escalation count", n)
			}
		})
	}
}

func TestTextAbandonOwnerLossCannotInventOrRepeatPenalty(t *testing.T) {
	db, base := textValueDB(t)
	ctx := context.Background()
	owner, s := ownerReady(t, db, base)
	at := valueTime(time.Now())
	m, ids := ownerMatch(t, owner, s, db, at, true)
	a := TextAbandon{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Seat: 0, At: at.Add(20 * time.Second)}
	if err := s.Abandon(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := owner.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Abandon(ctx, a); err == nil {
		t.Fatal("lost owner wrote penalty")
	}
	_, next := ownerReady(t, db, base)
	if err := next.Abandon(ctx, a); !errors.Is(err, ErrValueFence) {
		t.Fatal("successor accepted old owner incident", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_abandons`); n != 1 {
		t.Fatal("recovery invented incidents", n)
	}
	if n := valueCount(t, db, `SELECT sum(abandon_count) FROM queue_cooldowns`); n != 1 {
		t.Fatal("recovery repeated penalty", n)
	}
}

func TestTextAbandonActualMigrationParityAndDown(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(fmt.Sprint(retained), func(t *testing.T) {
			db := transitionDesignDB(t, 23, true)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			before := transitionLegacySnapshot(t, ctx, db)
			path := transitionMigrationPath(t, 24)
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			after := transitionLegacySnapshot(t, ctx, db)
			for _, old := range before.Tables {
				if old.Name == "schema_migrations" {
					continue
				}
				found := false
				for _, current := range after.Tables {
					if old.Name == current.Name {
						found = true
						if !reflect.DeepEqual(old, current) {
							t.Fatal("migration changed retained table", old.Name)
						}
					}
				}
				if !found {
					t.Fatal("migration lost retained table", old.Name)
				}
			}
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, after, transitionLegacySnapshot(t, ctx, db))
			if retained {
				// This is a schema24 downgrade proof, so seed the retained
				// schema24 representation directly. Current gameplay requires
				// later sanction/bonus authority and cannot run on this prefix.
				// Runtime Abandon behavior is covered by the current-head tests.
				account, match := valueAccount(t, db), uuid.NewString()
				at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
				if _, err := db.ExecContext(ctx, `INSERT INTO text_matches(id,room_id,contract,contract_hash,owner_id,fence,state,prototype,created_at,started_at) VALUES($1,'migration24-fixture','{"historical_fixture":true}',$2,$3,1,'started',false,$4,$4)`, match, repeatHash(), uuid.NewString(), at); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, `INSERT INTO text_abandons(match_id,account_id,seat,occurred_at,body_hash,prior_count,applied_count,duration_seconds,cooldown_until) VALUES($1,$2,0,$3,$4,0,1,30,$3::timestamptz+interval '30 seconds')`, match, account, at, repeatHash()); err != nil {
					t.Fatal(err)
				}
			}
			var receiptBefore string
			if retained {
				if err := db.QueryRowContext(ctx, `SELECT to_jsonb(a)::text FROM text_abandons a`).Scan(&receiptBefore); err != nil {
					t.Fatal(err)
				}
			}

			err := runMigration(db, path, "exact text abandonment down", func(m *migrate.Migrate) error { return m.Steps(-1) })
			if retained {
				if err == nil {
					t.Fatal("lossy receipt rollback accepted")
				}
				if n := valueCount(t, db, `SELECT count(*) FROM text_abandons`); n != 1 {
					t.Fatal("receipt lost", n)
				}
				var receiptAfter string
				if err := db.QueryRowContext(ctx, `SELECT to_jsonb(a)::text FROM text_abandons a`).Scan(&receiptAfter); err != nil || receiptBefore != receiptAfter {
					t.Fatal("retained receipt changed", err)
				}

			} else {
				if err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
			}
		})
	}
}

func TestTextAbandonRejectsUngenuineAndFencedIncidents(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	m, ids := valuePreparedMatch(t, s, db, at, false)
	a := TextAbandon{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Seat: 0, At: at.Add(20 * time.Second)}
	if err := s.Abandon(ctx, a); !errors.Is(err, ErrValueFence) {
		t.Fatal("prepared incident accepted", err)
	}
	if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, at); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*TextAbandon){func(a *TextAbandon) { a.At = at.Add(-time.Second) }, func(a *TextAbandon) { a.AccountID = uuid.NewString() }, func(a *TextAbandon) { a.Seat = 4 }, func(a *TextAbandon) { a.Owner = uuid.NewString() }, func(a *TextAbandon) { a.Epoch = 2 }} {
		invalid := a
		mutate(&invalid)
		if err := s.Abandon(ctx, invalid); err == nil {
			t.Fatal("invalid incident accepted", invalid)
		}
	}
	if err := s.Interrupt(ctx, m.Contract.MatchID, m.Owner, 1, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.Abandon(ctx, a); !errors.Is(err, ErrValueFence) {
		t.Fatal("interrupted incident invented", err)
	}
	prototype, pids := valueMatch(t, s, db, at, true)
	a = TextAbandon{MatchID: prototype.Contract.MatchID, Owner: prototype.Owner, Epoch: 1, AccountID: pids[0], Seat: 0, At: at.Add(20 * time.Second)}
	if err := s.Abandon(ctx, a); err != nil {
		t.Fatal(err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_abandons`); n != 0 {
		t.Fatal("exempt/invented receipt", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM queue_cooldowns`); n != 0 {
		t.Fatal("exempt/invented penalty", n)
	}
}

func cooldownTestPolicy(t *testing.T) config.TuningConfig {
	t.Helper()
	data, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var policy config.TuningConfig
	if err = yaml.Unmarshal(data, &policy); err != nil {
		t.Fatal(err)
	}
	return policy
}
