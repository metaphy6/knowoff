package store

import (
	"context"
	"github.com/golang-migrate/migrate/v4"
	"reflect"
	"testing"
	"time"
)

func TestAvatarRevisionActualMigrationPreservesLegacyAndRefusesLossyDown(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "new_revision"}[changed], func(t *testing.T) {
			db := transitionDesignDB(t, 22, true)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			before := transitionLegacySnapshot(t, ctx, db)
			preserved := func() string {
				var value string
				if err := db.QueryRowContext(ctx, `SELECT jsonb_build_object('accounts',(SELECT jsonb_agg(to_jsonb(a)-'avatar_revision' ORDER BY id) FROM accounts a),'avatars',(SELECT jsonb_agg(to_jsonb(c)-'revision' ORDER BY account_id) FROM custom_avatars c))::text`).Scan(&value); err != nil {
					t.Fatal(err)
				}
				return value
			}
			original := preserved()
			path := transitionMigrationPath(t, 23)
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			after := transitionLegacySnapshot(t, ctx, db)
			for _, old := range before.Tables {
				if old.Name == "accounts" || old.Name == "custom_avatars" || old.Name == "schema_migrations" {
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
					t.Fatal("table lost", old.Name)
				}
			}
			if preserved() != original {
				t.Fatal("retained avatar/account changed")
			}
			if n := valueCount(t, db, `SELECT (SELECT count(*) FROM accounts WHERE avatar_revision<>0)+(SELECT count(*) FROM custom_avatars WHERE revision IS NOT NULL)`); n != 0 {
				t.Fatal("historical moderation fabricated")
			}
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, after, transitionLegacySnapshot(t, ctx, db))
			if changed {
				if _, err := db.ExecContext(ctx, `UPDATE accounts SET avatar_revision=1`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, `UPDATE accounts SET avatar_revision=0`); err == nil {
					t.Fatal("revision went backwards")
				}
			}
			err := runMigration(db, path, "exact avatar revision down", func(m *migrate.Migrate) error { return m.Steps(-1) })
			if changed {
				if err == nil {
					t.Fatal("discarded retained moderation fences")
				}
				if n := valueCount(t, db, `SELECT count(*) FROM accounts WHERE avatar_revision=1`); n == 0 {
					t.Fatal("revision lost")
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
