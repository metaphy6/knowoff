package admin

import (
	"context"
	"testing"
)

func TestErasedAdminCredentialsCannotIssueOrUseSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	ctx := context.Background()
	account := newAccount(t, db)
	email := "erased-admin@example.invalid"
	if err := m.CreateAdmin(ctx, account, email, "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	actor, err := m.Authenticate(ctx, email, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	session, csrf, _, err := m.CreateSession(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE admin_accounts SET email=NULL,password_hash=NULL,totp_secret=NULL,backup_codes='{}',role='erased',credentials_erased_at=clock_timestamp() WHERE id=$1`, actor.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = m.CreateSession(ctx, actor.ID); err == nil {
		t.Fatal("erased actor issued fresh session")
	}
	if _, _, err = m.ValidateSession(ctx, session, csrf); err == nil {
		t.Fatal("erased actor retained session authority")
	}
	if _, err = m.Authenticate(ctx, email, "test-password"); err == nil {
		t.Fatal("erased password authenticated")
	}
	if _, _, err = m.TOTPSecretForEmail(ctx, email); err == nil {
		t.Fatal("erased TOTP recovered")
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = m.AuthorizeSessionTx(ctx, tx, session, csrf); err == nil {
		t.Fatal("erased actor authorized mutation")
	}
}
