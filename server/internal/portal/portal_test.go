package portal

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/profile"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/lib/pq"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return setupTestDBWithTerms(t, true)
}

func setupTestDBWithTerms(t *testing.T, seedTerms bool) *sql.DB {
	t.Helper()
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	token := os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	u, parseErr := url.Parse(dsn)
	if parseErr != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(token) || u == nil || u.Scheme != "postgres" || u.Path != "/knowoff_test_"+token || (u.Hostname() != "postgres" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.Fragment != "" {
		t.Fatal("disposable runner PostgreSQL target required")
	}
	for key := range u.Query() {
		if key != "sslmode" {
			t.Fatal("unexpected disposable DSN parameter")
		}
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("disposable postgres unavailable: %v", err)
	}
	var actual string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&actual); err != nil || actual != "knowoff_test_"+token {
		db.Close()
		t.Fatal("refusing non-disposable database")
	}
	// Identity was checked above. Rebuild this disposable fixture rather than
	// weakening production consent/value guards to truncate retained history.
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset disposable portal schema: %v", err)
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if seedTerms {
		if _, err := db.Exec(`INSERT INTO portal_terms(version,title,body,active_from) VALUES('v1','Terms','Terms body',now())`); err != nil {
			t.Fatalf("seed terms: %v", err)
		}
	}
	return db
}

func TestValidMediaType_ImageAndTextOnly(t *testing.T) {
	for typ, valid := range map[string]bool{"text": true, "image": true, "gif": false, "video": false, "": false} {
		if got := ValidMediaType(typ); got != valid {
			t.Errorf("ValidMediaType(%q)=%v, want %v", typ, got, valid)
		}
	}
}

func TestDatabaseMediaTypes_ImageAndTextOnly(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	topicID := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO challenge_topics(id,week_start,week_end,nown_media_id) VALUES($1,'2026-09-07','2026-09-13',$2)`, topicID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"text", "image", "gif", "video"} {
		accountID := newAccount(t, db)
		for _, query := range []string{
			`INSERT INTO portal_submissions(account_id,media_type,content,terms_version,terms_accepted_at) VALUES($1,$2,'format test','v1',now())`,
			`INSERT INTO challenge_entries(account_id,entry_type,content,terms_version,terms_accepted_at,topic_id) VALUES($1,$2,'format test','v1',now(),'` + topicID + `')`,
		} {
			_, err := db.Exec(query, accountID, typ)
			if typ == "text" || typ == "image" {
				if err != nil {
					t.Fatalf("supported media type %s: %v", typ, err)
				}
			} else {
				var pgErr *pq.Error
				if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
					t.Fatalf("type %s needs a check violation, got %v", typ, err)
				}
			}
		}
	}
}

func newAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uuid.New().String()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO accounts (id, nickname) VALUES ($1, $2)`, id, "test-"+id[:8],
	); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO profiles (account_id) VALUES ($1)`, id,
	); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_terms_versions(version,body,active_from) VALUES('synthetic-user-terms-v1','Synthetic fixture user terms',now()-interval '1 day') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_terms_acceptances(account_id,version,accepted_at) VALUES($1,'synthetic-user-terms-v1',now())`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newAdmin(t *testing.T, db *sql.DB) string {
	t.Helper()
	acctID := newAccount(t, db)
	adminID := uuid.New().String()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO admin_accounts (id, account_id, email, password_hash, totp_secret, backup_codes, role) VALUES ($1, $2, $3, $4, $5, $6, 'admin')`,
		adminID, acctID, "admin-"+adminID[:8]+"@test.local", "hash", "secret", `{}`,
	); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	return adminID
}

type fakeAudit struct{}

func (fakeAudit) LogAction(ctx context.Context, adminID, action, entityType, entityID string, before, after map[string]any) error {
	return nil
}

type fakeAuth struct{}

func (fakeAuth) ValidateAccessToken(ctx context.Context, token string) (string, error) {
	return token, nil
}
func (fakeAuth) RevokeAccount(ctx context.Context, accountID string) error { return nil }

func (fakeAuth) ValidateAccessTokenTx(ctx context.Context, tx *sql.Tx, token string) (string, error) {
	if err := lockPortalAccounts(ctx, tx, token); err != nil {
		return "", err
	}
	return token, nil
}

func (f fakeAuth) ValidateAccessBindingTx(ctx context.Context, tx *sql.Tx, token string) (auth.AccessBinding, error) {
	account, err := f.ValidateAccessTokenTx(ctx, tx, token)
	if err != nil {
		return auth.AccessBinding{}, err
	}
	hash := "fixture-" + account
	if _, err = tx.ExecContext(ctx, `INSERT INTO auth_installations(device_hash) VALUES($1) ON CONFLICT DO NOTHING`, hash); err != nil {
		return auth.AccessBinding{}, err
	}
	return auth.AccessBinding{AccountID: account, DeviceHash: hash}, nil
}

var _ AuthClient = fakeAuth{}

func testConfig() *config.Config {
	return &config.Config{
		Trust: config.TrustConfig{UserTermsVersion: "synthetic-user-terms-v1"},
		Tuning: config.TuningConfig{
			Contract: config.ContractTuning{MaxTextBytes: 2000},
			Portal: config.PortalTuning{
				MinAccountLevelToApply:          1,
				SubmissionsPerContributorPerDay: 10,
				GuardFreezeMaxH:                 48,
				MaxTextSubmissionLength:         2000,
				TermsVersion:                    "v1",
			},
			Hand: config.HandTuning{
				Size:     5,
				DrawPile: 3,
			},
			Dealing: config.DealingTuning{
				MinHighPerNown:    2,
				MinDistantPerNown: 2,
			},
			Noin: config.NoinTuning{
				ChallengeWinner:          1000,
				ContributorAcceptedAsset: 100,
			},
			Liquidity: config.LiquidityTuning{
				NoinMinHumans: 2,
			},
			LiveOps: config.LiveOpsTuning{
				ChallengeMaxEntries:     100,
				ChallengeVotesPerPlayer: 1,
			},
			Progression: config.ProgressionTuning{},
		},
	}
}

func newTestManager(t *testing.T, db *sql.DB) *Manager {
	t.Helper()
	cfg := testConfig()
	pm := profile.NewManager(db)
	em := economy.NewManager(db, cfg)
	m := NewManager(Deps{
		Screener: acceptingTextScreener{},
		DB:       db,
		Config:   cfg,
		Auth:     fakeAuth{},
		Profile:  pm,
		Economy:  em,
		Admin:    fakeAudit{},
	})
	m.SetAccountDisconnect(func(context.Context, string) error { return nil })
	return m
}

func TestApplyForRole_RequiresLevel(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	account := newAccount(t, db)

	cfg := testConfig()
	cfg.Tuning.Portal.MinAccountLevelToApply = 5
	mgr.cfg = cfg

	if err := mgr.ApplyForRole(context.Background(), account, RoleContributor); err == nil {
		t.Fatal("expected level requirement error")
	}
}

func TestGrantRole_ApprovesApplication(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	account := newAccount(t, db)
	adminID := newAdmin(t, db)

	if err := mgr.ApplyForRole(context.Background(), account, RoleContributor); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if err := mgr.GrantRole(context.Background(), adminID, account, RoleContributor); err != nil {
		t.Fatalf("grant: %v", err)
	}

	has, err := mgr.HasRole(context.Background(), account, RoleContributor)
	if err != nil {
		t.Fatalf("has role: %v", err)
	}
	if !has {
		t.Fatal("expected role granted")
	}
}

func TestGrantRole_Hierarchy(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	account := newAccount(t, db)
	adminID := newAdmin(t, db)
	ctx := context.Background()

	if err := mgr.GrantRole(ctx, adminID, account, RoleCurator); err != nil {
		t.Fatalf("grant curator: %v", err)
	}
	has, err := mgr.HasRole(ctx, account, RoleContributor)
	if err != nil {
		t.Fatalf("has role: %v", err)
	}
	if !has {
		t.Fatal("curator should satisfy contributor check")
	}
	active, err := mgr.ActiveRole(ctx, account)
	if err != nil {
		t.Fatalf("active role: %v", err)
	}
	if active != RoleCurator {
		t.Fatalf("expected curator as active role, got %s", active)
	}
}

func TestRejectApplication(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	account := newAccount(t, db)
	adminID := newAdmin(t, db)
	ctx := context.Background()

	if err := mgr.ApplyForRole(ctx, account, RoleContributor); err != nil {
		t.Fatalf("apply: %v", err)
	}
	apps, err := mgr.ListApplications(ctx, string(ApplicationPending))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 pending app, got %d", len(apps))
	}
	if err := mgr.RejectApplication(ctx, adminID, apps[0].ID, "no"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	app, err := mgr.GetApplication(ctx, apps[0].ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if app.Status != ApplicationRejected {
		t.Fatalf("expected rejected, got %s", app.Status)
	}
}

func TestSubmissionLifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	account := newAccount(t, db)
	adminID := newAdmin(t, db)

	ctx := context.Background()
	if err := mgr.GrantRole(ctx, adminID, account, RoleContributor); err != nil {
		t.Fatalf("grant role: %v", err)
	}

	draft, err := mgr.CreateDraft(ctx, account, MediaText, "a funny caption", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	if err := mgr.SubmitDraft(ctx, account, draft.ID); err != nil {
		t.Fatalf("submit: %v", err)
	}

	subs, err := mgr.ListSubmissions(ctx, "", string(StatusSubmitted))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 submitted, got %d", len(subs))
	}

	balBefore, err := mgr.economy.Wallet.Balance(ctx, account)
	if err != nil {
		t.Fatalf("balance before: %v", err)
	}

	if err := mgr.DecideSubmission(ctx, adminID, subs[0].ID, true, ""); err != nil {
		t.Fatalf("approve: %v", err)
	}

	got, err := mgr.GetSubmission(ctx, subs[0].ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusApproved {
		t.Fatalf("expected approved, got %s", got.Status)
	}

	balAfter, err := mgr.economy.Wallet.Balance(ctx, account)
	if err != nil {
		t.Fatalf("balance after: %v", err)
	}
	if balAfter-balBefore != int64(mgr.cfg.Tuning.Noin.ContributorAcceptedAsset) {
		t.Fatalf("expected balance increase %d, got %d", mgr.cfg.Tuning.Noin.ContributorAcceptedAsset, balAfter-balBefore)
	}
	prof, err := mgr.profile.Get(ctx, account, true)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	found := false
	for _, c := range prof.ContributorCredits {
		if c == subs[0].ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected contributor credit added")
	}
}

func TestSubmission_DailyCap(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	account := newAccount(t, db)
	adminID := newAdmin(t, db)
	ctx := context.Background()
	mgr.cfg.Tuning.Portal.SubmissionsPerContributorPerDay = 1

	if err := mgr.GrantRole(ctx, adminID, account, RoleContributor); err != nil {
		t.Fatalf("grant role: %v", err)
	}

	d1, err := mgr.CreateDraft(ctx, account, MediaText, "one", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if err := mgr.SubmitDraft(ctx, account, d1.ID); err != nil {
		t.Fatalf("submit first: %v", err)
	}

	d2, err := mgr.CreateDraft(ctx, account, MediaText, "two", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("draft second: %v", err)
	}
	if err := mgr.SubmitDraft(ctx, account, d2.ID); err == nil {
		t.Fatal("expected daily cap error")
	}
}

func TestFreezeLifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	target := newAccount(t, db)
	adminID := newAdmin(t, db)
	guardID := guardAccount(t, mgr, db, adminID)

	ctx := context.Background()
	if err := mgr.FreezeAccount(ctx, guardID, target, "spam"); err != nil {
		t.Fatalf("freeze: %v", err)
	}

	freezes, err := mgr.ListActiveFreezes(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(freezes) != 1 {
		t.Fatalf("expected 1 freeze, got %d", len(freezes))
	}

	id := freezes[0]["id"].(string)
	if err := mgr.ConvertFreezeToBan(ctx, adminID, id, "confirmed spam"); err != nil {
		t.Fatalf("convert ban: %v", err)
	}

	freezes, _ = mgr.ListActiveFreezes(ctx)
	if len(freezes) != 0 {
		t.Fatalf("expected freeze resolved, got %d", len(freezes))
	}
}

func TestFreeze_AutoExpiry(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	target := newAccount(t, db)
	guardID := guardAccount(t, mgr, db, newAdmin(t, db))

	ctx := context.Background()
	if err := mgr.FreezeAccount(ctx, guardID, target, "spam"); err != nil {
		t.Fatalf("freeze: %v", err)
	}

	future := time.Now().Add(49 * time.Hour)
	mgr.nowFn = func() time.Time { return future }

	if err := mgr.ExpireFreezes(ctx); err != nil {
		t.Fatalf("expire: %v", err)
	}

	freezes, _ := mgr.ListActiveFreezes(ctx)
	if len(freezes) != 0 {
		t.Fatalf("expected auto-expired freeze, got %d", len(freezes))
	}
}

func approvedTopicMedia(t *testing.T, mgr *Manager, db *sql.DB, adminID string) string {
	t.Helper()
	account := newAccount(t, db)
	ctx := context.Background()
	if err := mgr.GrantRole(ctx, adminID, account, RoleContributor); err != nil {
		t.Fatalf("grant contributor: %v", err)
	}
	draft, err := mgr.CreateDraft(ctx, account, MediaText, "topic nown", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("draft topic: %v", err)
	}
	if err := mgr.SubmitDraft(ctx, account, draft.ID); err != nil {
		t.Fatalf("submit topic: %v", err)
	}
	if err := mgr.DecideSubmission(ctx, adminID, draft.ID, true, ""); err != nil {
		t.Fatalf("approve topic: %v", err)
	}
	return draft.ID
}

func TestChallenge_RaceToSlot100(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	admin := newAdmin(t, db)
	ctx := context.Background()
	topic, err := mgr.CreateChallengeTopic(ctx, admin, weekMonday(time.Now().UTC()), approvedTopicMedia(t, mgr, db, admin))
	if err != nil {
		t.Fatal(err)
	}
	accounts := make([]string, 150)
	for i := range accounts {
		accounts[i] = newAccount(t, db)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 10)
	accepted, full := 0, 0
	var firstErr error
	for _, account := range accounts {
		wg.Add(1)
		go func(account string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, err := mgr.SubmitChallengeEntry(ctx, account, topic.ID, MediaText, "entry", ContributionConsent{Version: "v1", Accepted: true})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				accepted++
			} else if err.Error() == "challenge_full" {
				full++
			} else if firstErr == nil {
				firstErr = err
			}
		}(account)
	}
	wg.Wait()
	if accepted != 100 || full != 50 || firstErr != nil {
		t.Fatalf("first-100 intake: accepted=%d full=%d other=%v", accepted, full, firstErr)
	}
	var visible int
	if err = db.QueryRow(`SELECT count(*) FROM challenge_entries WHERE topic_id=$1 AND status='approved'`, topic.ID).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible != 0 {
		t.Fatal("intake bypassed review")
	}
}

func TestChallenge_VoteImmutable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	adminID := newAdmin(t, db)
	voter := newAccount(t, db)
	ctx := context.Background()

	nownID := approvedTopicMedia(t, mgr, db, adminID)
	weekStart := weekMonday(time.Now().UTC())
	topic, err := mgr.CreateChallengeTopic(ctx, adminID, weekStart, nownID)
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	e1acct := newAccount(t, db)
	e2acct := newAccount(t, db)
	e1, err := mgr.SubmitChallengeEntry(ctx, e1acct, topic.ID, MediaText, "e1", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("entry1: %v", err)
	}
	e2, err := mgr.SubmitChallengeEntry(ctx, e2acct, topic.ID, MediaText, "e2", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("entry2: %v", err)
	}

	if err := mgr.ApproveChallengeEntry(ctx, adminID, e1.ID); err != nil {
		t.Fatalf("approve1: %v", err)
	}
	if err := mgr.ApproveChallengeEntry(ctx, adminID, e2.ID); err != nil {
		t.Fatalf("approve2: %v", err)
	}

	if err := mgr.VoteChallengeEntry(ctx, voter, topic.ID, e1.ID); err != nil {
		t.Fatalf("first vote: %v", err)
	}
	if err := mgr.VoteChallengeEntry(ctx, voter, topic.ID, e2.ID); err == nil {
		t.Fatal("expected immutable vote error")
	}
}

func TestPortalHandler_NoAdminRoutes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	h := mgr.Handler()

	req, err := http.NewRequest("GET", "/admin/portal/applications", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for admin route on portal ingress, got %d", rr.Code)
	}
}

func TestChallenge_CloseIdempotent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	adminID := newAdmin(t, db)
	voter := newAccount(t, db)
	winnerAcct := newAccount(t, db)
	ctx := context.Background()

	nownID := approvedTopicMedia(t, mgr, db, adminID)
	weekStart := weekMonday(time.Now().UTC())
	topic, err := mgr.CreateChallengeTopic(ctx, adminID, weekStart, nownID)
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	entry, err := mgr.SubmitChallengeEntry(ctx, winnerAcct, topic.ID, MediaText, "winner", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	if err := mgr.ApproveChallengeEntry(ctx, adminID, entry.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := mgr.VoteChallengeEntry(ctx, voter, topic.ID, entry.ID); err != nil {
		t.Fatalf("vote: %v", err)
	}

	balBefore, err := mgr.economy.Wallet.Balance(ctx, winnerAcct)
	if err != nil {
		t.Fatalf("balance before: %v", err)
	}
	profBefore, err := mgr.profile.Get(ctx, winnerAcct, true)
	if err != nil {
		t.Fatalf("profile before: %v", err)
	}

	winner1, err := mgr.CloseChallengeWeek(ctx, adminID, topic.ID)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	winner2, err := mgr.CloseChallengeWeek(ctx, adminID, topic.ID)
	if err != nil {
		t.Fatalf("close retry: %v", err)
	}
	if winner1.ID != winner2.ID {
		t.Fatal("close not idempotent")
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM challenge_winners WHERE topic_id = $1`, topic.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count winners: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 winner, got %d", count)
	}

	balAfter, err := mgr.economy.Wallet.Balance(ctx, winnerAcct)
	if err != nil {
		t.Fatalf("balance after: %v", err)
	}
	if balAfter-balBefore != int64(mgr.cfg.Tuning.Noin.ChallengeWinner) {
		t.Fatalf("expected winner noin %d, got %d", mgr.cfg.Tuning.Noin.ChallengeWinner, balAfter-balBefore)
	}
	profAfter, err := mgr.profile.Get(ctx, winnerAcct, true)
	if err != nil {
		t.Fatalf("profile after: %v", err)
	}
	if profAfter.WeekWinnerTitles != profBefore.WeekWinnerTitles+1 {
		t.Fatalf("expected title count +1, got %d", profAfter.WeekWinnerTitles)
	}
}

type retiredSimulatorBody struct {
	*strings.Reader
	reads int
}

func (b *retiredSimulatorBody) Read(p []byte) (int, error) {
	b.reads++
	return b.Reader.Read(p)
}

func TestDealSimulatorHandlerRetiredBeforeContentWork(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	account := newAccount(t, db)
	adminID := newAdmin(t, db)
	ctx := context.Background()
	if err := mgr.GrantRole(ctx, adminID, account, RoleCurator); err != nil {
		t.Fatalf("grant curator: %v", err)
	}
	ordinary := newAccount(t, db)
	var submissionsBefore, entriesBefore int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM portal_submissions),(SELECT count(*) FROM noin_ledger)`).Scan(&submissionsBefore, &entriesBefore); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method+"_non_curator", func(t *testing.T) {
			body := &retiredSimulatorBody{Reader: strings.NewReader("untrusted=body")}
			req := httptest.NewRequest(method, "/portal/simulate", body)
			req.Header.Set("Authorization", "Bearer "+ordinary)
			rr := httptest.NewRecorder()
			mgr.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusForbidden || body.reads != 0 {
				t.Fatal("retired route lost its live role guard", rr.Code, body.reads)
			}
		})
	}

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			body := &retiredSimulatorBody{Reader: strings.NewReader("table_size=6&nown_id=nown-sim&seed=1")}
			req := httptest.NewRequest(method, "/portal/simulate", body)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Authorization", "Bearer "+account)
			rr := httptest.NewRecorder()
			mgr.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusGone || !strings.Contains(rr.Body.String(), "retired") {
				t.Errorf("expected explicit retired410, got %d: %s", rr.Code, rr.Body.String())
			}
			if body.reads != 0 || strings.Contains(rr.Body.String(), "card-h-") {
				t.Error("retired simulator read form or returned legacy content")
			}
		})
	}
	req := httptest.NewRequest(http.MethodGet, "/portal/", nil)
	req.Header.Set("Authorization", "Bearer "+account)
	rr := httptest.NewRecorder()
	mgr.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), `href="/portal/simulate"`) {
		t.Fatal("current portal still advertises retired simulator", rr.Code)
	}
	var submissions, entries int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM portal_submissions),(SELECT count(*) FROM noin_ledger)`).Scan(&submissions, &entries); err != nil || submissions != submissionsBefore || entries != entriesBefore {
		t.Fatal("retired simulator changed content or value", submissionsBefore, entriesBefore, submissions, entries, err)
	}
}

func TestEnsureActiveTermsVersion(t *testing.T) {
	db := setupTestDBWithTerms(t, false)
	defer db.Close()
	mgr := newTestManager(t, db)
	ctx := context.Background()

	if err := mgr.EnsureActiveTermsVersion(ctx); err == nil {
		t.Fatal("missing owner-authored terms were silently manufactured")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM portal_terms`).Scan(&count); err != nil || count != 0 {
		t.Fatal("terms check created legal text", count, err)
	}
	if _, err := db.Exec(`INSERT INTO portal_terms(version,title,body,active_from) VALUES('v1','Fixture terms','Owner supplied fixture text',now()-interval '1 minute')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := mgr.EnsureActiveTermsVersion(ctx); err != nil {
			t.Fatal("published terms unavailable", err)
		}
	}
	var body string
	if err := db.QueryRow(`SELECT body FROM portal_terms WHERE version='v1'`).Scan(&body); err != nil || body != "Owner supplied fixture text" {
		t.Fatal("terms changed", body, err)
	}
}

func TestChallenge_SlotReopensAfterRejection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	adminID := newAdmin(t, db)
	acct1 := newAccount(t, db)
	acct2 := newAccount(t, db)
	acct3 := newAccount(t, db)
	ctx := context.Background()

	nownID := approvedTopicMedia(t, mgr, db, adminID)
	weekStart := weekMonday(time.Now().UTC())
	topic, err := mgr.CreateChallengeTopic(ctx, adminID, weekStart, nownID)
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	e1, err := mgr.SubmitChallengeEntry(ctx, acct1, topic.ID, MediaText, "one", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("entry1: %v", err)
	}
	e2, err := mgr.SubmitChallengeEntry(ctx, acct2, topic.ID, MediaText, "two", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("entry2: %v", err)
	}
	if err := mgr.ApproveChallengeEntry(ctx, adminID, e1.ID); err != nil {
		t.Fatalf("approve1: %v", err)
	}
	if err := mgr.ApproveChallengeEntry(ctx, adminID, e2.ID); err != nil {
		t.Fatalf("approve2: %v", err)
	}

	if err := mgr.RejectChallengeEntry(ctx, adminID, e1.ID, "reopen slot"); err != nil {
		t.Fatalf("reject1: %v", err)
	}

	e3, err := mgr.SubmitChallengeEntry(ctx, acct3, topic.ID, MediaText, "three", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("entry3: %v", err)
	}
	if err := mgr.ApproveChallengeEntry(ctx, adminID, e3.ID); err != nil {
		t.Fatalf("approve3: %v", err)
	}

	got, err := mgr.GetChallengeEntry(ctx, e3.ID)
	if err != nil {
		t.Fatalf("get e3: %v", err)
	}
	if got.SlotNumber != 1 {
		t.Fatalf("expected reopened slot 1, got %d", got.SlotNumber)
	}
}

func TestVoteChallengeEntry_WrongTopic(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	adminID := newAdmin(t, db)
	voter := newAccount(t, db)
	owner := newAccount(t, db)
	ctx := context.Background()

	nownID := approvedTopicMedia(t, mgr, db, adminID)
	topic1, err := mgr.CreateChallengeTopic(ctx, adminID, weekMonday(time.Now().UTC()), nownID)
	if err != nil {
		t.Fatalf("topic1: %v", err)
	}
	topic2, err := mgr.CreateChallengeTopic(ctx, adminID, weekMonday(time.Now().UTC()).AddDate(0, 0, 7), nownID)
	if err != nil {
		t.Fatalf("topic2: %v", err)
	}

	entry, err := mgr.SubmitChallengeEntry(ctx, owner, topic1.ID, MediaText, "entry", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	if err := mgr.ApproveChallengeEntry(ctx, adminID, entry.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}

	if err := mgr.VoteChallengeEntry(ctx, voter, topic2.ID, entry.ID); err == nil {
		t.Fatal("expected error voting for entry under wrong topic")
	}
}

type acceptingTextScreener struct{}

func (acceptingTextScreener) ScreenText(context.Context, string) error { return nil }
