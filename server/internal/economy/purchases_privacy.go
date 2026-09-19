package economy

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// DrainPrivacyBilling is a closed executor adapter. Production does not register
// it. It never invokes receipt application, entitlement sync, refunds or grants.
// Complete means known provider work stopped. It does not attest ownership,
// verification or processor erasure for legacy/unknown retained receipt rows.
func (p *Purchases) DrainPrivacyBilling(ctx context.Context, executor *sql.DB, request string, budget int) (bool, error) {
	id, err := uuid.Parse(request)
	if err != nil || id == uuid.Nil || id.String() != request || executor == nil || budget < 1 || budget > 100 || p.billing.HTTPTimeoutS < 1 || p.billing.HTTPTimeoutS > 30 {
		return false, ErrBillingProof
	}
	for i := 0; i < budget; i++ {
		var raw []byte
		callCtx, cancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
		err = executor.QueryRowContext(callCtx, `SELECT privacy_begin_billing_drain($1)`, request).Scan(&raw)
		cancel()
		if err != nil {
			return false, err
		}
		var unit struct {
			Purchase string           `json:"purchase_id"`
			Ready    bool             `json:"attempt_ready"`
			Platform PurchasePlatform `json:"platform"`
		}
		if err = billingJSON(raw, &unit); err != nil {
			return false, err
		}
		if !unit.Ready || !p.BillingAvailable(unit.Platform) {
			continue
		}
		verifier, ok := p.verifiers[unit.Platform].(BillingAcknowledgementVerifier)
		if !ok {
			continue
		}
		select {
		case p.verificationSlots <- struct{}{}:
		default:
			return false, ErrBillingBusy
		}
		err = p.drainPrivacyAttempt(ctx, executor, request, unit.Purchase, verifier)
		<-p.verificationSlots
		if err != nil {
			return false, err
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	defer cancel()
	var raw []byte
	if err = executor.QueryRowContext(callCtx, `SELECT privacy_finish_billing_drain($1)`, request).Scan(&raw); err != nil {
		return false, err
	}
	var result struct{ Complete bool }
	if err = billingJSON(raw, &result); err != nil {
		return false, err
	}
	return result.Complete, nil
}
func (p *Purchases) drainPrivacyAttempt(ctx context.Context, executor *sql.DB, request, purchase string, verifier BillingAcknowledgementVerifier) error {
	var raw []byte
	attempt := uuid.NewString()
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	err := executor.QueryRowContext(callCtx, `SELECT privacy_claim_billing_attempt($1,$2,'ack',$3,$4)`, request, purchase, attempt, p.billing.HTTPTimeoutS).Scan(&raw)
	cancel()
	if err != nil {
		return err
	}
	var claim struct {
		Generation int64
		Deadline   time.Time `json:"deadline_at"`
		RequestSHA string    `json:"request_sha256"`
		ProofSHA   string    `json:"proof_sha256"`
		Request    json.RawMessage
		Proof      json.RawMessage
	}
	if err = billingJSON(raw, &claim); err != nil {
		return err
	}
	requestSHA, e := hex.DecodeString(claim.RequestSHA)
	if e != nil || len(requestSHA) != 32 {
		return ErrBillingProof
	}
	proofSHA, e := hex.DecodeString(claim.ProofSHA)
	if e != nil || len(proofSHA) != 32 {
		return ErrBillingProof
	}
	var receipt ReceiptRequest
	var proof VerifiedPurchase
	if billingJSON(claim.Request, &receipt) != nil || billingJSON(claim.Proof, &proof) != nil {
		return ErrBillingProof
	}
	providerCtx, providerCancel := context.WithDeadline(ctx, claim.Deadline)
	outcome := "unavailable"
	if providerCtx.Err() == nil && claim.Deadline.After(time.Now()) {
		outcome, err = verifier.AcknowledgeOutcome(providerCtx, receipt, proof)
		if err != nil {
			outcome = "unavailable"
		}
	}
	if providerCtx.Err() != nil || !claim.Deadline.After(time.Now()) {
		outcome = "unavailable"
	}
	providerCancel()
	// A failed finish leaves the exact in-flight lease for restart, never done.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	defer finishCancel()
	return executor.QueryRowContext(finishCtx, `SELECT privacy_finish_billing_attempt($1,$2,'ack',$3,$4,$5,$6,$7)`, request, purchase, claim.Generation, attempt, requestSHA, proofSHA, outcome).Scan(&raw)
}
