package admin

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/notices"
	"github.com/knowoff/knowoff/server/internal/portal"
	"github.com/knowoff/knowoff/server/internal/profile"
	"github.com/knowoff/knowoff/server/internal/reports"
	"github.com/knowoff/knowoff/server/internal/store"
)

// These fixtures exercise database authority only; no fixture is a release or
// editorial approval. Their sessions and isolated database are disposable.
func mutationAdmin(t *testing.T, db *sql.DB) (*Manager, string, string, string, string) {
	t.Helper()
	m := NewManager(db, testConfig(), nil)
	account := newAccount(t, db)
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
	return m, a.ID, account, sid, csrf
}

func mustMutationSQL(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestAdminMutationRejectsSessionExpiryDuringDomainWait(t *testing.T) {
	for _, kind := range []string{"portal_terms", "user_terms", "application_approve", "application_reject", "role_grant", "role_revoke", "report_resolve", "report_triage", "archive_start", "archive_batch", "notice_withdraw", "capture", "capture_replay", "release_takedown", "guard_dismiss", "challenge_topic", "challenge_entry", "challenge_close"} {
		t.Run(kind, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			m, actor, _, sid, csrf := mutationAdmin(t, db)
			ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
			defer cancel()
			target := newAccount(t, db)
			id := uuid.NewString()
			cfg := testConfig()
			cfg.Tuning.Contract.MaxTextBytes = 200
			cfg.Tuning.Portal.MaxTextSubmissionLength = 200
			pm := portal.NewManager(portal.Deps{DB: db, Config: cfg, Admin: m, Screener: mutationScreenFunc(func(context.Context, string) error { return nil })})
			var lockSQL, waitSQL, effectSQL string
			var lockArgs, effectArgs []any
			var run func(context.Context) error
			switch kind {
			case "portal_terms":
				lockSQL = `LOCK TABLE portal_terms IN EXCLUSIVE MODE`
				waitSQL = "INSERT INTO portal_terms"
				effectSQL = `SELECT EXISTS(SELECT 1 FROM portal_terms WHERE version=$1)`
				effectArgs = []any{id}
				run = func(ctx context.Context) error {
					return pm.CreateTermsVersion(ctx, actor, id, "Synthetic terms", "Synthetic fixture wording", time.Now())
				}
			case "user_terms":
				lockSQL = `LOCK TABLE user_terms_versions IN EXCLUSIVE MODE`
				waitSQL = "INSERT INTO user_terms_versions"
				effectSQL = `SELECT EXISTS(SELECT 1 FROM user_terms_versions WHERE version=$1)`
				effectArgs = []any{id}
				run = func(ctx context.Context) error {
					return store.NewTextTrustStore(db).PublishTerms(ctx, actor, id, "Synthetic fixture wording", time.Now())
				}
			case "application_approve", "application_reject":
				mustMutationSQL(t, db, `INSERT INTO portal_role_applications(id,account_id,role,status) VALUES($1,$2,'contributor','pending')`, id, target)
				lockSQL = `SELECT id FROM portal_role_applications WHERE id=$1 FOR UPDATE`
				lockArgs = []any{id}
				waitSQL = "portal_role_applications"
				effectSQL = `SELECT status<>'pending' FROM portal_role_applications WHERE id=$1`
				effectArgs = []any{id}
				run = func(ctx context.Context) error {
					if kind == "application_approve" {
						return pm.ApproveApplication(ctx, actor, id)
					}
					return pm.RejectApplication(ctx, actor, id, "Reviewed fixture")
				}
			case "role_grant", "role_revoke":
				if kind == "role_revoke" {
					if err := pm.GrantRole(ctx, actor, target, portal.RoleContributor); err != nil {
						t.Fatal(err)
					}
				}
				lockSQL = `LOCK TABLE portal_roles IN EXCLUSIVE MODE`
				waitSQL = "portal_roles"
				effectSQL = `SELECT EXISTS(SELECT 1 FROM portal_roles WHERE account_id=$1 AND role='contributor' AND revoked_at IS NULL)`
				if kind == "role_revoke" {
					effectSQL = `SELECT revoked_at IS NOT NULL FROM portal_roles WHERE account_id=$1 AND role='contributor'`
				}
				effectArgs = []any{target}
				run = func(ctx context.Context) error {
					if kind == "role_grant" {
						return pm.GrantRole(ctx, actor, target, portal.RoleContributor)
					}
					return pm.RevokeRole(ctx, actor, target, portal.RoleContributor)
				}
			case "report_resolve", "report_triage":
				mustMutationSQL(t, db, `INSERT INTO report_cases(id,case_key,kind,target_account_id) VALUES($1,$3,'conduct',$2)`, id, target, id)
				lockSQL = `SELECT id FROM report_cases WHERE id=$1 FOR UPDATE`
				lockArgs = []any{id}
				waitSQL = "report_cases"
				effectSQL = `SELECT status<>'new' FROM report_cases WHERE id=$1`
				effectArgs = []any{id}
				run = func(ctx context.Context) error {
					if kind == "report_resolve" {
						return reports.NewManager(db).ResolveCase(ctx, actor, id, "dismissed", "Synthetic reviewed case", nil)
					}
					return m.Triage(ctx, actor, "cases", id, "in_review")
				}
			case "archive_start":
				lockSQL = `LOCK TABLE portal_submissions IN ROW EXCLUSIVE MODE`
				waitSQL = "LOCK TABLE portal_submissions,challenge_entries"
				effectSQL = `SELECT EXISTS(SELECT 1 FROM text_archive_progress)`
				run = func(ctx context.Context) error { return store.NewTextArchiveStore(db).StartAs(ctx, actor, 2) }
			case "archive_batch":
				archive := store.NewTextArchiveStore(db)
				if err := archive.Start(ctx, 2); err != nil {
					t.Fatal(err)
				}
				lockSQL = `SELECT job_id FROM text_archive_progress WHERE job_id=1 FOR UPDATE`
				waitSQL = "text_archive_progress WHERE job_id=1 FOR UPDATE"
				effectSQL = `SELECT phase<>'copy' FROM text_archive_progress WHERE job_id=1`
				run = func(ctx context.Context) error { _, err := archive.BatchAs(ctx, actor, 2); return err }
			case "guard_dismiss":
				mustMutationSQL(t, db, `INSERT INTO guard_freezes(id,account_id,frozen_by,reason,frozen_at,expires_at) VALUES($1,$2,$3,'Synthetic fixture',now(),now()+interval '1 hour')`, id, target, actor)
				lockSQL = `SELECT id FROM guard_freezes WHERE id=$1 FOR UPDATE`
				lockArgs = []any{id}
				waitSQL = "guard_freezes WHERE id="
				effectSQL = `SELECT dismissed_at IS NOT NULL FROM guard_freezes WHERE id=$1`
				effectArgs = []any{id}
				run = func(ctx context.Context) error { return pm.DismissFreeze(ctx, actor, id) }
			case "capture", "capture_replay", "challenge_topic", "challenge_entry", "challenge_close":
				source := uuid.NewString()
				mustMutationSQL(t, db, `INSERT INTO portal_terms(version,title,body,active_from) VALUES('synthetic-v1','Synthetic fixture terms','Explicit synthetic fixture wording',now()-interval '1 day')`)
				mustMutationSQL(t, db, `INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at,decided_at,decided_by) VALUES($1,$2,'text','Synthetic approved source','approved','synthetic-v1',now()-interval '1 day',now(),$3)`, source, target, actor)
				day := time.Now().UTC().Truncate(24 * time.Hour)
				monday := day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
				if kind == "capture" || kind == "capture_replay" || kind == "challenge_topic" {
					lockSQL = `SELECT id FROM portal_submissions WHERE id=$1 FOR UPDATE`
					lockArgs = []any{source}
					waitSQL = "portal_submissions"
					if kind == "capture" || kind == "capture_replay" {
						effectSQL = `SELECT EXISTS(SELECT 1 FROM text_accepted_inputs WHERE source_id=$1)`
						effectArgs = []any{source}
						releases := store.NewTextReleaseStore(db, cfg.Tuning, func(context.Context, string) error { return nil })
						if kind == "capture_replay" {
							if _, err := releases.CaptureAccepted(ctx, actor, "portal_submission", source); err != nil {
								t.Fatal(err)
							}
							effectSQL = `SELECT count(*)<>1 FROM text_accepted_inputs WHERE source_id=$1`
						}
						run = func(ctx context.Context) error {
							_, err := releases.CaptureAccepted(ctx, actor, "portal_submission", source)
							return err
						}
					} else {
						effectSQL = `SELECT EXISTS(SELECT 1 FROM challenge_topics WHERE nown_media_id=$1)`
						effectArgs = []any{source}
						run = func(ctx context.Context) error {
							_, err := pm.CreateChallengeTopic(ctx, actor, monday, source)
							return err
						}
					}
				} else {
					topic, err := pm.CreateChallengeTopic(ctx, actor, monday, source)
					if err != nil {
						t.Fatal(err)
					}
					if kind == "challenge_entry" {
						mustMutationSQL(t, db, `INSERT INTO challenge_entries(id,account_id,topic_id,entry_type,content,status,terms_version,terms_accepted_at) VALUES($1,$2,$3,'text','Synthetic submitted entry','submitted','synthetic-v1',now())`, id, target, topic.ID)
						lockSQL = `SELECT id FROM challenge_entries WHERE id=$1 FOR UPDATE`
						lockArgs = []any{id}
						waitSQL = "challenge_entries WHERE id="
						effectSQL = `SELECT status<>'submitted' FROM challenge_entries WHERE id=$1`
						effectArgs = []any{id}
						run = func(ctx context.Context) error { return pm.ApproveChallengeEntry(ctx, actor, id) }
					} else {
						lockSQL = `LOCK TABLE admin_audit_log IN EXCLUSIVE MODE`
						waitSQL = "INSERT INTO admin_audit_log"
						effectSQL = `SELECT closed_at IS NOT NULL FROM challenge_topics WHERE id=$1`
						effectArgs = []any{topic.ID}
						run = func(ctx context.Context) error { _, err := pm.CloseChallengeWeek(ctx, actor, topic.ID); return err }
					}
				}
			case "release_takedown":
				// Raw row only exercises administrative withdrawal; it is never loaded,
				// certified, activated or advertised as a playable content release.
				mustMutationSQL(t, db, `INSERT INTO text_releases(release_id,language,rules_version,manifest_sha256,snapshot_sha256,bundle,access_class,entitlement_key,published_by) VALUES($1,'en','text-v1',repeat('a',64),repeat('b',64),'{}','core','',$2)`, id, actor)
				lockSQL = `LOCK TABLE text_releases IN SHARE ROW EXCLUSIVE MODE`
				waitSQL = "LOCK TABLE text_releases"
				effectSQL = `SELECT withdrawn_at IS NOT NULL FROM text_releases WHERE release_id=$1`
				effectArgs = []any{id}
				run = func(ctx context.Context) error {
					return store.NewTextReleaseStore(db, cfg.Tuning, nil).Takedown(ctx, actor, id, "Synthetic withdrawal")
				}
			case "notice_withdraw":
				nm := notices.NewManager(db, testConfig(), nil)
				note, err := nm.CreateNotice(ctx, notices.Notice{Type: notices.NoticeAnnouncement, Title: map[string]string{"en": "Synthetic notice"}, Body: map[string]string{"en": "Synthetic fixture"}})
				if err != nil {
					t.Fatal(err)
				}
				lockSQL = `SELECT id FROM system_notices WHERE id=$1 FOR UPDATE`
				lockArgs = []any{note}
				waitSQL = "system_notices WHERE id="
				effectSQL = `SELECT withdrawn_at IS NOT NULL FROM system_notices WHERE id=$1`
				effectArgs = []any{note}
				run = func(ctx context.Context) error { return nm.WithdrawNotice(ctx, note, actor) }
			}
			var initialAudits int
			if err := db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE admin_id=$1`, actor).Scan(&initialAudits); err != nil {
				t.Fatal(err)
			}
			barrier, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer barrier.Rollback()
			if _, err := barrier.ExecContext(ctx, lockSQL, lockArgs...); err != nil {
				t.Fatal(err)
			}
			mustMutationSQL(t, db, `UPDATE admin_sessions SET expires_at=clock_timestamp()+interval '1 second' WHERE id=$1`, sid)
			var checked atomic.Int32
			bound := store.WithAdminAuthorization(ctx, actor, func(ctx context.Context, tx *sql.Tx) (string, error) {
				actor, err := m.AuthorizeSessionTx(ctx, tx, sid, csrf)
				if err == nil {
					checked.Add(1)
				}
				return actor, err
			})
			result := make(chan error, 1)
			go func() { result <- run(bound) }()
			waitAdminSessionLock(t, ctx, db, waitSQL)
			// An early authority check is mandatory, including batch and notice paths.
			if checked.Load() == 0 {
				t.Error("domain lock preceded initiating-session authority")
			}
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				var expired bool
				if err := db.QueryRowContext(ctx, `SELECT clock_timestamp()>=expires_at FROM admin_sessions WHERE id=$1`, sid).Scan(&expired); err != nil {
					t.Fatal(err)
				}
				if expired {
					break
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			if err := barrier.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-result; err == nil {
				t.Error("session expired behind domain lock but mutation returned success")
			}
			var effect bool
			if err := db.QueryRowContext(ctx, effectSQL, effectArgs...).Scan(&effect); err != nil {
				t.Fatal(err)
			}
			if effect {
				t.Error("expired session committed domain effects")
			}
			var audits int
			if err := db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE admin_id=$1`, actor).Scan(&audits); err != nil {
				t.Fatal(err)
			}
			if audits != initialAudits {
				t.Error("expired session committed success audit")
			}
		})
	}
}

func TestNoticeAndAvatarMutationsRequireInitiatingActor(t *testing.T) {
	for _, kind := range []string{"notice_create", "notice_withdraw", "avatar"} {
		for _, failure := range []string{"logout", "omitted_actor", "mismatched_actor", "audit_failure"} {
			t.Run(kind+"/"+failure, func(t *testing.T) {
				db := setupTestDB(t)
				defer db.Close()
				m, actor, _, sid, csrf := mutationAdmin(t, db)
				otherM, other, _, _, _ := mutationAdmin(t, db)
				_ = otherM
				ctx := store.WithAdminAuthorization(t.Context(), actor, func(ctx context.Context, tx *sql.Tx) (string, error) { return m.AuthorizeSessionTx(ctx, tx, sid, csrf) })
				named := actor
				if failure == "omitted_actor" {
					named = ""
				}
				if failure == "mismatched_actor" {
					named = other
				}
				target := newAccount(t, db)
				signals := &mutationNoticeSignals{}
				nm := notices.NewManager(db, testConfig(), signals)
				notified := make(chan struct{}, 1)
				nm.SetChangeNotifier(func(context.Context) error {
					signals.broadcasts.Add(1)
					select {
					case notified <- struct{}{}:
					default:
					}
					return nil
				})
				note, err := nm.CreateNotice(t.Context(), notices.Notice{Type: notices.NoticeAnnouncement, Title: map[string]string{"en": "Existing notice"}, Body: map[string]string{"en": "Existing synthetic fixture"}})
				if err != nil {
					t.Fatal(err)
				}
				// Establish the committed fixture's actual text invalidation before
				// checking that rejected mutations emit nothing further.
				pollCtx, stopPoll := context.WithCancel(t.Context())
				pollDone := make(chan struct{})
				pollErrors := make(chan error, 1)
				go func() {
					defer close(pollDone)
					nm.Run(pollCtx, func(err error) {
						select {
						case pollErrors <- err:
						default:
						}
					})
				}()
				select {
				case <-notified:
				case err := <-pollErrors:
					stopPoll()
					<-pollDone
					t.Fatal(err)
				case <-time.After(5 * time.Second):
					stopPoll()
					<-pollDone
					t.Fatal("initial notice invalidation missing")
				}
				stopPoll()
				<-pollDone
				mustMutationSQL(t, db, `INSERT INTO custom_avatars(account_id,blob,content_type) VALUES($1,$2,'image/png')`, target, []byte("synthetic-image"))
				mustMutationSQL(t, db, `INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'custom_avatar','unlocked')`, target)
				if failure == "logout" {
					if err := m.DestroySession(t.Context(), sid); err != nil {
						t.Fatal(err)
					}
				}
				if failure == "audit_failure" {
					mustMutationSQL(t, db, `CREATE FUNCTION reject_mutation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit failure'; END $$`)
					mustMutationSQL(t, db, `CREATE TRIGGER reject_mutation_audit BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION reject_mutation_audit()`)
					defer func() {
						mustMutationSQL(t, db, `DROP TRIGGER reject_mutation_audit ON admin_audit_log`)
						mustMutationSQL(t, db, `DROP FUNCTION reject_mutation_audit()`)
					}()
				}
				switch kind {
				case "notice_create":
					start := time.Now().UTC().Add(-time.Minute)
					n := notices.Notice{Type: notices.NoticeMaintenance, MaintenanceStart: &start, MaintenanceDurationMin: 60, Title: map[string]string{"en": "Unauthorized notice"}, Body: map[string]string{"en": "Synthetic fixture"}}
					if named != "" {
						id := uuid.MustParse(named)
						n.CreatedBy = &id
					}
					_, err = nm.CreateNotice(ctx, n)
				case "notice_withdraw":
					if named == "" {
						err = nm.WithdrawNotice(ctx, note)
					} else {
						err = nm.WithdrawNotice(ctx, note, named)
					}
				case "avatar":
					ctx = context.WithValue(ctx, ctxAdminIDKey{}, named)
					r := httptest.NewRequest(http.MethodPost, "/admin/avatars/"+target+"/takedown", nil).WithContext(ctx)
					r.SetPathValue("account_id", target)
					w := httptest.NewRecorder()
					m.avatarTakedown(w, r)
					if w.Code == http.StatusSeeOther {
						err = nil
					} else {
						err = fmt.Errorf("refused: %d", w.Code)
					}
				}
				if err == nil {
					t.Error("unsafe initiating actor accepted")
				}
				if signals.broadcasts.Load() != 1 {
					t.Error("unauthorized notice broadcast")
				}
				if err := nm.MarkMaintenanceDrain(t.Context()); err != nil {
					t.Fatal(err)
				}
				if !signals.ready.Load() {
					t.Error("unauthorized maintenance paused admission")
				}
				var count int
				if err := db.QueryRow(`SELECT count(*) FROM system_notices`).Scan(&count); err != nil || count != 1 {
					t.Error("notice mutation escaped rollback", count, err)
				}
				var withdrawn bool
				if err := db.QueryRow(`SELECT withdrawn_at IS NOT NULL FROM system_notices WHERE id=$1`, note).Scan(&withdrawn); err != nil || withdrawn {
					t.Error("notice withdrawn by stale actor", err)
				}
				if err := db.QueryRow(`SELECT count(*) FROM custom_avatars WHERE account_id=$1`, target).Scan(&count); err != nil || count != 1 {
					t.Error("avatar mutation escaped rollback", count, err)
				}
				if err := db.QueryRow(`SELECT count(*) FROM entitlements WHERE account_id=$1 AND entitlement_type='custom_avatar'`, target).Scan(&count); err != nil || count != 1 {
					t.Error("paid entitlement changed", count, err)
				}
			})
		}
	}
}

type mutationScreenFunc func(context.Context, string) error

func (f mutationScreenFunc) ScreenText(ctx context.Context, text string) error { return f(ctx, text) }

func TestContributionDecisionRechecksActorAfterScreening(t *testing.T) {
	for _, approve := range []bool{true, false} {
		for _, change := range []string{"none", "logout", "ban", "role_loss", "revoke_sessions", "csrf_rotation", "audit_failure"} {
			t.Run(fmt.Sprintf("approve=%v/%s", approve, change), func(t *testing.T) {
				db := setupTestDB(t)
				defer db.Close()
				m, actor, account, sid, csrf := mutationAdmin(t, db)
				ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
				defer cancel()
				cfg := testConfig()
				cfg.Tuning.Contract.MaxTextBytes = 200
				cfg.Tuning.Noin.ContributorAcceptedAsset = 7
				contributor := newAccount(t, db)
				id := uuid.NewString()
				mustMutationSQL(t, db, `INSERT INTO portal_terms(version,title,body,active_from) VALUES('synthetic-v1','Synthetic fixture terms','Explicit synthetic fixture wording',now()-interval '1 day')`)
				mustMutationSQL(t, db, `INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at) VALUES($1,$2,'text','Synthetic contribution fixture','submitted','synthetic-v1',now())`, id, contributor)
				mustMutationSQL(t, db, `INSERT INTO noin_wallets(account_id,balance) VALUES($1,11)`, contributor)
				entered, resume := make(chan struct{}), make(chan struct{})
				var once sync.Once
				defer once.Do(func() { close(resume) })
				screen := mutationScreenFunc(func(ctx context.Context, text string) error {
					if text != "Synthetic contribution fixture" {
						return fmt.Errorf("unexpected screened text")
					}
					close(entered)
					select {
					case <-resume:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
				pm := portal.NewManager(portal.Deps{DB: db, Config: cfg, Admin: m, Economy: economy.NewManager(db, cfg), Profile: profile.NewManager(db), Screener: screen})
				bound := store.WithAdminAuthorization(ctx, actor, func(ctx context.Context, tx *sql.Tx) (string, error) { return m.AuthorizeSessionTx(ctx, tx, sid, csrf) })
				result := make(chan error, 1)
				decide := func() {
					result <- pm.DecideSubmission(bound, actor, id, approve, "Reviewed synthetic fixture", portal.ContentRevision("Synthetic contribution fixture"))
				}
				if approve {
					go decide()
					select {
					case <-entered:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					// Provider I/O must not hold any authority locks.
					tx, err := db.BeginTx(ctx, nil)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = tx.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE NOWAIT`, account); err != nil {
						tx.Rollback()
						t.Fatal("provider screening held actor lock", err)
					}
					if _, err = tx.ExecContext(ctx, `SELECT id FROM admin_sessions WHERE id=$1 FOR UPDATE NOWAIT`, sid); err != nil {
						tx.Rollback()
						t.Fatal("provider screening held session lock", err)
					}
					if err = tx.Commit(); err != nil {
						t.Fatal(err)
					}
				}
				switch change {
				case "logout":
					if err := m.DestroySession(ctx, sid); err != nil {
						t.Fatal(err)
					}
				case "ban":
					mustMutationSQL(t, db, `UPDATE accounts SET banned_at=now() WHERE id=$1`, account)
				case "role_loss":
					mustMutationSQL(t, db, `UPDATE admin_accounts SET role='viewer' WHERE id=$1`, actor)
				case "revoke_sessions":
					am := auth.NewManager(db, []byte("test-key-32-bytes-long-for-hs256!!"), "test", "test", time.Hour, time.Hour, auth.OAuthProviders{})
					if err := am.RevokeSessions(ctx, account); err != nil {
						t.Fatal(err)
					}
				case "csrf_rotation":
					mustMutationSQL(t, db, `UPDATE admin_sessions SET csrf_token=$2 WHERE id=$1`, sid, uuid.NewString())
				case "audit_failure":
					mustMutationSQL(t, db, `CREATE FUNCTION reject_contribution_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit failure'; END $$`)
					mustMutationSQL(t, db, `CREATE TRIGGER reject_contribution_audit BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION reject_contribution_audit()`)
					defer func() {
						mustMutationSQL(t, db, `DROP TRIGGER reject_contribution_audit ON admin_audit_log`)
						mustMutationSQL(t, db, `DROP FUNCTION reject_contribution_audit()`)
					}()
				}
				if approve {
					once.Do(func() { close(resume) })
				} else {
					go decide()
				}
				err := <-result
				if (err == nil) != (change == "none") {
					t.Errorf("decision error=%v expected success=%v", err, change == "none")
				}
				if change == "none" {
					if err := pm.DecideSubmission(bound, actor, id, approve, "Reviewed synthetic fixture"); err == nil {
						t.Error("already decided submission accepted again")
					}
				}
				var status string
				var balance, credits, ledger, audits int
				if err := db.QueryRow(`SELECT status FROM portal_submissions WHERE id=$1`, id).Scan(&status); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow(`SELECT balance FROM noin_wallets WHERE account_id=$1`, contributor).Scan(&balance); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow(`SELECT cardinality(contributor_credits) FROM profiles WHERE account_id=$1`, contributor).Scan(&credits); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1`, contributor).Scan(&ledger); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE admin_id=$1`, actor).Scan(&audits); err != nil {
					t.Fatal(err)
				}
				wantStatus, wantBalance, wantCredits, wantAudits := "submitted", 11, 0, 0
				if change == "none" {
					wantStatus = "rejected"
					wantAudits = 1
					if approve {
						wantStatus = "approved"
						wantBalance = 18
						wantCredits = 1
					}
				}
				if status != wantStatus || balance != wantBalance || credits != wantCredits || ledger != wantCredits || audits != wantAudits {
					t.Errorf("decision leaked effects: status=%s balance=%d credits=%d ledger=%d audits=%d", status, balance, credits, ledger, audits)
				}
			})
		}
	}
}

type mutationNoticeSignals struct {
	broadcasts atomic.Int32
	ready      atomic.Bool
}

func (s *mutationNoticeSignals) SetReady(ready bool) { s.ready.Store(ready) }

func TestBrowserBoundArchiveCannotUseUnnamedWorkerPath(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprint(batch), func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			m, actor, _, sid, csrf := mutationAdmin(t, db)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			archive := store.NewTextArchiveStore(db)
			if batch {
				if err := archive.Start(ctx, 2); err != nil {
					t.Fatal(err)
				}
			}
			bound := store.WithAdminAuthorization(ctx, actor, func(ctx context.Context, tx *sql.Tx) (string, error) { return m.AuthorizeSessionTx(ctx, tx, sid, csrf) })
			var err error
			if batch {
				_, err = archive.Batch(bound, 2)
			} else {
				err = archive.Start(bound, 2)
			}
			if err == nil {
				t.Error("browser-bound request bypassed actor proof through worker API")
			}
			var progress int
			if err := db.QueryRow(`SELECT count(*) FROM text_archive_progress WHERE phase='copy'`).Scan(&progress); err != nil {
				t.Fatal(err)
			}
			want := 0
			if batch {
				want = 1
			}
			if progress != want {
				t.Error("unauthorized archive request changed progress")
			}
		})
	}
}

func TestAvatarTakedownLocksTargetAndAuditsAtomically(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m, actor, _, sid, csrf := mutationAdmin(t, db)
	target := newAccount(t, db)
	mustMutationSQL(t, db, `INSERT INTO custom_avatars(account_id,blob,content_type) VALUES($1,$2,'image/png')`, target, []byte("synthetic-image"))
	mustMutationSQL(t, db, `INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'custom_avatar','unlocked')`, target)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	barrier, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback()
	if _, err := barrier.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, target); err != nil {
		t.Fatal(err)
	}
	bound := store.WithAdminAuthorization(ctx, actor, func(ctx context.Context, tx *sql.Tx) (string, error) { return m.AuthorizeSessionTx(ctx, tx, sid, csrf) })
	result := make(chan error, 1)
	go func() { result <- m.TakedownAvatar(bound, actor, target) }()
	waitAdminSessionLock(t, ctx, db, "accounts WHERE id=")
	var avatars, entitlements int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM custom_avatars WHERE account_id=$1),(SELECT count(*) FROM entitlements WHERE account_id=$1 AND entitlement_type='custom_avatar')`, target).Scan(&avatars, &entitlements); err != nil {
		t.Fatal(err)
	}
	if avatars != 1 || entitlements != 1 {
		t.Fatal("takedown changed target before account lock")
	}
	if err := barrier.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	var audits int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM custom_avatars WHERE account_id=$1),(SELECT count(*) FROM entitlements WHERE account_id=$1 AND entitlement_type='custom_avatar'),(SELECT count(*) FROM admin_audit_log WHERE admin_id=$2 AND action='avatar_takedown' AND target_id=$1::text)`, target, actor).Scan(&avatars, &entitlements, &audits); err != nil {
		t.Fatal(err)
	}
	if avatars != 0 || entitlements != 1 || audits != 1 {
		t.Fatalf("takedown avatar=%d entitlement=%d audits=%d", avatars, entitlements, audits)
	}
}

func TestAvatarTakedownRetainsUnlockAndFencesInFlightUpload(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m, actor, _, sid, csrf := mutationAdmin(t, db)
	target := newAccount(t, db)
	mustMutationSQL(t, db, `UPDATE accounts SET avatar='custom' WHERE id=$1`, target)
	mustMutationSQL(t, db, `INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'custom_avatar','unlocked')`, target)
	bound := store.WithAdminAuthorization(t.Context(), actor, func(ctx context.Context, tx *sql.Tx) (string, error) { return m.AuthorizeSessionTx(ctx, tx, sid, csrf) })
	if err := m.TakedownAvatar(bound, actor, target); err != nil {
		t.Fatal(err)
	}
	var selected string
	var revision int64
	var owned, ledgers int
	if err := db.QueryRow(`SELECT avatar,avatar_revision,(SELECT count(*) FROM entitlements WHERE account_id=$1 AND entitlement_type='custom_avatar'),(SELECT count(*) FROM noin_ledger WHERE account_id=$1) FROM accounts WHERE id=$1`, target).Scan(&selected, &revision, &owned, &ledgers); err != nil {
		t.Fatal(err)
	}
	if selected != "default" || revision != 1 || owned != 1 || ledgers != 0 {
		t.Fatalf("takedown=%s/%d owned=%d ledger=%d", selected, revision, owned, ledgers)
	}
}
