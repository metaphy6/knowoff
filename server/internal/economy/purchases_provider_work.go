package economy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

type billingVerificationAttempt struct {
	account, id string
	slot        int
	generation  int64
	request     [32]byte
	deadline    time.Time
}

func (p *Purchases) claimVerification(ctx context.Context, account string, raw []byte) (billingVerificationAttempt, error) {
	a := billingVerificationAttempt{account: account, id: uuid.NewString(), request: sha256.Sum256(raw)}
	if p.billing.MaxConcurrentRequests < 1 || p.billing.MaxConcurrentRequests > 32 || p.billing.HTTPTimeoutS < 1 || p.billing.HTTPTimeoutS > 30 {
		return a, ErrBillingUnavailable
	}
	err := store.WithValueTransaction(ctx, p.db, func(tx *sql.Tx) error {
		if err := store.LockValueAccount(ctx, tx, account); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, `SELECT s.slot,coalesce(v.generation,0)+1 FROM generate_series(1,$2) s(slot) LEFT JOIN billing_verification_slots v ON v.account_id=$1 AND v.slot=s.slot WHERE v.slot IS NULL OR v.state='stopped' OR v.deadline_at<=clock_timestamp() ORDER BY s.slot LIMIT 1`, account, p.billing.MaxConcurrentRequests).Scan(&a.slot, &a.generation)
		if err == sql.ErrNoRows {
			return ErrBillingBusy
		}
		if err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `INSERT INTO billing_verification_slots(account_id,slot,generation,attempt_id,request_sha256,deadline_at,state) VALUES($1,$2,$3,$4,$5,clock_timestamp()+make_interval(secs=>$6),'in_flight') ON CONFLICT(account_id,slot) DO UPDATE SET generation=EXCLUDED.generation,attempt_id=EXCLUDED.attempt_id,request_sha256=EXCLUDED.request_sha256,deadline_at=EXCLUDED.deadline_at,state='in_flight',updated_at=clock_timestamp() RETURNING deadline_at`, account, a.slot, a.generation, a.id, a.request[:], p.billing.HTTPTimeoutS).Scan(&a.deadline)
	})
	return a, err
}
func (a billingVerificationAttempt) finishTx(ctx context.Context, tx *sql.Tx) error {
	// applyProof/applySubscription have already locked discovered source keys and
	// the account. A stale/expired slot rolls that entire value transaction back.
	var generation int64
	var attempt, state string
	var request []byte
	var deadline time.Time
	if err := tx.QueryRowContext(ctx, `SELECT generation,attempt_id,request_sha256,deadline_at,state FROM billing_verification_slots WHERE account_id=$1 AND slot=$2 FOR UPDATE`, a.account, a.slot).Scan(&generation, &attempt, &request, &deadline, &state); err != nil {
		return err
	}
	if generation != a.generation || attempt != a.id || string(request) != string(a.request[:]) || !deadline.Equal(a.deadline) || state != "in_flight" {
		return ErrBillingConflict
	}
	var live bool
	if err := tx.QueryRowContext(ctx, `SELECT $1::timestamptz>clock_timestamp()`, deadline).Scan(&live); err != nil {
		return err
	}
	if !live {
		return ErrBillingConflict
	}
	_, err := tx.ExecContext(ctx, `UPDATE billing_verification_slots SET state='stopped',updated_at=clock_timestamp() WHERE account_id=$1 AND slot=$2`, a.account, a.slot)
	if err != nil {
		return err
	}
	return billingDeadlineLive(ctx, tx, a.deadline)
}
func (p *Purchases) uncertainVerification(ctx context.Context, a billingVerificationAttempt) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	defer cancel()
	// Failure here leaves the exact in-flight deadline for restart reconciliation.
	_ = store.WithValueTransaction(ctx, p.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, a.account); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE billing_verification_slots SET state='uncertain',updated_at=clock_timestamp() WHERE account_id=$1 AND slot=$2 AND generation=$3 AND attempt_id=$4 AND request_sha256=$5 AND state='in_flight'`, a.account, a.slot, a.generation, a.id, a.request[:])
		return err
	})
}

type billingProviderSource struct {
	purchase, account, kind, key string
	platform                     PurchasePlatform
	request, proof               []byte
	taskState                    string
	observationState             string
	keys                         []string
}
type billingProviderAttempt struct {
	source         billingProviderSource
	operation, id  string
	generation     int64
	request, proof [32]byte
	deadline       time.Time
}

var errNoBillingWork = errors.New("billing.no_provider_work")

func billingProviderSourceRead(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, purchase, operation string) (billingProviderSource, error) {
	s := billingProviderSource{purchase: purchase}
	var verified bool
	err := q.QueryRowContext(ctx, `SELECT sp.account_id,sp.platform,CASE WHEN ss.initial_purchase_id IS NOT NULL THEN 'subscription' ELSE 'purchase' END,coalesce(ss.source_key,b.original_key,''),
 CASE WHEN $2='ack' THEN coalesce(st.request,pt.request,'{}'::jsonb) ELSE coalesce(CASE WHEN ss.platform='google_play' AND o.product_id IS NOT NULL THEN jsonb_set(sp.raw_receipt,'{product_id}',to_jsonb(o.product_id)) ELSE sp.raw_receipt END,'{}'::jsonb) END,
 coalesce(st.proof,pt.proof,'{}'::jsonb),coalesce(st.state,pt.state,''),coalesce(o.state,''),
 (ss.initial_purchase_id IS NOT NULL OR b.product_kind='noin') AND NOT EXISTS(SELECT 1 FROM billing_subscription_replacements r WHERE r.platform=ss.platform AND r.predecessor_key=ss.source_key)
 FROM store_purchases sp LEFT JOIN billing_transactions b ON b.purchase_id=sp.id LEFT JOIN billing_subscription_sources ss ON ss.initial_purchase_id=sp.id
 LEFT JOIN billing_provider_tasks pt ON pt.purchase_id=sp.id LEFT JOIN billing_subscription_tasks st ON (st.platform,st.source_key)=(ss.platform,ss.source_key)
 LEFT JOIN billing_subscription_current c ON(c.platform,c.source_key)=(ss.platform,ss.source_key) LEFT JOIN billing_subscription_observations o ON(o.platform,o.source_key,o.id)=(c.platform,c.source_key,c.observation_id)
 WHERE sp.id=$1`, purchase, operation).Scan(&s.account, &s.platform, &s.kind, &s.key, &s.request, &s.proof, &s.taskState, &s.observationState, &verified)
	if err != nil {
		return s, err
	}
	if !verified || s.key == "" {
		return s, errNoBillingWork
	}
	s.keys = []string{s.key}
	if s.kind == "subscription" {
		rows, e := q.QueryContext(ctx, `SELECT predecessor_key,successor_key FROM billing_subscription_replacements WHERE platform=$1 AND (predecessor_key=$2 OR successor_key=$2)`, s.platform, s.key)
		if e != nil {
			return s, e
		}
		defer rows.Close()
		for rows.Next() {
			var a, b string
			if e = rows.Scan(&a, &b); e != nil {
				return s, e
			}
			s.keys = append(s.keys, a, b)
		}
		if e = rows.Err(); e != nil {
			return s, e
		}
	}
	slices.Sort(s.keys)
	s.keys = slices.Compact(s.keys)
	return s, nil
}
func billingProviderSourceLock(ctx context.Context, tx *sql.Tx, s billingProviderSource, operation string, active bool) (billingProviderSource, error) {
	for _, key := range s.keys {
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT account_id FROM billing_account_sources WHERE platform=$1 AND original_key=$2 FOR UPDATE`, s.platform, key).Scan(&owner); err != nil {
			return s, err
		}
		if owner != s.account {
			return s, ErrBillingConflict
		}
	}
	fresh, err := billingProviderSourceRead(ctx, tx, s.purchase, operation)
	if err != nil {
		return s, err
	}
	if fresh.account != s.account || fresh.key != s.key || fresh.kind != s.kind || !slices.Equal(fresh.keys, s.keys) {
		return s, ErrBillingConflict
	}
	if active {
		if err = store.LockValueAccount(ctx, tx, s.account); err != nil {
			return s, err
		}
	} else {
		if _, err = tx.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, s.account); err != nil {
			return s, err
		}
	}
	var owner string
	if err = tx.QueryRowContext(ctx, `SELECT account_id FROM store_purchases WHERE id=$1 FOR UPDATE`, s.purchase).Scan(&owner); err != nil {
		return s, err
	}
	if owner != s.account {
		return s, ErrBillingConflict
	}
	return billingProviderSourceRead(ctx, tx, s.purchase, operation)
}
func (p *Purchases) claimProviderWork(ctx context.Context, purchase, operation string) (billingProviderAttempt, error) {
	a := billingProviderAttempt{operation: operation, id: uuid.NewString()}
	if operation != "ack" && operation != "observe" {
		return a, ErrBillingProof
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	defer cancel()
	source, err := billingProviderSourceRead(ctx, p.db, purchase, operation)
	if err != nil {
		return a, err
	}
	err = store.WithValueTransaction(ctx, p.db, func(tx *sql.Tx) error {
		fresh, e := billingProviderSourceLock(ctx, tx, source, operation, true)
		if e != nil {
			return e
		}
		if operation == "ack" && fresh.taskState != "pending" {
			return errNoBillingWork
		}
		if operation == "ack" && fresh.kind == "subscription" && fresh.observationState != "purchased" && fresh.observationState != "grace" {
			return errNoBillingWork
		}
		a.source = fresh
		a.request = sha256.Sum256(fresh.request)
		a.proof = sha256.Sum256(fresh.proof)
		if _, e = tx.ExecContext(ctx, `INSERT INTO billing_provider_work(purchase_id,operation,work_kind) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, purchase, operation, fresh.kind); e != nil {
			return e
		}
		var state string
		var deadline sql.NullTime
		var binding sql.NullString
		if e = tx.QueryRowContext(ctx, `SELECT generation,state,deadline_at,privacy_request_id FROM billing_provider_work WHERE purchase_id=$1 AND operation=$2 FOR UPDATE`, purchase, operation).Scan(&a.generation, &state, &deadline, &binding); e != nil {
			return e
		}
		if binding.Valid {
			return ErrBillingConflict
		}
		if state == "in_flight" && deadline.Valid {
			var live bool
			if e = tx.QueryRowContext(ctx, `SELECT $1::timestamptz>clock_timestamp()`, deadline.Time).Scan(&live); e != nil {
				return e
			}
			if live {
				return ErrBillingBusy
			}
		}
		a.generation++
		return tx.QueryRowContext(ctx, `UPDATE billing_provider_work SET generation=$3,attempt_id=$4,state='in_flight',deadline_at=clock_timestamp()+make_interval(secs=>$5),request_sha256=$6,proof_sha256=$7,last_outcome='',updated_at=clock_timestamp() WHERE purchase_id=$1 AND operation=$2 RETURNING deadline_at`, purchase, operation, a.generation, a.id, p.billing.HTTPTimeoutS, a.request[:], a.proof[:]).Scan(&a.deadline)
	})
	return a, err
}
func (a billingProviderAttempt) finishTx(ctx context.Context, tx *sql.Tx, outcome string) error {
	state := "stopped"
	if outcome == "unavailable" {
		state = "uncertain"
	}
	result, err := tx.ExecContext(ctx, `UPDATE billing_provider_work SET state=$8,last_outcome=$9,updated_at=clock_timestamp() WHERE purchase_id=$1 AND operation=$2 AND generation=$3 AND attempt_id=$4 AND request_sha256=$5 AND proof_sha256=$6 AND deadline_at=$7 AND state='in_flight'`, a.source.purchase, a.operation, a.generation, a.id, a.request[:], a.proof[:], a.deadline, state, outcome)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrBillingConflict
	}
	if a.operation == "observe" {
		if a.source.kind == "subscription" {
			result, err = tx.ExecContext(ctx, `UPDATE billing_subscription_current SET checked_at=clock_timestamp() WHERE platform=$1 AND source_key=$2`, a.source.platform, a.source.key)
		} else {
			result, err = tx.ExecContext(ctx, `UPDATE billing_transactions SET checked_at=clock_timestamp() WHERE purchase_id=$1`, a.source.purchase)
		}
		if err != nil {
			return err
		}
		n, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrBillingConflict
		}
	}
	if outcome != "unavailable" {
		return billingDeadlineLive(ctx, tx, a.deadline)
	}
	return nil
}
func (p *Purchases) finishProviderWork(ctx context.Context, a billingProviderAttempt, outcome string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	defer cancel()
	return store.WithValueTransaction(ctx, p.db, func(tx *sql.Tx) error {
		current, err := billingProviderSourceLock(ctx, tx, a.source, a.operation, false)
		if err != nil {
			return err
		}
		if sha256.Sum256(current.request) != a.request || sha256.Sum256(current.proof) != a.proof {
			return ErrBillingConflict
		}
		if err = a.finishTx(ctx, tx, outcome); err != nil {
			return err
		}
		if a.operation != "ack" {
			return nil
		}
		state := "pending"
		if outcome == "observed_complete" || outcome == "post_succeeded" || outcome == "no_server_operation" {
			state = "done"
		}
		var result sql.Result
		if current.kind == "subscription" {
			result, err = tx.ExecContext(ctx, `UPDATE billing_subscription_tasks SET state=$3,attempts=attempts+1,updated_at=clock_timestamp() WHERE platform=$1 AND source_key=$2 AND state='pending'`, current.platform, current.key, state)
		} else {
			result, err = tx.ExecContext(ctx, `UPDATE billing_provider_tasks SET state=$2,attempts=attempts+1,updated_at=clock_timestamp() WHERE purchase_id=$1 AND state='pending'`, current.purchase, state)
		}
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrBillingConflict
		}
		if outcome != "unavailable" {
			return billingDeadlineLive(ctx, tx, a.deadline)
		}
		return nil
	})
}
func (p *Purchases) acknowledgeWork(ctx context.Context, purchase string) error {
	a, err := p.claimProviderWork(ctx, purchase, "ack")
	if errors.Is(err, errNoBillingWork) {
		return nil
	}
	if err != nil {
		return err
	}
	var req ReceiptRequest
	var proof VerifiedPurchase
	if billingJSON(a.source.request, &req) != nil || billingJSON(a.source.proof, &proof) != nil {
		return ErrBillingProof
	}
	providerCtx, cancel := context.WithDeadline(ctx, a.deadline)
	defer cancel()
	outcome := "unavailable"
	if providerCtx.Err() == nil && a.deadline.After(time.Now()) {
		if v, ok := p.verifiers[req.Platform].(BillingAcknowledgementVerifier); ok {
			outcome, err = v.AcknowledgeOutcome(providerCtx, req, proof)
		} else {
			err = p.verifiers[req.Platform].Acknowledge(providerCtx, req, proof)
			if err == nil {
				outcome = "post_succeeded"
			}
		}
	} else {
		err = ErrBillingUnavailable
	}
	if err != nil {
		outcome = "unavailable"
	}
	if providerCtx.Err() != nil || !a.deadline.After(time.Now()) {
		outcome = "unavailable"
		err = ErrBillingUnavailable
	}
	if e := p.finishProviderWork(ctx, a, outcome); e != nil {
		return e
	}
	return err
}

// Concrete production adapters distinguish observed completion, successful POST
// and Apple local no-op. Legacy injected fixtures retain their explicit success.
type BillingAcknowledgementVerifier interface {
	AcknowledgeOutcome(context.Context, ReceiptRequest, VerifiedPurchase) (string, error)
}

func billingDeadlineLive(ctx context.Context, tx *sql.Tx, deadline time.Time) error {
	var live bool
	if err := tx.QueryRowContext(ctx, `SELECT $1::timestamptz>clock_timestamp()`, deadline).Scan(&live); err != nil {
		return err
	}
	if !live {
		return ErrBillingConflict
	}
	return nil
}
