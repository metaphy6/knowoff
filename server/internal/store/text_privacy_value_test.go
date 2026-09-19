package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// A privileged deletion fixture, not a claim that public deletion is enabled.
func textPrivacyFence(t *testing.T, db *sql.DB, account string) string {
	t.Helper()
	request, _ := privacyPrepare(t, db, account)
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=clock_timestamp() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestTextPrivacyAcceptedWorkAndDeletionBothCommitOrders(t *testing.T) {
	for _, operation := range []string{"award", "settlement", "interruption"} {
		for _, workFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/work_first=%v", operation, workFirst), func(t *testing.T) {
				db, s := textValueDB(t)
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				at := valueTime(time.Now().UTC())
				m, ids := valueMatch(t, s, db, at, false)
				ordered := append([]string(nil), ids...)
				sort.Strings(ordered)
				account := ordered[0]
				if operation == "settlement" {
					if err := s.Finish(ctx, textPrivacyOutcome(m, ids, at)); err != nil {
						t.Fatal(err)
					}
				}
				work := func() error {
					switch operation {
					case "award":
						_, err := s.Award(ctx, TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: account, Kind: "correct_vote", Ordinal: 1, Amount: s.tuning.Noin.CorrectVote, At: at})
						return err
					case "settlement":
						return s.SettlePending(ctx, m.Contract.MatchID)
					default:
						return s.Interrupt(ctx, m.Contract.MatchID, m.Owner, 1, at)
					}
				}
				request := uuid.NewString()
				fence := func(tx *sql.Tx) error {
					if _, err := tx.ExecContext(ctx, `SELECT privacy_prepare_verified_request($1,$2,$3,$4)`, request, account, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)); err != nil {
						return err
					}
					_, err := tx.ExecContext(ctx, `UPDATE accounts SET deleted_at=clock_timestamp() WHERE id=$1`, account)
					return err
				}
				done := make(chan error, 1)
				if !workFirst {
					tx, err := db.BeginTx(ctx, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Rollback()
					if err = fence(tx); err != nil {
						t.Fatal(err)
					}
					go func() { done <- work() }()
					waitAdmissionLock(t, ctx, db, "SELECT deleted_at FROM accounts")
					if err = tx.Commit(); err != nil {
						t.Fatal(err)
					}
					if err = <-done; err != nil {
						t.Fatal(err)
					}
				} else {
					held, release := make(chan struct{}), make(chan struct{})
					var once sync.Once
					defer func() {
						select {
						case <-release:
						default:
							close(release)
						}
					}()
					s.beforeCommit = func() error {
						once.Do(func() { close(held) })
						select {
						case <-release:
							return nil
						case <-ctx.Done():
							return ctx.Err()
						}
					}
					go func() { done <- work() }()
					select {
					case <-held:
					case err := <-done:
						t.Fatal("work did not reach commit", err)
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					deleted := make(chan error, 1)
					go func() { deleted <- WithValueTransaction(ctx, db, fence) }()
					waitAdmissionLock(t, ctx, db, "privacy_prepare_verified_request")
					close(release)
					if err := <-done; err != nil {
						t.Fatal(err)
					}
					if err := <-deleted; err != nil {
						t.Fatal(err)
					}
				}
				want := int64(1)
				if workFirst {
					want = 0
				}
				if n := valueCount(t, db, `SELECT count(*) FROM text_value_erasure_dispositions WHERE account_id=$1 AND operation=$2`, account, map[string]string{"award": "award_correct_vote", "settlement": "settlement", "interruption": "interruption"}[operation]); n != want {
					t.Fatalf("dispositions=%d want=%d", n, want)
				}
				if operation != "award" && valueCount(t, db, `SELECT count(*) FROM text_outbox WHERE account_id<>$1`, account) != 3 {
					t.Fatal("survivor delivery changed")
				}
			})
		}
	}
}

func TestTextPrivacyDispositionRetentionAndBinding(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	at := valueTime(time.Now().UTC())
	m, ids := valueMatch(t, s, db, at, false)
	textPrivacyFence(t, db, ids[0])
	textPrivacyFence(t, db, ids[1])
	if err := s.Finish(ctx, textPrivacyOutcome(m, ids, at)); err != nil {
		t.Fatal(err)
	}
	if err := s.settle(ctx, m.Contract.MatchID, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE text_settlements SET state='erased',erased_at=clock_timestamp(),erasure_disposition_id=(SELECT erasure_disposition_id FROM text_settlements WHERE account_id=$1) WHERE account_id=$2 AND state='pending'`, ids[0], ids[1]); err == nil {
		t.Fatal("pending settlement accepted another subject's valid disposition")
	}
	if err := s.SettlePending(ctx, m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE text_value_erasure_dispositions SET body_sha256=repeat('a',64)`,
		`DELETE FROM text_value_erasure_dispositions`,
		`TRUNCATE text_value_erasure_dispositions CASCADE`,
		`UPDATE text_settlements SET state='pending',erasure_disposition_id=NULL,erased_at=NULL WHERE state='erased'`,
		`UPDATE text_settlements SET erasure_disposition_id=(SELECT erasure_disposition_id FROM text_settlements WHERE account_id=$2) WHERE account_id=$1`,
		`INSERT INTO text_value_erasure_dispositions SELECT gen_random_uuid(),request_id,admission_id,$2,'abandon',0,body_sha256,contract_sha256,policy_sha256,NULL,occurred_at,recorded_at FROM text_value_erasure_dispositions WHERE account_id=$1`,
	} {
		// Some statements have no parameters; keep the rejected mutation's bind arity exact.
		var err error
		if strings.Contains(q, "$2") {
			_, err = db.Exec(q, ids[0], ids[1])
		} else {
			_, err = db.Exec(q)
		}
		if err == nil {
			t.Fatal("retained or cross-subject evidence changed", q)
		}
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_value_erasure_dispositions`); n != 2 {
		t.Fatal("retained dispositions changed", n)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000036_text_value_erasure.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, string(down)); err == nil {
		t.Fatal("retained down succeeded")
	}
	if _, err = conn.ExecContext(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
}

func textPrivacyOutcome(m TextMatchRecord, ids []string, at time.Time) TextOutcome {
	o := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: m.Epoch, Kind: "completed", Winner: "nower", At: at}
	for seat, id := range ids {
		role := "nower"
		if seat == 0 {
			role = "donower"
		}
		o.Players = append(o.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 20})
	}
	return o
}

func TestTextPrivacyErasedFirstDoesNotStrandSurvivors(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	at := valueTime(time.Now().UTC())
	m, ids := valueMatch(t, s, db, at, false)
	ordered := append([]string(nil), ids...)
	sort.Strings(ordered)
	erased := ordered[0]
	textPrivacyFence(t, db, erased)
	if _, err := db.Exec(`DELETE FROM profiles WHERE account_id=$1`, erased); err != nil {
		t.Fatal(err)
	}
	o := textPrivacyOutcome(m, ids, at.Add(time.Second))
	if err := s.Finish(ctx, o); err != nil {
		t.Fatal(err)
	}
	if n, err := s.RecoverPending(ctx, 1); err != nil || n != 1 {
		t.Fatalf("accepted recovery stranded survivors: count=%d err=%v", n, err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE state='applied'`); n != 3 {
		t.Fatalf("survivor settlements=%d", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE account_id=$1 AND state='erased' AND applied_at IS NULL AND effects IS NULL`, erased); n != 1 {
		t.Fatal("erased settlement was absent or described as paid")
	}
	for _, table := range []string{"profiles", "text_award_receipts", "noin_ledger", "daily_noin_earned", "text_outbox"} {
		if n := valueCount(t, db, `SELECT count(*) FROM `+table+` WHERE account_id=$1`, erased); n != 0 {
			t.Fatalf("erased account acquired %s rows=%d", table, n)
		}
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_outbox WHERE account_id<>$1`, erased); n != 3 {
		t.Fatalf("survivor delivery count=%d", n)
	}
	if n, err := s.RecoverPending(ctx, 1); err != nil || n != 0 {
		t.Fatalf("terminal recovery replay: count=%d err=%v", n, err)
	}
	if r, err := s.Reconcile(ctx); err != nil || r.Erased != 1 || r.Pending != 0 || r.MissingEffects != 0 || r.LedgerMismatches != 0 || r.WalletMismatches != 0 {
		t.Fatalf("erased provenance: %+v %v", r, err)
	}
	if err := s.Finish(ctx, o); err != nil {
		t.Fatal("original outcome replay", err)
	}
	o.Players[0].Points++
	if err := s.Finish(ctx, o); !errors.Is(err, ErrValueConflict) {
		t.Fatal("changed outcome accepted", err)
	}
}

func TestTextPrivacyErasedAwardAdvancesWithoutPaidReceipt(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	at := valueTime(time.Now().UTC())
	m, ids := valueMatch(t, s, db, at, false)
	textPrivacyFence(t, db, ids[0])
	a := TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: m.Epoch, AccountID: ids[0], Kind: "correct_vote", Ordinal: 1, Amount: s.tuning.Noin.CorrectVote, At: at.Add(time.Second)}
	if n, err := s.Award(ctx, a); err != nil || n != 0 {
		t.Fatalf("erased accepted award: credited=%d err=%v", n, err)
	}
	if err := s.Interrupt(ctx, m.Contract.MatchID, m.Owner, m.Epoch, at.Add(2*time.Second)); err != nil {
		t.Fatal("erased participant stranded interruption", err)
	}
	if n, err := s.Award(ctx, a); err != nil || n != 0 {
		t.Fatalf("erased event replay after terminal: credited=%d err=%v", n, err)
	}
	a.Amount++
	if _, err := s.Award(ctx, a); !errors.Is(err, ErrValueConflict) {
		t.Fatal("changed erased event accepted", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_award_receipts WHERE account_id=$1`, ids[0]); n != 0 {
		t.Fatal("erasure fabricated paid/capped award receipt")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_outbox WHERE account_id=$1`, ids[0]); n != 0 {
		t.Fatal("interruption resurrected erased delivery")
	}
}

func TestTextPrivacyDeliveryFenceSuppressesEnqueueWithoutAcknowledgment(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	at := valueTime(time.Now().UTC())
	m, ids := valueMatch(t, s, db, at, false)
	if err := s.Finish(ctx, textPrivacyOutcome(m, ids, at)); err != nil {
		t.Fatal(err)
	}
	if err := s.SettlePending(ctx, m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	worker := uuid.NewString()
	deliveries, err := s.ClaimAccountDeliveries(ctx, worker, ids[:1], at.Add(time.Second), time.Minute, 1)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("claim %+v %v", deliveries, err)
	}
	textPrivacyFence(t, db, ids[0])
	called := 0
	err = s.WithTextDeliveryEnqueue(ctx, worker, deliveries[0], func() error { called++; return nil })
	if !errors.Is(err, ErrValueFence) || called != 0 {
		t.Fatalf("post-fence enqueue ran=%d err=%v", called, err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_outbox WHERE account_id=$1 AND acknowledged_at IS NOT NULL`, ids[0]); n != 0 {
		t.Fatal("suppression was falsely acknowledged")
	}
	if rows, err := s.ClaimAccountDeliveries(ctx, worker, ids[:1], at.Add(time.Hour), time.Minute, 1); err != nil || len(rows) != 0 {
		t.Fatalf("fenced delivery was reclaimed: %+v %v", rows, err)
	}
	if _, err := s.PrivateAwards(ctx, m.Contract.MatchID, ids[0]); !errors.Is(err, ErrValueFence) {
		t.Fatal("deleted account private receipt read", err)
	}
	if err := s.AcknowledgeDelivery(ctx, deliveries[0].ID, worker, ids[0], at.Add(2*time.Second)); !errors.Is(err, ErrValueFence) {
		t.Fatal("deleted account acknowledged", err)
	}
	if r, err := s.Reconcile(ctx); err != nil || r.Suppressed != 1 || r.Undelivered != 3 || r.MissingEffects != 0 {
		t.Fatalf("suppression provenance: %+v %v", r, err)
	}
}

func TestTextPrivacyGenuineReplayAndUnacceptedWorkRefusal(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	at := valueTime(time.Now().UTC())
	m, ids := valueMatch(t, s, db, at, false)
	a := TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Kind: "correct_vote", Ordinal: 1, Amount: s.tuning.Noin.CorrectVote, At: at}
	credited, err := s.Award(ctx, a)
	if err != nil || credited <= 0 {
		t.Fatal(credited, err)
	}
	textPrivacyFence(t, db, ids[0])
	if got, err := s.Award(ctx, a); err != nil || got != credited {
		t.Fatal("original paid receipt changed", got, err)
	}
	for _, mutation := range []string{"changed_replay", "bad_amount", "unknown_seat", "old_time", "wrong_owner"} {
		changed := a
		changed.Ordinal = 2
		switch mutation {
		case "changed_replay":
			changed.Ordinal = 1
			changed.Amount++
		case "bad_amount":
			changed.Amount++
		case "unknown_seat":
			changed.AccountID = valueAccount(t, db)
		case "old_time":
			changed.At = at.Add(-time.Second)
		case "wrong_owner":
			changed.Owner = uuid.NewString()
		}
		if _, err := s.Award(ctx, changed); err == nil {
			t.Fatal("unaccepted event erased", mutation)
		}
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_value_erasure_dispositions`); n != 0 {
		t.Fatal("unaccepted disposition", n)
	}
	if err := s.Reserve(ctx, TextReservation{ID: uuid.NewString(), AccountID: ids[0], EntryPath: "local", At: at}); err == nil {
		t.Fatal("ordinary admission used tombstone authority")
	}
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, ids[1]); err != nil {
		t.Fatal(err)
	}
	a.AccountID = ids[1]
	if _, err := s.Award(ctx, a); err == nil {
		t.Fatal("unbacked tombstone accepted")
	}
}

func TestTextPrivacyAbandonCancellationAndLostOwnerRecovery(t *testing.T) {
	db, base := textValueDB(t)
	ctx := context.Background()
	at := valueTime(time.Now().UTC())
	owner, s := ownerReady(t, db, base)
	active, ids := ownerMatch(t, owner, s, db, at, true)
	prepared, waiting := ownerMatch(t, owner, s, db, at, false)
	standalone := valueAccount(t, db)
	reservation := uuid.NewString()
	if err := s.Reserve(ctx, TextReservation{ID: reservation, AccountID: standalone, EntryPath: "local", At: at}); err != nil {
		t.Fatal(err)
	}
	for _, account := range []string{ids[0], waiting[0], standalone} {
		textPrivacyFence(t, db, account)
	}
	abandon := TextAbandon{MatchID: active.Contract.MatchID, Owner: active.Owner, Epoch: 1, AccountID: ids[0], Seat: 0, At: at}
	if err := s.Abandon(ctx, abandon); err != nil {
		t.Fatal(err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM queue_cooldowns WHERE account_id=$1`, ids[0]); n != 0 {
		t.Fatal("erasure recreated cooldown")
	}
	if err := s.Start(ctx, prepared.Contract.MatchID, prepared.Owner, 1, at); err == nil {
		t.Fatal("prepared match started with deleted seat")
	}
	if err := owner.Release(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release(ctx)
	replacement, err := base.WithOwner(next)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := next.RecoverLostOwners(ctx, replacement, 10)
	if err != nil || !recovered.Done || recovered.Cancelled != 1 || recovered.Interrupted != 1 || recovered.Released != 1 {
		t.Fatalf("owner recovery %+v %v", recovered, err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_outbox WHERE match_id=$1`, active.Contract.MatchID); n != 3 {
		t.Fatal("survivor interruption deliveries", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_admissions WHERE match_id=$1 AND state='released'`, prepared.Contract.MatchID); n != 4 {
		t.Fatal("prepared cleanup", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_admissions WHERE id=$1 AND state='released'`, reservation); n != 1 {
		t.Fatal("standalone cleanup", n)
	}
}

func TestTextPrivacyErasureCommitFailureIsAtomic(t *testing.T) {
	for _, operation := range []string{"award", "settlement", "interruption"} {
		t.Run(operation, func(t *testing.T) {
			db, s := textValueDB(t)
			ctx := context.Background()
			at := valueTime(time.Now().UTC())
			m, ids := valueMatch(t, s, db, at, false)
			textPrivacyFence(t, db, ids[0])
			if operation == "settlement" {
				if err := s.Finish(ctx, textPrivacyOutcome(m, ids, at)); err != nil {
					t.Fatal(err)
				}
			}
			failure := errors.New("injected erasure commit failure")
			s.beforeCommit = func() error { return failure }
			var err error
			switch operation {
			case "award":
				_, err = s.Award(ctx, TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Kind: "correct_vote", Ordinal: 1, Amount: s.tuning.Noin.CorrectVote, At: at})
			case "settlement":
				err = s.settle(ctx, m.Contract.MatchID, ids[0])
			default:
				err = s.Interrupt(ctx, m.Contract.MatchID, m.Owner, 1, at)
			}
			if !errors.Is(err, failure) {
				t.Fatal(err)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_value_erasure_dispositions`); n != 0 {
				t.Fatal("partial erasure receipt", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE state IN ('applied','erased')`); n != 0 {
				t.Fatal("partial terminal effects", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_outbox`); n != 0 {
				t.Fatal("partial private delivery", n)
			}
		})
	}
}

func TestTextPrivacyEnqueueBothOrdersAndNoCallbackRetry(t *testing.T) {
	for _, enqueueFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(enqueueFirst), func(t *testing.T) {
			db, s := textValueDB(t)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			at := valueTime(time.Now().UTC())
			m, ids := valueMatch(t, s, db, at, false)
			if err := s.Finish(ctx, textPrivacyOutcome(m, ids, at)); err != nil {
				t.Fatal(err)
			}
			if err := s.SettlePending(ctx, m.Contract.MatchID); err != nil {
				t.Fatal(err)
			}
			worker := uuid.NewString()
			rows, err := s.ClaimAccountDeliveries(ctx, worker, ids[:1], at.Add(time.Second), time.Minute, 1)
			if err != nil || len(rows) != 1 {
				t.Fatal(rows, err)
			}
			row := rows[0]
			calls := 0
			// A serializable external-effect error must never retry an enqueue.
			if err := s.WithTextDeliveryEnqueue(ctx, worker, row, func() error { calls++; return &pq.Error{Code: "40001"} }); err == nil || calls != 1 {
				t.Fatal("enqueue callback retried", calls, err)
			}
			calls = 0
			request := uuid.NewString()
			fence := func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `SELECT privacy_prepare_verified_request($1,$2,$3,$4)`, request, ids[0], bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
				return err
			}
			done := make(chan error, 1)
			if !enqueueFirst {
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if err = fence(tx); err != nil {
					t.Fatal(err)
				}
				go func() { done <- s.WithTextDeliveryEnqueue(ctx, worker, row, func() error { calls++; return nil }) }()
				waitAdmissionLock(t, ctx, db, "SELECT deleted_at FROM accounts")
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
				if err = <-done; !errors.Is(err, ErrValueFence) || calls != 0 {
					t.Fatal("post-fence enqueue", calls, err)
				}
			} else {
				held, release := make(chan struct{}), make(chan struct{})
				defer func() {
					select {
					case <-release:
					default:
						close(release)
					}
				}()
				go func() {
					done <- s.WithTextDeliveryEnqueue(ctx, worker, row, func() error {
						calls++
						close(held)
						select {
						case <-release:
							return nil
						case <-ctx.Done():
							return ctx.Err()
						}
					})
				}()
				select {
				case <-held:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				deleted := make(chan error, 1)
				go func() { deleted <- WithValueTransaction(ctx, db, fence) }()
				waitAdmissionLock(t, ctx, db, "privacy_prepare_verified_request")
				close(release)
				if err := <-done; err != nil || calls != 1 {
					t.Fatal("pre-fence enqueue", calls, err)
				}
				if err := <-deleted; err != nil {
					t.Fatal(err)
				}
				// This gate permits pre-fence queueing; D6 must handle queued frames
				// at transport teardown before enabling public deletion routes.
			}
			if valueCount(t, db, `SELECT count(*) FROM text_outbox WHERE acknowledged_at IS NOT NULL`) != 0 {
				t.Fatal("enqueue or suppression fabricated ACK")
			}
		})
	}
}

func TestTextPrivacySurvivorReceiptsRemainExactAcrossLocalAndLowPopulation(t *testing.T) {
	for _, local := range []bool{false, true} {
		for _, kind := range []string{"completed", "scored_low_population"} {
			t.Run(fmt.Sprintf("local=%v/%s", local, kind), func(t *testing.T) {
				db, s := textValueDB(t)
				ctx := context.Background()
				at := valueTime(time.Now().UTC())
				m, ids := valueUnpreparedMatch(t, s, db, at, false)
				if local {
					for i, id := range ids {
						if err := s.CancelReservation(ctx, m.AdmissionIDs[i], id); err != nil {
							t.Fatal(err)
						}
						m.AdmissionIDs[i] = uuid.NewString()
						if err := s.Reserve(ctx, TextReservation{ID: m.AdmissionIDs[i], AccountID: id, EntryPath: "local", At: at}); err != nil {
							t.Fatal(err)
						}
					}
					m.Contract.Eligibility.EntryPath = "local"
					m.Contract.Eligibility.Leaderboard = false
					m.SponsorAccountID = ids[0]
				}
				if err := s.Prepare(ctx, m, at); err != nil {
					t.Fatal(err)
				}
				if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, at); err != nil {
					t.Fatal(err)
				}
				o := textPrivacyOutcome(m, ids, at)
				o.Kind = kind
				if kind == "scored_low_population" {
					o.Winner = ""
					o.Players[0].Absent = true
				}
				if err := s.Finish(ctx, o); err != nil {
					t.Fatal(err)
				}
				if err := s.settle(ctx, m.Contract.MatchID, ids[3]); err != nil {
					t.Fatal(err)
				}
				snapshot := func() []string {
					var out []string
					for _, table := range []string{"text_award_receipts", "noin_ledger", "text_settlements", "text_outbox", "profiles", "noin_wallets"} {
						var body string
						if err := db.QueryRow(`SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM `+table+` r WHERE account_id=$1`, ids[3]).Scan(&body); err != nil {
							t.Fatal(err)
						}
						out = append(out, body)
					}
					return out
				}
				before := snapshot()
				textPrivacyFence(t, db, ids[0])
				if err := s.SettlePending(ctx, m.Contract.MatchID); err != nil {
					t.Fatal(err)
				}
				after := snapshot()
				if !reflect.DeepEqual(before, after) {
					t.Fatal("survivor committed bytes changed")
				}
				if valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE state='applied'`) != 3 || valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE state='erased'`) != 1 {
					t.Fatal("terminal disposition coverage")
				}
			})
		}
	}
}

func TestTextPrivacyEnqueueLeaseExpiresDuringOutboxWait(t *testing.T) {
	db, s := textValueDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	at := valueTime(time.Now().UTC())
	m, ids := valueMatch(t, s, db, at, false)
	if err := s.Finish(ctx, textPrivacyOutcome(m, ids, at)); err != nil {
		t.Fatal(err)
	}
	if err := s.SettlePending(ctx, m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	worker := uuid.NewString()
	rows, err := s.ClaimAccountDeliveries(ctx, worker, ids[:1], at.Add(time.Second), time.Minute, 1)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE text_outbox SET claim_until=clock_timestamp()+interval '300 milliseconds' WHERE id=$1`, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	block, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer block.Rollback()
	if _, err = block.ExecContext(ctx, `SELECT id FROM text_outbox WHERE id=$1 FOR UPDATE`, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	calls := 0
	done := make(chan error, 1)
	go func() { done <- s.WithTextDeliveryEnqueue(ctx, worker, rows[0], func() error { calls++; return nil }) }()
	waitAdmissionLock(t, ctx, db, "SELECT payload,claim_until FROM text_outbox")
	// The row itself is unchanged while held, so a pre-lock predicate must not
	// substitute for fresh PostgreSQL lease authority after the wait.
	if _, err := db.ExecContext(ctx, `SELECT pg_sleep(0.35)`); err != nil {
		t.Fatal(err)
	}
	if err = block.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, ErrValueFence) || calls != 0 {
		t.Fatal("expired enqueue", calls, err)
	}
	if valueCount(t, db, `SELECT count(*) FROM text_outbox WHERE acknowledged_at IS NOT NULL`) != 0 {
		t.Fatal("expired lease acknowledged")
	}
}

func TestTextPrivacyMigrationEmptyDownAndReupgradePreserveOriginal(t *testing.T) {
	db, s := textValueDB(t)
	at := valueTime(time.Now().UTC())
	m, ids := valueMatch(t, s, db, at, false)
	if err := s.Finish(t.Context(), textPrivacyOutcome(m, ids, at)); err != nil {
		t.Fatal(err)
	}
	if err := s.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var body string
		if err := db.QueryRow(`SELECT jsonb_agg(to_jsonb(s)-'erasure_disposition_id'-'erased_at' ORDER BY account_id)::text FROM text_settlements s`).Scan(&body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	before := snapshot()
	if err := runMigration(db, "../../migrations", "empty erasure rollback", func(m *migrate.Migrate) error { return m.Migrate(35) }); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(); got != before {
		t.Fatal("down changed original settlement")
	}
	if err := MigrateUp(db, transitionMigrationPath(t, 36)); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(); got != before {
		t.Fatal("upgrade changed original settlement")
	}
	if valueCount(t, db, `SELECT count(*) FROM text_value_erasure_dispositions`) != 0 {
		t.Fatal("migration invented erasure")
	}
}

func TestTextPrivacyMigrationRefusesMismatchedRequestSubjectAtomically(t *testing.T) {
	db := transitionDesignDB(t, 35, false)
	one, two := valueAccount(t, db), valueAccount(t, db)
	privacyPrepare(t, db, one)
	// Explicit historical corruption: pre-36 FKs did not bind these subjects.
	if _, err := db.Exec(`UPDATE account_deletion_fences SET account_id=$2 WHERE account_id=$1`, one, two); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(db, transitionMigrationPath(t, 36)); err == nil {
		t.Fatal("mismatched request/admission subject migrated")
	}
	if valueCount(t, db, `SELECT count(*) FROM pg_constraint WHERE conrelid='privacy_requests'::regclass AND conname='privacy_request_subject'`) != 0 {
		t.Fatal("failed migration left partial authority constraint")
	}
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('text_value_erasure_dispositions') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("failed migration left new authority table", exists, err)
	}
	if valueCount(t, db, `SELECT count(*) FROM privacy_requests WHERE account_id=$1`, one) != 1 || valueCount(t, db, `SELECT count(*) FROM account_deletion_fences WHERE account_id=$1`, two) != 1 {
		t.Fatal("refusal rewrote historical evidence")
	}
}
