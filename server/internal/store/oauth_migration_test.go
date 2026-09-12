package store

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
)

func TestOAuthActualMigrationParityAndRetainedReceiptDown(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "retained_receipt"}[retained], func(t *testing.T) {
			db := transitionDesignDB(t, 19, true)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			before := transitionLegacySnapshot(t, ctx, db)
			path := transitionMigrationPath(t, 20)
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
					if current.Name == old.Name {
						found = true
						if !reflect.DeepEqual(old, current) {
							t.Fatal("OAuth changed retained table", old.Name)
						}
					}
				}
				if !found {
					t.Fatal("retained table lost", old.Name)
				}
			}
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, after, transitionLegacySnapshot(t, ctx, db))
			if retained {
				if _, err := db.ExecContext(ctx, `INSERT INTO oauth_flows(id,provider,intent,state_hash,completion_hash,nonce_hash,code_verifier,requester_hash,created_at,expires_at) VALUES($1,'google','restore',$2,$2,$2,$2,$2,statement_timestamp(),statement_timestamp()+interval '10 minutes')`, uuid.NewString(), strings.Repeat("a", 43)); err != nil {
					t.Fatal(err)
				}
			}
			err := runMigration(db, path, "OAuth exact down", func(m *migrate.Migrate) error { return m.Steps(-1) })
			if retained {
				if err == nil {
					t.Fatal("rollback discarded callback replay receipt")
				}
				if n := valueCount(t, db, `SELECT count(*) FROM oauth_flows`); n != 1 {
					t.Fatal("receipt disappeared", n)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
				if n := valueCount(t, db, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='oauth_flows'`); n != 0 {
					t.Fatal("OAuth table survived down")
				}
			}
		})
	}
}
