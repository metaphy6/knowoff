package portal

import (
	"context"
	"database/sql"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/lib/pq"
	"sync"
	"testing"
	"time"
)

func TestChallengeTieUsesAcceptedTimeNotReusedSlot(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	m.cfg.Tuning.LiveOps.ChallengeMaxEntries = 2
	topic, err := m.CreateChallengeTopic(ctx, admin, weekMonday(time.Now().UTC()), approvedTopicMedia(t, m, db, admin))
	if err != nil {
		t.Fatal(err)
	}
	submit := func(text string) *ChallengeEntry {
		t.Helper()
		e, err := m.SubmitChallengeEntry(ctx, newAccount(t, db), topic.ID, MediaText, text, ContributionConsent{Version: "v1", Accepted: true})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	first := submit("rejected initial entry")
	earliest := submit("earliest accepted survivor")
	if err = m.RejectChallengeEntry(ctx, admin, first.ID, "rejected"); err != nil {
		t.Fatal(err)
	}
	later := submit("later accepted replacement")
	for _, e := range []*ChallengeEntry{earliest, later} {
		if err = m.ApproveChallengeEntry(ctx, admin, e.ID); err != nil {
			t.Fatal(err)
		}
	}
	if later.SlotNumber != 1 || earliest.SlotNumber != 2 {
		t.Fatalf("fixture slots %d %d", later.SlotNumber, earliest.SlotNumber)
	}
	winner, err := m.CloseChallengeWeek(ctx, admin, topic.ID)
	if err != nil {
		t.Fatal(err)
	}
	if winner == nil || winner.ID != earliest.ID {
		t.Fatalf("tie chose reused slot: winner=%+v earliest=%s", winner, earliest.ID)
	}
}

func TestChallengeCrownDoesNotRegressAndEmptyWeekKeepsHolder(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	monday := weekMonday(time.Now().UTC()).AddDate(0, 0, 7)
	nown := approvedTopicMedia(t, m, db, admin)
	create := func(at time.Time, withEntry bool) (string, string) {
		t.Helper()
		m.nowFn = func() time.Time { return at }
		topic, e := m.CreateChallengeTopic(ctx, admin, at, nown)
		if e != nil {
			t.Fatal(e)
		}
		if !withEntry {
			return topic.ID, ""
		}
		owner := newAccount(t, db)
		entry, e := m.SubmitChallengeEntry(ctx, owner, topic.ID, MediaText, "a reviewed weekly entry", ContributionConsent{Version: "v1", Accepted: true})
		if e != nil {
			t.Fatal(e)
		}
		if e = m.ApproveChallengeEntry(ctx, admin, entry.ID); e != nil {
			t.Fatal(e)
		}
		return topic.ID, owner
	}
	older, _ := create(monday, true)
	newer, holder := create(monday.AddDate(0, 0, 7), true)
	if _, err := m.CloseChallengeWeek(ctx, admin, newer); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CloseChallengeWeek(ctx, admin, older); err != nil {
		t.Fatal(err)
	}
	empty, _ := create(monday.AddDate(0, 0, 14), false)
	if winner, err := m.CloseChallengeWeek(ctx, admin, empty); err != nil || winner != nil {
		t.Fatalf("empty winner %v %v", winner, err)
	}
	var current string
	if err := db.QueryRow(`SELECT account_id FROM challenge_current_winner WHERE singleton`).Scan(&current); err != nil || current != holder {
		t.Fatalf("crown regressed %s %v", current, err)
	}
}

func TestCommunityGrantOccurrenceDaySurvivesTransactionRetry(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	owner := newAccount(t, db)
	occurrence := time.Date(2031, 3, 4, 0, 30, 0, 0, time.FixedZone("UTC+3", 3*60*60))
	attempts := 0
	err := store.WithValueTransaction(ctx, db, func(tx *sql.Tx) error {
		attempts++
		if _, err := m.economy.Wallet.GrantTxAt(ctx, tx, owner, economy.LedgerChallengeWinner, 1000, "synthetic retry proof", 0, occurrence); err != nil {
			return err
		}
		if attempts == 1 {
			return &pq.Error{Code: "40001", Message: "synthetic serialization failure after wallet write"}
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("retry %d %v", attempts, err)
	}
	var count, total int
	var day time.Time
	if err = db.QueryRow(`SELECT count(*),sum(amount),min(server_day) FROM noin_ledger WHERE account_id=$1 AND event_type='challenge_winner'`, owner).Scan(&count, &total, &day); err != nil || count != 1 || total != 1000 || !day.Equal(time.Date(2031, 3, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("retry occurrence %d %d %v %v", count, total, day, err)
	}
}

func TestChallengeCloseRetriesEntireCrownAfterAuditSerializationFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	topic, err := m.CreateChallengeTopic(ctx, admin, weekMonday(time.Now().UTC()), approvedTopicMedia(t, m, db, admin))
	if err != nil {
		t.Fatal(err)
	}
	owner := newAccount(t, db)
	entry, err := m.SubmitChallengeEntry(ctx, owner, topic.ID, MediaText, "a transactionally crowned response", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.ApproveChallengeEntry(ctx, admin, entry.ID); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE SEQUENCE community_close_retry; CREATE FUNCTION community_fail_close_once() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='challenge_week_close' AND nextval('community_close_retry')=1 THEN RAISE EXCEPTION 'synthetic retry after all crown writes' USING ERRCODE='40001'; END IF; RETURN NEW; END $$; CREATE TRIGGER community_close_retry BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION community_fail_close_once()`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := db.Exec(`DROP TRIGGER community_close_retry ON admin_audit_log;DROP FUNCTION community_fail_close_once();DROP SEQUENCE community_close_retry`); e != nil {
			t.Error(e)
		}
	}()
	winner, err := m.CloseChallengeWeek(ctx, admin, topic.ID)
	if err != nil || winner == nil || winner.ID != entry.ID {
		t.Fatalf("retry crown %v %v", winner, err)
	}
	var attempts, ledger, titles, audits, receipts int
	if err = db.QueryRow(`SELECT last_value FROM community_close_retry`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct {
		query string
		out   *int
	}{{`SELECT count(*) FROM noin_ledger WHERE account_id=$1 AND event_type='challenge_winner'`, &ledger}, {`SELECT week_winner_titles FROM profiles WHERE account_id=$1`, &titles}, {`SELECT count(*) FROM admin_audit_log WHERE target_id=$1 AND action='challenge_week_close'`, &audits}, {`SELECT count(*) FROM challenge_winners WHERE topic_id=$1`, &receipts}} {
		id := owner
		if p.out == &audits || p.out == &receipts {
			id = topic.ID
		}
		if err = db.QueryRow(p.query, id).Scan(p.out); err != nil {
			t.Fatal(err)
		}
	}
	if attempts != 2 || ledger != 1 || titles != 1 || audits != 1 || receipts != 1 {
		t.Fatalf("partial or duplicate crown %d %d %d %d %d", attempts, ledger, titles, audits, receipts)
	}
}

func TestChallengeMaintenancePublishesPreparedTopicAndClosesOnce(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	monday := weekMonday(time.Now().UTC()).AddDate(0, 0, 7)
	nown := approvedTopicMedia(t, m, db, admin)
	topic, err := m.CreateChallengeTopic(ctx, admin, monday, nown)
	if err != nil {
		t.Fatal(err)
	}
	m.nowFn = func() time.Time { return monday.Add(-time.Microsecond) }
	if err = m.RunCommunityMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	var active bool
	if err = db.QueryRow(`SELECT activated_at IS NOT NULL FROM challenge_topics WHERE id=$1`, topic.ID).Scan(&active); err != nil || active {
		t.Fatalf("early activation %v %v", active, err)
	}
	m.nowFn = func() time.Time { return monday }
	if err = m.RunCommunityMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	if current, e := m.CurrentChallengeTopic(ctx); e != nil || current == nil || current.ID != topic.ID {
		t.Fatalf("Monday topic %v %v", current, e)
	}
	// Acceptance and human approval are explicit, despite automated scheduling.
	owner := newAccount(t, db)
	entry, err := m.SubmitChallengeEntry(ctx, owner, topic.ID, MediaText, "synthetic weekly answer", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.ApproveChallengeEntry(ctx, admin, entry.ID); err != nil {
		t.Fatal(err)
	}
	m.nowFn = func() time.Time { return monday.AddDate(0, 0, 7) }
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- m.RunCommunityMaintenance(ctx) }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	restarted := newTestManager(t, db)
	restarted.nowFn = m.nowFn
	if err = restarted.RunCommunityMaintenance(ctx); err != nil {
		t.Fatal(err)
	}
	var crowns, ledger, activations, closes int
	for _, pair := range []struct {
		q string
		n *int
	}{{`SELECT count(*) FROM challenge_winners WHERE topic_id=$1`, &crowns}, {`SELECT count(*) FROM noin_ledger WHERE account_id=$1 AND event_type='challenge_winner'`, &ledger}, {`SELECT count(*) FROM admin_audit_log WHERE target_id=$1 AND action='challenge_topic_activate'`, &activations}, {`SELECT count(*) FROM admin_audit_log WHERE target_id=$1 AND action='challenge_week_close'`, &closes}} {
		id := topic.ID
		if pair.n == &ledger {
			id = owner
		}
		if err = db.QueryRow(pair.q, id).Scan(pair.n); err != nil {
			t.Fatal(err)
		}
	}
	if crowns != 1 || ledger != 1 || activations != 1 || closes != 1 {
		t.Fatalf("duplicate effects %d %d %d %d", crowns, ledger, activations, closes)
	}
	var payoutDay time.Time
	if err = db.QueryRow(`SELECT server_day FROM noin_ledger WHERE account_id=$1 AND event_type='challenge_winner'`, owner).Scan(&payoutDay); err != nil || !payoutDay.Equal(monday.AddDate(0, 0, 7)) {
		t.Fatalf("payout lost occurrence day %v %v", payoutDay, err)
	}
	var current string
	if err = db.QueryRow(`SELECT account_id FROM challenge_current_winner WHERE singleton`).Scan(&current); err != nil || current != owner {
		t.Fatalf("current title %s %v", current, err)
	}
	if topic, e := m.CurrentChallengeTopic(ctx); e != nil || topic != nil {
		t.Fatalf("invented next topic %+v %v", topic, e)
	}
}

func TestChallengeScheduledWithdrawalFailsClosed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	admin := newAdmin(t, db)
	nown := approvedTopicMedia(t, m, db, admin)
	monday := weekMonday(time.Now().UTC()).AddDate(0, 0, 7)
	topic, err := m.CreateChallengeTopic(ctx, admin, monday, nown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE portal_submissions SET status='rejected',rejection_reason='withdrawn source' WHERE id=$1`, nown); err != nil {
		t.Fatal(err)
	}
	m.nowFn = func() time.Time { return monday }
	if err = m.RunCommunityMaintenance(ctx); err == nil {
		t.Fatal("withdrawn scheduled source silently treated as successful publication")
	}
	if current, e := m.CurrentChallengeTopic(ctx); e != nil || current != nil {
		t.Fatalf("withdrawn source public %v %v", current, e)
	}
	var active bool
	if err = db.QueryRow(`SELECT activated_at IS NOT NULL FROM challenge_topics WHERE id=$1`, topic.ID).Scan(&active); err != nil || active {
		t.Fatalf("withdrawn activation %v %v", active, err)
	}
}
