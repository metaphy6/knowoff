package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
)

// Store-only delivery proofs bind an explicit test authority; the Admin package
// independently tests the real initiating browser session, CSRF and expiration.
func leaderboardTestActor(t *testing.T, db *sql.DB) (string, context.Context) {
	t.Helper()
	actor, account := uuid.NewString(), valueAccount(t, db)
	if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, account); err != nil {
		t.Fatal(err)
	}
	return actor, WithAdminAuthorization(t.Context(), actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
}
func leaderboardTestDecision(t *testing.T, db *sql.DB) LeaderboardAdminReceipt {
	t.Helper()
	actor, ctx := leaderboardTestActor(t, db)
	target := valueAccount(t, db)
	if _, err := db.Exec(`INSERT INTO leaderboard_weeks(week_id,start_at,end_at) VALUES('2026-08-03','2026-08-03','2026-08-10')`); err != nil {
		t.Fatal(err)
	}
	c := LeaderboardAdminCommand{ID: uuid.NewString(), Kind: "exclude", WeekID: "2026-08-03", TargetAccountID: target, Reason: "Synthetic eligibility review"}
	r, err := NewLeaderboardAdminStore(db).Decide(ctx, actor, c)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLeaderboardAdminRetainedSQLProtection(t *testing.T) {
	db, _ := textValueDB(t)
	leaderboardTestDecision(t, db)
	before := consentSnapshot(t, db)
	for _, q := range []string{`UPDATE leaderboard_admin_decisions SET reason='rewrite'`, `DELETE FROM leaderboard_admin_decisions`, `UPDATE leaderboard_admin_results SET result='{}'`, `DELETE FROM leaderboard_admin_results`, `TRUNCATE leaderboard_admin_results`, `TRUNCATE leaderboard_admin_decisions CASCADE`, `TRUNCATE leaderboard_weeks CASCADE`} {
		requireConsentRefusal(t, db, "", q, "P0001")
		assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
	}
	if _, err := db.Exec(`CREATE ROLE leaderboard_restricted NOLOGIN; GRANT USAGE ON SCHEMA public TO leaderboard_restricted; GRANT SELECT,INSERT,UPDATE,DELETE,TRUNCATE ON ALL TABLES IN SCHEMA public TO leaderboard_restricted`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"leaderboard_admin_decisions", "leaderboard_admin_results"} {
		requireConsentRefusal(t, db, "leaderboard_restricted", "UPDATE "+table+" SET "+map[string]string{"leaderboard_admin_decisions": "reason=reason", "leaderboard_admin_results": "result=result"}[table], "P0001")
		requireConsentRefusal(t, db, "leaderboard_restricted", "TRUNCATE "+table+" CASCADE", "P0001")
		if _, err := db.Exec("ALTER TABLE " + table + " ENABLE ROW LEVEL SECURITY; CREATE POLICY hidden ON " + table + " FOR SELECT USING(false)"); err != nil {
			t.Fatal(err)
		}
		requireConsentRefusal(t, db, "leaderboard_restricted", "TRUNCATE "+table+" CASCADE", "42501")
	}
	assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
}

func TestLeaderboardAdminMigration29To30ParityAndDown(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty_down", true: "retained_refusal"}[retained], func(t *testing.T) {
			db := transitionDesignDB(t, 29, true)
			before := consentSnapshot(t, db)
			path := transitionMigrationPath(t, 30)
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
				leaderboardTestDecision(t, db)
				before = consentSnapshot(t, db)
				down, err := os.ReadFile("../../migrations/000030_leaderboard_admin.down.sql")
				if err != nil {
					t.Fatal(err)
				}
				tx, err := db.BeginTx(t.Context(), nil)
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
				assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
			} else {
				if err := runMigration(db, path, "exact leaderboard30 down", func(m *migrate.Migrate) error { return m.Steps(-1) }); err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
			}
		})
	}
}

func TestLeaderboardAdminPendingMatchRestartAndOrdinaryCloseConverge(t *testing.T) {
	for _, worker := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary_close", true: "restart_worker"}[worker], func(t *testing.T) {
			db, s := textValueDB(t)
			at := time.Date(2026, 9, 6, 23, 59, 0, 0, time.UTC)
			m, ids := valueMatch(t, s, db, at, false)
			actor, ctx := leaderboardTestActor(t, db)
			ops := NewLeaderboardAdminStore(db)
			week := valueWeek(at).Format("2006-01-02")
			exclude := LeaderboardAdminCommand{ID: uuid.NewString(), Kind: "exclude", WeekID: week, TargetAccountID: ids[0], Reason: "Synthetic accepted work exclusion"}
			if _, err := ops.Decide(ctx, actor, exclude); err != nil {
				t.Fatal(err)
			}
			closeCommand := LeaderboardAdminCommand{ID: uuid.NewString(), Kind: "close", WeekID: week, Reason: "Synthetic pending close"}
			r, err := ops.Decide(ctx, actor, closeCommand)
			if err != nil || r.Status != "pending" {
				t.Fatal(r, err)
			}
			if _, err = s.CloseLeaderboardWeek(t.Context(), week, time.Now()); !errors.Is(err, ErrWeekPending) {
				t.Fatal("live work not pending", err)
			}
			if err = NewLeaderboardAdminStore(db).ResumePending(t.Context(), 1); err != nil {
				t.Fatal(err)
			}
			r, err = ops.Get(t.Context(), closeCommand.ID)
			if err != nil || r.Status != "pending" {
				t.Fatal("premature close receipt", r, err)
			}
			o := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: at.Add(30 * time.Second)}
			for seat, id := range ids {
				role := "nower"
				if seat == 0 {
					role = "donower"
				}
				o.Players = append(o.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 20})
			}
			if err = s.Finish(t.Context(), o); err != nil {
				t.Fatal(err)
			}
			if worker {
				err = NewLeaderboardAdminStore(db).ResumePending(t.Context(), 1)
			} else {
				_, err = s.CloseLeaderboardWeek(t.Context(), week, time.Now())
			}
			if err != nil {
				t.Fatal(err)
			}
			r, err = ops.Get(t.Context(), closeCommand.ID)
			if err != nil || r.Status != "closed" {
				t.Fatal(r, err)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM leaderboard_history WHERE week_id=$1`, week); n != 3 {
				t.Fatal("excluded history", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM leaderboard_history WHERE week_id=$1 AND account_id=$2`, week, ids[0]); n != 0 {
				t.Fatal("excluded included", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM leaderboard_entries WHERE week_id=$1 AND points=20`, week); n != 4 {
				t.Fatal("raw accrual lost", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE state='applied'`); n != 4 {
				t.Fatal("settlement lost", n)
			}
			before := consentSnapshot(t, db)
			if err = NewLeaderboardAdminStore(db).ResumePending(t.Context(), 1); err != nil {
				t.Fatal(err)
			}
			if _, err = s.CloseLeaderboardWeek(t.Context(), week, time.Now()); err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
		})
	}
}

func TestLeaderboardAdminCloseDrainIsBoundedAndContinues(t *testing.T) {
	db, s := textValueDB(t)
	at := time.Date(2026, 9, 6, 23, 59, 0, 0, time.UTC)
	for range 33 {
		m, ids := valueMatch(t, s, db, at, false)
		o := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: at.Add(30 * time.Second)}
		for seat, id := range ids {
			role := "nower"
			if seat == 0 {
				role = "donower"
			}
			o.Players = append(o.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 20})
		}
		if err := s.Finish(t.Context(), o); err != nil {
			t.Fatal(err)
		}
	}
	actor, ctx := leaderboardTestActor(t, db)
	week := valueWeek(at).Format("2006-01-02")
	ops := NewLeaderboardAdminStore(db)
	c := LeaderboardAdminCommand{ID: uuid.NewString(), Kind: "close", WeekID: week, Reason: "Synthetic bounded close"}
	if _, err := ops.Decide(ctx, actor, c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseLeaderboardWeek(t.Context(), week, time.Now()); !errors.Is(err, ErrWeekPending) {
		t.Fatal("unbounded drain", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE state='pending'`); n != 4 {
		t.Fatal("wrong bounded remainder", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM leaderboard_history`); n != 0 {
		t.Fatal("partial snapshot", n)
	}
	if err := ops.ResumePending(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM leaderboard_history`); n != 132 {
		t.Fatal("lost accumulated history", n)
	}
	r, err := ops.Get(t.Context(), c.ID)
	if err != nil || r.Status != "closed" {
		t.Fatal(r, err)
	}
}
