package store

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
)

func TestAdminOperationsDownSerializesConcurrentReceiptWriter(t *testing.T) {
	for _, writerFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "writer_before_down", false: "down_before_writer"}[writerFirst], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
				db := transitionDesignDB(t, 27, false)
				db.SetMaxOpenConns(4) // two real transactions plus a lock observer
			account, actor, id := uuid.NewString(), uuid.NewString(), uuid.NewString()
			if _, err := db.ExecContext(ctx, `INSERT INTO accounts(id,nickname) VALUES($1,'down race fixture')`, account); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,'down@local','fixture','fixture')`, actor, account); err != nil {
				t.Fatal(err)
			}
			down, err := os.ReadFile("../../migrations/000027_admin_operations.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			insert := `INSERT INTO admin_operation_decisions(id,actor_admin_id,kind,target_account_id,amount,reason,affected_accounts,request_hash) VALUES($1,$2,'noin_grant',NULLIF($3,'')::uuid,1,'fixture',jsonb_build_array($3::text),decode(repeat('03',32),'hex'))`
			writer, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Rollback()
			migration, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer migration.Rollback()
			var writerPID, migrationPID int
			if err = writer.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
				t.Fatal(err)
			}
			if err = migration.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&migrationPID); err != nil {
				t.Fatal(err)
			}
			waitBlocked := func(pid int) {
				t.Helper()
				for {
					var waiting bool
					if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND NOT granted AND locktype='relation')`, pid).Scan(&waiting); err != nil {
						t.Fatal(err)
					}
					if waiting {
						return
					}
					select {
					case <-ctx.Done():
						t.Fatal("SQL never waited behind conflicting table lock")
					case <-time.After(5 * time.Millisecond):
					}
				}
			}
			done := make(chan error, 1)
			if writerFirst {
				if _, err = writer.ExecContext(ctx, insert, id, actor, account); err != nil {
					t.Fatal(err)
				}
				go func() {
					_, e := migration.ExecContext(ctx, string(down))
					if e == nil {
						e = migration.Commit()
					} else {
						migration.Rollback()
					}
					done <- e
				}()
				waitBlocked(migrationPID)
				if err = writer.Commit(); err != nil {
					t.Fatal(err)
				}
				if err = <-done; err == nil {
					t.Fatal("down dropped a receipt committed after its empty check")
				}
				if n := valueCount(t, db, `SELECT count(*) FROM admin_operation_decisions WHERE id=$1`, id); n != 1 {
					t.Fatal("committed decision disappeared", n)
				}
			} else {
				if _, err = migration.ExecContext(ctx, string(down)); err != nil {
					t.Fatal(err)
				}
				go func() {
					_, e := writer.ExecContext(ctx, insert, id, actor, account)
					if e == nil {
						e = writer.Commit()
					} else {
						writer.Rollback()
					}
					done <- e
				}()
				waitBlocked(writerPID)
				if err = migration.Commit(); err != nil {
					t.Fatal(err)
				}
				if err = <-done; err == nil {
					t.Fatal("writer claimed success after schema was removed")
				}
				var exists bool
				if err = db.QueryRowContext(ctx, `SELECT to_regclass('public.admin_operation_decisions') IS NOT NULL`).Scan(&exists); err != nil || exists {
					t.Fatal("exact empty down failed", exists, err)
				}
			}
		})
	}
}

func TestAdminOperationsActualMigrationPreservesHead26AndRefusesLoss(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty_down", true: "retained_receipts"}[populated], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			db := transitionDesignDB(t, 26, false)
			account, actor := uuid.NewString(), uuid.NewString()
			for _, q := range []string{
				`INSERT INTO accounts(id,nickname) VALUES($1,'migration retained operator')`,
				`INSERT INTO noin_wallets(account_id,balance) VALUES($1,91)`,
				`INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,'fixture',91,'retained',CURRENT_DATE)`,
				`INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'custom_avatar','retained')`,
			} {
				if _, err := db.ExecContext(ctx, q, account); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,'fixture@local','fixture','fixture')`, actor, account); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO admin_audit_log(admin_id,action,before_state,after_state) VALUES($1,'retained_fixture','{"before":1}','{"after":2}')`, actor); err != nil {
				t.Fatal(err)
			}
			before := transitionLegacySnapshot(t, ctx, db)
			path := transitionMigrationPath(t, 27)
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			after := transitionLegacySnapshot(t, ctx, db)
			if after.Version == nil || *after.Version != 27 || after.Dirty {
				t.Fatal("not actual clean head27")
			}
			for _, old := range before.Tables {
				if old.Name == "schema_migrations" {
					continue
				}
				found := false
				for _, now := range after.Tables {
					if now.Name == old.Name {
						found = reflect.DeepEqual(old, now)
					}
				}
				if !found {
					t.Fatal("changed prior retained table", old.Name)
				}
			}
			if n := valueCount(t, db, `SELECT (SELECT count(*) FROM admin_operation_decisions)+(SELECT count(*) FROM admin_operation_results)`); n != 0 {
				t.Fatal("migration manufactured decisions", n)
			}
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, after, transitionLegacySnapshot(t, ctx, db))
			for _, q := range []string{`UPDATE admin_audit_log SET action=action`, `DELETE FROM admin_audit_log`, `TRUNCATE admin_audit_log`} {
				if _, err := db.ExecContext(ctx, q); err == nil {
					t.Fatal("historical audit rewritable", q)
				}
			}
			if populated {
				id := uuid.NewString()
				if _, err := db.ExecContext(ctx, `INSERT INTO admin_operation_decisions(id,actor_admin_id,kind,target_account_id,amount,reason,affected_accounts,request_hash) VALUES($1,$2,'noin_grant',NULLIF($3,'')::uuid,4,'fixture',jsonb_build_array($3::text),decode(repeat('01',32),'hex'))`, id, actor, account); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, `INSERT INTO admin_operation_results(operation_id,outcome,result) VALUES($1,'obsolete','{"outcome":"obsolete","detail":"fixture"}')`, id); err != nil {
					t.Fatal(err)
				}
			}
			err := runMigration(db, path, "operator receipts exact down", func(m *migrate.Migrate) error { return m.Steps(-1) })
			if populated {
				if err == nil {
					t.Fatal("discarded retained decisions")
				}
				if n := valueCount(t, db, `SELECT (SELECT count(*) FROM admin_operation_decisions)+(SELECT count(*) FROM admin_operation_results)`); n != 2 {
					t.Fatal("retained receipts changed", n)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
				if n := valueCount(t, db, `SELECT count(*) FROM pg_trigger WHERE tgname IN ('admin_audit_immutable','admin_audit_truncate','admin_operation_immutable','admin_operation_truncate')`); n != 0 {
					t.Fatal("migration27 trigger remains", n)
				}
			}
		})
	}
}
