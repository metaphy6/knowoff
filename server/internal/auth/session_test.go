package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestSessionRevocationPreservesIdentityAndValue(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	m := newTestManager(db)
	device := HashDevice(uuid.NewString())
	pair, err := m.AuthenticateDevice(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	account := pair.AccountID
	for _, query := range []string{
		`INSERT INTO oauth_links(account_id,provider,provider_subject) VALUES($1::text::uuid,'google',$1::text)`,
		`INSERT INTO noin_wallets(account_id,balance) VALUES($1,57)`,
		`UPDATE profiles SET xp=15,overall_points=102,non_converted_points=102 WHERE account_id=$1`,
		`INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'theme_pack','retained')`,
		`INSERT INTO portal_browser_sessions(token_hash,account_id,csrf_token,expires_at) VALUES($1::text,$1::text::uuid,$1::text,now()+interval '1 hour')`,
		`INSERT INTO portal_login_requests(browser_hash,pairing_code,csrf_token,account_id,expires_at) VALUES($1::text,$1::text,$1::text,$1::text::uuid,now()+interval '1 hour')`,
		`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1::text::uuid,$1::text::uuid,$1::text,'fixture','fixture')`,
		`INSERT INTO admin_sessions(admin_id,csrf_token,expires_at) VALUES($1::text::uuid,$1::text,now()+interval '1 hour')`,
	} {
		if _, err := db.ExecContext(ctx, query, account); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() string {
		var body string
		if err := db.QueryRowContext(ctx, `SELECT jsonb_build_array((SELECT to_jsonb(a)-'session_epoch' FROM accounts a WHERE id=$1),(SELECT jsonb_agg(to_jsonb(d)) FROM device_tokens d WHERE account_id=$1),(SELECT jsonb_agg(to_jsonb(o)) FROM oauth_links o WHERE account_id=$1),(SELECT to_jsonb(p) FROM profiles p WHERE account_id=$1),(SELECT to_jsonb(w) FROM noin_wallets w WHERE account_id=$1),(SELECT jsonb_agg(to_jsonb(e)) FROM entitlements e WHERE account_id=$1),(SELECT to_jsonb(a) FROM admin_accounts a WHERE account_id=$1))::text`, account).Scan(&body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	before := snapshot()
	rolledBack, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RevokeSessionsTx(ctx, rolledBack, account); err != nil {
		rolledBack.Rollback()
		t.Fatal(err)
	}
	if err := rolledBack.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ValidateAccessToken(ctx, pair.AccessToken); err != nil {
		t.Fatal("rolled-back revocation escaped its transaction", err)
	}
	var retainedSessions int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM portal_browser_sessions WHERE account_id=$1`, account).Scan(&retainedSessions); err != nil || retainedSessions != 1 {
		t.Fatal("rolled-back derived session deletion escaped", retainedSessions, err)
	}
	claims, err := m.parseToken(pair.AccessToken, TokenAccess)
	if err != nil {
		t.Fatal(err)
	}
	// Omitting both additions models an already-issued legacy player token.
	claims.Purpose = ""
	legacy, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.signingKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ValidateAccessToken(ctx, legacy); err != nil {
		t.Fatal("legacy epoch-zero rejected", err)
	}
	if err := m.RevokeSessions(ctx, account); err != nil {
		t.Fatal(err)
	}
	if snapshot() != before {
		t.Fatal("session revocation changed identity, status, or value")
	}
	for _, token := range []string{pair.AccessToken, legacy} {
		if _, err := m.ValidateAccessToken(ctx, token); err == nil {
			t.Fatal("revoked access accepted")
		}
	}
	if _, err := m.Refresh(ctx, pair.RefreshToken); err == nil {
		t.Fatal("revoked refresh accepted")
	}
	for _, query := range []string{`SELECT count(*) FROM portal_browser_sessions WHERE account_id=$1`, `SELECT count(*) FROM portal_login_requests WHERE account_id=$1`, `SELECT count(*) FROM admin_sessions WHERE admin_id=$1`} {
		var n int
		if err := db.QueryRowContext(ctx, query, account).Scan(&n); err != nil || n != 0 {
			t.Fatal("derived session survived", n, err)
		}
	}
	// Reauthentication must restore the same account, at the new epoch.
	fresh, err := m.AuthenticateDevice(ctx, device)
	if err != nil || fresh.AccountID != account {
		t.Fatal("identity could not be restored", err)
	}
	freshClaims, err := m.parseToken(fresh.AccessToken, TokenAccess)
	if err != nil || freshClaims.SessionEpoch != 1 {
		t.Fatal("fresh session missing epoch", err)
	}
	if _, err := m.ValidateAccessToken(ctx, fresh.AccessToken); err != nil {
		t.Fatal(err)
	}
	freshClaims.SessionEpoch = -1
	invalid, err := jwt.NewWithClaims(jwt.SigningMethodHS256, freshClaims).SignedString(m.signingKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ValidateAccessToken(ctx, invalid); err == nil {
		t.Fatal("negative session epoch accepted")
	}
}

func TestDerivedSessionTokenValidationSharesRevocationTransaction(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	ctx := context.Background()
	p, err := m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	for _, revoked := range []bool{false, true} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if revoked {
			if err := m.RevokeSessionsTx(ctx, tx, p.AccountID); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
		}
		account, err := m.ValidateAccessTokenTx(ctx, tx, p.AccessToken)
		if (err != nil) != revoked || !revoked && account != p.AccountID {
			tx.Rollback()
			t.Fatal("derived credential ignored transaction epoch", account, err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
	}
	claims, err := m.parseToken(p.AccessToken, TokenAccess)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO auth_revocations(token_id,expires_at) VALUES($1,$2)`, claims.ID, claims.ExpiresAt.Time); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ValidateAccessTokenTx(ctx, tx, p.AccessToken); err == nil {
		t.Fatal("derived session ignored revoked JTI")
	}
}

func TestEnforcedSessionRevocationRechecksCanonicalSanction(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	m := newTestManager(db)
	for _, sanction := range []string{"clear", "expired", "suspended", "banned", "deleted"} {
		t.Run(sanction, func(t *testing.T) {
			device := HashDevice(uuid.NewString())
			p, err := m.AuthenticateDevice(ctx, device)
			if err != nil {
				t.Fatal(err)
			}
			updates := map[string]string{"expired": `suspended_until=now()-interval '1 second'`, "suspended": `suspended_until=now()+interval '1 hour'`, "banned": `banned_at=now()`, "deleted": `deleted_at=now()`}
			if update := updates[sanction]; update != "" {
				if _, err := db.ExecContext(ctx, `UPDATE accounts SET `+update+` WHERE id=$1`, p.AccountID); err != nil {
					t.Fatal(err)
				}
			}
			active := sanction == "suspended" || sanction == "banned" || sanction == "deleted"
			got, err := m.RevokeEnforcedSessions(ctx, p.AccountID)
			if err != nil || got != active {
				t.Fatal("canonical enforcement mismatch", got, err)
			}
			var epoch int64
			if err := db.QueryRowContext(ctx, `SELECT session_epoch FROM accounts WHERE id=$1`, p.AccountID).Scan(&epoch); err != nil {
				t.Fatal(err)
			}
			if active && epoch != 1 || !active && epoch != 0 {
				t.Fatal("unexpected revocation epoch", epoch)
			}
			if _, err := db.ExecContext(ctx, `UPDATE accounts SET banned_at=NULL,suspended_until=NULL,deleted_at=NULL WHERE id=$1`, p.AccountID); err != nil {
				t.Fatal(err)
			}
			if _, err := m.ValidateAccessToken(ctx, p.AccessToken); (err != nil) != active {
				t.Fatal("old token resurrection or late revocation", err)
			}
			fresh, err := m.AuthenticateDevice(ctx, device)
			if err != nil || fresh.AccountID != p.AccountID {
				t.Fatal("sanction deleted device identity", err)
			}
			if active, err := m.RevokeEnforcedSessions(ctx, p.AccountID); err != nil || active {
				t.Fatal("late hook enforced lifted sanction", active, err)
			}
			if _, err := m.ValidateAccessToken(ctx, fresh.AccessToken); err != nil {
				t.Fatal("late hook killed fresh session", err)
			}
		})
	}
}

// Wait for a real database lock barrier, not an assumed goroutine schedule.
func waitSessionLock(t *testing.T, ctx context.Context, db *sql.DB, fragment string) {
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
			t.Fatal("database lock barrier missing", ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
}

func TestRefreshAndSessionRevocationSerializeBothCommitOrders(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	for _, refreshFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "revoke_first", true: "refresh_first"}[refreshFirst], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			p, err := m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
			if err != nil {
				t.Fatal(err)
			}
			barrier, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer barrier.Rollback()
			if refreshFirst {
				_, err = barrier.ExecContext(ctx, `LOCK TABLE auth_revocations IN SHARE ROW EXCLUSIVE MODE`)
			} else {
				_, err = barrier.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, p.AccountID)
			}
			if err != nil {
				t.Fatal(err)
			}
			type result struct {
				pair *TokenPair
				err  error
			}
			refreshed := make(chan result, 1)
			go func() { pair, err := m.Refresh(ctx, p.RefreshToken); refreshed <- result{pair, err} }()
			if refreshFirst {
				waitSessionLock(t, ctx, db, "INSERT INTO auth_revocations")
				revoked := make(chan error, 1)
				go func() { revoked <- m.RevokeSessions(ctx, p.AccountID) }()
				waitSessionLock(t, ctx, db, "UPDATE accounts SET session_epoch")
				if err := barrier.Commit(); err != nil {
					t.Fatal(err)
				}
				r := <-refreshed
				if r.err != nil {
					t.Fatal("earlier refresh failed", r.err)
				}
				if err := <-revoked; err != nil {
					t.Fatal(err)
				}
				if _, err := m.ValidateAccessToken(ctx, r.pair.AccessToken); err == nil {
					t.Fatal("concurrent refresh escaped epoch revocation")
				}
				if _, err := m.Refresh(ctx, r.pair.RefreshToken); err == nil {
					t.Fatal("replacement refresh escaped epoch revocation")
				}
			} else {
				waitSessionLock(t, ctx, db, "FOR UPDATE")
				if err := m.RevokeSessionsTx(ctx, barrier, p.AccountID); err != nil {
					t.Fatal(err)
				}
				if err := barrier.Commit(); err != nil {
					t.Fatal(err)
				}
				if r := <-refreshed; r.err == nil {
					t.Fatal("old refresh upgraded to new epoch")
				}
			}
		})
	}
}
