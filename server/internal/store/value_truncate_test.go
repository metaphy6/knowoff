package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

var retainedValueTables = []string{"noin_ledger", "text_award_receipts", "text_first_win_claims", "text_settlements", "text_outbox", "leaderboard_history"}

func retainedValueFixture(t *testing.T) (*sql.DB, *TextValueStore) {
	t.Helper()
	db, s := textValueDB(t)
	at := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	m, accounts := valueMatch(t, s, db, at, false)
	out := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: at.Add(time.Minute)}
	for seat, id := range accounts {
		role := "nower"
		if seat == 3 {
			role = "donower"
		}
		out.Players = append(out.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 20})
	}
	if err := s.Finish(t.Context(), out); err != nil {
		t.Fatal(err)
	}
	if err := s.SettlePending(t.Context(), out.MatchID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseLeaderboardWeek(t.Context(), "2026-08-17", at.AddDate(0, 0, 7)); err != nil {
		t.Fatal(err)
	}
	for _, table := range retainedValueTables {
		if valueCount(t, db, "SELECT count(*) FROM "+pq.QuoteIdentifier(table)) == 0 {
			t.Fatalf("empty fixture: %s", table)
		}
	}
	return db, s
}

func TestRetainedValueRefusesDirectAndCascadeTruncate(t *testing.T) {
	db, s := retainedValueFixture(t)
	for _, table := range append(append([]string(nil), retainedValueTables...), "accounts") {
		t.Run(table, func(t *testing.T) {
			before := valueCount(t, db, "SELECT count(*) FROM noin_ledger")
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			_, mutation := tx.Exec("TRUNCATE " + pq.QuoteIdentifier(table) + " RESTART IDENTITY CASCADE")
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if mutation == nil {
				t.Errorf("retained %s was truncatable", table)
			}
			if valueCount(t, db, "SELECT count(*) FROM noin_ledger") != before {
				t.Fatal("refusal lost ledger rows")
			}
		})
	}
	report, err := s.Reconcile(t.Context())
	if err != nil || report.Pending != 0 || report.MissingEffects != 0 || report.LedgerMismatches != 0 || report.WalletMismatches != 0 {
		t.Fatalf("reconciliation changed: %+v %v", report, err)
	}
}

func TestRetainedValueNonOwnerAndHiddenRowsCannotBypassGuard(t *testing.T) {
	db, _ := retainedValueFixture(t)
	db.SetMaxOpenConns(1)
	role := pq.QuoteIdentifier("value_guard_" + strings.ReplaceAll(uuid.NewString(), "-", ""))
	if _, err := db.Exec("CREATE ROLE " + role + " NOLOGIN NOSUPERUSER NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []string{"RESET ROLE", "ALTER TABLE noin_ledger DISABLE ROW LEVEL SECURITY", "DROP OWNED BY " + role, "DROP ROLE " + role} {
			if _, err := db.Exec(q); err != nil {
				t.Error(err)
			}
		}
	})
	// Deliberately overgrant in this isolated fixture to exercise trigger defense
	// even against a mistaken grant. Runtime role provisioning is a separate gate.
	for _, q := range []string{"GRANT USAGE ON SCHEMA public TO " + role, "GRANT SELECT,INSERT,UPDATE,DELETE,TRUNCATE ON ALL TABLES IN SCHEMA public TO " + role, "GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO " + role} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	execAsRole := func(q string) error {
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.Exec("SET LOCAL ROLE " + role); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(q)
		return err
	}
	if err := execAsRole(`INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) SELECT account_id,'guard_test',1,'disposable fixture',CURRENT_DATE FROM noin_ledger LIMIT 1`); err != nil {
		t.Fatal("allowed append failed", err)
	}
	for _, q := range []string{"UPDATE noin_ledger SET amount=amount+1", "DELETE FROM noin_ledger", "TRUNCATE noin_ledger CASCADE"} {
		err := execAsRole(q)
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "P0001" {
			t.Fatalf("expected trigger refusal for %s: %v", q, err)
		}
	}
	if _, err := db.Exec("ALTER TABLE noin_ledger ENABLE ROW LEVEL SECURITY"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("SET LOCAL ROLE " + role); err != nil {
		t.Fatal(err)
	}
	var visible int
	if err := tx.QueryRow("SELECT count(*) FROM noin_ledger").Scan(&visible); err != nil || visible != 0 {
		t.Fatal("hidden-row fixture failed", visible, err)
	}
	_, err = tx.Exec("TRUNCATE noin_ledger CASCADE")
	var pg *pq.Error
	if !errors.As(err, &pg) || pg.Code != "42501" || !strings.Contains(pg.Message, "row-level security") {
		t.Fatal("row policy hid retained history from guard", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if valueCount(t, db, "SELECT count(*) FROM noin_ledger") == 0 {
		t.Fatal("hidden ledger erased")
	}
}

func TestRetainedValueAppendAndTruncateSerialize(t *testing.T) {
	for _, appendFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "append_first", false: "empty_truncate_first"}[appendFirst], func(t *testing.T) {
			db, _ := textValueDB(t)
			account := valueAccount(t, db)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			insert := `INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,'guard_test',1,'disposable fixture',CURRENT_DATE)`
			truncate := "TRUNCATE noin_ledger CASCADE"
			if appendFirst {
				_, err = tx.ExecContext(ctx, insert, account)
			} else {
				_, err = tx.ExecContext(ctx, truncate)
			}
			if err != nil {
				t.Fatal(err)
			}
			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			var pid int
			if err := conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				var err error
				if appendFirst {
					_, err = conn.ExecContext(ctx, truncate)
				} else {
					_, err = conn.ExecContext(ctx, insert, account)
				}
				result <- err
			}()
			for {
				var waiting bool
				if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event_type='Lock')", pid).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case err := <-result:
					t.Fatal("operation did not wait for conflicting table lock", err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(time.Millisecond):
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			err = <-result
			if appendFirst {
				var pg *pq.Error
				if !errors.As(err, &pg) || pg.Code != "P0001" {
					t.Fatal("committed append was not protected", err)
				}
			} else if err != nil {
				t.Fatal("append after empty truncate failed", err)
			}
			if valueCount(t, db, "SELECT count(*) FROM noin_ledger") != 1 {
				t.Fatal("serialized append lost or duplicated")
			}
		})
	}
}

func TestRetainedValueMigration24To25ParityAndExactDown(t *testing.T) {
	for _, seed := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty_down", true: "retained_refuses_down"}[seed], func(t *testing.T) {
			db := transitionDesignDB(t, 24, seed)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			before := transitionLegacySnapshot(t, ctx, db)
			path := transitionMigrationPath(t, 25)
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			after := transitionLegacySnapshot(t, ctx, db)
			if after.Version == nil || *after.Version != 25 || after.Dirty {
				t.Fatal("migration did not reach clean25")
			}
			for _, old := range before.Tables {
				if old.Name == "schema_migrations" {
					continue
				}
				found := false
				for _, current := range after.Tables {
					if current.Name == old.Name {
						found = true
						if !reflect.DeepEqual(old, current) {
							t.Fatalf("retained table changed: %s", old.Name)
						}
					}
				}
				if !found {
					t.Fatalf("table lost: %s", old.Name)
				}
			}
			for name, hash := range before.MigrationFiles {
				if after.MigrationFiles[name] != hash {
					t.Fatalf("applied migration changed: %s", name)
				}
			}
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, after, transitionLegacySnapshot(t, ctx, db))
			err := runMigration(db, path, "exact retained value25 down", func(m *migrate.Migrate) error { return m.Steps(-1) })
			if seed {
				if err == nil {
					t.Fatal("populated25 down accepted")
				}
				if valueCount(t, db, "SELECT count(*) FROM noin_ledger") == 0 {
					t.Fatal("refused down lost ledger")
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
