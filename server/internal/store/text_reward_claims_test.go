package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/knowoff/knowoff/server/internal/config"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func rewardConfig() config.RewardedConfig {
	return config.RewardedConfig{Enabled: true, MaxQueryBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 2, KeyCacheS: 60, KeyRefreshMinS: 1, MaxConcurrentRequests: 4, ClaimTTLS: 60, MaxClaimsPerMatchWindow: 2, AdUnits: map[string]config.RewardedUnit{"123": {RewardItem: "match_bonus", RewardAmount: 1}}}
}
func rewardAuthorize(context.Context, *sql.Tx) error { return nil }
func rewardSettled(t *testing.T, db *sql.DB, s *TextValueStore, at time.Time) (TextMatchRecord, []string) {
	return rewardSettledPremium(t, db, s, at, false)
}
func rewardSettledPremium(t *testing.T, db *sql.DB, s *TextValueStore, at time.Time, premium bool) (TextMatchRecord, []string) {
	t.Helper()
	m, ids := valuePreparedMatch(t, s, db, at, false)
	if premium {
		if _, e := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',$2)`, ids[0], at.Add(time.Hour)); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.Start(t.Context(), m.Contract.MatchID, m.Owner, 1, at); e != nil {
		t.Fatal(e)
	}
	o := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: at.Add(time.Second)}
	for i, id := range ids {
		role := "nower"
		if i == 0 {
			role = "donower"
		}
		o.Players = append(o.Players, TextPlayerResult{AccountID: id, Seat: i, Role: role})
	}
	if err := s.Finish(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if err := s.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	return m, ids
}
func TestTextRewardClaimReplacementWindowAndConcurrentBudget(t *testing.T) {
	db, v := textValueDB(t)
	at := time.Now().UTC().Truncate(time.Millisecond)
	m, ids := rewardSettled(t, db, v, at.Add(-time.Minute))
	s, err := NewTextRewardStore(db, rewardConfig())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return at }
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other, _ := NewTextRewardStore(db, rewardConfig())
			other.now = s.now
			_, e := other.IssueClaim(t.Context(), m.Contract.MatchID, ids[0], "123", rewardAuthorize)
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for e := range results {
		if e == nil {
			accepted++
		} else if !errors.Is(e, ErrRewardClaimBusy) {
			t.Fatal(e)
		}
	}
	if accepted != 2 {
		t.Fatalf("accepted=%d", accepted)
	}
	at = at.Add(time.Minute - time.Millisecond)
	if _, e := s.IssueClaim(t.Context(), m.Contract.MatchID, ids[0], "123", rewardAuthorize); !errors.Is(e, ErrRewardClaimBusy) {
		t.Fatal("early window", e)
	}
	at = at.Add(time.Millisecond)
	claim, e := s.IssueClaim(t.Context(), m.Contract.MatchID, ids[0], "123", rewardAuthorize)
	if e != nil || claim.Claim == "" {
		t.Fatal(claim, e)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_reward_claims`); n != 3 {
		t.Fatal(n)
	}
}
func TestTextRewardSSVExactDelayedReplayAndTemporalBounds(t *testing.T) {
	db, v := textValueDB(t)
	at := time.Now().UTC().Truncate(time.Millisecond)
	m, ids := rewardSettled(t, db, v, at.Add(-time.Minute))
	s, _ := NewTextRewardStore(db, rewardConfig())
	s.now = func() time.Time { return at }
	claim, e := s.IssueClaim(t.Context(), m.Contract.MatchID, ids[0], "123", rewardAuthorize)
	if e != nil {
		t.Fatal(e)
	}
	base := VerifiedRewardInteraction{TransactionID: "abcdef", Fingerprint: strings.Repeat("b", 64), Claim: claim.Claim, AdUnit: "123", OccurredAt: at}
	before := valueCount(t, db, `SELECT COALESCE(sum(amount),0) FROM noin_ledger`)
	for _, occurred := range []time.Time{at.Add(-time.Millisecond), claim.ExpiresAt, claim.ExpiresAt.Add(time.Millisecond)} {
		bad := base
		bad.OccurredAt = occurred
		if e = s.RecordVerifiedSSV(t.Context(), bad); !errors.Is(e, ErrRewardClaimInvalid) {
			t.Fatal("invalid occurrence", occurred, e)
		}
	}
	at = claim.ExpiresAt.Add(time.Hour)
	if e = s.RecordVerifiedSSV(t.Context(), base); e != nil {
		t.Fatal("late delivery", e)
	}
	if e = s.RecordVerifiedSSV(t.Context(), base); e != nil {
		t.Fatal("exact replay", e)
	}
	bad := base
	bad.Fingerprint = strings.Repeat("c", 64)
	if e = s.RecordVerifiedSSV(t.Context(), bad); !errors.Is(e, ErrRewardClaimConflict) {
		t.Fatal("conflicting replay", e)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_reward_ssv_receipts`); n != 1 {
		t.Fatal(n)
	}
	if e = s.RecordVerifiedSSV(t.Context(), base); e != nil {
		t.Fatal(e)
	}
	if after := valueCount(t, db, `SELECT COALESCE(sum(amount),0) FROM noin_ledger`); after != before {
		t.Fatal("verification granted value")
	}
}

func TestTextRewardSchemaRetainsExactVerificationWithoutValue(t *testing.T) {
	db, values := textValueDB(t)
	at := valueTime(time.Now().UTC())
	m, ids := valueMatch(t, values, db, at, false)
	hash := strings.Repeat("a", 64)
	if _, err := db.Exec(`INSERT INTO text_reward_claims(claim_hash,match_id,account_id,ad_unit,issued_at,expires_at) VALUES($1,$2,$3,'123',$4,$5)`, hash, m.Contract.MatchID, ids[0], at, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO text_reward_ssv_receipts(provider_transaction_id,fingerprint,claim_hash,occurred_at,received_at) VALUES('abcdef',$1,$2,$3,$4)`, strings.Repeat("b", 64), hash, at, at.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE text_reward_claims SET ad_unit='999'`,
		`DELETE FROM text_reward_claims`,
		`TRUNCATE text_reward_claims CASCADE`,
		`UPDATE text_reward_ssv_receipts SET fingerprint=repeat('c',64)`,
		`DELETE FROM text_reward_ssv_receipts`,
		`TRUNCATE text_reward_ssv_receipts`,
	} {
		if _, err := db.Exec(query); err == nil {
			t.Fatalf("retained identity changed: %s", query)
		}
	}
	if _, err := db.Exec(`INSERT INTO text_reward_ssv_receipts(provider_transaction_id,fingerprint,claim_hash,occurred_at,received_at) VALUES('abcdef',$1,$2,$3,$3)`, strings.Repeat("c", 64), hash, at); err == nil {
		t.Fatal("provider transaction reused")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger`); n != 0 {
		t.Fatalf("verification granted value: %d", n)
	}
}

func TestTextRewardRefusalAndFinalAuthorizationRollback(t *testing.T) {
	for _, mode := range []string{"active", "interrupted", "prototype", "cross_account", "historical_unknown", "premium", "deleted", "fenced", "development", "missing_authorization", "final_authorization", "commit_failure"} {
		t.Run(mode, func(t *testing.T) {
			db, v := textValueDB(t)
			at := time.Now().UTC().Truncate(time.Millisecond)
			var m TextMatchRecord
			var ids []string
			if mode == "active" || mode == "interrupted" || mode == "prototype" {
				m, ids = valueMatch(t, v, db, at, mode == "prototype")
				if mode == "interrupted" {
					if e := v.Interrupt(t.Context(), m.Contract.MatchID, m.Owner, 1, at); e != nil {
						t.Fatal(e)
					}
				}
			} else {
				m, ids = rewardSettledPremium(t, db, v, at, mode == "premium")
			}
			s, _ := NewTextRewardStore(db, rewardConfig())
			s.now = func() time.Time { return at.Add(time.Minute) }
			account := ids[0]
			authorize := rewardAuthorize
			switch mode {
			case "cross_account":
				account = valueAccount(t, db)
			case "historical_unknown":
				// Simulate a retained pre31 match, for which migration31 intentionally
				// creates no guessed historical eligibility row.
				if _, e := db.Exec(`ALTER TABLE text_bonus_eligibility DISABLE TRIGGER text_bonus_eligibility_immutable; DELETE FROM text_bonus_eligibility; ALTER TABLE text_bonus_eligibility ENABLE TRIGGER text_bonus_eligibility_immutable`); e != nil {
					t.Fatal(e)
				}
			case "deleted":
				if _, e := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, account); e != nil {
					t.Fatal(e)
				}
			case "fenced":
				if _, e := db.Exec(`WITH r AS (INSERT INTO privacy_requests(id,account_id,policy_version,manifest_sha256,proof_sha256,status_token_sha256,verified_at,active_due_at,evidence_until,phase) VALUES(gen_random_uuid(),$1,'deletion-2026-09-19',decode(repeat('a',64),'hex'),decode(repeat('b',64),'hex'),decode(repeat('c',64),'hex'),now(),now()+interval '30 days',now()+interval '180 days','prepared') RETURNING id,account_id) INSERT INTO account_deletion_fences(account_id,request_id,fenced_at) SELECT account_id,id,now() FROM r`, account); e != nil {
					t.Fatal(e)
				}
			case "development":
				if _, e := db.Exec(`ALTER TABLE accounts DISABLE TRIGGER account_auth_purpose; UPDATE accounts SET auth_purpose='development'; ALTER TABLE accounts ENABLE TRIGGER account_auth_purpose`); e != nil {
					t.Fatal(e)
				}
			case "missing_authorization":
				authorize = nil
			case "final_authorization":
				calls := 0
				authorize = func(context.Context, *sql.Tx) error {
					calls++
					if calls == 2 {
						return errors.New("expired after write wait")
					}
					return nil
				}
			case "commit_failure":
				s.beforeCommit = func() error { return errors.New("commit rejected") }
			}
			claim, e := s.IssueClaim(t.Context(), m.Contract.MatchID, account, "123", authorize)
			if e == nil || claim.Claim != "" {
				t.Fatalf("refusal leaked claim: %+v %v", claim, e)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_reward_claims`); n != 0 {
				t.Fatal("refusal retained claim", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_reward_ssv_receipts`); n != 0 {
				t.Fatal("refusal retained verification", n)
			}
		})
	}
}

func TestTextRewardSSVCrossClaimConflictAndRollback(t *testing.T) {
	db, v := textValueDB(t)
	at := time.Now().UTC().Truncate(time.Millisecond)
	m, ids := rewardSettled(t, db, v, at.Add(-time.Minute))
	s, _ := NewTextRewardStore(db, rewardConfig())
	s.now = func() time.Time { return at }
	a, e := s.IssueClaim(t.Context(), m.Contract.MatchID, ids[0], "123", rewardAuthorize)
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.IssueClaim(t.Context(), m.Contract.MatchID, ids[1], "123", rewardAuthorize)
	if e != nil {
		t.Fatal(e)
	}
	p := VerifiedRewardInteraction{TransactionID: "abcdef", Fingerprint: strings.Repeat("a", 64), Claim: a.Claim, AdUnit: "123", OccurredAt: at}
	s.beforeCommit = func() error { return errors.New("receipt failure") }
	if e = s.RecordVerifiedSSV(t.Context(), p); e == nil {
		t.Fatal("failure swallowed")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_reward_ssv_receipts`); n != 0 {
		t.Fatal("partial receipt")
	}
	s.beforeCommit = nil
	if e = s.RecordVerifiedSSV(t.Context(), p); e != nil {
		t.Fatal(e)
	}
	p.Claim = b.Claim
	if e = s.RecordVerifiedSSV(t.Context(), p); !errors.Is(e, ErrRewardClaimConflict) {
		t.Fatal("cross-account transaction reused", e)
	}
	p.TransactionID = "abcd00"
	if _, e = db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, ids[1]); e != nil {
		t.Fatal(e)
	}
	if e = s.RecordVerifiedSSV(t.Context(), p); e == nil {
		t.Fatal("deleted account acquired new receipt")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_reward_ssv_receipts`); n != 1 {
		t.Fatal(n)
	}
}

func TestTextRewardDeletionAndClaimBothLockOrders(t *testing.T) {
	for _, claimFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete_first", true: "claim_first"}[claimFirst], func(t *testing.T) {
			db, v := textValueDB(t)
			at := time.Now().UTC().Truncate(time.Millisecond)
			m, ids := rewardSettled(t, db, v, at.Add(-time.Minute))
			s, _ := NewTextRewardStore(db, rewardConfig())
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			if !claimFirst {
				tx, e := db.BeginTx(ctx, nil)
				if e != nil {
					t.Fatal(e)
				}
				defer tx.Rollback()
				if e = LockValueAccount(ctx, tx, ids[0]); e != nil {
					t.Fatal(e)
				}
				if _, e = tx.ExecContext(ctx, `UPDATE accounts SET deleted_at=now() WHERE id=$1`, ids[0]); e != nil {
					t.Fatal(e)
				}
				go func() { _, e := s.IssueClaim(ctx, m.Contract.MatchID, ids[0], "123", rewardAuthorize); done <- e }()
				waitAdmissionLock(t, ctx, db, "SELECT id FROM accounts")
				if e = tx.Commit(); e != nil {
					t.Fatal(e)
				}
				if e = <-done; e == nil {
					t.Fatal("claim passed committed deletion")
				}
			} else {
				held, release := make(chan struct{}), make(chan struct{})
				calls := 0
				guard := func(context.Context, *sql.Tx) error {
					calls++
					if calls == 1 {
						close(held)
						<-release
					}
					return nil
				}
				go func() { _, e := s.IssueClaim(ctx, m.Contract.MatchID, ids[0], "123", guard); done <- e }()
				<-held
				deleted := make(chan error, 1)
				go func() {
					deleted <- WithValueTransaction(ctx, db, func(tx *sql.Tx) error {
						if e := LockValueAccount(ctx, tx, ids[0]); e != nil {
							return e
						}
						_, e := tx.ExecContext(ctx, `UPDATE accounts SET deleted_at=now() WHERE id=$1`, ids[0])
						return e
					})
				}()
				waitAdmissionLock(t, ctx, db, "SELECT id FROM accounts")
				close(release)
				if e := <-done; e != nil {
					t.Fatal(e)
				}
				if e := <-deleted; e != nil {
					t.Fatal(e)
				}
			}
			want := int64(0)
			if claimFirst {
				want = 1
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_reward_claims`); n != want {
				t.Fatal(n, want)
			}
		})
	}
}

func TestTextRewardCanceledAndDownPreserveEvidence(t *testing.T) {
	db, v := textValueDB(t)
	at := time.Now().UTC().Truncate(time.Millisecond)
	m, ids := rewardSettled(t, db, v, at.Add(-time.Minute))
	s, _ := NewTextRewardStore(db, rewardConfig())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e := s.IssueClaim(ctx, m.Contract.MatchID, ids[0], "123", rewardAuthorize); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_reward_claims`); n != 0 {
		t.Fatal(n)
	}
	if _, e := s.IssueClaim(t.Context(), m.Contract.MatchID, ids[0], "123", rewardAuthorize); e != nil {
		t.Fatal(e)
	}
	down, e := os.ReadFile(filepath.Join("..", "..", "migrations", "000034_text_reward_verification.down.sql"))
	if e != nil {
		t.Fatal(e)
	}
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	if _, e = conn.ExecContext(t.Context(), string(down)); e == nil {
		t.Fatal("retained evidence dropped")
	}
	if _, e = conn.ExecContext(t.Context(), "ROLLBACK"); e != nil {
		t.Fatal(e)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_reward_claims`); n != 1 {
		t.Fatal(n)
	}
}
