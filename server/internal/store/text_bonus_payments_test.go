package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func bonusSnapshot(t *testing.T, db *sql.DB, account string) string {
	t.Helper()
	var result string
	if err := db.QueryRow(`SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text) FROM noin_wallets x WHERE account_id=$1),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text) FROM noin_ledger x WHERE account_id=$1),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text) FROM daily_noin_earned x WHERE account_id=$1),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text) FROM text_bonus_payments x WHERE account_id=$1),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text) FROM text_bonus_payment_items x WHERE account_id=$1))::text`, account).Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func bonusProof(t *testing.T, db *sql.DB, match, account string, at time.Time, txn string) {
	t.Helper()
	r, err := NewTextRewardStore(db, rewardConfig())
	if err != nil {
		t.Fatal(err)
	}
	r.now = func() time.Time { return at }
	c, err := r.IssueClaim(t.Context(), match, account, "123", rewardAuthorize)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.RecordVerifiedSSV(t.Context(), VerifiedRewardInteraction{TransactionID: txn, Fingerprint: strings.Repeat(txn[:1], 64), Claim: c.Claim, AdUnit: "123", OccurredAt: at.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
}

func TestTextBonusPaymentSSVConcurrentDelayedAndStableProof(t *testing.T) {
	db, v := textValueDB(t)
	at := valueTime(time.Now().UTC().Add(-time.Hour))
	m, ids := rewardSettled(t, db, v, at)
	if _, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0]); !errors.Is(err, ErrTextBonusIneligible) {
		t.Fatal("unverified payout", err)
	}
	// Every interaction was issued before any callback arrived. New ad offers
	// stop once proof exists; delayed callbacks retain their original intervals.
	rc, _ := NewTextRewardStore(db, rewardConfig())
	proofs := make([]VerifiedRewardInteraction, 0, 3)
	for i, txn := range []string{"bbbb", "aaaa", "0000"} {
		issued := at.Add(time.Minute)
		if i == 2 {
			issued = at.Add(3 * time.Minute)
		}
		rc.now = func() time.Time { return issued }
		c, err := rc.IssueClaim(t.Context(), m.Contract.MatchID, ids[0], "123", rewardAuthorize)
		if err != nil {
			t.Fatal(err)
		}
		proofs = append(proofs, VerifiedRewardInteraction{TransactionID: txn, Fingerprint: strings.Repeat(txn[:1], 64), Claim: c.Claim, AdUnit: "123", OccurredAt: issued.Add(time.Second)})
	}
	for _, proof := range proofs[:2] {
		if err := rc.RecordVerifiedSSV(t.Context(), proof); err != nil {
			t.Fatal(err)
		}
	}
	// Both claims are expired now; their signed occurrence remains valid.
	var wg sync.WaitGroup
	results := make(chan TextBonusPayment, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := NewTextValueStore(db, v.tuning)
			p, e := other.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
			results <- p
			failures <- e
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	var prior string
	for p := range results {
		b, _ := json.Marshal(p)
		if p.Source != "ssv" || p.ProviderTransactionID != "aaaa" || p.Credited <= 0 {
			t.Fatal(p)
		}
		if prior != "" && prior != string(b) {
			t.Fatal("replay changed receipt")
		}
		prior = string(b)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_payments`); n != 1 {
		t.Fatal(n)
	}
	if err := db.QueryRow(`SELECT count(*) FROM text_reward_ssv_receipts`).Scan(new(int)); err != nil {
		t.Fatal(err)
	}
	// A later smaller proof must not change the retained selected transaction.
	if err := rc.RecordVerifiedSSV(t.Context(), proofs[2]); err != nil {
		t.Fatal(err)
	}
	p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
	if err != nil || p.ProviderTransactionID != "aaaa" {
		t.Fatal(p, err)
	}
	r, err := v.Reconcile(t.Context())
	if err != nil || r.LedgerMismatches != 0 || r.WalletMismatches != 0 {
		t.Fatal("bonus reconciliation", r, err)
	}
}

func TestTextBonusPaymentOriginalDaysCapAndPinnedStart(t *testing.T) {
	db, v := textValueDB(t)
	at := valueDay(time.Now().UTC()).Add(-24*time.Hour - 5*time.Second)
	m, ids := valuePreparedMatch(t, v, db, at, false)
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',$2)`, ids[0], at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := v.Start(t.Context(), m.Contract.MatchID, m.Owner, 1, at); err != nil {
		t.Fatal(err)
	}
	for i, stamp := range []time.Time{at.Add(time.Second), at.Add(6 * time.Second)} {
		if _, err := v.Award(t.Context(), TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Kind: "correct_vote", Ordinal: i + 1, Amount: v.tuning.Noin.CorrectVote, At: stamp}); err != nil {
			t.Fatal(err)
		}
	}
	o := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "scored_low_population", At: at.Add(10 * time.Second)}
	for i, id := range ids {
		role := "nower"
		if i == 0 {
			role = "donower"
		}
		o.Players = append(o.Players, TextPlayerResult{AccountID: id, Seat: i, Role: role})
	}
	if err := v.Finish(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if err := v.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	for i, day := range []time.Time{valueDay(at), valueDay(o.At)} {
		var earned int
		if err := db.QueryRow(`SELECT earned FROM daily_noin_earned WHERE account_id=$1 AND server_day=$2`, ids[0], day).Scan(&earned); err != nil {
			t.Fatal(err)
		}
		delta := v.tuning.Noin.DailyEarnCap - earned - (i + 1)
		if delta < 0 {
			t.Fatal("fixture cap too small")
		}
		if _, err := db.Exec(`INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,'points_conversion',$2,'fixture earlier earned value',$3)`, ids[0], delta, day); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE daily_noin_earned SET earned=earned+$3 WHERE account_id=$1 AND server_day=$2`, ids[0], day, delta); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE noin_wallets SET balance=balance+$2 WHERE account_id=$1`, ids[0], delta); err != nil {
			t.Fatal(err)
		}
	}
	// Current entitlement and config cannot override the authoritative start.
	if _, err := db.Exec(`DELETE FROM entitlements WHERE account_id=$1`, ids[0]); err != nil {
		t.Fatal(err)
	}
	v.tuning.Noin.DailyEarnCap = 0
	p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
	if err != nil || p.Credited != 3 || p.Requested <= 3 {
		t.Fatal(p, err)
	}
	byDay := map[string]int{}
	for _, i := range p.Items {
		byDay[i.ServerDay.Format("2006-01-02")] += i.Credited
	}
	if byDay[valueDay(at).Format("2006-01-02")] != 1 || byDay[valueDay(o.At).Format("2006-01-02")] != 2 {
		t.Fatal(byDay)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM daily_noin_earned WHERE account_id=$1 AND server_day=CURRENT_DATE`, ids[0]); n != 0 {
		t.Fatal("callback-day bucket invented")
	}
	before := bonusSnapshot(t, db, ids[0])
	p2, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
	if err != nil || p2.Credited != 3 || bonusSnapshot(t, db, ids[0]) != before {
		t.Fatal(p2, err)
	}
}

func TestTextBonusPaymentRollbackAndHistoricalDeletion(t *testing.T) {
	db, v := textValueDB(t)
	m, ids := rewardSettledPremium(t, db, v, valueTime(time.Now().UTC().Add(-time.Minute)), true)
	before := bonusSnapshot(t, db, ids[0])
	injected := errors.New("before bonus commit")
	v.beforeCommit = func() error { return injected }
	if p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0]); !errors.Is(err, injected) || p.MatchID != "" {
		t.Fatal("false success", p, err)
	}
	if bonusSnapshot(t, db, ids[0]) != before {
		t.Fatal("partial payment survived rollback")
	}
	v.beforeCommit = nil
	p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	before = bonusSnapshot(t, db, ids[0])
	textPrivacyFence(t, db, ids[0])
	replay, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(p)
	b, _ := json.Marshal(replay)
	if string(a) != string(b) || bonusSnapshot(t, db, ids[0]) != before {
		t.Fatal("historical replay changed value")
	}
}

func TestTextBonusPaymentIneligibleAndRecoveryProgress(t *testing.T) {
	db, v := textValueDB(t)
	at := valueTime(time.Now().UTC().Add(-time.Minute))
	m, ids := rewardSettled(t, db, v, at)
	for i, id := range ids[:3] {
		bonusProof(t, db, m.Contract.MatchID, id, at.Add(10*time.Second), fmt.Sprintf("aa%02x", i))
	}
	textPrivacyFence(t, db, ids[0])
	before := bonusSnapshot(t, db, ids[0])
	if _, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0]); !errors.Is(err, ErrTextBonusIneligible) {
		t.Fatal(err)
	}
	if n, err := v.RecoverBonuses(t.Context(), 10); err != nil || n != 2 {
		t.Fatal("recovery stranded survivors", n, err)
	}
	if bonusSnapshot(t, db, ids[0]) != before {
		t.Fatal("deleted value recreated")
	}
	if n, err := v.RecoverBonuses(t.Context(), 10); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	// A new Premium purchase does not retrospectively replace missing ad proof.
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',NULL)`, ids[3]); err != nil {
		t.Fatal(err)
	}
	if _, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[3]); !errors.Is(err, ErrTextBonusIneligible) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := v.ApplyBonus(ctx, m.Contract.MatchID, ids[3]); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestTextBonusPaymentZeroBaseAndAlreadyCappedAreTerminal(t *testing.T) {
	for _, zeroBase := range []bool{true, false} {
		t.Run(fmt.Sprint(zeroBase), func(t *testing.T) {
			db, v := textValueDB(t)
			at := valueTime(time.Now().UTC().Add(-time.Minute))
			m, ids := valuePreparedMatch(t, v, db, at, false)
			if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type) VALUES($1,'premium_monthly')`, ids[0]); err != nil {
				t.Fatal(err)
			}
			if err := v.Start(t.Context(), m.Contract.MatchID, m.Owner, 1, at); err != nil {
				t.Fatal(err)
			}
			if zeroBase {
				if _, err := db.Exec(`INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES($1,$2,$3)`, ids[0], valueDay(at), v.tuning.Noin.DailyEarnCap); err != nil {
					t.Fatal(err)
				}
			}
			if err := v.Finish(t.Context(), textPrivacyOutcome(m, ids, at)); err != nil {
				t.Fatal(err)
			}
			if err := v.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
				t.Fatal(err)
			}
			if !zeroBase {
				if _, err := db.Exec(`UPDATE daily_noin_earned SET earned=$2 WHERE account_id=$1`, ids[0], v.tuning.Noin.DailyEarnCap); err != nil {
					t.Fatal(err)
				}
			}
			p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
			if err != nil || p.Credited != 0 || len(p.Items) == 0 {
				t.Fatal(p, err)
			}
			if (p.Requested == 0) != zeroBase {
				t.Fatal("zero source/cap conflated", p)
			}
			for _, i := range p.Items {
				if i.LedgerID != nil || i.Credited != 0 {
					t.Fatal(i)
				}
			}
			before := bonusSnapshot(t, db, ids[0])
			if _, err = v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0]); err != nil {
				t.Fatal(err)
			}
			if bonusSnapshot(t, db, ids[0]) != before {
				t.Fatal("zero retry wrote value")
			}
			if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger WHERE event_type='match_bonus'`); n != 0 {
				t.Fatal(n)
			}
		})
	}
}

func TestTextBonusPaymentDeletionBothLockOrders(t *testing.T) {
	for _, paymentFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(paymentFirst), func(t *testing.T) {
			db, v := textValueDB(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			m, ids := rewardSettledPremium(t, db, v, valueTime(time.Now().UTC().Add(-time.Minute)), true)
			account := ids[0]
			beforeSurvivor := bonusSnapshot(t, db, ids[1])
			done := make(chan error, 1)
			fence := func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `SELECT privacy_prepare_verified_request($1,$2,$3,$4)`, uuid.NewString(), account, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
				return err
			}
			pay := func() { _, err := v.ApplyBonus(ctx, m.Contract.MatchID, account); done <- err }
			if paymentFirst {
				held, release := make(chan struct{}), make(chan struct{})
				var once sync.Once
				v.beforeCommit = func() error {
					once.Do(func() { close(held) })
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				go pay()
				select {
				case <-held:
				case err := <-done:
					t.Fatal(err)
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
			} else {
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if err = fence(tx); err != nil {
					t.Fatal(err)
				}
				go pay()
				waitAdmissionLock(t, ctx, db, "SELECT deleted_at,auth_purpose FROM accounts")
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
				if err = <-done; !errors.Is(err, ErrTextBonusIneligible) {
					t.Fatal(err)
				}
			}
			want := int64(0)
			if paymentFirst {
				want = 1
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_payments`); n != want {
				t.Fatal(n, want)
			}
			if bonusSnapshot(t, db, ids[1]) != beforeSurvivor {
				t.Fatal("other participant value changed")
			}
		})
	}
}

func TestTextBonusPaymentEveryWriteFailureAndDeferredBinding(t *testing.T) {
	cases := []struct{ name, table, events, body string }{
		{"payment_write", "text_bonus_payments", "INSERT", `RAISE EXCEPTION 'injected payment';`},
		{"item_write", "text_bonus_payment_items", "INSERT", `RAISE EXCEPTION 'injected item';`},
		{"ledger_write", "noin_ledger", "INSERT", `RAISE EXCEPTION 'injected ledger';`},
		{"wallet_write", "noin_wallets", "INSERT OR UPDATE", `RAISE EXCEPTION 'injected wallet';`},
		{"day_write", "daily_noin_earned", "UPDATE", `RAISE EXCEPTION 'injected day';`},
		{"missing_item", "text_bonus_payment_items", "INSERT", `RETURN NULL;`},
		{"source_hash", "text_bonus_payments", "INSERT", `NEW.source_sha256:=repeat('f',64); RETURN NEW;`},
		{"totals", "text_bonus_payments", "INSERT", `NEW.requested:=NEW.requested+1; RETURN NEW;`},
		{"item_body", "text_bonus_payment_items", "INSERT", `NEW.body_sha256:=repeat('e',64); RETURN NEW;`},
		{"item_day", "text_bonus_payment_items", "INSERT", `NEW.server_day:=NEW.server_day+1; RETURN NEW;`},
		{"ledger_amount", "noin_ledger", "INSERT", `NEW.amount:=NEW.amount+1; RETURN NEW;`},
		{"ledger_identity", "noin_ledger", "INSERT", `NEW.payload:=NEW.payload||'{"ordinal":99}'::jsonb; RETURN NEW;`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, v := textValueDB(t)
			m, ids := rewardSettledPremium(t, db, v, valueTime(time.Now().UTC().Add(-time.Minute)), true)
			before := bonusSnapshot(t, db, ids[0])
			if _, err := db.Exec(`CREATE FUNCTION test_bonus_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN ` + c.body + ` END $$; CREATE TRIGGER test_bonus_fault BEFORE ` + c.events + ` ON ` + c.table + ` FOR EACH ROW EXECUTE FUNCTION test_bonus_fault()`); err != nil {
				t.Fatal(err)
			}
			p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
			if err == nil || p.MatchID != "" {
				t.Fatal("invalid write committed", p, err)
			}
			if bonusSnapshot(t, db, ids[0]) != before {
				t.Fatal("failed write retained partial value")
			}
		})
	}
}

func TestTextBonusPaymentRetentionMigrationAndSourceCorruption(t *testing.T) {
	db, v := textValueDB(t)
	m, ids := rewardSettledPremium(t, db, v, valueTime(time.Now().UTC().Add(-time.Minute)), true)
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000038_text_bonus_payments.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	// Roll back newer dependent schemas first; the genuine base settlement
	// survives the actual empty payment-schema rollback and complete re-upgrade.
	if err = runMigration(db, "../../migrations", "bonus payment rollback", func(m *migrate.Migrate) error { return m.Migrate(37) }); err != nil {
		t.Fatal(err)
	}
	if err = MigrateUp(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err = v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0]); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE text_bonus_payments SET credited=0`, `DELETE FROM text_bonus_payments`, `TRUNCATE text_bonus_payments CASCADE`, `UPDATE text_bonus_payment_items SET credited=0`, `DELETE FROM text_bonus_payment_items`, `TRUNCATE text_bonus_payment_items CASCADE`} {
		if _, err = db.Exec(q); err == nil {
			t.Fatal("retained bonus evidence changed", q)
		}
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(string(down))
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "retained bonus value prevents rollback") {
		t.Fatal("retained rollback did not reach its value guard", err)
	}
	// Privileged corruption fixture proves replay and reconciliation detect a
	// changed original source instead of silently paying a different award set.
	if _, err = db.Exec(`ALTER TABLE text_award_receipts DISABLE TRIGGER text_award_immutable; UPDATE text_award_receipts SET body_hash=repeat('a',64) WHERE account_id='` + ids[0] + `'; ALTER TABLE text_award_receipts ENABLE TRIGGER text_award_immutable`); err != nil {
		t.Fatal(err)
	}
	before := bonusSnapshot(t, db, ids[0])
	if _, err = v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0]); !errors.Is(err, ErrValueConflict) {
		t.Fatal(err)
	}
	if bonusSnapshot(t, db, ids[0]) != before {
		t.Fatal("source conflict changed value")
	}
	r, err := v.Reconcile(t.Context())
	if err != nil || r.BonusMismatches == 0 {
		t.Fatal(r, err)
	}
}

func TestTextBonusPaymentSourceHashIgnoresSessionFormatting(t *testing.T) {
	db, v := textValueDB(t)
	m, ids := rewardSettledPremium(t, db, v, valueTime(time.Now().UTC().Add(-time.Minute)), true)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var before, after string
	q := `SELECT text_bonus_source($1,$2)::text`
	if err = tx.QueryRow(q, m.Contract.MatchID, ids[0]).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`SET LOCAL TimeZone='Pacific/Auckland'; SET LOCAL DateStyle='German, DMY'`); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(q, m.Contract.MatchID, ids[0]).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("source fingerprint depends on session date/time formatting")
	}
}

func TestTextBonusPaymentInvalidStatusHistoryAndSource(t *testing.T) {
	for _, kind := range []string{"interrupted", "pending", "prototype", "reward_disabled", "unknown_history", "development", "partial_source"} {
		t.Run(kind, func(t *testing.T) {
			db, v := textValueDB(t)
			at := valueTime(time.Now().UTC().Add(-time.Minute))
			m, ids := valueUnpreparedMatch(t, v, db, at, kind == "prototype")
			if kind == "reward_disabled" {
				m.Contract.Eligibility.Rewards = false
				m.Contract.Eligibility.Leaderboard = false
			}
			if err := v.Prepare(t.Context(), m, at); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type) VALUES($1,'premium_monthly')`, ids[0]); err != nil {
				t.Fatal(err)
			}
			if err := v.Start(t.Context(), m.Contract.MatchID, m.Owner, 1, at); err != nil {
				t.Fatal(err)
			}
			if kind == "interrupted" {
				if err := v.Interrupt(t.Context(), m.Contract.MatchID, m.Owner, 1, at.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := v.Finish(t.Context(), textPrivacyOutcome(m, ids, at)); err != nil {
					t.Fatal(err)
				}
				if kind != "pending" {
					if err := v.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
						t.Fatal(err)
					}
				}
			}
			switch kind {
			case "unknown_history":
				if _, err := db.Exec(`ALTER TABLE text_bonus_eligibility DISABLE TRIGGER text_bonus_eligibility_immutable; DELETE FROM text_bonus_eligibility WHERE account_id='` + ids[0] + `'; ALTER TABLE text_bonus_eligibility ENABLE TRIGGER text_bonus_eligibility_immutable`); err != nil {
					t.Fatal(err)
				}
			case "development":
				// Privileged historical fixture: normal account-purpose mutation remains forbidden.
				if _, err := db.Exec(`ALTER TABLE accounts DISABLE TRIGGER account_auth_purpose; UPDATE accounts SET auth_purpose='development' WHERE id='` + ids[0] + `'; ALTER TABLE accounts ENABLE TRIGGER account_auth_purpose`); err != nil {
					t.Fatal(err)
				}
			case "partial_source":
				if _, err := db.Exec(`ALTER TABLE text_award_receipts DISABLE TRIGGER text_award_immutable; DELETE FROM text_award_receipts WHERE account_id='` + ids[0] + `'; ALTER TABLE text_award_receipts ENABLE TRIGGER text_award_immutable`); err != nil {
					t.Fatal(err)
				}
			}
			before := bonusSnapshot(t, db, ids[0])
			p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
			want := ErrTextBonusIneligible
			if kind == "partial_source" {
				want = ErrValueConflict
			}
			if !errors.Is(err, want) || p.MatchID != "" || bonusSnapshot(t, db, ids[0]) != before {
				t.Fatal("invalid bonus accepted or mutated value", p, err)
			}
		})
	}
}

func TestTextBonusPaymentEmptyAwardsAndSSVSubjectBinding(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		db, v := textValueDB(t)
		at := valueTime(time.Now().UTC().Add(-time.Minute))
		m, ids := valuePreparedMatch(t, v, db, at, false)
		if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type) VALUES($1,'premium_monthly')`, ids[0]); err != nil {
			t.Fatal(err)
		}
		if err := v.Start(t.Context(), m.Contract.MatchID, m.Owner, 1, at); err != nil {
			t.Fatal(err)
		}
		o := textPrivacyOutcome(m, ids, at)
		o.Players[0].Absent = true
		if err := v.Finish(t.Context(), o); err != nil {
			t.Fatal(err)
		}
		if err := v.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
			t.Fatal(err)
		}
		p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
		if err != nil || p.Requested != 0 || p.Credited != 0 || len(p.Items) != 0 {
			t.Fatal(p, err)
		}
		if _, err = v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0]); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("ssv_subject", func(t *testing.T) {
		db, v := textValueDB(t)
		at := valueTime(time.Now().UTC().Add(-time.Minute))
		m, ids := rewardSettled(t, db, v, at)
		bonusProof(t, db, m.Contract.MatchID, ids[0], at.Add(time.Second), "aaaa")
		bonusProof(t, db, m.Contract.MatchID, ids[1], at.Add(time.Second), "bbbb")
		if _, err := db.Exec(`CREATE FUNCTION test_bonus_wrong_subject() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.provider_transaction_id:='bbbb'; RETURN NEW; END $$; CREATE TRIGGER test_bonus_wrong_subject BEFORE INSERT ON text_bonus_payments FOR EACH ROW EXECUTE FUNCTION test_bonus_wrong_subject()`); err != nil {
			t.Fatal(err)
		}
		before := bonusSnapshot(t, db, ids[0])
		if _, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0]); err == nil {
			t.Fatal("another account proof accepted")
		}
		if bonusSnapshot(t, db, ids[0]) != before {
			t.Fatal("cross-account proof retained value")
		}
	})
}

func applyBonusForTest(t *testing.T, s *TextValueStore, match, account string) (int, int) {
	t.Helper()
	got, err := s.ApplyBonus(t.Context(), match, account)
	if err != nil {
		t.Fatal(err)
	}
	return got.Requested, got.Credited
}

func TestTextBonusPaymentPremiumCreditsGenuineBaseOnce(t *testing.T) {
	db, v := textValueDB(t)
	m, ids := rewardSettledPremium(t, db, v, valueTime(time.Now().UTC().Add(-time.Minute)), true)
	var base, before int
	if err := db.QueryRow(`SELECT sum(credited) FROM text_award_receipts WHERE match_id=$1 AND account_id=$2`, m.Contract.MatchID, ids[0]).Scan(&base); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT balance FROM noin_wallets WHERE account_id=$1`, ids[0]).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if base <= 0 {
		t.Fatal("fixture has no genuinely credited base")
	}
	for range 2 {
		requested, credited := applyBonusForTest(t, v, m.Contract.MatchID, ids[0])
		if requested != base || credited != base {
			t.Fatal("bonus differs from credited base", requested, credited, base)
		}
	}
	var balance int
	if err := db.QueryRow(`SELECT balance FROM noin_wallets WHERE account_id=$1`, ids[0]).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != before+base {
		t.Fatal("bonus was not exactly once", balance, before, base)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_payments`); n != 1 {
		t.Fatal("payment identities", n)
	}
}
