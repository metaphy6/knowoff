package auth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/knowoff/knowoff/server/internal/privacy"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeletionCapabilityConfirmationSurvivesGameplaySanction(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	manager := newTestManager(db)
	pair, err := manager.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.parseToken(pair.AccessToken, TokenAccess)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, capability, intent, request := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	nonce, capHash, statusHash := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{3}, 32)
	var got string
	if err = db.QueryRow(`SELECT privacy_begin_enrollment($1,$2,$3,$4,$5,$6,$7,$8)`, enrollment, pair.AccountID, nonce, time.Now().Add(time.Minute), claims.SessionEpoch, claims.ID, claims.ExpiresAt.Time, claims.DeviceHash).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT privacy_enroll_capability($1,$2,$3,$4)`, enrollment, nonce, capability, capHash).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if err = manager.RevokeSessions(t.Context(), pair.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE accounts SET banned_at=now() WHERE id=$1`, pair.AccountID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT privacy_begin_deletion_intent($1,$2,$3,$4,$5)`, intent, capability, capHash, nonce, time.Now().Add(time.Minute)).Scan(&got); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = db.QueryRow(`SELECT privacy_confirm_deletion($1,$2,$3,$4,$5,$6,$7)`, intent, nonce, capability, capHash, request, statusHash, pair.AccountID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		var receipt struct {
			RequestID string `json:"request_id"`
		}
		if err = json.Unmarshal([]byte(got), &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.RequestID != request {
			t.Fatal("request identity changed", got)
		}
	}
	var deleted bool
	if err = db.QueryRow(`SELECT deleted_at IS NOT NULL FROM accounts WHERE id=$1`, pair.AccountID).Scan(&deleted); err != nil || !deleted {
		t.Fatal("deletion fence not applied", err)
	}
	if _, err = manager.ValidateAccessToken(t.Context(), pair.AccessToken); err == nil {
		t.Fatal("old credential revived")
	}
	var phase string
	if err = db.QueryRow(`SELECT phase FROM privacy_requests WHERE id=$1`, request).Scan(&phase); err != nil || phase != "prepared" {
		t.Fatal("unbound suppression advertised confirmed", phase, err)
	}
}

type deletionPublisherProbe struct {
	requests []privacy.SuppressionRequest
	fail     bool
	sequence int64
}

func (p *deletionPublisherProbe) Publish(_ context.Context, request privacy.SuppressionRequest) (privacy.SuppressionReceipt, error) {
	p.requests = append(p.requests, request)
	if p.fail {
		return privacy.SuppressionReceipt{}, errors.New("synthetic publisher unavailable")
	}
	if p.sequence == 0 {
		p.sequence = time.Now().UnixNano()
	}
	return privacy.SuppressionReceipt{Sequence: p.sequence, Digest: [32]byte{9}}, nil
}
func TestDeletionServiceRequiresDurableSuppressionBeforeConfirmation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	manager := newTestManager(db)
	pair, err := manager.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	publisher := &deletionPublisherProbe{fail: true}
	deletion, err := NewDeletionManager(manager, db, publisher, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := deletion.BeginEnrollment(t.Context(), pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	capID, capSecret := uuid.NewString(), base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32))
	if err = deletion.EnrollCapability(t.Context(), enrollment.ID, enrollment.Secret, capID, capSecret); err != nil {
		t.Fatal(err)
	}
	intent, err := deletion.BeginCapabilityIntent(t.Context(), capID, capSecret)
	if err != nil {
		t.Fatal(err)
	}
	command := DeletionConfirmCommand{AccountID: pair.AccountID, IntentID: intent.ID, IntentSecret: intent.Secret, CapabilityID: capID, CapabilitySecret: capSecret, RequestID: uuid.NewString(), StatusSecret: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{5}, 32))}
	result, err := deletion.Confirm(t.Context(), command)
	if err != nil || result.Phase != "prepared" {
		t.Fatal("uncertain append acknowledged", result, err)
	}
	publisher.fail = false
	result, err = deletion.Confirm(t.Context(), command)
	if err != nil || result.Phase != "suppression_bound" {
		t.Fatal("durable receipt not bound", result, err)
	}
	if len(publisher.requests) != 2 || publisher.requests[0] != publisher.requests[1] {
		t.Fatal("retry changed suppression identity")
	}
	if publisher.requests[0].AccountID != pair.AccountID || publisher.requests[0].VerifiedAt.IsZero() {
		t.Fatal("publisher lacked authoritative metadata")
	}
	status, err := deletion.Status(t.Context(), command.StatusSecret)
	if err != nil || !bytes.Contains(status, []byte(`"suppression_bound"`)) {
		t.Fatal("status unavailable after revocation", string(status), err)
	}
	if bytes.Contains(status, []byte(pair.AccountID)) || bytes.Contains(status, []byte(capSecret)) {
		t.Fatal("status leaked identity or credential")
	}
}

func deletionTestSecret(t *testing.T) string {
	t.Helper()
	value, err := randomCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func deletionTestEnrollment(t *testing.T, d *DeletionManager, access string) (string, string) {
	t.Helper()
	enrollment, err := d.BeginEnrollment(t.Context(), access)
	if err != nil {
		t.Fatal(err)
	}
	id, secret := uuid.NewString(), deletionTestSecret(t)
	if err = d.EnrollCapability(t.Context(), enrollment.ID, enrollment.Secret, id, secret); err != nil {
		t.Fatal(err)
	}
	return id, secret
}
func deletionTestCommand(t *testing.T, d *DeletionManager, account, id, secret string) DeletionConfirmCommand {
	t.Helper()
	intent, err := d.BeginCapabilityIntent(t.Context(), id, secret)
	if err != nil {
		t.Fatal(err)
	}
	return DeletionConfirmCommand{AccountID: account, IntentID: intent.ID, IntentSecret: intent.Secret, CapabilityID: id, CapabilitySecret: secret, RequestID: uuid.NewString(), StatusSecret: deletionTestSecret(t)}
}
func TestDeletionCapabilityRejectsWrongAuthorityAndSecurityRevocation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	pair, err := m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDeletionManager(m, db, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	id, secret := deletionTestEnrollment(t, d, pair.AccessToken)
	c := deletionTestCommand(t, d, pair.AccountID, id, secret)
	for _, change := range []func(*DeletionConfirmCommand){func(x *DeletionConfirmCommand) { x.AccountID = uuid.NewString() }, func(x *DeletionConfirmCommand) { x.IntentSecret = deletionTestSecret(t) }, func(x *DeletionConfirmCommand) { x.CapabilitySecret = deletionTestSecret(t) }, func(x *DeletionConfirmCommand) { x.CapabilityID = uuid.NewString() }} {
		bad := c
		change(&bad)
		if _, err = d.Confirm(t.Context(), bad); err == nil {
			t.Fatal("wrong authority confirmed")
		}
	}
	if err = d.SecurityRevoke(t.Context(), pair.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Confirm(t.Context(), c); err == nil {
		t.Fatal("security-revoked intent confirmed")
	}
	if _, err = d.BeginCapabilityIntent(t.Context(), id, secret); !errors.Is(err, ErrDeletionRecoveryRequired) {
		t.Fatal("revoked guest capability lacks honest recovery result", err)
	}
	if _, err = d.BeginEnrollment(t.Context(), pair.AccessToken); err == nil {
		t.Fatal("security-revoked gameplay credential reenrolled")
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM privacy_requests WHERE account_id=$1`, pair.AccountID).Scan(&count); err != nil || count != 0 {
		t.Fatal("negative authority wrote request", count, err)
	}
}
func TestDeletionProviderProofIsSameAccountFreshAndSingleUse(t *testing.T) {
	for _, scenario := range []string{"success", "wrong-account", "unlinked", "wrong-nonce", "revoked-before-callback", "revoked-before-confirm"} {
		t.Run(scenario, func(t *testing.T) {
			f := newOAuthFixture(t)
			pair, err := f.m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
			if err != nil {
				t.Fatal(err)
			}
			subject := uuid.NewString()
			if scenario != "unlinked" {
				if err = f.m.LinkOAuth(t.Context(), pair.AccountID, "google", subject, ""); err != nil {
					t.Fatal(err)
				}
			}
			d, err := NewDeletionManager(f.m, f.m.db, nil, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			expected := pair.AccountID
			if scenario == "wrong-account" {
				other, e := f.m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
				if e != nil {
					t.Fatal(e)
				}
				expected = other.AccountID
			}
			intent, err := d.BeginProviderIntent(t.Context(), "google", expected)
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(intent.URL)
			if err != nil {
				t.Fatal(err)
			}
			state, nonce := u.Query().Get("state"), u.Query().Get("nonce")
			if scenario == "wrong-nonce" {
				nonce = "wrong"
			}
			f.token("delete-code", subject, nonce, nil)
			if scenario == "revoked-before-callback" {
				if err = d.SecurityRevoke(t.Context(), pair.AccountID); err != nil {
					t.Fatal(err)
				}
			}
			// Gameplay sanctions deliberately leave provider deletion proof usable.
			if _, err = f.m.db.Exec(`UPDATE accounts SET banned_at=now(),session_epoch=session_epoch+1 WHERE id=$1`, pair.AccountID); err != nil {
				t.Fatal(err)
			}
			err = d.CompleteProviderIntent(t.Context(), "google", state, "delete-code")
			accepted := scenario == "success" || scenario == "revoked-before-confirm"
			if (err == nil) != accepted {
				t.Fatal("provider proof outcome", err)
			}
			if err = d.CompleteProviderIntent(t.Context(), "google", state, "delete-code"); err == nil {
				t.Fatal("callback replay accepted")
			}
			if f.exchanges != 1 {
				t.Fatal("callback re-exchanged provider code", f.exchanges)
			}
			if accepted {
				status, e := d.IntentStatus(t.Context(), intent.ID, intent.Secret)
				if e != nil || !bytes.Contains(status, []byte(pair.AccountID)) {
					t.Fatal("private confirmation identity missing", string(status), e)
				}
				if scenario == "revoked-before-confirm" {
					if e = d.SecurityRevoke(t.Context(), pair.AccountID); e != nil {
						t.Fatal(e)
					}
				}
				result, e := d.Confirm(t.Context(), DeletionConfirmCommand{AccountID: pair.AccountID, IntentID: intent.ID, IntentSecret: intent.Secret, RequestID: uuid.NewString(), StatusSecret: deletionTestSecret(t)})
				if scenario == "success" && (e != nil || result.Phase != "prepared") {
					t.Fatal(result, e)
				}
				if scenario != "success" && e == nil {
					t.Fatal("revoked provider intent confirmed")
				}
			}
			var links int
			if err = f.m.db.QueryRow(`SELECT count(*) FROM oauth_links WHERE account_id=$1`, pair.AccountID).Scan(&links); err != nil {
				t.Fatal(err)
			}
			want := 1
			if scenario == "unlinked" {
				want = 0
			}
			if links != want {
				t.Fatal("deletion changed provider links")
			}
		})
	}
}
func TestDeletionConcreteSuppressionSurvivesProcessReopen(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	pair, err := m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	journalDir, minimumDir, backup := filepath.Join(root, "journal"), filepath.Join(root, "minimum"), filepath.Join(root, "backups")
	if err = os.Mkdir(backup, 0700); err != nil {
		t.Fatal(err)
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	installation := uuid.NewString()
	mc := privacy.FixtureMinimumConfig{Directory: minimumDir, JournalDirectory: journalDir, RestorableRoots: []string{backup}, InstallationID: installation, KeyID: "fixture-key-1", SigningKey: key}
	minimum, err := privacy.CreateFixtureMinimum(mc)
	if err != nil {
		t.Fatal(err)
	}
	defer minimum.Close()
	cfg := privacy.FixtureJournalConfig{Directory: journalDir, RestorableRoots: []string{backup}, InstallationID: installation, KeyID: "fixture-key-1", SigningKey: key, SelectorKey: bytes.Repeat([]byte{17}, 32), Minimum: minimum}
	journal, err := privacy.CreateFixtureJournal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDeletionManager(m, db, journal, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	id, secret := deletionTestEnrollment(t, d, pair.AccessToken)
	c := deletionTestCommand(t, d, pair.AccountID, id, secret)
	result, err := d.Confirm(t.Context(), c)
	if err != nil || result.Phase != "suppression_bound" {
		t.Fatal(result, err)
	}
	first, err := minimum.Load(t.Context())
	if err != nil || first.Sequence != 1 {
		t.Fatal("independent minimum not committed", first, err)
	}
	journal.Close()
	minimum.Close()
	minimum, err = privacy.OpenFixtureMinimum(mc)
	if err != nil {
		t.Fatal(err)
	}
	defer minimum.Close()
	cfg.Minimum = minimum
	journal, err = privacy.OpenFixtureJournal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	restarted, err := NewDeletionManager(m, db, journal, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result, err = restarted.Confirm(t.Context(), c)
	if err != nil || result.Phase != "suppression_bound" {
		t.Fatal("restart replay failed", result, err)
	}
	second, err := minimum.Load(t.Context())
	if err != nil || second != first {
		t.Fatal("restart duplicated journal", second, err)
	}
	var digest []byte
	var sequence int64
	if err = db.QueryRow(`SELECT suppression_sequence,suppression_sha256 FROM privacy_requests WHERE id=$1`, c.RequestID).Scan(&sequence, &digest); err != nil || sequence != first.Sequence || !bytes.Equal(digest, first.Digest[:]) {
		t.Fatal("DB receipt differs from independent authority", err)
	}
}

func TestDeletionLateWriteWaitCannotExtendAuthority(t *testing.T) {
	for _, operation := range []string{"enroll", "enroll-credential", "confirm"} {
		t.Run(operation, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			m := newTestManager(db)
			pair, err := m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
			if err != nil {
				t.Fatal(err)
			}
			d, err := NewDeletionManager(m, db, nil, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			var intentID string
			var run func() error
			if strings.HasPrefix(operation, "enroll") {
				intent, e := d.BeginEnrollment(t.Context(), pair.AccessToken)
				if e != nil {
					t.Fatal(e)
				}
				intentID = intent.ID
				id, secret := uuid.NewString(), deletionTestSecret(t)
				run = func() error { return d.EnrollCapability(t.Context(), intent.ID, intent.Secret, id, secret) }
			} else {
				id, secret := deletionTestEnrollment(t, d, pair.AccessToken)
				c := deletionTestCommand(t, d, pair.AccountID, id, secret)
				intentID = c.IntentID
				run = func() error { _, e := d.Confirm(t.Context(), c); return e }
			}
			expiryColumn := "expires_at"
			if operation == "enroll-credential" {
				expiryColumn = "credential_until"
			}
			if _, err = db.Exec(`UPDATE privacy_deletion_intents SET `+expiryColumn+`=clock_timestamp()+interval '400 milliseconds' WHERE id=$1`, intentID); err != nil {
				t.Fatal(err)
			}
			barrier, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer barrier.Rollback()
			table := "privacy_deletion_capabilities"
			if operation == "confirm" {
				table = "portal_browser_sessions"
			}
			if _, err = barrier.Exec(`LOCK TABLE ` + table + ` IN SHARE MODE`); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- run() }()
			deadline := time.Now().Add(3 * time.Second)
			for {
				var waiting bool
				if err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT public.privacy_%')`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("mutation never reached real SQL lock")
				}
				time.Sleep(time.Millisecond)
			}
			var expired bool
			for !expired {
				if err = db.QueryRow(`SELECT `+expiryColumn+`<=clock_timestamp() FROM privacy_deletion_intents WHERE id=$1`, intentID).Scan(&expired); err != nil {
					t.Fatal(err)
				}
				if !expired {
					time.Sleep(5 * time.Millisecond)
				}
			}
			if err = barrier.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err == nil {
				t.Fatal("late write wait extended deletion authority")
			}
			var deleted bool
			var epoch int64
			if err = db.QueryRow(`SELECT deleted_at IS NOT NULL,session_epoch FROM accounts WHERE id=$1`, pair.AccountID).Scan(&deleted, &epoch); err != nil || deleted || epoch != 0 {
				t.Fatal("failed authority mutated account", deleted, epoch, err)
			}
			var state string
			if err = db.QueryRow(`SELECT state FROM privacy_deletion_intents WHERE id=$1`, intentID).Scan(&state); err != nil || state != "pending" {
				t.Fatal("failed authority consumed intent", state, err)
			}
			var count int
			if err = db.QueryRow(`SELECT count(*) FROM privacy_requests WHERE account_id=$1`, pair.AccountID).Scan(&count); err != nil || count != 0 {
				t.Fatal("failed authority retained request", count, err)
			}
		})
	}
}

func TestDeletionPendingIntentBudgetBoundsAuthenticatedCreation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	pair, err := m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDeletionManager(m, db, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err = d.BeginEnrollment(t.Context(), pair.AccessToken); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = d.BeginEnrollment(t.Context(), pair.AccessToken); err == nil {
		t.Fatal("eleventh live intent exceeded durable account budget")
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM privacy_deletion_intents WHERE account_id=$1`, pair.AccountID).Scan(&count); err != nil || count != 10 {
		t.Fatal("capacity failure added intent", count, err)
	}
}

func TestDeletionConcurrentExactConfirmAndConflictRollback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	pair, err := m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDeletionManager(m, db, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	id, secret := deletionTestEnrollment(t, d, pair.AccessToken)
	command := deletionTestCommand(t, d, pair.AccountID, id, secret)
	outcomes := make(chan error, 12)
	for range 12 {
		go func() {
			result, e := d.Confirm(t.Context(), command)
			if e == nil && (result.RequestID != command.RequestID || result.Phase != "prepared") {
				e = errors.New("exact replay changed request")
			}
			outcomes <- e
		}()
	}
	for range 12 {
		if e := <-outcomes; e != nil {
			t.Fatal(e)
		}
	}
	var count, epoch int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM privacy_requests WHERE account_id=$1),session_epoch FROM accounts WHERE id=$1`, pair.AccountID).Scan(&count, &epoch); err != nil || count != 1 || epoch != 1 {
		t.Fatal("concurrent confirmation duplicated effects", count, epoch, err)
	}
	for _, change := range []func(*DeletionConfirmCommand){func(c *DeletionConfirmCommand) { c.RequestID = uuid.NewString() }, func(c *DeletionConfirmCommand) { c.StatusSecret = deletionTestSecret(t) }} {
		bad := command
		change(&bad)
		if _, err = d.Confirm(t.Context(), bad); err == nil {
			t.Fatal("changed retry accepted")
		}
	}
}

func TestDeletionFinalWriteFailureRollsBackEveryFence(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	pair, err := m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDeletionManager(m, db, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	id, secret := deletionTestEnrollment(t, d, pair.AccessToken)
	command := deletionTestCommand(t, d, pair.AccountID, id, secret)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	function, trigger := "fixture_delete_fault_"+suffix, "fixture_delete_fault_"+suffix
	if _, err = db.Exec(`CREATE FUNCTION public.` + function + `() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic final delete fault';END$$; CREATE TRIGGER ` + trigger + ` BEFORE DELETE ON portal_browser_sessions FOR EACH STATEMENT EXECUTE FUNCTION public.` + function + `() `); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := db.Exec(`DROP TRIGGER ` + trigger + ` ON portal_browser_sessions; DROP FUNCTION public.` + function + `() `); e != nil {
			t.Error(e)
		}
	}()
	if _, err = d.Confirm(t.Context(), command); err == nil {
		t.Fatal("injected final mutation succeeded")
	}
	var deleted bool
	var epoch, requests, fences int
	if err = db.QueryRow(`SELECT deleted_at IS NOT NULL,session_epoch,(SELECT count(*) FROM privacy_requests WHERE account_id=$1),(SELECT count(*) FROM account_deletion_fences WHERE account_id=$1) FROM accounts WHERE id=$1`, pair.AccountID).Scan(&deleted, &epoch, &requests, &fences); err != nil || deleted || epoch != 0 || requests != 0 || fences != 0 {
		t.Fatal("partial confirmation escaped rollback", deleted, epoch, requests, fences, err)
	}
	var state string
	if err = db.QueryRow(`SELECT state FROM privacy_deletion_intents WHERE id=$1`, command.IntentID).Scan(&state); err != nil || state != "pending" {
		t.Fatal("fault consumed proof", state, err)
	}
	if _, err = m.ValidateAccessToken(t.Context(), pair.AccessToken); err != nil {
		t.Fatal("fault revoked original session", err)
	}
}

func TestDeletionExpiredCleanupDoesNotHoldIntentWhileWaitingForAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	pair, err := m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDeletionManager(m, db, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := d.BeginEnrollment(t.Context(), pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE privacy_deletion_intents SET created_at=clock_timestamp()-interval '2 seconds',expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, expired.ID); err != nil {
		t.Fatal(err)
	}
	claims, err := m.parseToken(pair.AccessToken, TokenAccess)
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err = blocker.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, pair.AccountID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		var got string
		e := db.QueryRow(`SELECT public.privacy_begin_enrollment($1,$2,$3,$4,$5,$6,$7,$8)`, uuid.NewString(), pair.AccountID, bytes.Repeat([]byte{20}, 32), time.Now().Add(time.Minute), claims.SessionEpoch, claims.ID, claims.ExpiresAt.Time, claims.DeviceHash).Scan(&got)
		done <- e
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting bool
		if err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT public.privacy_begin_enrollment%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("begin never waited for account")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err = blocker.Exec(`SET LOCAL lock_timeout='150ms'`); err != nil {
		t.Fatal(err)
	}
	_, lockErr := blocker.Exec(`SELECT id FROM privacy_deletion_intents WHERE id=$1 FOR UPDATE`, expired.ID)
	if err = blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if lockErr != nil {
		t.Fatal("begin retained intent lock while waiting for account", lockErr)
	}
}

func TestDeletionIdentifiersRejectUnpublishableZeroRequest(t *testing.T) {
	if deletionID(uuid.Nil.String()) {
		t.Fatal("zero UUID accepted although suppression authority refuses it")
	}
	if !deletionID(uuid.NewString()) {
		t.Fatal("canonical nonzero request rejected")
	}
}
