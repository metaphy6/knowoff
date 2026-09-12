package admin

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
	"github.com/knowoff/knowoff/server/internal/portal"
	"github.com/knowoff/knowoff/server/internal/reports"
	"github.com/knowoff/knowoff/server/internal/store"
)

func TestPortalAdminWritesRecheckActorAfterMiddleware(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	pm := portal.NewManager(portal.Deps{DB: db, Config: testConfig(), Admin: m})
	am := auth.NewManager(db, []byte("test-key-32-bytes-long-for-hs256!!"), "test", "test", time.Hour, time.Hour, auth.OAuthProviders{})
	for _, action := range []string{"terms", "grant", "approve", "revoke", "reject", "user_terms", "report_case"} {
		for _, change := range []string{"none", "logout", "revoke_sessions", "ban", "deleted", "suspend", "role_loss", "expiry", "csrf_rotation"} {
			t.Run(action+"/"+change, func(t *testing.T) {
				ctx := t.Context()
				actor := newAccount(t, db)
				target := newAccount(t, db)
				if err := m.CreateAdmin(ctx, actor, actor+"@test.local", "test-password", "admin"); err != nil {
					t.Fatal(err)
				}
				admin, err := m.Authenticate(ctx, actor+"@test.local", "test-password")
				if err != nil {
					t.Fatal(err)
				}
				sid, csrf, _, err := m.CreateSession(ctx, admin.ID)
				if err != nil {
					t.Fatal(err)
				}
				application := uuid.NewString()
				if _, err := db.Exec(`INSERT INTO portal_role_applications(id,account_id,role,status) VALUES($1,$2,'contributor','pending')`, application, target); err != nil {
					t.Fatal(err)
				}
				if action == "report_case" {
					if _, err := db.Exec(`INSERT INTO report_cases(id,case_key,kind,target_account_id) VALUES($1,$3,'conduct',$2)`, application, target, application); err != nil {
						t.Fatal(err)
					}
				}
				if action == "revoke" {
					if err := pm.GrantRole(ctx, admin.ID, target, portal.RoleContributor); err != nil {
						t.Fatal(err)
					}
				}
				var priorAudits int
				if err := db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE admin_id=$1`, admin.ID).Scan(&priorAudits); err != nil {
					t.Fatal(err)
				}
				called := false
				h := m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
					// This is the exact boundary after live session + CSRF middleware,
					// before the domain transaction acquires its actor locks.
					switch change {
					case "logout":
						err = m.DestroySession(ctx, sid)
					case "revoke_sessions":
						err = am.RevokeSessions(ctx, actor)
					case "ban":
						_, err = db.Exec(`UPDATE accounts SET banned_at=now() WHERE id=$1`, actor)
					case "deleted":
						_, err = db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, actor)
					case "suspend":
						_, err = db.Exec(`UPDATE accounts SET suspended_until=now()+interval '1 hour' WHERE id=$1`, actor)
					case "role_loss":
						_, err = db.Exec(`UPDATE admin_accounts SET role='viewer' WHERE id=$1`, admin.ID)
					case "expiry":
						_, err = db.Exec(`UPDATE admin_sessions SET expires_at=now()-interval '1 second' WHERE id=$1`, sid)
					case "csrf_rotation":
						_, err = db.Exec(`UPDATE admin_sessions SET csrf_token=$2 WHERE id=$1`, sid, uuid.NewString())
					}
					if err != nil {
						t.Fatal(err)
					}
					switch action {
					case "user_terms":
						err = store.NewTextTrustStore(db).PublishTerms(r.Context(), adminIDFromContext(r.Context()), target, "Explicit synthetic fixture wording", time.Now())
					case "report_case":
						err = reports.NewManager(db).ResolveCase(r.Context(), adminIDFromContext(r.Context()), application, "dismissed", "Reviewed synthetic fixture", nil)
					case "terms":
						err = pm.CreateTermsVersion(r.Context(), adminIDFromContext(r.Context()), target, "Synthetic terms", "Explicit fixture wording", time.Now())
					case "approve":
						err = pm.ApproveApplication(r.Context(), adminIDFromContext(r.Context()), application)
					case "grant":
						err = pm.GrantRole(r.Context(), adminIDFromContext(r.Context()), target, portal.RoleContributor)
					case "revoke":
						err = pm.RevokeRole(r.Context(), adminIDFromContext(r.Context()), target, portal.RoleContributor)
					case "reject":
						err = pm.RejectApplication(r.Context(), adminIDFromContext(r.Context()), application, "Reviewed application")
					}
					if err != nil {
						http.Error(w, "denied", http.StatusForbidden)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				}))
				form := url.Values{"csrf_token": {csrf}}
				r := httptest.NewRequest(http.MethodPost, "/admin/portal/", strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if !called {
					t.Fatal("request never crossed valid middleware")
				}
				wantSuccess := change == "none"
				if (w.Code == http.StatusNoContent) != wantSuccess {
					t.Errorf("stale %s %s request status=%d", change, action, w.Code)
				}
				var effect bool
				switch action {
				case "user_terms":
					err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_terms_versions WHERE version=$1)`, target).Scan(&effect)
				case "report_case":
					err = db.QueryRow(`SELECT status='resolved' FROM report_cases WHERE id=$1`, application).Scan(&effect)
				case "terms":
					err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM portal_terms WHERE version=$1)`, target).Scan(&effect)
				case "grant", "approve":
					err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM portal_roles WHERE account_id=$1 AND role='contributor' AND revoked_at IS NULL)`, target).Scan(&effect)
				case "revoke":
					err = db.QueryRow(`SELECT revoked_at IS NOT NULL FROM portal_roles WHERE account_id=$1 AND role='contributor'`, target).Scan(&effect)
				case "reject":
					err = db.QueryRow(`SELECT status='rejected' FROM portal_role_applications WHERE id=$1`, application).Scan(&effect)
				}
				if err != nil || effect != wantSuccess {
					t.Errorf("unauthorized effect=%v expected=%v err=%v", effect, wantSuccess, err)
				}
				var audits int
				if err := db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE admin_id=$1`, admin.ID).Scan(&audits); err != nil {
					t.Fatal(err)
				}
				wantAudits := priorAudits
				if wantSuccess {
					wantAudits++
				}
				if audits != wantAudits {
					t.Errorf("unauthorized or missing audit count=%d expected=%d", audits, wantAudits)
				}
			})
		}
	}
}

func TestAdminSessionTransactionRejectsExpiryAfterLockWait(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	m := NewManager(db, testConfig(), nil)
	actor := newAccount(t, db)
	if err := m.CreateAdmin(ctx, actor, actor+"@test.local", "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(ctx, actor+"@test.local", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	sid, csrf, _, err := m.CreateSession(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	barrier, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback()
	if _, err := barrier.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, actor); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		tx, err := db.BeginTx(ctx, nil)
		if err == nil {
			defer tx.Rollback()
			_, err = m.AuthorizeSessionTx(ctx, tx, sid, csrf)
		}
		result <- err
	}()
	waitAdminSessionLock(t, ctx, db, "FOR UPDATE")
	if _, err := barrier.ExecContext(ctx, `UPDATE admin_sessions SET expires_at=clock_timestamp() WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	if err := barrier.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err == nil {
		t.Fatal("expired waiting request authorized")
	}
}

func TestAdminSessionAuthorizationSerializesRevocationBothOrders(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	for _, checker := range []string{"session", "shared"} {
		for _, change := range []string{"logout", "ban", "role_loss"} {
			for _, first := range []string{"authorization", "revocation"} {
				t.Run(checker+"/"+change+"/"+first, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
					defer cancel()
					account := newAccount(t, db)
					if err := m.CreateAdmin(ctx, account, account+"@test.local", "test-password", "admin"); err != nil {
						t.Fatal(err)
					}
					a, err := m.Authenticate(ctx, account+"@test.local", "test-password")
					if err != nil {
						t.Fatal(err)
					}
					sid, csrf, _, err := m.CreateSession(ctx, a.ID)
					if err != nil {
						t.Fatal(err)
					}
					authorizeTx := func(ctx context.Context, tx *sql.Tx) (string, error) {
						if checker == "session" {
							return m.AuthorizeSessionTx(ctx, tx, sid, csrf)
						}
						bound := store.WithAdminAuthorization(ctx, a.ID, func(ctx context.Context, tx *sql.Tx) (string, error) {
							return m.AuthorizeSessionTx(ctx, tx, sid, csrf)
						})
						return a.ID, store.LockAdminTx(bound, tx, a.ID, []string{"admin"})
					}
					var lockSQL, mutation, target, waitFragment string
					switch change {
					case "logout":
						lockSQL, mutation, target, waitFragment = `SELECT id FROM admin_sessions WHERE id=$1 FOR UPDATE`, `DELETE FROM admin_sessions WHERE id=$1`, sid, "FROM admin_sessions WHERE id="
					case "ban":
						lockSQL, mutation, target, waitFragment = `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, `UPDATE accounts SET banned_at=clock_timestamp() WHERE id=$1`, account, "FROM accounts WHERE id="
					case "role_loss":
						lockSQL, mutation, target, waitFragment = `SELECT id FROM admin_accounts WHERE id=$1 FOR UPDATE`, `UPDATE admin_accounts SET role='viewer' WHERE id=$1`, a.ID, "FROM admin_accounts WHERE id="
					}
					tx, err := db.BeginTx(ctx, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Rollback()
					result := make(chan error, 1)
					if first == "authorization" {
						if actor, err := authorizeTx(ctx, tx); err != nil || actor != a.ID {
							t.Fatal("valid actor", err)
						}
						go func() { _, err := db.ExecContext(ctx, mutation, target); result <- err }()
						waitAdminSessionLock(t, ctx, db, mutation)
						if err := tx.Commit(); err != nil {
							t.Fatal(err)
						}
						if err := <-result; err != nil {
							t.Fatal(err)
						}
						check, err := db.BeginTx(ctx, nil)
						if err != nil {
							t.Fatal(err)
						}
						defer check.Rollback()
						if _, err := authorizeTx(ctx, check); err == nil {
							t.Fatal("later request escaped committed revocation")
						}
					} else {
						if _, err := tx.ExecContext(ctx, lockSQL, target); err != nil {
							t.Fatal(err)
						}
						go func() {
							check, err := db.BeginTx(ctx, nil)
							if err == nil {
								defer check.Rollback()
								_, err = authorizeTx(ctx, check)
							}
							result <- err
						}()
						waitAdminSessionLock(t, ctx, db, waitFragment)
						if _, err := tx.ExecContext(ctx, mutation, target); err != nil {
							t.Fatal(err)
						}
						if err := tx.Commit(); err != nil {
							t.Fatal(err)
						}
						if err := <-result; err == nil {
							t.Fatal("earlier revocation allowed waiting request")
						}
					}
				})
			}
		}
	}
}
