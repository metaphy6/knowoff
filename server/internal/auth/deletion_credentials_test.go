package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

func deletionCredentialsBound(t *testing.T, f *oauthFixture, d *DeletionManager, c DeletionConfirmCommand) {
	t.Helper()
	if _, err := d.Confirm(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.db.Exec(`SELECT privacy_bind_suppression($1,(SELECT COALESCE((SELECT suppression_sequence FROM privacy_requests WHERE id=$1),max(suppression_sequence)+1,1) FROM privacy_requests),$2)`, c.RequestID, bytes.Repeat([]byte{71}, 32)); err != nil {
		t.Fatal(err)
	}
}
func deletionCredentialsErase(t *testing.T, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, request string) {
	t.Helper()
	for range 100 {
		var raw []byte
		if err := q.QueryRowContext(t.Context(), `SELECT privacy_erase_credentials_batch($1,128)`, request).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Complete bool `json:"complete"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if result.Complete {
			return
		}
	}
	t.Fatal("credential erasure did not complete")
}
func deletionCredentialsWait(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE pid<>pg_backend_pid() AND wait_event_type='Lock' AND query LIKE $1`, query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("expected PostgreSQL lock wait absent")
}
func TestDeletionCredentialsProviderCallbackCannotRestoreErasedProof(t *testing.T) {
	for _, first := range []string{"callback", "cleanup"} {
		t.Run(first, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			f := newOAuthFixture(t)
			pair, err := f.m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
			if err != nil {
				t.Fatal(err)
			}
			flow, state, nonce := f.start("link", pair.AccessToken)
			f.token("credential-delete", uuid.NewString(), nonce, nil)
			d, err := NewDeletionManager(f.m, f.m.db, nil, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			capID, secret := deletionTestEnrollment(t, d, pair.AccessToken)
			command := deletionTestCommand(t, d, pair.AccountID, capID, secret)
			done := make(chan error, 1)
			if first == "callback" {
				entered := make(chan struct{})
				resume := make(chan struct{})
				defer func() {
					select {
					case <-resume:
					default:
						close(resume)
					}
				}()
				f.m.oauthHTTP = &http.Client{Transport: oauthTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.String() == "https://oauth2.googleapis.com/token" {
						close(entered)
						select {
						case <-resume:
						case <-r.Context().Done():
							return nil, r.Context().Err()
						}
					}
					return f.roundTrip(r)
				})}
				go func() { done <- f.m.CompleteOAuth(ctx, "google", state, "credential-delete") }()
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("callback did not claim proof")
				}
				deletionCredentialsBound(t, f, d, command)
				deletionCredentialsErase(t, f.m.db, command.RequestID)
				close(resume)
			} else {
				deletionCredentialsBound(t, f, d, command)
				tx, e := f.m.db.Begin()
				if e != nil {
					t.Fatal(e)
				}
				defer tx.Rollback()
				deletionCredentialsErase(t, tx, command.RequestID)
				go func() { done <- f.m.CompleteOAuth(ctx, "google", state, "credential-delete") }()
				deletionCredentialsWait(t, f.m.db, "WITH claim AS (SELECT id,code_verifier FROM oauth_flows%")
				if e = tx.Commit(); e != nil {
					t.Fatal(e)
				}
			}
			select {
			case err = <-done:
				if err == nil {
					t.Fatal("callback restored erased identity")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("callback did not finish")
			}
			if _, err = f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret); err == nil {
				t.Fatal("erased OAuth flow minted token")
			}
			if _, err = d.Confirm(t.Context(), command); err != nil {
				t.Fatal("HTTP service confirmation retry lost", err)
			}
			var count int
			if err = f.m.db.QueryRow(`SELECT (SELECT count(*) FROM oauth_links WHERE account_id=$1)+(SELECT count(*) FROM oauth_flows WHERE account_id=$1)`, pair.AccountID).Scan(&count); err != nil || count != 0 {
				t.Fatal("erased provider proof recreated", err)
			}
		})
	}
}
func TestDeletionCredentialsResultIssuanceAndCleanupBothOrders(t *testing.T) {
	for _, first := range []string{"result", "cleanup"} {
		t.Run(first, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			f := newOAuthFixture(t)
			pair, err := f.m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
			if err != nil {
				t.Fatal(err)
			}
			flow, state, nonce := f.start("link", pair.AccessToken)
			f.token("credential-result", uuid.NewString(), nonce, nil)
			if err = f.m.CompleteOAuth(ctx, "google", state, "credential-result"); err != nil {
				t.Fatal(err)
			}
			d, err := NewDeletionManager(f.m, f.m.db, nil, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			capID, secret := deletionTestEnrollment(t, d, pair.AccessToken)
			command := deletionTestCommand(t, d, pair.AccountID, capID, secret)
			if first == "result" {
				blocker, e := f.m.db.Begin()
				if e != nil {
					t.Fatal(e)
				}
				defer blocker.Rollback()
				if _, e = blocker.Exec(`SELECT id FROM oauth_flows WHERE id=$1 FOR UPDATE`, flow.FlowID); e != nil {
					t.Fatal(e)
				}
				issued := make(chan error, 1)
				go func() { _, e := f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret); issued <- e }()
				deletionCredentialsWait(t, f.m.db, "SELECT session_epoch,issued_at,access_id,refresh_id,issuance_config_hash,device_hash FROM oauth_flows%")
				confirmed := make(chan error, 1)
				go func() { _, e := d.Confirm(t.Context(), command); confirmed <- e }()
				deletionCredentialsWait(t, f.m.db, "SELECT public.privacy_confirm_deletion%")
				if e = blocker.Commit(); e != nil {
					t.Fatal(e)
				}
				if e = <-issued; e != nil {
					t.Fatal("pre-deletion issuance failed", e)
				}
				if e = <-confirmed; e != nil {
					t.Fatal(e)
				}
				deletionCredentialsBound(t, f, d, command)
				deletionCredentialsErase(t, f.m.db, command.RequestID)
			} else {
				deletionCredentialsBound(t, f, d, command)
				tx, e := f.m.db.Begin()
				if e != nil {
					t.Fatal(e)
				}
				defer tx.Rollback()
				deletionCredentialsErase(t, tx, command.RequestID)
				refused := make(chan error, 1)
				go func() { _, e := f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret); refused <- e }()
				deletionCredentialsWait(t, f.m.db, "%FOR UPDATE%")
				if e = tx.Commit(); e != nil {
					t.Fatal(e)
				}
				if e = <-refused; e == nil {
					t.Fatal("result bypassed committed account fence")
				}
			}
			if _, err = f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret); err == nil {
				t.Fatal("erased issuance receipt replayed")
			}
			if _, err = d.Confirm(t.Context(), command); err != nil {
				t.Fatal("confirmation replay failed", err)
			}
		})
	}
}

func TestDeletionCredentialsVerifiedProviderConfirmationReplaysAfterErasure(t *testing.T) {
	f := newOAuthFixture(t)
	pair, err := f.m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	subject := uuid.NewString()
	if err = f.m.LinkOAuth(t.Context(), pair.AccountID, "google", subject, ""); err != nil {
		t.Fatal(err)
	}
	d, err := NewDeletionManager(f.m, f.m.db, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := d.BeginProviderIntent(t.Context(), "google", pair.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(intent.URL)
	if err != nil {
		t.Fatal(err)
	}
	f.token("credential-provider-confirm", subject, target.Query().Get("nonce"), nil)
	if err = d.CompleteProviderIntent(t.Context(), "google", target.Query().Get("state"), "credential-provider-confirm"); err != nil {
		t.Fatal(err)
	}
	command := DeletionConfirmCommand{AccountID: pair.AccountID, IntentID: intent.ID, IntentSecret: intent.Secret, RequestID: uuid.NewString(), StatusSecret: deletionTestSecret(t)}
	deletionCredentialsBound(t, f, d, command)
	deletionCredentialsErase(t, f.m.db, command.RequestID)
	if _, err = d.Confirm(t.Context(), command); err != nil {
		t.Fatal("verified provider confirmation no longer replays", err)
	}
	changed := command
	changed.CapabilityID = uuid.NewString()
	changed.CapabilitySecret = deletionTestSecret(t)
	if _, err = d.Confirm(t.Context(), changed); err == nil {
		t.Fatal("provider receipt accepted added capability tuple")
	}
}
