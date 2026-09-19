package portal

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/store"
)

func TestPortalInstallationSanctionRejectsDerivedBrowser(t *testing.T) {
	db := setupBrowserDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	am := auth.NewManager(db, []byte("disposable-installation-signing-key"), "test", "test", time.Hour, 24*time.Hour, auth.OAuthProviders{})
	m.auth = am
	hash := auth.HashDevice(uuid.NewString())
	target, err := am.AuthenticateDevice(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	other := newAccount(t, db)
	actor := uuid.NewString()
	adminAccount := newAccount(t, db)
	if _, err = db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, adminAccount); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO portal_browser_sessions(token_hash,account_id,csrf_token,expires_at,device_hash) VALUES($1,$2,'csrf',clock_timestamp()+interval '1 hour',$3)`, portalHash("test-cookie"), other, hash); err != nil {
		t.Fatal(err)
	}
	check := func() int {
		r := httptest.NewRequest("GET", "/portal/", nil)
		r.AddCookie(&http.Cookie{Name: portalSessionCookie, Value: "test-cookie"})
		w := httptest.NewRecorder()
		m.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if got := check(); got != 200 {
		t.Fatal("fixture browser unavailable", got)
	}
	ctx := store.WithAdminAuthorization(t.Context(), actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
	c := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target.AccountID, Reason: "Known installation"}
	if _, err = store.NewAccountSanctionStore(db).Apply(ctx, actor, c, am.RevokeSessionsTx); err != nil {
		t.Fatal(err)
	}
	if got := check(); got == 200 {
		t.Fatal("sanctioned installation retained other account browser authority")
	}
}

func TestPortalBrowserMutationRechecksCredentialAfterAccountWait(t *testing.T) {
	for _, change := range []string{"installation_sanction", "cookie_expiry"} {
		t.Run(change, func(t *testing.T) {
			db := setupBrowserDB(t)
			defer db.Close()
			m := newTestManager(t, db)
			am := auth.NewManager(db, []byte("disposable-installation-signing-key"), "test", "test", time.Hour, 24*time.Hour, auth.OAuthProviders{})
			hash := auth.HashDevice(uuid.NewString())
			target, err := am.AuthenticateDevice(t.Context(), hash)
			if err != nil {
				t.Fatal(err)
			}
			actorAccount, guard, victim := newAccount(t, db), newAccount(t, db), newAccount(t, db)
			actor := uuid.NewString()
			if _, err = db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, actorAccount); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO portal_roles(account_id,role,granted_by) VALUES($1,'guard',$2)`, guard, actor); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO portal_browser_sessions(token_hash,account_id,csrf_token,expires_at,device_hash) VALUES($1,$2,'csrf',clock_timestamp()+interval '1 hour',$3)`, portalHash("race-cookie"), guard, hash); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			barrier, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer barrier.Rollback()
			if _, err = barrier.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, guard); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", "/portal/guard/freeze", strings.NewReader(url.Values{"account_id": {victim}, "reason": {"reported conduct"}, "csrf_token": {"csrf"}}.Encode())).WithContext(ctx)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.AddCookie(&http.Cookie{Name: portalSessionCookie, Value: "race-cookie"})
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { m.Handler().ServeHTTP(w, r); close(done) }()
			for {
				var n int
				if err = db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM accounts%FOR UPDATE%'`).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n > 0 {
					break
				}
				select {
				case <-done:
					t.Fatal("mutation bypassed account barrier")
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(time.Millisecond):
				}
			}
			if change == "cookie_expiry" {
				_, err = db.ExecContext(ctx, `UPDATE portal_browser_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE token_hash=$1`, portalHash("race-cookie"))
			} else {
				authority := store.WithAdminAuthorization(ctx, actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
				_, err = store.NewAccountSanctionStore(db).Apply(authority, actor, store.AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target.AccountID, Reason: "captured installation"}, am.RevokeSessionsTx)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = barrier.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var freezes int
			if err = db.QueryRow(`SELECT count(*) FROM guard_freezes WHERE account_id=$1`, victim).Scan(&freezes); err != nil || freezes != 0 {
				t.Fatal("stale browser committed mutation", freezes, err, w.Code)
			}
		})
	}
}

func TestPortalBrowserDerivationAndInstallationSanctionBothOrders(t *testing.T) {
	for _, deriveFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "sanction_first", true: "derive_first"}[deriveFirst], func(t *testing.T) {
			db := setupBrowserDB(t)
			defer db.Close()
			m := newTestManager(t, db)
			am := auth.NewManager(db, []byte("disposable-race-key"), "test", "test", time.Hour, 24*time.Hour, auth.OAuthProviders{})
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			hash := auth.HashDevice(uuid.NewString())
			target, err := am.AuthenticateDevice(ctx, hash)
			if err != nil {
				t.Fatal(err)
			}
			other, adminAccount := newAccount(t, db), newAccount(t, db)
			actor := uuid.NewString()
			if _, err = db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, adminAccount); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO portal_login_requests(browser_hash,pairing_code,csrf_token,account_id,expires_at,device_hash) VALUES($1,'ABCDEFGH','csrf',$2,clock_timestamp()+interval '1 hour',$3)`, portalHash("ordered-browser"), other, hash); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", "/portal/session", strings.NewReader("csrf_token=csrf")).WithContext(ctx)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.AddCookie(&http.Cookie{Name: portalLoginCookie, Value: "ordered-browser"})
			w := httptest.NewRecorder()
			done := make(chan struct{})
			derive := func() { m.loginContinue(w, r); close(done) }
			authority := store.WithAdminAuthorization(ctx, actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
			command := store.AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target.AccountID, Reason: "ordered derived browser"}
			sanctioned := make(chan error, 1)
			wait := func(fragment string) {
				t.Helper()
				for {
					var n int
					if err = db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1`, "%"+fragment+"%").Scan(&n); err != nil {
						t.Fatal(err)
					}
					if n > 0 {
						return
					}
					select {
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					case <-time.After(time.Millisecond):
					}
				}
			}
			if deriveFirst {
				barrier, e := db.BeginTx(ctx, nil)
				if e != nil {
					t.Fatal(e)
				}
				defer barrier.Rollback()
				if _, e = barrier.ExecContext(ctx, `LOCK TABLE portal_browser_sessions IN SHARE MODE`); e != nil {
					t.Fatal(e)
				}
				go derive()
				wait("DELETE FROM portal_browser_sessions")
				go func() {
					_, e := store.NewAccountSanctionStore(db).Apply(authority, actor, command, am.RevokeSessionsTx)
					sanctioned <- e
				}()
				wait("auth_installations")
				if e = barrier.Commit(); e != nil {
					t.Fatal(e)
				}
			} else {
				captured, release := make(chan struct{}), make(chan struct{})
				go func() {
					_, e := store.NewAccountSanctionStore(db).Apply(authority, actor, command, func(c context.Context, tx *sql.Tx, a string) error {
						close(captured)
						<-release
						return am.RevokeSessionsTx(c, tx, a)
					})
					sanctioned <- e
				}()
				select {
				case <-captured:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				go derive()
				wait("auth_installations")
				close(release)
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if err = <-sanctioned; err != nil {
				t.Fatal(err)
			}
			var sessions int
			if err = db.QueryRow(`SELECT count(*) FROM portal_browser_sessions WHERE account_id=$1`, other).Scan(&sessions); err != nil {
				t.Fatal(err)
			}
			if deriveFirst {
				if sessions != 1 {
					t.Fatal("earlier derivation failed", sessions, w.Code, w.Body.String())
				}
				for _, cookie := range w.Result().Cookies() {
					if cookie.Name == portalSessionCookie {
						check := httptest.NewRequest("GET", "/portal/", nil)
						check.AddCookie(cookie)
						out := httptest.NewRecorder()
						m.Handler().ServeHTTP(out, check)
						if out.Code == 200 {
							t.Fatal("derived session escaped later sanction")
						}
					}
				}
			} else if sessions != 0 {
				t.Fatal("later derivation escaped sanction")
			}
		})
	}
}

func TestPortalPairingRejectsJWTExpiredDuringPairingRowWait(t *testing.T) {
	db := setupBrowserDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	am := auth.NewManager(db, []byte("disposable-race-key"), "test", "test", 2*time.Second, 24*time.Hour, auth.OAuthProviders{})
	m.auth = am
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	pair, err := am.AuthenticateDevice(ctx, auth.HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO portal_login_requests(browser_hash,pairing_code,csrf_token,expires_at) VALUES($1,'ABCDEFGH','csrf',clock_timestamp()+interval '1 hour')`, portalHash("expiry-browser")); err != nil {
		t.Fatal(err)
	}
	barrier, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback()
	if _, err = barrier.ExecContext(ctx, `SELECT pairing_code FROM portal_login_requests WHERE pairing_code='ABCDEFGH' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/portal/connect", strings.NewReader(`{"code":"ABCDEFGH"}`)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { m.ConnectHandler().ServeHTTP(w, r); close(done) }()
	for {
		var n int
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%UPDATE portal_login_requests%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			break
		}
		select {
		case <-done:
			t.Fatal("pairing skipped barrier", w.Code, w.Body.String())
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	time.Sleep(time.Until(pair.ExpiresAt) + 20*time.Millisecond)
	if err = barrier.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM portal_login_requests WHERE account_id IS NOT NULL`).Scan(&n); err != nil || n != 0 {
		t.Fatal("expired credential retained pairing approval", n, err, w.Code)
	}
}
