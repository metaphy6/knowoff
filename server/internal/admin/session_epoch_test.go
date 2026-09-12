package admin

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/auth"
)

func TestAdminSessionIssuanceRechecksStatusAndAuthenticationEpoch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	m := NewManager(db, testConfig(), nil)
	authManager := auth.NewManager(db, []byte("test-key-32-bytes-long-for-hs256!!"), "test", "test", time.Hour, time.Hour, auth.OAuthProviders{})
	account := newAccount(t, db)
	if err := m.CreateAdmin(ctx, account, "session-epoch@example.com", "hunter2", "admin"); err != nil {
		t.Fatal(err)
	}
	verified, err := m.Authenticate(ctx, "session-epoch@example.com", "hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if verified.SessionEpoch != 0 {
		t.Fatal("unexpected initial epoch")
	}
	if err := authManager.RevokeSessions(ctx, account); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := m.CreateSessionForEpoch(ctx, verified.ID, verified.SessionEpoch); err == nil {
		t.Fatal("pre-revocation password verification minted a later session")
	}
	verified, err = m.Authenticate(ctx, "session-epoch@example.com", "hunter2")
	if err != nil || verified.SessionEpoch != 1 {
		t.Fatal("fresh password verification lost epoch", err)
	}
	id, csrf, _, err := m.CreateSessionForEpoch(ctx, verified.ID, verified.SessionEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.ValidateSession(ctx, id, csrf); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{`banned_at=now()`, `suspended_until=now()+interval '1 hour'`, `deleted_at=now()`} {
		if _, err := db.ExecContext(ctx, `UPDATE accounts SET `+status+` WHERE id=$1`, account); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Authenticate(ctx, "session-epoch@example.com", "hunter2"); err == nil {
			t.Fatal("inactive admin authenticated")
		}
		if _, _, _, err := m.CreateSession(ctx, verified.ID); err == nil {
			t.Fatal("inactive admin session created")
		}
		if _, _, _, err := m.CreateSessionForEpoch(ctx, verified.ID, verified.SessionEpoch); err == nil {
			t.Fatal("inactive verified admin session created")
		}
		if _, err := db.ExecContext(ctx, `UPDATE accounts SET banned_at=NULL,suspended_until=NULL,deleted_at=NULL WHERE id=$1`, account); err != nil {
			t.Fatal(err)
		}
	}
}

func waitAdminSessionLock(t *testing.T, ctx context.Context, db *sql.DB, fragment string) {
	t.Helper()
	for {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND query LIKE $1`, "%"+fragment+"%").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("missing real session lock barrier", ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
}

func TestAdminSessionAndRevocationSerializeBothCommitOrders(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	authManager := auth.NewManager(db, []byte("test-key-32-bytes-long-for-hs256!!"), "test", "test", time.Hour, time.Hour, auth.OAuthProviders{})
	for _, sessionFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "revocation_first", true: "session_first"}[sessionFirst], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			account := newAccount(t, db)
			email := account + "@example.com"
			if err := m.CreateAdmin(ctx, account, email, "hunter2", "admin"); err != nil {
				t.Fatal(err)
			}
			a, err := m.Authenticate(ctx, email, "hunter2")
			if err != nil {
				t.Fatal(err)
			}
			barrier, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer barrier.Rollback()
			if sessionFirst {
				_, err = barrier.ExecContext(ctx, `LOCK TABLE admin_sessions IN SHARE ROW EXCLUSIVE MODE`)
			} else {
				_, err = barrier.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account)
			}
			if err != nil {
				t.Fatal(err)
			}
			type result struct {
				id, csrf string
				err      error
			}
			issued := make(chan result, 1)
			go func() {
				id, csrf, _, err := m.CreateSessionForEpoch(ctx, a.ID, a.SessionEpoch)
				issued <- result{id, csrf, err}
			}()
			if sessionFirst {
				waitAdminSessionLock(t, ctx, db, "INSERT INTO admin_sessions")
				revoked := make(chan error, 1)
				go func() { revoked <- authManager.RevokeSessions(ctx, account) }()
				waitAdminSessionLock(t, ctx, db, "UPDATE accounts SET session_epoch")
				if err := barrier.Commit(); err != nil {
					t.Fatal(err)
				}
				r := <-issued
				if r.err != nil {
					t.Fatal(r.err)
				}
				if err := <-revoked; err != nil {
					t.Fatal(err)
				}
				if _, _, err := m.ValidateSession(ctx, r.id, r.csrf); err == nil {
					t.Fatal("earlier session escaped revocation")
				}
			} else {
				waitAdminSessionLock(t, ctx, db, "FOR UPDATE")
				if err := authManager.RevokeSessionsTx(ctx, barrier, account); err != nil {
					t.Fatal(err)
				}
				if err := barrier.Commit(); err != nil {
					t.Fatal(err)
				}
				if r := <-issued; r.err == nil {
					t.Fatal("revoked login upgraded to a new session")
				}
			}
			var n int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM admin_sessions WHERE admin_id=$1`, a.ID).Scan(&n); err != nil || n != 0 {
				t.Fatal("unrevoked admin session survived", n, err)
			}
		})
	}
}
