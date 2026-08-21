package portal

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/profile"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/pkg/media"
	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://knowoff:knowoff@localhost:5432/knowoff_test?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := db.ExecContext(context.Background(),
		`TRUNCATE TABLE challenge_votes, challenge_entries, challenge_winners, challenge_topics,
		 portal_submission_counts, portal_submissions, portal_terms, portal_role_applications,
		 portal_roles, guard_freezes RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate portal tables: %v", err)
	}
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO portal_terms (version, title, body, active_from) VALUES ('v1', 'Terms', 'Terms body', now())
		 ON CONFLICT (version) DO NOTHING`); err != nil {
		t.Fatalf("seed terms: %v", err)
	}
	return db
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

func (fakeAuth) ValidateAccessToken(ctx context.Context, token string) (string, error) { return token, nil }
func (fakeAuth) RevokeAccount(ctx context.Context, accountID string) error             { return nil }

var _ AuthClient = fakeAuth{}

func testConfig() *config.Config {
	return &config.Config{
		Tuning: config.TuningConfig{
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
				BandHigh:          0.55,
				BandLow:           0.30,
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
	pm := profile.NewManager(db, cfg.Tuning.Progression)
	em := economy.NewManager(db, cfg)
	return NewManager(Deps{
		DB:      db,
		Config:  cfg,
		Auth:    fakeAuth{},
		Profile: pm,
		Economy: em,
		Admin:   fakeAudit{},
	})
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

	draft, err := mgr.CreateDraft(ctx, account, MediaText, "a funny caption")
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

	d1, err := mgr.CreateDraft(ctx, account, MediaText, "one")
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if err := mgr.SubmitDraft(ctx, account, d1.ID); err != nil {
		t.Fatalf("submit first: %v", err)
	}

	d2, err := mgr.CreateDraft(ctx, account, MediaText, "two")
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
	guardID := newAdmin(t, db)
	adminID := newAdmin(t, db)

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
	guardID := newAdmin(t, db)

	ctx := context.Background()
	if err := mgr.FreezeAccount(ctx, guardID, target, "spam"); err != nil {
		t.Fatalf("freeze: %v", err)
	}

	// Move freeze into the past.
	if _, err := db.ExecContext(ctx,
		`UPDATE guard_freezes SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatalf("update expiry: %v", err)
	}

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
	draft, err := mgr.CreateDraft(ctx, account, MediaText, "topic nown")
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
	adminID := newAdmin(t, db)
	ctx := context.Background()

	nownID := approvedTopicMedia(t, mgr, db, adminID)
	weekStart := time.Now().UTC().Truncate(24 * time.Hour)
	topic, err := mgr.CreateChallengeTopic(ctx, adminID, weekStart, nownID)
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	mgr.cfg.Tuning.LiveOps.ChallengeMaxEntries = 100

	// Pre-create accounts and submit entries to avoid exhausting the pool.
	entries := make([]*ChallengeEntry, 150)
	for i := range entries {
		acct := newAccount(t, db)
		e, err := mgr.SubmitChallengeEntry(ctx, acct, topic.ID, MediaText, "entry")
		if err != nil {
			t.Fatalf("submit entry %d: %v", i, err)
		}
		entries[i] = e
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	var mu sync.Mutex
	var approved int
	var full int
	var firstErr error
	for _, e := range entries {
		wg.Add(1)
		go func(e *ChallengeEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			err := mgr.ApproveChallengeEntry(ctx, adminID, e.ID)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				approved++
			} else if err.Error() == "challenge full" {
				full++
			} else if firstErr == nil {
				firstErr = err
			}
		}(e)
	}
	wg.Wait()

	if approved != 100 {
		t.Fatalf("expected exactly 100 approved, got %d (full=%d firstErr=%v)", approved, full, firstErr)
	}
	if full != 50 {
		t.Fatalf("expected 50 'challenge full' rejections, got %d", full)
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
	weekStart := time.Now().UTC().Truncate(24 * time.Hour)
	topic, err := mgr.CreateChallengeTopic(ctx, adminID, weekStart, nownID)
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	e1acct := newAccount(t, db)
	e2acct := newAccount(t, db)
	e1, err := mgr.SubmitChallengeEntry(ctx, e1acct, topic.ID, MediaText, "e1")
	if err != nil {
		t.Fatalf("entry1: %v", err)
	}
	e2, err := mgr.SubmitChallengeEntry(ctx, e2acct, topic.ID, MediaText, "e2")
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
	weekStart := time.Now().UTC().Truncate(24 * time.Hour)
	topic, err := mgr.CreateChallengeTopic(ctx, adminID, weekStart, nownID)
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	entry, err := mgr.SubmitChallengeEntry(ctx, winnerAcct, topic.ID, MediaText, "winner")
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

func TestDealSimulatorHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	mgr.media = media.NewManager(makeTestPack())
	account := newAccount(t, db)
	adminID := newAdmin(t, db)
	ctx := context.Background()
	if err := mgr.GrantRole(ctx, adminID, account, RoleCurator); err != nil {
		t.Fatalf("grant curator: %v", err)
	}

	form := "table_size=6&nown_id=nown-sim&seed=1"
	req := httptest.NewRequest(http.MethodPost, "/portal/simulate", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+account)
	rr := httptest.NewRecorder()
	mgr.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Deal Simulator Result") {
		t.Fatalf("expected result heading, got %s", body)
	}
	if !strings.Contains(body, "card-h-") {
		t.Fatal("expected dealt cards in output")
	}
}

func makeTestPack() *media.Pack {
	const dim = 8
	n := &media.MediaItem{
		ID:         "nown-sim",
		Type:       media.MediaTypeText,
		Content:    "test nown",
		Embedding:  normalizeVector([]float32{1, 0, 0, 0, 0, 0, 0, 0}),
		Tags:       []string{"text"},
		ToneBucket: "millennial-cope",
		Rating:     media.RatingEveryone,
		License:    "CC0-1.0",
	}
	var cards []*media.CardItem
	for i := 0; i < 20; i++ {
		cards = append(cards, &media.CardItem{
			ID:         fmt.Sprintf("card-h-%d", i),
			Type:       media.MediaTypeText,
			Content:    "high",
			Embedding:  normalizeVector([]float32{1, 0, 0, 0, 0, 0, 0, 0}),
			Tags:       []string{"text"},
			ToneBucket: "millennial-cope",
		})
	}
	for i := 0; i < 20; i++ {
		cards = append(cards, &media.CardItem{
			ID:         fmt.Sprintf("card-d-%d", i),
			Type:       media.MediaTypeText,
			Content:    "distant",
			Embedding:  distantVector(dim),
			Tags:       []string{"text"},
			ToneBucket: "gen-z-absurdism",
		})
	}
	for i := 0; i < 30; i++ {
		cards = append(cards, &media.CardItem{
			ID:         fmt.Sprintf("card-c-%d", i),
			Type:       media.MediaTypeText,
			Content:    "chaos",
			Embedding:  randomUnitVector(i + 1000),
			Tags:       []string{"text"},
			ToneBucket: "chaos",
		})
	}
	pack := &media.Pack{
		Manifest: media.Manifest{PackTag: "test-portal"},
		Media:    []*media.MediaItem{n},
		Cards:    cards,
	}
	pack.Candidates = media.BuildCandidates(pack.Media, pack.Cards, media.DealingTuning{
		BandHigh:          0.55,
		BandLow:           0.30,
		MinHighPerNown:    2,
		MinDistantPerNown: 2,
	})
	return pack
}

func normalizeVector(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	n := float32(math.Sqrt(sum))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / n
	}
	return out
}

func distantVector(dim int) []float32 {
	v := make([]float32, dim)
	v[0] = 0.58
	for i := 1; i < dim; i++ {
		v[i] = 0.30
	}
	return normalizeVector(v)
}

func randomUnitVector(seed int) []float32 {
	rng := rand.New(rand.NewSource(int64(seed)))
	v := make([]float32, 8)
	for i := range v {
		v[i] = rng.Float32()*2 - 1
	}
	return normalizeVector(v)
}

func TestEnsureActiveTermsVersion(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	ctx := context.Background()

	// setupTestDB seeds v1; clear it to test Ensure.
	if _, err := db.ExecContext(ctx, `DELETE FROM portal_terms WHERE version = 'v1'`); err != nil {
		t.Fatalf("delete terms: %v", err)
	}
	if err := mgr.EnsureActiveTermsVersion(ctx); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	var version string
	if err := db.QueryRowContext(ctx, `SELECT version FROM portal_terms WHERE version = 'v1'`).Scan(&version); err != nil {
		t.Fatalf("missing terms row: %v", err)
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
	weekStart := time.Now().UTC().Truncate(24 * time.Hour)
	topic, err := mgr.CreateChallengeTopic(ctx, adminID, weekStart, nownID)
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}

	e1, err := mgr.SubmitChallengeEntry(ctx, acct1, topic.ID, MediaText, "one")
	if err != nil {
		t.Fatalf("entry1: %v", err)
	}
	e2, err := mgr.SubmitChallengeEntry(ctx, acct2, topic.ID, MediaText, "two")
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

	e3, err := mgr.SubmitChallengeEntry(ctx, acct3, topic.ID, MediaText, "three")
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
	topic1, err := mgr.CreateChallengeTopic(ctx, adminID, time.Now().UTC().Truncate(24*time.Hour), nownID)
	if err != nil {
		t.Fatalf("topic1: %v", err)
	}
	topic2, err := mgr.CreateChallengeTopic(ctx, adminID, time.Now().UTC().Add(7*24*time.Hour).Truncate(24*time.Hour), nownID)
	if err != nil {
		t.Fatalf("topic2: %v", err)
	}

	entry, err := mgr.SubmitChallengeEntry(ctx, owner, topic1.ID, MediaText, "entry")
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
