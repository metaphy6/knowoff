package portal

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

func TestChallengeIntakeLimitAppliesBeforeReview(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	mgr.cfg.Tuning.LiveOps.ChallengeMaxEntries = 2
	admin := newAdmin(t, db)
	ctx := context.Background()
	topic, err := mgr.CreateChallengeTopic(ctx, admin, weekMonday(time.Now().UTC()), approvedTopicMedia(t, mgr, db, admin))
	if err != nil {
		t.Fatal(err)
	}
	first, err := mgr.SubmitChallengeEntry(ctx, newAccount(t, db), topic.ID, MediaText, "first", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.SubmitChallengeEntry(ctx, newAccount(t, db), topic.ID, MediaText, "second", ContributionConsent{Version: "v1", Accepted: true}); err != nil {
		t.Fatal(err)
	}
	third := newAccount(t, db)
	if _, err := mgr.SubmitChallengeEntry(ctx, third, topic.ID, MediaText, "third", ContributionConsent{Version: "v1", Accepted: true}); err == nil {
		t.Fatal("intake accepted an entry beyond the cap before review")
	}
	if err := mgr.RejectChallengeEntry(ctx, admin, first.ID, "not a fit"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.SubmitChallengeEntry(ctx, third, topic.ID, MediaText, "third", ContributionConsent{Version: "v1", Accepted: true}); err != nil {
		t.Fatalf("rejection should reopen intake: %v", err)
	}
}

func TestChallengeClosedWeekRejectsNewVote(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	admin := newAdmin(t, db)
	ctx := context.Background()
	topic, err := mgr.CreateChallengeTopic(ctx, admin, weekMonday(time.Now().UTC()), approvedTopicMedia(t, mgr, db, admin))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := mgr.SubmitChallengeEntry(ctx, newAccount(t, db), topic.ID, MediaText, "candidate", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.ApproveChallengeEntry(ctx, admin, entry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CloseChallengeWeek(ctx, admin, topic.ID); err != nil {
		t.Fatal(err)
	}
	if err := mgr.VoteChallengeEntry(ctx, newAccount(t, db), topic.ID, entry.ID); err == nil {
		t.Fatal("closed challenge accepted a vote")
	}
}

func TestChallengeConsentAndPrivateSnapshot(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	admin := newAdmin(t, db)
	ctx := context.Background()
	topic, err := mgr.CreateChallengeTopic(ctx, admin, weekMonday(time.Now().UTC()), approvedTopicMedia(t, mgr, db, admin))
	if err != nil {
		t.Fatal(err)
	}
	owner := newAccount(t, db)
	viewer := newAccount(t, db)
	if _, err = mgr.SubmitChallengeEntry(ctx, owner, topic.ID, MediaText, "private"); err == nil || err.Error() != "terms_required" {
		t.Fatalf("missing consent: %v", err)
	}
	if _, err = mgr.SubmitChallengeEntry(ctx, owner, topic.ID, MediaText, "private", ContributionConsent{Version: "stale", Accepted: true}); err == nil || err.Error() != "terms_outdated" {
		t.Fatalf("stale consent: %v", err)
	}
	e, err := mgr.SubmitChallengeEntry(ctx, owner, topic.ID, MediaText, "private", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := mgr.ChallengeSnapshot(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot["entries"].([]map[string]any)) != 0 || snapshot["own_entry"] != nil {
		t.Fatal("unscreened entry leaked")
	}
	mine, err := mgr.ChallengeSnapshot(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if mine["own_entry"] == nil || mine["can_submit"] != false {
		t.Fatal("own pending entry not represented")
	}
	if err = mgr.ApproveChallengeEntry(ctx, admin, e.ID); err != nil {
		t.Fatal(err)
	}
	if err = mgr.VoteChallengeEntry(ctx, viewer, topic.ID, e.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err = mgr.ChallengeSnapshot(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["voted_entry_id"] != e.ID || snapshot["can_vote"] != false {
		t.Fatal("immutable vote state missing")
	}
	if _, err = mgr.CloseChallengeWeek(ctx, admin, topic.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err = mgr.ChallengeSnapshot(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == nil || snapshot["can_vote"] != false || snapshot["can_submit"] != false {
		t.Fatal("closed current-week result missing or interactive")
	}
}

func TestChallengeSundayEndsAtNextMonday(t *testing.T) {
	monday := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	topic := &ChallengeTopic{WeekStart: monday, WeekEnd: monday.AddDate(0, 0, 6)}
	if !topicOpen(topic, monday.AddDate(0, 0, 6).Add(23*time.Hour)) {
		t.Fatal("Sunday evening was cut off")
	}
	if topicOpen(topic, monday.AddDate(0, 0, 7)) {
		t.Fatal("next Monday still open")
	}
}

func TestChallengeConcurrentClosePaysOnce(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	mgr := newTestManager(t, db)
	admin := newAdmin(t, db)
	ctx := context.Background()
	topic, err := mgr.CreateChallengeTopic(ctx, admin, weekMonday(time.Now().UTC()), approvedTopicMedia(t, mgr, db, admin))
	if err != nil {
		t.Fatal(err)
	}
	owner := newAccount(t, db)
	mgr.cfg.Tuning.Noin.DailyEarnCap = 300
	if _, err := db.Exec(`INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES($1,CURRENT_DATE,300)`, owner); err != nil {
		t.Fatal(err)
	}
	e, err := mgr.SubmitChallengeEntry(ctx, owner, topic.ID, MediaText, "winner", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = mgr.ApproveChallengeEntry(ctx, admin, e.ID); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { _, err := mgr.CloseChallengeWeek(ctx, admin, topic.ID); results <- err }()
	}
	for i := 0; i < 8; i++ {
		if err = <-results; err != nil {
			t.Fatal(err)
		}
	}
	balance, err := mgr.economy.Wallet.Balance(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if balance != int64(mgr.cfg.Tuning.Noin.ChallengeWinner) {
		t.Fatalf("challenge reward capped as play income: %d", balance)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1 AND event_type='challenge_winner'`, owner).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reward count %d", count)
	}
	if err = db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE target_id=$1 AND action='challenge_week_close'`, topic.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("audit close count %d", count)
	}
}

func TestChallengeDatesUseUTCWithNonUTCDatabase(t *testing.T) {
	fixtureDB := setupTestDB(t)
	defer fixtureDB.Close()
	// Migrations retain a dedicated connection. Use a fresh single-connection
	// pool so the session timezone applies to every statement in this test.
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://knowoff:knowoff@localhost:5432/knowoff_test?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`SET TIME ZONE 'America/Los_Angeles'`); err != nil {
		t.Fatal(err)
	}
	mgr := newTestManager(t, db)
	admin := newAdmin(t, db)
	ctx := context.Background()
	monday := weekMonday(time.Now().UTC())
	topic, err := mgr.CreateChallengeTopic(ctx, admin, monday, approvedTopicMedia(t, mgr, db, admin))
	if err != nil {
		t.Fatal(err)
	}
	if !topic.PublishedAt.Equal(monday) {
		t.Fatalf("publication shifted by DB timezone: %v", topic.PublishedAt)
	}
	for _, now := range []time.Time{monday, monday.AddDate(0, 0, 6).Add(23 * time.Hour)} {
		got, err := mgr.currentChallengeTopicAt(ctx, now)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.ID != topic.ID {
			t.Fatalf("UTC topic absent at %v", now)
		}
	}
	if got, err := mgr.currentChallengeTopicAt(ctx, monday.AddDate(0, 0, 7)); err != nil || got != nil {
		t.Fatalf("expired topic selected: %v %v", got, err)
	}
}
