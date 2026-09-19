package admin

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

func TestDirectAccountSanctionRefusesAdminLoginAndSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	ctx := t.Context()
	account := newAccount(t, db)
	email := "sanction-" + uuid.NewString() + "@test.invalid"
	if err := m.CreateAdmin(ctx, account, email, "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	admin, err := m.Authenticate(ctx, email, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	actorAccount := newAccount(t, db)
	actor := uuid.NewString()
	if _, err = db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, actorAccount); err != nil {
		t.Fatal(err)
	}
	approved := store.WithAdminAuthorization(ctx, actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
	c := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: account, Reason: "Direct independent sanction"}
	revoke := func(context.Context, *sql.Tx, string) error { return nil }
	if _, err = store.NewAccountSanctionStore(db).Apply(approved, actor, c, revoke); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Authenticate(ctx, email, "test-password"); err == nil {
		t.Fatal("directly sanctioned admin authenticated")
	}
	if _, _, _, err = m.CreateSession(ctx, admin.ID); err == nil {
		t.Fatal("directly sanctioned admin issued session")
	}
}

func TestAccountSanctionRechecksInitiatingSessionAfterInstallationWait(t *testing.T) {
	db := operatorDB(t)
	_, actor, sid, _, authority := operatorActor(t, db)
	target := newAccount(t, db)
	hash := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO auth_installations(device_hash) VALUES($1);`, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2)`, target, hash); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(authority, 6*time.Second)
	defer cancel()
	var expiry time.Time
	if err := db.QueryRow(`UPDATE admin_sessions SET expires_at=clock_timestamp()+interval '2 seconds' WHERE id=$1 RETURNING expires_at`, sid).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	barrier, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback()
	if _, err = barrier.ExecContext(ctx, `SELECT device_hash FROM auth_installations WHERE device_hash=$1 FOR UPDATE`, hash); err != nil {
		t.Fatal(err)
	}
	command := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target, Reason: "expired initiating browser"}
	done := make(chan error, 1)
	go func() {
		r, e := store.NewAccountSanctionStore(db).Apply(ctx, actor, command, func(context.Context, *sql.Tx, string) error { return nil })
		if e != nil && r.Command.ID != "" {
			t.Error("failed authorization exposed receipt")
		}
		done <- e
	}()
	for {
		var n int
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%auth_installations%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	time.Sleep(time.Until(expiry) + 20*time.Millisecond)
	if err = barrier.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("expired Admin completed sanction")
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM admin_operation_decisions WHERE id=$1`, command.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal("failed authority retained decision", n, err)
	}
}
