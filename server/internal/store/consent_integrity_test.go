package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

var consentTables = []string{"portal_terms", "user_terms_versions", "user_terms_acceptances"}

func consentSnapshot(t *testing.T, db *sql.DB) transitionLegacyState {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	return transitionLegacySnapshot(t, ctx, db)
}

func seedConsent(t *testing.T, db *sql.DB) string {
	t.Helper()
	account := valueAccount(t, db)
	for _, q := range []string{
		`INSERT INTO portal_terms(version,title,body,active_from) VALUES('consent-v1','Synthetic title','Original synthetic contribution terms','2026-08-01')`,
		`INSERT INTO portal_submissions(account_id,media_type,content,terms_version,terms_accepted_at) VALUES($1,'text','Synthetic submission','consent-v1','2026-08-02')`,
		`INSERT INTO user_terms_versions(version,body,active_from) VALUES('consent-v1','Original synthetic user terms','2026-08-01')`,
		`INSERT INTO user_terms_acceptances(account_id,version,accepted_at) VALUES($1,'consent-v1','2026-08-02')`,
	} {
		var err error
		if strings.Contains(q, "$1") {
			_, err = db.Exec(q, account)
		} else {
			_, err = db.Exec(q)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return account
}

func requireConsentRefusal(t *testing.T, db *sql.DB, role, statement, code string) {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if role != "" {
		if _, err = tx.Exec("SET LOCAL ROLE " + pq.QuoteIdentifier(role)); err != nil {
			t.Fatal(err)
		}
	}
	// A rejected mutation must abort unrelated effects from the same transaction.
	if _, err = tx.Exec(`INSERT INTO user_terms_versions(version,body,active_from) VALUES('rolled-back','Synthetic rollback marker',now())`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(statement)
	var pg *pq.Error
	if !errors.As(err, &pg) || string(pg.Code) != code {
		t.Errorf("%s: expected SQLSTATE %s, got %v", statement, code, err)
		return
	}
	if err = tx.Commit(); err == nil {
		t.Error("refused mutation committed its transaction")
	}
}

func TestConsentRefusesSQLRewriteAndCascadeTruncate(t *testing.T) {
	db, _ := textValueDB(t)
	seedConsent(t, db)
	// An unreferenced version is still immutable; a FK refusal alone is insufficient.
	if _, err := db.Exec(`INSERT INTO portal_terms(version,title,body) VALUES('unused','Synthetic','Original')`); err != nil {
		t.Fatal(err)
	}
	before := consentSnapshot(t, db)
	statements := []string{
		`UPDATE portal_terms SET body='Rewritten' WHERE version='consent-v1'`,
		`UPDATE portal_terms SET title='Rewritten'`,
		`UPDATE portal_terms SET active_from=active_from+interval '1 day'`,
		`UPDATE portal_terms SET created_at=created_at+interval '1 day'`,
		`DELETE FROM portal_terms WHERE version='unused'`,
		`DELETE FROM portal_terms WHERE version='consent-v1'`,
		`UPDATE user_terms_versions SET body='Rewritten'`,
		`DELETE FROM user_terms_versions`,
		`UPDATE user_terms_acceptances SET accepted_at=accepted_at+interval '1 day'`,
		`DELETE FROM user_terms_acceptances`,
		`TRUNCATE portal_terms RESTART IDENTITY CASCADE`,
		`TRUNCATE user_terms_versions CASCADE`,
		`TRUNCATE user_terms_acceptances`,
		`TRUNCATE accounts CASCADE`,
	}
	for _, q := range statements {
		t.Run(q, func(t *testing.T) {
			requireConsentRefusal(t, db, "", q, "P0001")
			assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
		})
	}
}

func TestConsentNonOwnerAndHiddenRowsCannotBypassTruncate(t *testing.T) {
	db, _ := textValueDB(t)
	seedConsent(t, db)
	role := "consent_guard_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pq.QuoteIdentifier(role)
	if _, err := db.Exec("CREATE ROLE " + quoted + " NOLOGIN NOSUPERUSER NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, table := range consentTables {
			if _, err := db.Exec("ALTER TABLE " + table + " DISABLE ROW LEVEL SECURITY"); err != nil {
				t.Error(err)
			}
		}
		for _, q := range []string{"DROP OWNED BY " + quoted, "DROP ROLE " + quoted} {
			if _, err := db.Exec(q); err != nil {
				t.Error(err)
			}
		}
	})
	// Deliberately overgrant on the disposable database to prove trigger defense.
	for _, q := range []string{"GRANT USAGE ON SCHEMA public TO " + quoted, "GRANT SELECT,INSERT,UPDATE,DELETE,TRUNCATE ON ALL TABLES IN SCHEMA public TO " + quoted, "GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO " + quoted} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	before := consentSnapshot(t, db)
	for _, table := range consentTables {
		for _, q := range []string{"UPDATE " + table + " SET " + map[string]string{"portal_terms": "body=body", "user_terms_versions": "body=body", "user_terms_acceptances": "accepted_at=accepted_at"}[table], "DELETE FROM " + table, "TRUNCATE " + table + " CASCADE"} {
			requireConsentRefusal(t, db, role, q, "P0001")
		}
		if _, err := db.Exec("ALTER TABLE " + table + " ENABLE ROW LEVEL SECURITY"); err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err = tx.Exec("SET LOCAL ROLE " + quoted); err != nil {
			t.Fatal(err)
		}
		var visible int
		if err = tx.QueryRow("SELECT count(*) FROM " + table).Scan(&visible); err != nil || visible != 0 {
			t.Fatal("RLS fixture not hidden", visible, err)
		}
		_, err = tx.Exec("TRUNCATE " + table + " CASCADE")
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "42501" || !strings.Contains(pg.Message, "row-level security") {
			t.Error("hidden retained consent erased", table, err)
		}
		if err = tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("ALTER TABLE " + table + " DISABLE ROW LEVEL SECURITY"); err != nil {
			t.Fatal(err)
		}
	}
	assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
}

func TestConsentAppendAndAcceptanceReplayPreserveOriginal(t *testing.T) {
	db, _ := textValueDB(t)
	account := seedConsent(t, db)
	trust := NewTextTrustStore(db)
	before := consentSnapshot(t, db)
	if err := trust.AcceptTerms(t.Context(), account, "consent-v1", time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
	for _, q := range []string{
		`INSERT INTO portal_terms(version,title,body) VALUES('consent-v2','New synthetic title','New synthetic contribution terms')`,
		`INSERT INTO user_terms_versions(version,body,active_from) VALUES('consent-v2','New synthetic user terms','2026-08-01')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal("new terms blocked", err)
		}
	}
	if err := trust.RequireTerms(t.Context(), account, "consent-v2"); !errors.Is(err, ErrTextTerms) {
		t.Fatal("acceptance transferred", err)
	}
	at := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	for range 2 {
		if err := trust.AcceptTerms(t.Context(), account, "consent-v2", at); err != nil {
			t.Fatal(err)
		}
	}
	var accepted time.Time
	if err := db.QueryRow(`SELECT accepted_at FROM user_terms_acceptances WHERE account_id=$1 AND version='consent-v1'`, account).Scan(&accepted); err != nil || !accepted.Equal(time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("original consent timestamp changed", accepted, err)
	}
	if valueCount(t, db, `SELECT count(*) FROM user_terms_acceptances`) != 2 || valueCount(t, db, `SELECT count(*) FROM noin_ledger`) != 0 {
		t.Fatal("acceptance duplicated or paid")
	}
	var body string
	if err := db.QueryRow(`SELECT t.body FROM portal_submissions s JOIN portal_terms t ON t.version=s.terms_version WHERE s.account_id=$1`, account).Scan(&body); err != nil || body != "Original synthetic contribution terms" {
		t.Fatal("submission consent reference changed", body, err)
	}
}

func TestConsentMigration27To28ParityAndExactDown(t *testing.T) {
	for _, seed := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty_down", true: "retained_refuses_down"}[seed], func(t *testing.T) {
			db := transitionDesignDB(t, 27, seed)
			if seed {
				seedConsent(t, db)
			}
			before := consentSnapshot(t, db)
			path := transitionMigrationPath(t, 28)
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			after := consentSnapshot(t, db)
			if after.Version == nil || *after.Version != 28 || after.Dirty {
				t.Fatal("migration did not reach clean28")
			}
			for _, old := range before.Tables {
				if old.Name == "schema_migrations" {
					continue
				}
				found := false
				for _, current := range after.Tables {
					if old.Name == current.Name {
						found = true
						if !reflect.DeepEqual(old, current) {
							t.Fatal("original rows or references changed", old.Name)
						}
					}
				}
				if !found {
					t.Fatal("original table lost", old.Name)
				}
			}
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, after, consentSnapshot(t, db))
			if seed {
				down, err := os.ReadFile("../../migrations/000028_immutable_consent.down.sql")
				if err != nil {
					t.Fatal(err)
				}
				tx, err := db.BeginTx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				_, err = tx.Exec(string(down))
				var pg *pq.Error
				if !errors.As(err, &pg) || pg.Code != "P0001" || !strings.Contains(pg.Message, "retained consent") {
					t.Error("populated consent down did not refuse retained data", err)
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, after, consentSnapshot(t, db))
				requireConsentRefusal(t, db, "", `UPDATE portal_terms SET body=body`, "P0001")
			} else {
				if err := runMigration(db, path, "exact consent28 down", func(m *migrate.Migrate) error { return m.Steps(-1) }); err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, before, consentSnapshot(t, db))
				if err := MigrateUp(db, path); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`TRUNCATE portal_terms,user_terms_versions,user_terms_acceptances CASCADE`); err != nil {
					t.Fatal("empty truncate refused", err)
				}
			}
		})
	}
}
