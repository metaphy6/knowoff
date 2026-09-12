package admin

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/store"
)

func TestCommonAdminAuthorizationBindsActorBeforeLocks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	account := newAccount(t, db)
	other := newAccount(t, db)
	if err := m.CreateAdmin(t.Context(), account, account+"@test.local", "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(t.Context(), account+"@test.local", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	sid, csrf, _, err := m.CreateSession(t.Context(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"valid", "unbound", "mismatch", "omitted_actor", "omitted_binding", "nil_callback", "wrong_callback", "callback_error", "invalid_target", "empty_roles"} {
		t.Run(kind, func(t *testing.T) {
			ctx := t.Context()
			actor, expected := a.ID, a.ID
			targets := []string{other, account, other}
			original := append([]string(nil), targets...)
			roles := []string{"admin"}
			calls := 0
			authorize := func(ctx context.Context, tx *sql.Tx) (string, error) {
				calls++
				if kind == "wrong_callback" {
					return other, nil
				}
				if kind == "callback_error" {
					return "", errors.New("synthetic authorization failure")
				}
				return m.AuthorizeSessionTx(ctx, tx, sid, csrf)
			}
			switch kind {
			case "mismatch":
				actor = other
			case "omitted_actor":
				actor = ""
			case "omitted_binding":
				expected = ""
			case "nil_callback":
				authorize = nil
			case "invalid_target":
				targets = []string{"invalid-account"}
			case "empty_roles":
				roles = nil
			}
			if kind != "unbound" {
				ctx = store.WithAdminAuthorization(ctx, expected, authorize)
			}
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			err = store.LockAdminTx(ctx, tx, actor, roles, targets...)
			wantAllowed := kind == "valid" || kind == "unbound"
			if (err == nil) != wantAllowed {
				t.Fatalf("allowed=%v expected=%v error=%v", err == nil, wantAllowed, err)
			}
			if kind == "mismatch" || kind == "omitted_actor" || kind == "omitted_binding" || kind == "nil_callback" {
				if calls != 0 {
					t.Fatal("invalid binding invoked authorization callback")
				}
				var touched bool
				if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=pg_backend_pid() AND relation IN ('accounts'::regclass,'admin_accounts'::regclass,'admin_sessions'::regclass))`).Scan(&touched); err != nil {
					t.Fatal(err)
				}
				if touched {
					t.Fatal("invalid binding touched authority rows before refusal")
				}
			}
			if kind == "valid" || kind == "unbound" {
				for i := range targets {
					if targets[i] != original[i] {
						t.Fatal("caller-owned account list mutated")
					}
				}
				if kind == "valid" && calls != 1 {
					t.Fatalf("callback count=%d", calls)
				}
			}
		})
	}
}

func TestCommonAdminAuthorizationRetainsTrustedRolePolicy(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	account := newAccount(t, db)
	if err := m.CreateAdmin(t.Context(), account, account+"@test.local", "test-password", "superadmin"); err != nil {
		t.Fatal(err)
	}
	var actor string
	if err := db.QueryRow(`SELECT id FROM admin_accounts WHERE account_id=$1`, account).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	for _, allowed := range []bool{true, false} {
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		roles := []string{"admin"}
		if allowed {
			roles = append(roles, "superadmin")
		}
		err = store.LockAdminTx(t.Context(), tx, actor, roles)
		tx.Rollback()
		if (err == nil) != allowed {
			t.Fatalf("trusted role allowed=%v error=%v", allowed, err)
		}
	}
}

func TestCommonAdminAuthorizationLocksSortedAccountsBeforeRoles(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	m := NewManager(db, testConfig(), nil)
	accounts := []string{newAccount(t, db), newAccount(t, db), newAccount(t, db)}
	sort.Strings(accounts)
	if err := m.CreateAdmin(ctx, accounts[2], accounts[2]+"@test.local", "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(ctx, accounts[2]+"@test.local", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	sid, csrf, _, err := m.CreateSession(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	bound := store.WithAdminAuthorization(ctx, a.ID, func(ctx context.Context, tx *sql.Tx) (string, error) { return m.AuthorizeSessionTx(ctx, tx, sid, csrf) })
	barrier, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback()
	if err := store.LockValueAccount(ctx, barrier, accounts[0]); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		tx, err := db.BeginTx(bound, nil)
		if err == nil {
			defer tx.Rollback()
			err = store.LockAdminTx(bound, tx, a.ID, []string{"admin"}, accounts[2], accounts[1], accounts[0], accounts[0])
		}
		result <- err
	}()
	waitAdminSessionLock(t, ctx, db, "FROM accounts WHERE id=")
	// The blocked request must not already hold the actor account/role/session.
	// This transaction can lock those immediately before releasing the lowest ID.
	if _, err := barrier.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE NOWAIT`, accounts[2]); err != nil {
		t.Fatal("actor locked before lower target", err)
	}
	if _, err := barrier.ExecContext(ctx, `SELECT id FROM admin_accounts WHERE id=$1 FOR UPDATE NOWAIT`, a.ID); err != nil {
		t.Fatal("role locked before sorted accounts", err)
	}
	if _, err := barrier.ExecContext(ctx, `SELECT id FROM admin_sessions WHERE id=$1 FOR UPDATE NOWAIT`, sid); err != nil {
		t.Fatal("session locked before sorted accounts", err)
	}
	if err := barrier.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
