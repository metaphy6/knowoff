package auth

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

func TestInstallationConcurrentBootstrapRetainsOneAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	hash := HashDevice(uuid.NewString())
	var before, after int
	if err := db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	pairs := make([]*TokenPair, 12)
	errs := make([]error, 12)
	for i := range pairs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			pairs[i], errs[i] = m.AuthenticateDevice(context.Background(), hash)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, p := range pairs {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if p.AccountID != pairs[0].AccountID {
			t.Fatal("concurrent bootstrap created different accounts")
		}
	}
	if err := db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after-before != 1 {
		t.Fatal("losing bootstrap retained orphan account", before, after)
	}
}

func TestInstallationBindingRecoveryAfterAccessExpires(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	m.accessTTL = time.Second
	account, err := m.createAccount(t.Context(), "Delayed "+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	old, err := m.issueTokens(t.Context(), account, "")
	if err != nil {
		t.Fatal(err)
	}
	hash := HashDevice(uuid.NewString())
	first, err := m.BindInstallation(t.Context(), old.RefreshToken, hash)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(first.ExpiresAt) + 10*time.Millisecond)
	replay, err := m.BindInstallation(t.Context(), old.RefreshToken, hash)
	if err != nil || *replay != *first {
		t.Fatal("delayed lost response stranded account", err)
	}
	if _, err = m.Refresh(t.Context(), replay.RefreshToken); err != nil {
		t.Fatal("retained bound refresh unusable", err)
	}
}

func TestInstallationOAuthRequiresBinding(t *testing.T) {
	f := newOAuthFixture(t)
	if _, err := f.m.BeginOAuth(t.Context(), OAuthStartRequest{Provider: "google", Intent: "restore", Principal: uuid.NewString()}); err == nil {
		t.Fatal("new OAuth flow accepted missing installation")
	}
}

func TestInstallationLegacyOAuthReceiptSurvivesBindingLostResponse(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := t.Context()
	player, err := f.m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	flow, state, nonce := f.start("link", player.AccessToken)
	f.token("old-build", uuid.NewString(), nonce, nil)
	if err = f.m.CompleteOAuth(ctx, "google", state, "old-build"); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	aid, rid := uuid.NewString(), uuid.NewString()
	// Model a receipt already issued by the previous build with no device claim.
	if _, err = f.m.db.Exec(`UPDATE oauth_flows SET device_hash=NULL,issued_at=$2,access_id=$3,refresh_id=$4,issuance_config_hash=$5 WHERE id=$1`, flow.FlowID, at, aid, rid, f.m.oauthIssuanceConfigHash()); err != nil {
		t.Fatal(err)
	}
	old, err := f.m.signTokensAt(player.AccountID, "", "player", 0, at, aid, rid)
	if err != nil {
		t.Fatal(err)
	}
	before, err := f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret)
	if err != nil || *before != *old {
		t.Fatal("upgraded old issuance bytes", err)
	}
	hash := HashDevice(uuid.NewString())
	bound, err := f.m.BindInstallation(ctx, old.RefreshToken, hash)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret)
	if err != nil || *recovered != *old {
		t.Fatal("lost binding response cannot recover immutable old receipt", err)
	}
	replay, err := f.m.BindInstallation(ctx, recovered.RefreshToken, hash)
	if err != nil || *replay != *bound {
		t.Fatal("binding changed retry result", err)
	}
}

func TestInstallationSanctionRejectsLinkedAccountAndRefresh(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	ctx := t.Context()
	hash := HashDevice(uuid.NewString())
	target, err := m.AuthenticateDevice(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	other, err := m.createAccount(ctx, "Switch "+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := m.issueTokens(ctx, other, "")
	if err != nil {
		t.Fatal(err)
	}
	bound, err := m.BindInstallation(ctx, legacy.RefreshToken, hash)
	if err != nil {
		t.Fatal(err)
	}
	adminAccount, err := m.createAccount(ctx, "Admin "+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.NewString()
	if _, err = db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, adminAccount); err != nil {
		t.Fatal(err)
	}
	adminCtx := store.WithAdminAuthorization(ctx, actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
	command := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target.AccountID, Reason: "Known installation enforcement"}
	if _, err = store.NewAccountSanctionStore(db).Apply(adminCtx, actor, command, m.RevokeSessionsTx); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ValidateAccessToken(ctx, bound.AccessToken); err == nil {
		t.Fatal("sanctioned installation admitted another linked account")
	}
	if _, err = m.Refresh(ctx, bound.RefreshToken); err == nil {
		t.Fatal("sanctioned installation refreshed another linked account")
	}
	if _, err = m.AuthenticateDevice(ctx, hash); err == nil {
		t.Fatal("sanctioned installation bootstrapped")
	}
	if _, err = m.ValidateAccessToken(ctx, target.AccessToken); err == nil {
		t.Fatal("sanctioned account admitted")
	}
	if _, err = m.ValidateAccessToken(ctx, legacy.AccessToken); err == nil {
		t.Fatal("unbound legacy token admitted protected surface")
	}
}

func TestInstallationBindingRetainsSameAccountAndLostResponse(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	ctx := t.Context()
	account, err := m.createAccount(ctx, "Legacy OAuth account")
	if err != nil {
		t.Fatal(err)
	}
	old, err := m.issueTokens(ctx, account, "")
	if err != nil {
		t.Fatal(err)
	}
	hash := HashDevice(uuid.NewString())
	first, err := m.BindInstallation(ctx, old.RefreshToken, hash)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := m.BindInstallation(ctx, old.RefreshToken, hash)
	if err != nil {
		t.Fatal("lost response recovery", err)
	}
	if *first != *replay || first.AccountID != account {
		t.Fatal("binding changed account or immutable response")
	}
	claims, err := m.parseToken(first.AccessToken, TokenAccess)
	if err != nil || claims.DeviceHash != hash {
		t.Fatal("missing signed binding", err)
	}
	if _, err = m.BindInstallation(ctx, old.RefreshToken, HashDevice(uuid.NewString())); err == nil {
		t.Fatal("replayed old credential into another installation")
	}
	if _, err = m.Refresh(ctx, old.RefreshToken); err == nil {
		t.Fatal("binding did not consume old refresh")
	}
	if _, err = m.ValidateAccessToken(ctx, first.AccessToken); err != nil {
		t.Fatal(err)
	}
}

func TestInstallationWaitCannotExtendExpiredCredential(t *testing.T) {
	for _, operation := range []string{"access", "refresh", "binding"} {
		t.Run(operation, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			m := newTestManager(db)
			m.accessTTL = 2 * time.Second
			m.refreshTTL = 2 * time.Second
			ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
			defer cancel()
			hash := HashDevice(uuid.NewString())
			p, err := m.AuthenticateDevice(ctx, hash)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "binding" {
				p, err = m.issueTokens(ctx, p.AccountID, "")
				if err != nil {
					t.Fatal(err)
				}
			}
			originalClaims, err := m.parseToken(p.RefreshToken, TokenRefresh)
			if err != nil {
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
			done := make(chan error, 1)
			go func() {
				var e error
				switch operation {
				case "access":
					tx, be := db.BeginTx(ctx, nil)
					if be != nil {
						done <- be
						return
					}
					defer tx.Rollback()
					_, e = m.ValidateAccessTokenTx(ctx, tx, p.AccessToken)
				case "refresh":
					_, e = m.Refresh(ctx, p.RefreshToken)
				case "binding":
					_, e = m.BindInstallation(ctx, p.RefreshToken, hash)
				}
				done <- e
			}()
			waitSessionLock(t, ctx, db, "auth_installations")
			time.Sleep(time.Until(p.ExpiresAt) + 20*time.Millisecond)
			if err = barrier.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err == nil {
				t.Fatal("expired credential gained authority after installation wait")
			}
			var consumed int
			if err = db.QueryRow(`SELECT count(*) FROM auth_revocations WHERE token_id=$1`, originalClaims.ID).Scan(&consumed); err != nil || consumed != 0 {
				t.Fatal("expired refusal consumed credential", consumed, err)
			}
		})
	}
}

func TestInstallationIssuanceAndSanctionBothCommitOrders(t *testing.T) {
	for _, operation := range []string{"new_target_link", "shared_refresh", "oauth_callback", "oauth_result"} {
		for _, issueFirst := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "/sanction_first", true: "/issuance_first"}[issueFirst], func(t *testing.T) {
				f := newOAuthFixture(t)
				m, db := f.m, f.m.db
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				hash := HashDevice(uuid.NewString())
				target, err := m.AuthenticateDevice(ctx, hash)
				if err != nil {
					t.Fatal(err)
				}
				other, err := m.createAccount(ctx, "Other "+uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
				pair, err := m.issueTokens(ctx, other, hash)
				if err != nil {
					t.Fatal(err)
				}
				var flow *OAuthStart
				var state string
				if operation == "oauth_callback" || operation == "oauth_result" {
					var nonce string
					flow, state, nonce = f.start("link", pair.AccessToken)
					f.token("ordered-code", "ordered-subject-"+other, nonce, nil)
					if operation == "oauth_result" {
						if err = m.CompleteOAuth(ctx, "google", state, "ordered-code"); err != nil {
							t.Fatal(err)
						}
					}
				}
				actorAccount, err := m.createAccount(ctx, "Admin "+uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
				actor := uuid.NewString()
				if _, err = db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, actorAccount); err != nil {
					t.Fatal(err)
				}
				authority := store.WithAdminAuthorization(ctx, actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
				command := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target.AccountID, Reason: "ordered issuance sanction"}
				nextHash := HashDevice(uuid.NewString())
				issued := make(chan error, 1)
				sanctioned := make(chan error, 1)
				var result *TokenPair
				issue := func() {
					var e error
					switch operation {
					case "new_target_link":
						result, e = m.issueTokens(ctx, target.AccountID, nextHash)
					case "shared_refresh":
						result, e = m.Refresh(ctx, pair.RefreshToken)
					case "oauth_callback":
						e = m.CompleteOAuth(ctx, "google", state, "ordered-code")
					case "oauth_result":
						result, e = m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret)
					}
					issued <- e
				}
				if issueFirst {
					barrier, e := db.BeginTx(ctx, nil)
					if e != nil {
						t.Fatal(e)
					}
					defer barrier.Rollback()
					if _, e = barrier.ExecContext(ctx, `LOCK TABLE device_tokens IN SHARE MODE`); e != nil {
						t.Fatal(e)
					}
					go issue()
					waitSessionLock(t, ctx, db, "INSERT INTO device_tokens")
					go func() {
						_, e := store.NewAccountSanctionStore(db).Apply(authority, actor, command, m.RevokeSessionsTx)
						sanctioned <- e
					}()
					if operation == "new_target_link" {
						waitSessionLock(t, ctx, db, "FROM accounts")
					} else {
						waitSessionLock(t, ctx, db, "auth_installations")
					}
					if e = barrier.Commit(); e != nil {
						t.Fatal(e)
					}
					if e = <-issued; e != nil {
						t.Fatal("earlier issuance failed", e)
					}
					if e = <-sanctioned; e != nil {
						t.Fatal(e)
					}
					if result != nil {
						if _, e = m.ValidateAccessToken(ctx, result.AccessToken); e == nil {
							t.Fatal("earlier credentials escaped later sanction")
						}
					}
					if operation == "new_target_link" {
						var n int
						if e = db.QueryRow(`SELECT count(*) FROM account_sanction_installations WHERE operation_id=$1 AND device_hash=$2`, command.ID, nextHash).Scan(&n); e != nil || n != 1 {
							t.Fatal("capture missed committed new link", n, e)
						}
					}
				} else {
					captured, release := make(chan struct{}), make(chan struct{})
					go func() {
						_, e := store.NewAccountSanctionStore(db).Apply(authority, actor, command, func(c context.Context, tx *sql.Tx, a string) error {
							close(captured)
							<-release
							return m.RevokeSessionsTx(c, tx, a)
						})
						sanctioned <- e
					}()
					select {
					case <-captured:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					go issue()
					if operation == "new_target_link" {
						waitSessionLock(t, ctx, db, "FROM accounts")
					} else {
						waitSessionLock(t, ctx, db, "auth_installations")
					}
					close(release)
					if e := <-sanctioned; e != nil {
						t.Fatal(e)
					}
					if e := <-issued; e == nil {
						t.Fatal("later issuance escaped sanction")
					}
				}
			})
		}
	}
}

func TestInstallationWaitCannotExtendExpiredOAuthFlow(t *testing.T) {
	for _, resultPhase := range []bool{false, true} {
		t.Run(map[bool]string{false: "callback", true: "result"}[resultPhase], func(t *testing.T) {
			f := newOAuthFixture(t)
			m, db := f.m, f.m.db
			ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
			defer cancel()
			hash := HashDevice(uuid.NewString())
			p, err := m.AuthenticateDevice(ctx, hash)
			if err != nil {
				t.Fatal(err)
			}
			flow, state, nonce := f.start("link", p.AccessToken)
			f.token("expiry-code", "expiry-"+p.AccountID, nonce, nil)
			if resultPhase {
				if err = m.CompleteOAuth(ctx, "google", state, "expiry-code"); err != nil {
					t.Fatal(err)
				}
			}
			var expiry time.Time
			if err = db.QueryRow(`UPDATE oauth_flows SET expires_at=clock_timestamp()+interval '2 seconds' WHERE id=$1 RETURNING expires_at`, flow.FlowID).Scan(&expiry); err != nil {
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
			done := make(chan error, 1)
			go func() {
				if resultPhase {
					_, e := m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret)
					done <- e
				} else {
					done <- m.CompleteOAuth(ctx, "google", state, "expiry-code")
				}
			}()
			waitSessionLock(t, ctx, db, "auth_installations")
			time.Sleep(time.Until(expiry) + 20*time.Millisecond)
			if err = barrier.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err == nil {
				t.Fatal("expired OAuth flow advanced after installation wait")
			}
			var issued bool
			if err = db.QueryRow(`SELECT issued_at IS NOT NULL FROM oauth_flows WHERE id=$1`, flow.FlowID).Scan(&issued); err != nil || issued {
				t.Fatal("expired flow minted credentials", issued, err)
			}
		})
	}
}

func TestInstallationOAuthBeginRejectsBearerExpiredAtBudgetLock(t *testing.T) {
	f := newOAuthFixture(t)
	m, db := f.m, f.m.db
	m.accessTTL = 2 * time.Second
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	hash := HashDevice(uuid.NewString())
	p, err := m.AuthenticateDevice(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	barrier, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback()
	if _, err = barrier.ExecContext(ctx, `SELECT pg_advisory_xact_lock(69428041)`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, e := m.BeginOAuth(ctx, OAuthStartRequest{Provider: "google", Intent: "link", AccessToken: p.AccessToken, DeviceHash: hash, Principal: uuid.NewString()})
		done <- e
	}()
	waitSessionLock(t, ctx, db, "pg_advisory_xact_lock")
	time.Sleep(time.Until(p.ExpiresAt) + 20*time.Millisecond)
	if err = barrier.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("expired bearer delegated OAuth flow after budget wait")
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM oauth_flows WHERE account_id=$1`, p.AccountID).Scan(&n); err != nil || n != 0 {
		t.Fatal("expired bearer retained OAuth flow", n, err)
	}
}
