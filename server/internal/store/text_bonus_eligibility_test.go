package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
)

func TestTextBonusEligibilityCapturedAtStart(t *testing.T) {
	db, s := textValueDB(t)
	at := valueTime(time.Now().UTC())
	m, ids := valuePreparedMatch(t, s, db, at, false)
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',$2)`, ids[0], at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), m.Contract.MatchID, m.Owner, m.Epoch, at); err != nil {
		t.Fatal(err)
	}
	var eligible bool
	var started time.Time
	if err := db.QueryRow(`SELECT premium_bonus_eligible,started_at FROM text_bonus_eligibility WHERE match_id=$1 AND account_id=$2`, m.Contract.MatchID, ids[0]).Scan(&eligible, &started); err != nil {
		t.Fatal(err)
	}
	if !eligible || !started.Equal(at) {
		t.Fatalf("start evidence: eligible=%v at=%v", eligible, started)
	}
	if got := valueCount(t, db, `SELECT count(*) FROM text_bonus_eligibility WHERE match_id=$1`, m.Contract.MatchID); got != 4 {
		t.Fatalf("participant coverage=%d", got)
	}
}

func TestTextBonusEligibilitySixSeatsAndPlayPass(t *testing.T) {
	db, s := textValueDB(t)
	at := valueTime(time.Now().UTC())
	m, ids := valueUnpreparedMatch(t, s, db, at, false)
	for len(ids) < 6 {
		id, admission := valueAccount(t, db), uuid.NewString()
		if err := s.Reserve(t.Context(), TextReservation{ID: admission, AccountID: id, EntryPath: "quick_play", At: at}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		m.AdmissionIDs = append(m.AdmissionIDs, admission)
	}
	m.Contract.OriginalSize = 6
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,active_until) VALUES($1,'play_pass_1d',$2)`, ids[5], at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.Prepare(t.Context(), m, at); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context(), m.Contract.MatchID, m.Owner, 1, at); err != nil {
		t.Fatal(err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_eligibility WHERE match_id=$1 AND NOT premium_bonus_eligible`, m.Contract.MatchID); n != 6 {
		t.Fatalf("six-seat non-Premium coverage=%d", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_admissions WHERE match_id=$1 AND account_id=$2 AND access_kind='pass'`, m.Contract.MatchID, ids[5]); n != 1 {
		t.Fatal("Play Pass quota access missing")
	}
}

func bonusPreparedMatch(t *testing.T, s *TextValueStore, db *sql.DB, at time.Time, local, prototype, rewards bool) (TextMatchRecord, []string) {
	t.Helper()
	m, ids := valueUnpreparedMatch(t, s, db, at, prototype)
	if local {
		for i, id := range ids {
			if err := s.CancelReservation(context.Background(), m.AdmissionIDs[i], id); err != nil {
				t.Fatal(err)
			}
			m.AdmissionIDs[i] = uuid.NewString()
			if err := s.Reserve(context.Background(), TextReservation{ID: m.AdmissionIDs[i], AccountID: id, EntryPath: "local", Prototype: prototype, At: at}); err != nil {
				t.Fatal(err)
			}
		}
		m.Contract.Eligibility.EntryPath = "local"
		m.Contract.Eligibility.Leaderboard = false
	}
	m.Contract.Eligibility.Rewards = rewards
	if !rewards {
		m.Contract.Eligibility.Leaderboard = false
	}
	if err := s.Prepare(context.Background(), m, at); err != nil {
		t.Fatal(err)
	}
	return m, ids
}

func TestTextBonusEligibilityLocalAndDisabled(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		local, prototype, rewards, want bool
	}{
		{"local", true, false, true, true},
		{"quick_play", false, false, true, true},
		{"prototype", true, true, false, false},
		{"reward_disabled", false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, s := textValueDB(t)
			at := valueTime(time.Now().UTC())
			m, ids := bonusPreparedMatch(t, s, db, at, tc.local, tc.prototype, tc.rewards)
			// A legacy perpetual yearly entitlement retains its existing semantics.
			if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type) VALUES($1,'premium_yearly')`, ids[0]); err != nil {
				t.Fatal(err)
			}
			if err := s.Start(context.Background(), m.Contract.MatchID, m.Owner, 1, at); err != nil {
				t.Fatal(err)
			}
			var got bool
			if err := db.QueryRow(`SELECT premium_bonus_eligible FROM text_bonus_eligibility WHERE match_id=$1 AND account_id=$2`, m.Contract.MatchID, ids[0]).Scan(&got); err != nil || got != tc.want {
				t.Fatalf("eligibility=%v want=%v err=%v", got, tc.want, err)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_eligibility WHERE match_id=$1 AND account_id<>$2 AND NOT premium_bonus_eligible`, m.Contract.MatchID, ids[0]); n != 3 {
				t.Fatalf("free participant coverage=%d", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger`); n != 0 {
				t.Fatalf("capture paid value: %d", n)
			}
		})
	}
}

func TestTextBonusEligibilityExpiryAndReplay(t *testing.T) {
	db, s := textValueDB(t)
	at := valueTime(time.Now().UTC())
	start := at.Add(time.Minute)
	m, ids := valuePreparedMatch(t, s, db, at, false)
	for i, expiry := range []time.Time{start.Add(time.Hour), start, at.Add(time.Second)} {
		if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',$2)`, ids[i], expiry); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Start(context.Background(), m.Contract.MatchID, m.Owner, 1, start); err != nil {
		t.Fatal(err)
	}
	before := bonusEvidence(t, db, m.Contract.MatchID)
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_eligibility WHERE match_id=$1 AND premium_bonus_eligible`, m.Contract.MatchID); n != 1 {
		t.Fatalf("strict start expiry=%d", n)
	}
	// Subscription removal/purchase after start cannot rewrite the pinned decision.
	if _, err := db.Exec(`DELETE FROM entitlements WHERE account_id=$1;`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type) VALUES($1,'premium_monthly')`, ids[3]); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), m.Contract.MatchID, m.Owner, 1, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := bonusEvidence(t, db, m.Contract.MatchID); got != before {
		t.Fatal("retry recaptured changed subscriptions")
	}
}

func bonusEvidence(t *testing.T, db *sql.DB, match string) string {
	t.Helper()
	var raw string
	if err := db.QueryRow(`SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY account_id),'[]'::jsonb)::text FROM text_bonus_eligibility e WHERE match_id=$1`, match).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestTextBonusEligibilityStartRollback(t *testing.T) {
	for _, stage := range []string{"eligibility", "match"} {
		t.Run(stage, func(t *testing.T) {
			db, s := textValueDB(t)
			at := valueTime(time.Now().UTC())
			m, _ := valuePreparedMatch(t, s, db, at, false)
			injection := `CREATE FUNCTION fail_bonus_start() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_bonus_eligibility) THEN RAISE EXCEPTION 'injected eligibility failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER fail_bonus_start BEFORE INSERT ON text_bonus_eligibility FOR EACH ROW EXECUTE FUNCTION fail_bonus_start();`
			if stage == "match" {
				injection = `CREATE FUNCTION fail_bonus_start() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.state='started' THEN RAISE EXCEPTION 'injected final start failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER fail_bonus_start BEFORE UPDATE ON text_matches FOR EACH ROW EXECUTE FUNCTION fail_bonus_start();`
			}
			if _, err := db.Exec(injection); err != nil {
				t.Fatal(err)
			}
			if err := s.Start(context.Background(), m.Contract.MatchID, m.Owner, 1, at); err == nil {
				t.Fatal("injected start succeeded")
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_eligibility`); n != 0 {
				t.Fatalf("partial eligibility=%d", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_admissions WHERE match_id=$1 AND state='reserved'`, m.Contract.MatchID); n != 4 {
				t.Fatalf("partial admissions=%d", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_matches WHERE id=$1 AND state='prepared' AND started_at IS NULL`, m.Contract.MatchID); n != 1 {
				t.Fatal("partial match start")
			}
			if n := valueCount(t, db, `SELECT count(*) FROM daily_quickplay_counts WHERE count<>0`); n != 0 {
				t.Fatal("partial quota")
			}
		})
	}
}

func TestTextBonusEligibilityMigrationRetention(t *testing.T) {
	db, s := textValueDB(t)
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000031_text_bonus_eligibility.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	// Later migrations may reference eligibility. Roll them back in dependency
	// order before testing the exact empty 31 -> 30 -> 31 transition.
	if err := runMigration(db, "../../migrations", "bonus eligibility rollback", func(m *migrate.Migrate) error { return m.Migrate(30) }); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(db, transitionMigrationPath(t, 31)); err != nil {
		t.Fatal(err)
	}
	m, _ := valueMatch(t, s, db, valueTime(time.Now().UTC()), false)
	before := bonusEvidence(t, db, m.Contract.MatchID)
	for _, statement := range []string{
		`UPDATE text_bonus_eligibility SET premium_bonus_eligible=NOT premium_bonus_eligible`,
		`DELETE FROM text_bonus_eligibility`,
		`TRUNCATE text_bonus_eligibility`,
		`TRUNCATE text_admissions CASCADE`,
		string(down),
	} {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(statement); err == nil {
			t.Error("retained eligibility changed")
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		if got := bonusEvidence(t, db, m.Contract.MatchID); got != before {
			t.Fatal("retained bytes changed")
		}
	}
}

func TestTextBonusEligibilityHistoricalAbsenceStaysUnknown(t *testing.T) {
	db, s := textValueDB(t)
	// Build genuine pre-31 facts, with the migration version tracking the schema.
	if err := runMigration(db, "../../migrations", "pre-bonus historical fixture", func(m *migrate.Migrate) error { return m.Migrate(30) }); err != nil {
		t.Fatal(err)
	}

	at := valueTime(time.Now().UTC())
	m, ids := valuePreparedMatch(t, s, db, at, false)
	// Fixture of retained pre-31 start facts. The old schema has no eligibility
	// evidence, even if today's account owns Premium. No historical guess is safe.
	if _, err := db.Exec(`UPDATE text_matches SET state='started',started_at=$2 WHERE id=$1`, m.Contract.MatchID, at); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE text_admissions SET state='started' WHERE match_id=$1`, m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type) VALUES($1,'premium_yearly')`, ids[0]); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := db.QueryRow(`SELECT to_jsonb(m)::text FROM text_matches m WHERE id=$1`, m.Contract.MatchID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(db, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(db, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), m.Contract.MatchID, m.Owner, 1, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_eligibility`); n != 0 {
		t.Fatal("fabricated historical eligibility")
	}
	var after string
	if err := db.QueryRow(`SELECT to_jsonb(m)::text FROM text_matches m WHERE id=$1`, m.Contract.MatchID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatal("historical match changed")
	}
}

func TestTextBonusEligibilitySubscriptionSerialization(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		for _, startFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("revoke=%v/start_first=%v", revoke, startFirst), func(t *testing.T) {
				db, s := textValueDB(t)
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				at := valueTime(time.Now().UTC())
				m, ids := valuePreparedMatch(t, s, db, at, false)
				if revoke {
					if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type) VALUES($1,'premium_monthly')`, ids[0]); err != nil {
						t.Fatal(err)
					}
				}
				mutate := func(tx *sql.Tx) error {
					var err error
					if revoke {
						_, err = tx.ExecContext(ctx, `DELETE FROM entitlements WHERE account_id=$1 AND entitlement_type='premium_monthly'`, ids[0])
					} else {
						_, err = tx.ExecContext(ctx, `INSERT INTO entitlements(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',$2)`, ids[0], at.Add(time.Hour))
					}
					return err
				}
				startDone := make(chan error, 1)
				if startFirst {
					entered, release := make(chan struct{}), make(chan struct{})
					defer func() {
						select {
						case <-release:
						default:
							close(release)
						}
					}()
					s = s.WithStartGuard(func(ctx context.Context, _ *sql.Tx, _ TextMatchRecord, _ []string, _ time.Time) error {
						close(entered)
						select {
						case <-release:
							return nil
						case <-ctx.Done():
							return ctx.Err()
						}
					})
					go func() { startDone <- s.Start(ctx, m.Contract.MatchID, m.Owner, 1, at) }()
					select {
					case <-entered:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					changeDone := make(chan error, 1)
					go func() {
						changeDone <- WithValueTransaction(ctx, db, func(tx *sql.Tx) error {
							if err := LockValueAccount(ctx, tx, ids[0]); err != nil {
								return err
							}
							return mutate(tx)
						})
					}()
					waitBonusAccountLock(t, ctx, db)
					close(release)
					if err := <-startDone; err != nil {
						t.Fatal(err)
					}
					if err := <-changeDone; err != nil {
						t.Fatal(err)
					}
				} else {
					tx, err := db.BeginTx(ctx, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Rollback()
					if err := LockValueAccount(ctx, tx, ids[0]); err != nil {
						t.Fatal(err)
					}
					if err := mutate(tx); err != nil {
						t.Fatal(err)
					}
					go func() { startDone <- s.Start(ctx, m.Contract.MatchID, m.Owner, 1, at) }()
					waitBonusAccountLock(t, ctx, db)
					if err := tx.Commit(); err != nil {
						t.Fatal(err)
					}
					if err := <-startDone; err != nil {
						t.Fatal(err)
					}
				}
				var got bool
				if err := db.QueryRow(`SELECT premium_bonus_eligible FROM text_bonus_eligibility WHERE match_id=$1 AND account_id=$2`, m.Contract.MatchID, ids[0]).Scan(&got); err != nil {
					t.Fatal(err)
				}
				want := revoke
				if !startFirst {
					want = !revoke
				}
				if got != want {
					t.Fatalf("serialization got=%v want=%v", got, want)
				}
			})
		}
	}
}

func waitBonusAccountLock(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND query LIKE 'SELECT id FROM accounts WHERE id=$1%')`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
