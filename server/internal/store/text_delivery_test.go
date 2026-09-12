package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
	"github.com/lib/pq"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTextRecoveryDeliveryLeaseAndPrivateIdentity(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	m, ids := valueMatch(t, s, db, now, false)
	outcome := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: now, Players: []TextPlayerResult{}}
	for seat, id := range ids {
		role := "nower"
		if seat == 0 {
			role = "donower"
		}
		outcome.Players = append(outcome.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 20})
	}
	if err := s.Finish(ctx, outcome); err != nil {
		t.Fatal(err)
	}
	restarted := NewTextValueStore(db, s.tuning)
	if n, err := restarted.RecoverPending(ctx, 1); err != nil || n != 1 {
		t.Fatalf("recover: %d %v", n, err)
	}
	// Keep the recipient-filtered claim leased while the global worker lease
	// expires below; random account ordering must not change the replay target.
	selected, err := restarted.ClaimAccountDeliveries(ctx, uuid.NewString(), []string{ids[3]}, now.Add(time.Second), 10*time.Minute, 10)
	if err != nil || len(selected) != 1 || selected[0].AccountID != ids[3] {
		t.Fatalf("online-only claim %+v %v", selected, err)
	}
	worker, other := uuid.NewString(), uuid.NewString()
	first, err := restarted.ClaimDeliveries(ctx, worker, now.Add(time.Second), time.Minute, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("claim: %#v %v", first, err)
	}
	delivery := first[0]
	var body map[string]any
	if err := json.Unmarshal(delivery.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if body["match_id"] != m.Contract.MatchID || body["points"] != float64(20) {
		t.Fatal(body)
	}
	if _, ok := body["role"]; ok {
		t.Fatal("role added to private settlement unnecessarily")
	}
	if err := restarted.AcknowledgeDelivery(ctx, delivery.ID, other, delivery.AccountID, now.Add(2*time.Second)); !errors.Is(err, ErrValueFence) {
		t.Fatal("wrong worker ack", err)
	}
	if err := restarted.AcknowledgeDelivery(ctx, delivery.ID, worker, uuid.NewString(), now.Add(2*time.Second)); !errors.Is(err, ErrValueFence) {
		t.Fatal("wrong recipient ack", err)
	}
	next, err := restarted.ClaimDeliveries(ctx, other, now.Add(2*time.Minute), time.Minute, 1)
	if err != nil || len(next) != 1 || next[0].ID != delivery.ID {
		t.Fatalf("expired lease retry: %#v %v", next, err)
	}
	if err := restarted.AcknowledgeDelivery(ctx, delivery.ID, worker, delivery.AccountID, now.Add(2*time.Minute)); !errors.Is(err, ErrValueFence) {
		t.Fatal("stale worker ack", err)
	}
	for i := 0; i < 2; i++ {
		if err := restarted.AcknowledgeDelivery(ctx, delivery.ID, other, delivery.AccountID, now.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_outbox WHERE acknowledged_at IS NOT NULL`); n != 1 {
		t.Fatal(n)
	}
	if n, err := restarted.RecoverPending(ctx, 1); err != nil || n != 0 {
		t.Fatalf("replayed recovery: %d %v", n, err)
	}
	report, err := restarted.Reconcile(ctx)
	if err != nil || report.Pending != 0 || report.MissingEffects != 0 || report.LedgerMismatches != 0 || report.WalletMismatches != 0 {
		t.Fatalf("reconcile: %+v %v", report, err)
	}
	// A consistent wallet+ledger mutation must still be caught by receipt linkage.
	if _, err := db.Exec(`UPDATE noin_ledger SET amount=amount+1 WHERE id=(SELECT min(ledger_id) FROM text_award_receipts)`); err == nil {
		t.Fatal("ledger was mutable")
	}
	if _, err := db.Exec(`UPDATE text_award_receipts SET credited=credited+1 WHERE ledger_id=(SELECT min(ledger_id) FROM text_award_receipts)`); err == nil {
		t.Fatal("award receipt was mutable")
	}
	if _, err := db.Exec(`UPDATE text_outbox SET payload='{}' WHERE id=$1`, delivery.ID); err == nil {
		t.Fatal("outbox payload mutable")
	}
	if _, err := db.Exec(`DELETE FROM text_outbox WHERE id=$1`, delivery.ID); err == nil {
		t.Fatal("outbox identity deletable")
	}
}

func TestTextReconcileDetectsMissingAndUnlinkedEffects(t *testing.T) {
	for _, corruption := range []string{"missing_payload", "orphan_ledger", "unknown_match", "absent_match", "missing_settlement"} {
		t.Run(corruption, func(t *testing.T) {
			db, s := textValueDB(t)
			ctx := context.Background()
			now := time.Now().UTC()
			m, ids := valueMatch(t, s, db, now, false)
			o := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: now}
			for seat, id := range ids {
				role := "nower"
				if seat == 0 {
					role = "donower"
				}
				o.Players = append(o.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 20})
			}
			if err := s.Finish(ctx, o); err != nil {
				t.Fatal(err)
			}
			if corruption == "missing_settlement" {
				// Explicit test corruption bypasses protection to prove reconciliation.
				if _, err := db.Exec(`ALTER TABLE text_settlements DISABLE TRIGGER USER; DELETE FROM text_settlements; ALTER TABLE text_settlements ENABLE TRIGGER USER`); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := s.SettlePending(ctx, m.Contract.MatchID); err != nil {
					t.Fatal(err)
				}
				if corruption == "missing_payload" {
					if _, err := db.Exec(`ALTER TABLE noin_ledger DISABLE TRIGGER USER; UPDATE noin_ledger SET payload='{}'; ALTER TABLE noin_ledger ENABLE TRIGGER USER`); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := db.Exec(`INSERT INTO noin_ledger(account_id,event_type,amount,reason,payload,server_day) SELECT account_id,event_type,amount,reason,CASE $1 WHEN 'unknown_match' THEN jsonb_set(payload,'{match_id}',to_jsonb('ffffffff-ffff-ffff-ffff-ffffffffffff'::text)) WHEN 'absent_match' THEN payload-'match_id' ELSE payload END,server_day FROM noin_ledger LIMIT 1`, corruption); err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec(`UPDATE noin_wallets w SET balance=(SELECT SUM(amount) FROM noin_ledger l WHERE l.account_id=w.account_id)`); err != nil {
						t.Fatal(err)
					}
				}
			}
			r, err := s.Reconcile(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if corruption == "missing_settlement" && r.MissingEffects != 4 {
				t.Fatalf("missing intent reported clean: %+v", r)
			}
			if corruption != "missing_settlement" && (r.LedgerMismatches == 0 || r.WalletMismatches != 0) {
				t.Fatalf("ledger-balanced corruption reported clean: %+v", r)
			}
		})
	}
}

func TestTextWeekCloseDrainsAcceptedWorkAndSealsRanks(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 6, 23, 59, 0, 0, time.UTC)
	m, ids := valueMatch(t, s, db, now, false)
	week := valueWeek(now)
	weekID := week.Format("2006-01-02")
	if _, err := s.CloseLeaderboardWeek(ctx, weekID, week.AddDate(0, 0, 7)); !errors.Is(err, ErrWeekPending) {
		t.Fatalf("live accepted match must delay close: %v", err)
	}
	o := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: now.Add(30 * time.Second)}
	for seat, id := range ids {
		role := "nower"
		if seat == 0 {
			role = "donower"
		}
		o.Players = append(o.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 20})
	}
	if err := s.Finish(ctx, o); err != nil {
		t.Fatal("accepted pre-cutoff outcome", err)
	}
	ranks, err := s.CloseLeaderboardWeek(ctx, weekID, week.AddDate(0, 0, 7))
	if err != nil || len(ranks) != 4 {
		t.Fatalf("close: %+v %v", ranks, err)
	}
	for _, rank := range ranks {
		if rank.Rank != 1 || rank.Points != 20 {
			t.Fatal("ties should share rank", ranks)
		}
	}
	if _, err := db.Exec(`UPDATE leaderboard_entries SET points=999 WHERE week_id=$1`, weekID); err == nil {
		t.Fatal("closed points mutable")
	}
	if _, err := db.Exec(`INSERT INTO leaderboard_history(week_id,account_id,rank,points) VALUES($1,$2,1,999)`, weekID, valueAccount(t, db)); err == nil {
		t.Fatal("closed history accepted new row")
	}
	if _, err := db.Exec(`UPDATE leaderboard_weeks SET closed=false WHERE week_id=$1`, weekID); err == nil {
		t.Fatal("sealed week reopened")
	}
	replayed, err := s.CloseLeaderboardWeek(ctx, weekID, week.AddDate(0, 0, 8))
	if err != nil || !reflect.DeepEqual(ranks, replayed) {
		t.Fatalf("closed snapshot changed: %+v %v", replayed, err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE state='pending'`); n != 0 {
		t.Fatal(n)
	}
}

func TestTextRealEngineDurableHooksAndFinishRetry(t *testing.T) {
	db, store := textValueDB(t)
	ctx := context.Background()
	for _, mode := range gamecontract.AllModes() {
		t.Run(string(mode), func(t *testing.T) {
			now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
			record, accounts := valueUnpreparedMatch(t, store, db, now, false)
			limits := store.tuning.TextCatalog
			catalog, err := media.LoadTextPack("../../pkg/media/testdata/text-en", media.TextLimits{MaxTextBytes: store.tuning.Contract.MaxTextBytes, MaxRecords: limits.MaxRecords, MaxFileBytes: limits.MaxFileBytes, MaxBundleBytes: limits.MaxBundleBytes})
			if err != nil {
				t.Fatal(err)
			}
			deal, err := catalog.Deal(mode, 4, media.TextDealTuning{HandSize: store.tuning.Hand.Size, ReserveSize: store.tuning.Hand.DrawPile, MinHigh: store.tuning.Dealing.MinHighPerNown, MinDistant: store.tuning.Dealing.MinDistantPerNown, MaxSearchNodes: limits.MaxSearchNodes}, media.TextRandomness{Schedule: 1, Hands: 2, System: 3})
			if err != nil {
				t.Fatal(err)
			}
			record.Contract.ModeID = mode
			record.Contract.PackReleaseID = deal.ReleaseID
			record.Contract.PackSHA256 = deal.SnapshotSHA256
			if err := store.Prepare(ctx, record, now); err != nil {
				t.Fatal(err)
			}
			if err := store.Start(ctx, record.Contract.MatchID, record.Owner, 1, now); err != nil {
				t.Fatal(err)
			}
			failAfterOutcome := true
			finishCalls := 0
			hooks := game.TextHooks{
				Abandon: func(ctx context.Context, e game.TextAbandonEvent) error {
					return store.Abandon(ctx, TextAbandon{MatchID: e.MatchID, Owner: record.Owner, Epoch: 1, AccountID: accounts[e.Seat], Seat: e.Seat, At: e.OccurredAt})
				},
				Award: func(ctx context.Context, a game.TextAwardEvent) error {
					_, err := store.Award(ctx, TextAward{MatchID: a.MatchID, Owner: record.Owner, Epoch: 1, AccountID: accounts[a.Seat], Kind: a.Kind, Ordinal: a.Ordinal, Amount: a.Amount, At: a.OccurredAt})
					return err
				},
				Finish: func(ctx context.Context, result game.TextResult) error {
					finishCalls++
					o := TextOutcome{MatchID: result.Contract.MatchID, Owner: record.Owner, Epoch: 1, Kind: result.Outcome, Winner: result.Winner, At: result.OccurredAt, Players: []TextPlayerResult{}}
					for _, p := range result.Players {
						o.Players = append(o.Players, TextPlayerResult{AccountID: accounts[p.Seat], Seat: p.Seat, Role: p.Role, Points: int64(p.Points), CorrectVotes: p.CorrectVotes, VotesCast: p.VotesCast, Survivals: p.Survivals, Pokes: p.Pokes, Absent: p.Absent})
					}
					if err := store.Finish(ctx, o); err != nil {
						return err
					}
					if failAfterOutcome {
						failAfterOutcome = false
						return errors.New("injected after committed outcome")
					}
					return store.SettlePending(ctx, o.MatchID)
				},
			}
			engine, err := game.NewTextMatch(game.TextOptions{Contract: record.Contract, Deal: deal, Config: &config.Config{Tuning: store.tuning, WebSocket: config.WebSocketConfig{MaxMessageBytes: 65536}}, Now: func() time.Time { return now }, Seed: 42, Hooks: hooks})
			if err != nil {
				t.Fatal(err)
			}
			failed := false
			ended := false
			for step := 0; step < 100; step++ {
				snapshot, err := engine.Snapshot(0)
				if err != nil {
					t.Fatal(err)
				}
				if snapshot.Phase == v2.PhaseVerdict {
					ended = true
					break
				}
				now = time.UnixMilli(snapshot.DeadlineMS)
				if _, err := engine.Advance(ctx, now); err != nil {
					if failed || err.Error() != "injected after committed outcome" {
						t.Fatal(err)
					}
					failed = true
					if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE match_id=$1 AND state='pending'`, record.Contract.MatchID); n != 4 {
						t.Fatal(n)
					}
				}
			}
			if !ended || !failed || finishCalls != 2 {
				t.Fatal(fmt.Sprintf("ended=%v failed=%v finishCalls=%d", ended, failed, finishCalls))
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE match_id=$1 AND state='applied'`, record.Contract.MatchID); n != 4 {
				t.Fatal(n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_award_receipts WHERE match_id=$1 AND kind='match_completed'`, record.Contract.MatchID); n != 4 {
				t.Fatal("completion count", n)
			}
			if _, err := engine.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if err := store.SettlePending(ctx, record.Contract.MatchID); err != nil {
				t.Fatal(err)
			}
			report, err := store.Reconcile(ctx)
			if err != nil || report.Pending != 0 || report.MissingEffects != 0 || report.LedgerMismatches != 0 || report.WalletMismatches != 0 {
				t.Fatalf("%+v %v", report, err)
			}
		})
	}
}

func TestTextStartSerializesBeforeWeekClose(t *testing.T) {
	db, s := textValueDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Date(2026, 9, 6, 23, 59, 0, 123456789, time.UTC)
	m, ids := valuePreparedMatch(t, s, db, now, false)
	hold, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback()
	if err = LockValueAccount(ctx, hold, ids[0]); err != nil {
		t.Fatal(err)
	}
	started := make(chan error, 1)
	go func() { started <- s.Start(ctx, m.Contract.MatchID, m.Owner, 1, now) }()
	// Observe the actual blocked statement instead of guessing scheduling order.
	for {
		var blocked bool
		if err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT id FROM accounts WHERE id=%')`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err = <-started:
			t.Fatalf("start unexpectedly completed: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	cutoff := valueWeek(now).AddDate(0, 0, 7)
	closeCtx, closeCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, closeErr := s.CloseLeaderboardWeek(closeCtx, valueWeek(now).Format("2006-01-02"), cutoff)
	closeCancel()
	if closeErr == nil {
		t.Error("week sealed while accepted Start was in flight")
	}
	if err = hold.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = <-started; err != nil {
		t.Fatal(err)
	}
	if _, err = s.CloseLeaderboardWeek(ctx, valueWeek(now).Format("2006-01-02"), cutoff); !errors.Is(err, ErrWeekPending) {
		t.Fatalf("live start missing from barrier: %v", err)
	}
	// A persisted closing barrier refuses additional pre-boundary starts.
	other, _ := valuePreparedMatch(t, s, db, now, false)
	if err = s.Start(ctx, other.Contract.MatchID, other.Owner, 1, now); !errors.Is(err, ErrWeekClosed) {
		t.Fatalf("closing admission: %v", err)
	}
	if _, err = s.Award(ctx, TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Kind: "correct_vote", Ordinal: 1, Amount: s.tuning.Noin.CorrectVote, At: now}); err != nil {
		t.Fatalf("same instant nanosecond normalization: %v", err)
	}
}

func TestTextReconcileRefusesPolicyFilteredRows(t *testing.T) {
	db, s := textValueDB(t)
	db.SetMaxOpenConns(1)
	role := pq.QuoteIdentifier("text_reader_" + strings.ReplaceAll(uuid.NewString(), "-", ""))
	if _, err := db.Exec("CREATE ROLE " + role + " NOLOGIN NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{"RESET ROLE", "ALTER TABLE text_settlements DISABLE ROW LEVEL SECURITY", "DROP OWNED BY " + role, "DROP ROLE " + role} {
			if _, err := db.Exec(query); err != nil {
				t.Error(err)
			}
		}
	})
	for _, query := range []string{"ALTER TABLE text_settlements ENABLE ROW LEVEL SECURITY", "GRANT USAGE ON SCHEMA public TO " + role, "GRANT SELECT ON ALL TABLES IN SCHEMA public TO " + role, "SET ROLE " + role} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	var visible int
	if err := db.QueryRow(`SELECT count(*) FROM text_settlements`).Scan(&visible); err != nil || visible != 0 {
		t.Fatalf("restricted select %d %v", visible, err)
	}
	if _, err := s.Reconcile(context.Background()); err == nil {
		t.Fatal("policy-filtered reconciliation must fail closed")
	}
}
func TestTextEmptyClosedWeekCannotBeDeleted(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	week := "2026-08-17"
	if _, err := s.CloseLeaderboardWeek(ctx, week, time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM leaderboard_weeks WHERE week_id=$1`, week); err == nil {
		t.Fatal("sealed empty week deleted")
	}
}

func TestTextPrivateAwardsAuthenticatedReplay(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	m, ids := valueMatch(t, s, db, now, false)
	a := TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Kind: "correct_vote", Ordinal: 1, Amount: s.tuning.Noin.CorrectVote, At: now}
	credited, err := s.Award(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.PrivateAwards(ctx, a.MatchID, a.AccountID); err != nil || len(got) != 0 {
		t.Fatalf("live role-linked Noin exposed: %+v %v", got, err)
	}
	if err = s.Interrupt(ctx, a.MatchID, a.Owner, 1, now); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		r := NewTextValueStore(db, s.tuning)
		got, err := r.PrivateAwards(ctx, a.MatchID, a.AccountID)
		if err != nil || len(got) != 1 || got[0].Credited != credited || got[0].Kind != a.Kind || got[0].Ordinal != 1 {
			t.Fatalf("receipt replay %+v %v", got, err)
		}
	}
	if got, err := s.PrivateAwards(ctx, a.MatchID, ids[1]); err != nil || len(got) != 0 {
		t.Fatalf("another seat's private award: %+v %v", got, err)
	}
	if _, err := s.PrivateAwards(ctx, a.MatchID, valueAccount(t, db)); !errors.Is(err, ErrValueFence) {
		t.Fatalf("outsider authorized: %v", err)
	}
}
