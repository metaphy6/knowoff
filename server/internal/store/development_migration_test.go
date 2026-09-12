package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
)

func TestDevelopmentIdentityActualMigrationParityAndDown(t *testing.T) {
	for _, development := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_players", true: "retained_development"}[development], func(t *testing.T) {
			db := transitionDesignDB(t, 13, true)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			before := transitionLegacySnapshot(t, ctx, db)
			accountRows := func() string {
				var body string
				if err := db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(a)-'auth_purpose' ORDER BY id),'[]'::jsonb)::text FROM accounts a`).Scan(&body); err != nil {
					t.Fatal(err)
				}
				return body
			}
			originalAccounts := accountRows()
			path := transitionMigrationPath(t, 14)
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			after := transitionLegacySnapshot(t, ctx, db)
			for _, old := range before.Tables {
				if old.Name == "accounts" || old.Name == "schema_migrations" {
					continue
				}
				found := false
				for _, current := range after.Tables {
					if current.Name == old.Name {
						found = true
						if !reflect.DeepEqual(old, current) {
							t.Fatal("retained table changed", old.Name)
						}
					}
				}
				if !found {
					t.Fatal("retained table missing", old.Name)
				}
			}
			if accountRows() != originalAccounts {
				t.Fatal("existing account data changed")
			}
			if n := valueCount(t, db, `SELECT count(*) FROM accounts WHERE auth_purpose<>'player'`); n != 0 {
				t.Fatal("legacy account purpose fabricated", n)
			}
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, after, transitionLegacySnapshot(t, ctx, db))
			id := uuid.NewString()
			if development {
				if _, err := db.ExecContext(ctx, `INSERT INTO accounts(id,nickname,auth_purpose) VALUES($1,'Test retained','development')`, id); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, `UPDATE accounts SET auth_purpose='player' WHERE id=$1`, id); err == nil {
					t.Fatal("purpose conversion allowed")
				}
			}
			err := runMigration(db, path, "exact development identity down", func(m *migrate.Migrate) error { return m.Steps(-1) })
			if development {
				if err == nil {
					t.Fatal("lossy development rollback accepted")
				}
				if n := valueCount(t, db, `SELECT count(*) FROM accounts WHERE id=$1 AND auth_purpose='development'`, id); n != 1 {
					t.Fatal("development identity lost", n)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
				if n := valueCount(t, db, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='accounts' AND column_name='auth_purpose'`); n != 0 {
					t.Fatal("purpose column survived exact down")
				}
			}
		})
	}
}
