package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
)

func TestSessionEpochActualMigrationParityAndRefusedLossyDown(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy_zero", true: "revoked_epoch"}[revoked], func(t *testing.T) {
			db := transitionDesignDB(t, 16, true)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			before := transitionLegacySnapshot(t, ctx, db)
			accounts := func() string {
				var body string
				if err := db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(a)-'session_epoch' ORDER BY id),'[]'::jsonb)::text FROM accounts a`).Scan(&body); err != nil {
					t.Fatal(err)
				}
				return body
			}
			original := accounts()
			path := transitionMigrationPath(t, 17)
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
			if accounts() != original {
				t.Fatal("account state changed")
			}
			if n := valueCount(t, db, `SELECT count(*) FROM accounts WHERE session_epoch<>0`); n != 0 {
				t.Fatal("legacy epoch fabricated", n)
			}
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, after, transitionLegacySnapshot(t, ctx, db))
			if revoked {
				if _, err := db.ExecContext(ctx, `UPDATE accounts SET session_epoch=session_epoch+1`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, `UPDATE accounts SET session_epoch=0`); err == nil {
					t.Fatal("revocation epoch moved backwards")
				}
			}
			err := runMigration(db, path, "exact session epoch down", func(m *migrate.Migrate) error { return m.Steps(-1) })
			if revoked {
				if err == nil {
					t.Fatal("lossy epoch rollback reactivated old credentials")
				}
				if n := valueCount(t, db, `SELECT count(*) FROM accounts WHERE session_epoch=1`); n == 0 {
					t.Fatal("revocations disappeared")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
				if n := valueCount(t, db, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='accounts' AND column_name='session_epoch'`); n != 0 {
					t.Fatal("epoch column survived rollback")
				}
			}
		})
	}
}
