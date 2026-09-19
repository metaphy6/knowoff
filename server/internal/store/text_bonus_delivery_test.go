package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"testing"
	"time"
)

func TestTextBonusDeliveryPayloadCanonicalVectorAndNoPrivateSources(t *testing.T) {
	p := TextBonusPayment{MatchID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", AccountID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Source: "ssv", ProviderTransactionID: "private-provider", SourceHash: "private-source", Requested: 7, Credited: 4,
		Items: []TextBonusPaymentItem{{Kind: "private-role-award", ServerDay: time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC), BaseCredited: 5, Credited: 2}, {ServerDay: time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC), BaseCredited: 2, Credited: 2}}}
	payload, hash, err := p.DeliveryPayload()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"match_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","source":"rewarded_ad","requested":7,"credited":4,"days":[{"server_day":"2026-09-18","requested":2,"credited":2},{"server_day":"2026-09-19","requested":5,"credited":2}]}`
	if string(raw) != want {
		t.Fatal("payload contract", string(raw))
	}
	if hash != "21af88e07b6eefc4544e0cca2e07be47785c4bd9bc2563825c35b3113cd558fa" {
		t.Fatal("canonical vector hash", hash)
	}
}

func bonusPaid(t *testing.T) (*sql.DB, *TextValueStore, TextMatchRecord, []string, TextBonusPayment) {
	t.Helper()
	db, v := textValueDB(t)
	m, ids := rewardSettledPremium(t, db, v, valueTime(time.Now().UTC().Add(-time.Minute)), true)
	p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	return db, v, m, ids, p
}

func TestTextBonusDeliveryDurableLeaseAckAndLostResponse(t *testing.T) {
	db, v, _, ids, p := bonusPaid(t)
	ctx := t.Context()
	before := bonusSnapshot(t, db, ids[0])
	page, err := v.ClaimBonusDeliveries(ctx, ids[0], 20, []string{}, rewardAuthorize)
	if err != nil || len(page.Deliveries) != 1 {
		t.Fatal("payment has no independent delivery", len(page.Deliveries), err)
	}
	d := page.Deliveries[0]
	payload, hash, err := p.DeliveryPayload()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(d.Payload)
	want, _ := json.Marshal(payload)
	if d.PayloadSHA256 != hash || string(got) != string(want) || !valueUUID(d.DeliveryID) {
		t.Fatal("immutable payload mismatch")
	}
	var stored []byte
	if err = db.QueryRow(`SELECT lease_sha256 FROM text_bonus_outbox WHERE id=$1`, d.DeliveryID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 32 || string(stored) == d.Lease {
		t.Fatal("raw lease retained")
	}
	if again, err := v.ClaimBonusDeliveries(ctx, ids[0], 20, []string{}, rewardAuthorize); err != nil || len(again.Deliveries) != 0 {
		t.Fatal("active lease replaced", err)
	}
	if err = v.AcknowledgeBonusDelivery(ctx, ids[1], d.DeliveryID, d.Lease, rewardAuthorize); !errors.Is(err, ErrBonusDeliveryStale) {
		t.Fatal("cross account ack", err)
	}
	if _, err = db.Exec(`UPDATE text_bonus_outbox SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, d.DeliveryID); err != nil {
		t.Fatal(err)
	}
	restarted := NewTextValueStore(db, v.tuning)
	again, err := restarted.ClaimBonusDeliveries(ctx, ids[0], 20, []string{}, rewardAuthorize)
	if err != nil || len(again.Deliveries) != 1 {
		t.Fatal(err)
	}
	fresh := again.Deliveries[0]
	if fresh.DeliveryID != d.DeliveryID || fresh.PayloadSHA256 != d.PayloadSHA256 || fresh.Lease == d.Lease {
		t.Fatal("replacement changed stable identity")
	}
	if err = v.AcknowledgeBonusDelivery(ctx, ids[0], d.DeliveryID, d.Lease, rewardAuthorize); !errors.Is(err, ErrBonusDeliveryStale) {
		t.Fatal("old lease acknowledged replacement", err)
	}
	if err = restarted.AcknowledgeBonusDelivery(ctx, ids[0], fresh.DeliveryID, fresh.Lease, rewardAuthorize); err != nil {
		t.Fatal(err)
	}
	if err = v.AcknowledgeBonusDelivery(ctx, ids[0], fresh.DeliveryID, fresh.Lease, rewardAuthorize); err != nil {
		t.Fatal("lost ack exact retry", err)
	}
	// Device A lost its response and only knows the older lease. Authenticated
	// status sees Device B's committed replacement ACK without another mutation.
	status, err := v.ClaimBonusDeliveries(ctx, ids[0], 20, []string{d.DeliveryID, uuid.NewString()}, rewardAuthorize)
	if err != nil || len(status.Deliveries) != 0 || len(status.AcknowledgedIDs) != 1 || status.AcknowledgedIDs[0] != d.DeliveryID {
		t.Fatal("lost ack status", status.AcknowledgedIDs, err)
	}
	foreign, err := v.ClaimBonusDeliveries(ctx, ids[1], 20, []string{d.DeliveryID}, rewardAuthorize)
	if err != nil || len(foreign.AcknowledgedIDs) != 0 {
		t.Fatal("foreign ack disclosure", err)
	}
	if bonusSnapshot(t, db, ids[0]) != before {
		t.Fatal("delivery changed financial value")
	}
}

func TestTextBonusDeliveryFinalAuthorityRollsBackClaimAndAck(t *testing.T) {
	db, v, _, ids, _ := bonusPaid(t)
	calls := 0
	guard := func(context.Context, *sql.Tx) error {
		calls++
		if calls == 2 {
			return ErrBonusDeliveryUnauthorized
		}
		return nil
	}
	if p, err := v.ClaimBonusDeliveries(t.Context(), ids[0], 20, []string{}, guard); !errors.Is(err, ErrBonusDeliveryUnauthorized) || len(p.Deliveries) != 0 {
		t.Fatal("claim false success", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_outbox WHERE attempts<>0 OR lease_sha256 IS NOT NULL`); n != 0 {
		t.Fatal("failed claim retained leases")
	}
	p, err := v.ClaimBonusDeliveries(t.Context(), ids[0], 20, []string{}, rewardAuthorize)
	if err != nil {
		t.Fatal(err)
	}
	d := p.Deliveries[0]
	calls = 0
	if err = v.AcknowledgeBonusDelivery(t.Context(), ids[0], d.DeliveryID, d.Lease, guard); !errors.Is(err, ErrBonusDeliveryUnauthorized) {
		t.Fatal(err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_outbox WHERE acknowledged_at IS NOT NULL`); n != 0 {
		t.Fatal("failed ack retained state")
	}
	if _, err = v.ClaimBonusDeliveries(t.Context(), ids[0], 20, []string{}, nil); !errors.Is(err, ErrBonusDeliveryUnauthorized) {
		t.Fatal("missing authority", err)
	}
}

func bonusOutboxMigration(t *testing.T, db *sql.DB, direction string) error {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000040_text_bonus_delivery."+direction+".sql"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(string(raw)); err != nil {
		return err
	}
	return tx.Commit()
}

func TestTextBonusDeliveryBackfillPreservesSuppressedAndSurvivorProof(t *testing.T) {
	db, v, m, ids, p := bonusPaid(t)
	bonusProof(t, db, m.Contract.MatchID, ids[1], valueTime(time.Now().UTC().Add(-time.Second)), "abcd")
	if _, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[1]); err != nil {
		t.Fatal(err)
	}
	textPrivacyFence(t, db, ids[0])
	beforeErased, beforeSurvivor := bonusSnapshot(t, db, ids[0]), bonusSnapshot(t, db, ids[1])
	if err := bonusOutboxMigration(t, db, "down"); err == nil {
		t.Fatal("retained delivery rollback accepted")
	}
	// Privileged historical fixture: head39 retained payment receipts predate
	// independent delivery. Remove only fixture outbox rows, then run the real
	// empty down/up scripts against the unchanged financial/source evidence.
	if _, err := db.Exec(`ALTER TABLE text_bonus_outbox DISABLE TRIGGER text_bonus_outbox_delete; DELETE FROM text_bonus_outbox; ALTER TABLE text_bonus_outbox ENABLE TRIGGER text_bonus_outbox_delete`); err != nil {
		t.Fatal(err)
	}
	if err := bonusOutboxMigration(t, db, "down"); err != nil {
		t.Fatal(err)
	}
	if err := bonusOutboxMigration(t, db, "up"); err != nil {
		t.Fatal(err)
	}
	if bonusSnapshot(t, db, ids[0]) != beforeErased || bonusSnapshot(t, db, ids[1]) != beforeSurvivor {
		t.Fatal("backfill changed original financial bytes")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_outbox WHERE account_id=$1`, ids[0]); n != 0 {
		t.Fatal("suppressed historical delivery reconstructed")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_outbox WHERE account_id=$1`, ids[1]); n != 1 {
		t.Fatal("survivor delivery missing")
	}
	replay, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
	if err != nil || replay.SourceHash != p.SourceHash {
		t.Fatal("historical payment replay", err)
	}
	r, err := v.Reconcile(t.Context())
	if err != nil || r.BonusSuppressed != 1 || r.BonusPending != 1 || r.BonusDeliveryMismatches != 0 {
		t.Fatal(r, err)
	}
}

func TestTextBonusDeliveryBackfillRejectsCorruptPaymentAtomically(t *testing.T) {
	db, _, _, ids, _ := bonusPaid(t)
	if _, err := db.Exec(`ALTER TABLE text_bonus_outbox DISABLE TRIGGER text_bonus_outbox_delete; DELETE FROM text_bonus_outbox; ALTER TABLE text_bonus_outbox ENABLE TRIGGER text_bonus_outbox_delete`); err != nil {
		t.Fatal(err)
	}
	if err := bonusOutboxMigration(t, db, "down"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE text_award_receipts DISABLE TRIGGER text_award_immutable; UPDATE text_award_receipts SET body_hash=repeat('a',64) WHERE account_id='` + ids[0] + `'; ALTER TABLE text_award_receipts ENABLE TRIGGER text_award_immutable`); err != nil {
		t.Fatal(err)
	}
	before := bonusSnapshot(t, db, ids[0])
	if err := bonusOutboxMigration(t, db, "up"); err == nil {
		t.Fatal("corrupt payment backfilled")
	}
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('public.text_bonus_outbox') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("partial migration committed", exists, err)
	}
	if bonusSnapshot(t, db, ids[0]) != before {
		t.Fatal("failed backfill mutated retained value")
	}
}

func TestTextBonusDeliveryBackfillWithEarlierErasedParticipant(t *testing.T) {
	db, v := textValueDB(t)
	at := valueTime(time.Now().UTC().Add(-time.Minute))
	m, ids := valuePreparedMatch(t, v, db, at, false)
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',$2)`, ids[0], at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := v.Start(t.Context(), m.Contract.MatchID, m.Owner, 1, at); err != nil {
		t.Fatal(err)
	}
	textPrivacyFence(t, db, ids[1])
	if err := v.Finish(t.Context(), textPrivacyOutcome(m, ids, at)); err != nil {
		t.Fatal(err)
	}
	if err := v.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_value_erasure_dispositions WHERE account_id=$1`, ids[1]); n != 1 {
		t.Fatal("missing earlier erasure disposition", n)
	}
	before := bonusSnapshot(t, db, ids[0])
	if _, err = db.Exec(`ALTER TABLE text_bonus_outbox DISABLE TRIGGER text_bonus_outbox_delete; DELETE FROM text_bonus_outbox; ALTER TABLE text_bonus_outbox ENABLE TRIGGER text_bonus_outbox_delete`); err != nil {
		t.Fatal(err)
	}
	if err = bonusOutboxMigration(t, db, "down"); err != nil {
		t.Fatal(err)
	}
	if err = bonusOutboxMigration(t, db, "up"); err != nil {
		t.Fatal(err)
	}
	if bonusSnapshot(t, db, ids[0]) != before {
		t.Fatal("survivor financial evidence changed")
	}
	page, err := v.ClaimBonusDeliveries(t.Context(), ids[0], 20, []string{}, rewardAuthorize)
	_, hash, payloadErr := p.DeliveryPayload()
	if err != nil || payloadErr != nil || len(page.Deliveries) != 1 || page.Deliveries[0].PayloadSHA256 != hash {
		t.Fatal("survivor receipt did not survive earlier participant erasure", err, payloadErr)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_outbox WHERE account_id=$1`, ids[1]); n != 0 {
		t.Fatal("erased participant delivery reconstructed")
	}
}

func TestTextBonusDeliveryConcurrentClaimsDoNotOverlap(t *testing.T) {
	db, v, _, ids, _ := bonusPaid(t)
	before := bonusSnapshot(t, db, ids[0])
	type result struct {
		page TextBonusDeliveryPage
		err  error
	}
	results := make(chan result, 8)
	start := make(chan struct{})
	for range 8 {
		go func() {
			<-start
			other := NewTextValueStore(db, v.tuning)
			page, err := other.ClaimBonusDeliveries(t.Context(), ids[0], 1, []string{}, rewardAuthorize)
			results <- result{page, err}
		}()
	}
	close(start)
	claimed := 0
	for range 8 {
		r := <-results
		if r.err != nil || len(r.page.Deliveries) > 1 {
			t.Fatal("bounded concurrent claim", r.err)
		}
		claimed += len(r.page.Deliveries)
	}
	if claimed != 1 || valueCount(t, db, `SELECT sum(attempts) FROM text_bonus_outbox`) != 1 {
		t.Fatal("overlapping claim leases", claimed)
	}
	if bonusSnapshot(t, db, ids[0]) != before {
		t.Fatal("claim concurrency changed value")
	}
}

func TestTextBonusDeliveryDeferredPayloadBindsFinalItems(t *testing.T) {
	db, _, _, ids, _ := bonusPaid(t)
	before := bonusSnapshot(t, db, ids[0])
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// Preserve genuine payment, item and ledger evidence. Privileged fixture
	// removal only lets this transaction replay the same valid financial rows
	// using an adversarial insertion order; all guards are restored first.
	_, err = tx.Exec(`CREATE TEMP TABLE saved_bonus_payment ON COMMIT DROP AS TABLE text_bonus_payments;
CREATE TEMP TABLE saved_bonus_items ON COMMIT DROP AS TABLE text_bonus_payment_items;
ALTER TABLE text_bonus_outbox DISABLE TRIGGER text_bonus_outbox_delete;
ALTER TABLE text_bonus_payment_items DISABLE TRIGGER text_bonus_item_immutable;
ALTER TABLE text_bonus_payments DISABLE TRIGGER text_bonus_payment_immutable;
DELETE FROM text_bonus_outbox; DELETE FROM text_bonus_payment_items; DELETE FROM text_bonus_payments;
ALTER TABLE text_bonus_outbox ENABLE TRIGGER text_bonus_outbox_delete;
ALTER TABLE text_bonus_payment_items ENABLE TRIGGER text_bonus_item_immutable;
ALTER TABLE text_bonus_payments ENABLE TRIGGER text_bonus_payment_immutable;
INSERT INTO text_bonus_payments SELECT * FROM saved_bonus_payment;
INSERT INTO text_bonus_outbox(match_id,account_id,payload,payload_sha256)
SELECT match_id,account_id,jsonb_build_object('version',1,'match_id',match_id::text,'source','premium','requested',requested,'credited',credited,'days','[]'::jsonb),
encode(sha256(convert_to(replace(jsonb_build_array(1,match_id::text,'premium',requested,credited,'[]'::jsonb)::text,' ',''),'UTF8')),'hex') FROM saved_bonus_payment;
INSERT INTO text_bonus_payment_items SELECT * FROM saved_bonus_items;`)
	if err != nil {
		t.Fatal("fixture did not reach deferred checks", err)
	}
	err = tx.Commit()
	if err == nil || !strings.Contains(err.Error(), "bonus delivery payload mismatch") {
		t.Fatal("incomplete early payload committed against complete genuine final items", err)
	}
	if bonusSnapshot(t, db, ids[0]) != before {
		t.Fatal("rejected transaction changed financial bytes")
	}
}

func TestTextBonusDeliveryImmutableIdentityAndAtomicPayment(t *testing.T) {
	db, v, m, ids, _ := bonusPaid(t)
	for _, q := range []string{`UPDATE text_bonus_outbox SET payload='{}'`, `UPDATE text_bonus_outbox SET payload_sha256=repeat('a',64)`, `DELETE FROM text_bonus_outbox`, `TRUNCATE text_bonus_outbox CASCADE`, `UPDATE text_bonus_outbox SET lease_sha256=decode(repeat('a',64),'hex'),lease_until=clock_timestamp()+interval '1 day',attempts=1`} {
		if _, err := db.Exec(q); err == nil {
			t.Fatal("delivery identity/lease changed", q)
		}
	}
	bonusProof(t, db, m.Contract.MatchID, ids[1], valueTime(time.Now().UTC().Add(-time.Second)), "abab")
	before := bonusSnapshot(t, db, ids[1])
	if _, err := db.Exec(`CREATE FUNCTION test_bonus_outbox_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected outbox failure'; END $$; CREATE TRIGGER test_bonus_outbox_fault BEFORE INSERT ON text_bonus_outbox FOR EACH ROW EXECUTE FUNCTION test_bonus_outbox_fault()`); err != nil {
		t.Fatal(err)
	}
	if p, err := v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[1]); err == nil || p.MatchID != "" {
		t.Fatal("failed delivery claimed payment success", err)
	}
	if bonusSnapshot(t, db, ids[1]) != before {
		t.Fatal("payment committed without delivery")
	}
}

func TestTextBonusDeliveryDeletionBothLockOrders(t *testing.T) {
	for _, operation := range []string{"claim", "ack"} {
		for _, workFirst := range []bool{true, false} {
			t.Run(fmt.Sprint(operation, workFirst), func(t *testing.T) {
				db, v, _, ids, _ := bonusPaid(t)
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				account := ids[0]
				work := func() error {
					_, err := v.ClaimBonusDeliveries(ctx, account, 20, []string{}, rewardAuthorize)
					return err
				}
				if operation == "ack" {
					p, err := v.ClaimBonusDeliveries(ctx, account, 20, []string{}, rewardAuthorize)
					if err != nil {
						t.Fatal(err)
					}
					d := p.Deliveries[0]
					work = func() error { return v.AcknowledgeBonusDelivery(ctx, account, d.DeliveryID, d.Lease, rewardAuthorize) }
				}
				fence := func(tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx, `SELECT privacy_prepare_verified_request($1,$2,$3,$4)`, uuid.NewString(), account, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
					return err
				}
				done := make(chan error, 1)
				if workFirst {
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
					go func() { done <- work() }()
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
					go func() { done <- work() }()
					waitAdmissionLock(t, ctx, db, "SELECT deleted_at FROM accounts")
					if err = tx.Commit(); err != nil {
						t.Fatal(err)
					}
					if err = <-done; !errors.Is(err, ErrBonusDeliveryUnauthorized) {
						t.Fatal(err)
					}
				}
				wantAck := int64(0)
				if workFirst && operation == "ack" {
					wantAck = 1
				}
				if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_outbox WHERE acknowledged_at IS NOT NULL`); n != wantAck {
					t.Fatal("suppression became acknowledgment", n, wantAck)
				}
				if _, err := v.ClaimBonusDeliveries(ctx, account, 20, []string{}, rewardAuthorize); !errors.Is(err, ErrBonusDeliveryUnauthorized) {
					t.Fatal("fenced claim accepted", err)
				}
			})
		}
	}
}

func TestTextBonusDeliveryLeaseExpiresDuringOutboxWait(t *testing.T) {
	db, v, _, ids, _ := bonusPaid(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	p, err := v.ClaimBonusDeliveries(ctx, ids[0], 20, []string{}, rewardAuthorize)
	if err != nil {
		t.Fatal(err)
	}
	d := p.Deliveries[0]
	if _, err = db.Exec(`UPDATE text_bonus_outbox SET lease_until=clock_timestamp()+interval '250 milliseconds' WHERE id=$1`, d.DeliveryID); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT id FROM text_bonus_outbox WHERE id=$1 FOR UPDATE`, d.DeliveryID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- v.AcknowledgeBonusDelivery(ctx, ids[0], d.DeliveryID, d.Lease, rewardAuthorize) }()
	waitAdmissionLock(t, ctx, db, "SELECT lease_sha256,lease_until,acknowledged_at FROM text_bonus_outbox")
	if _, err = tx.ExecContext(ctx, `SELECT pg_sleep(0.3)`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, ErrBonusDeliveryStale) {
		t.Fatal("expired lease accepted after wait", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_bonus_outbox WHERE acknowledged_at IS NOT NULL`); n != 0 {
		t.Fatal("expired lease acknowledged")
	}
}
