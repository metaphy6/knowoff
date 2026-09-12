package economy

import (
	"context"
	"database/sql"
	"github.com/knowoff/knowoff/server/internal/config"
	"time"
)

// NewPlatformPurchases constructs real fixed-origin verifiers without making a
// network request. Invalid/absent credentials never select a simulated provider.
func NewPlatformPurchases(db *sql.DB, cfg *config.Config) (*Purchases, error) {
	if cfg == nil {
		return nil, ErrBillingUnavailable
	}
	c := cfg.Billing
	if (cfg.App.Env == "prod" || cfg.App.Env == "production") && (c.Google.AllowTestPurchases || c.Apple.Environment == "Sandbox") {
		return nil, ErrBillingUnavailable
	}
	if e := c.Validate(cfg.Tuning.Economy); e != nil {
		return nil, e
	}
	providers := map[PurchasePlatform]ReceiptVerifier{}
	if c.Google.Enabled {
		v, e := NewGoogleReceiptVerifier(c, nil)
		if e != nil {
			return nil, e
		}
		providers[PlatformGooglePlay] = v
	}
	if c.Apple.Enabled {
		v, e := NewAppleReceiptVerifier(c, nil)
		if e != nil {
			return nil, e
		}
		providers[PlatformAppStore] = v
	}
	return NewBillingPurchases(db, c, cfg.Tuning.Economy, providers)
}

// ProcessProviderTasks rereads one bounded batch of previously verified/pending
// purchases, plus pending acknowledgement tasks. A failed observation moves to
// the back of the persisted queue so one outage cannot starve other receipts.
func (p *Purchases) ProcessProviderTasks(ctx context.Context) (int, error) {
	if !p.BillingAvailable(PlatformGooglePlay) && !p.BillingAvailable(PlatformAppStore) {
		return 0, nil
	}
	limit := p.billing.TaskBatchSize
	if limit < 1 || limit > 100 {
		return 0, ErrBillingUnavailable
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	defer queryCancel()
	rows, e := p.db.QueryContext(queryCtx, `SELECT work_kind,work_key,platform,account_id,request FROM (
 SELECT 'purchase' AS work_kind,b.purchase_id::text AS work_key,b.platform,b.account_id,s.raw_receipt AS request,b.checked_at
 FROM billing_transactions b JOIN store_purchases s ON s.id=b.purchase_id WHERE b.product_kind='noin' AND b.state NOT IN ('revoked','canceled')
 UNION ALL
 SELECT 'subscription',s.source_key,s.platform,s.account_id,
 CASE WHEN s.platform='google_play' AND o.product_id IS NOT NULL THEN jsonb_set(sp.raw_receipt,'{product_id}',to_jsonb(o.product_id)) ELSE sp.raw_receipt END,c.checked_at
 FROM billing_subscription_sources s JOIN store_purchases sp ON sp.id=s.initial_purchase_id
 JOIN billing_subscription_current c ON (c.platform,c.source_key)=(s.platform,s.source_key)
 JOIN billing_subscription_observations o ON (o.platform,o.source_key,o.id)=(c.platform,c.source_key,c.observation_id)
 WHERE o.state<>'canceled' AND NOT EXISTS(SELECT 1 FROM billing_subscription_replacements r WHERE r.platform=s.platform AND r.predecessor_key=s.source_key)
 ) work WHERE (($2 AND platform='google_play') OR ($3 AND platform='app_store')) ORDER BY checked_at,work_kind,platform,work_key LIMIT $1`, limit, p.BillingAvailable(PlatformGooglePlay), p.BillingAvailable(PlatformAppStore))
	if e != nil {
		return 0, ErrBillingUnavailable
	}
	type task struct {
		kind, id, account string
		platform          PurchasePlatform
		request           ReceiptRequest
	}
	tasks := []task{}
	for rows.Next() {
		var t task
		var raw []byte
		if e = rows.Scan(&t.kind, &t.id, &t.platform, &t.account, &raw); e != nil {
			rows.Close()
			return 0, ErrBillingUnavailable
		}
		if billingJSON(raw, &t.request) != nil {
			rows.Close()
			return 0, ErrBillingProof
		}
		tasks = append(tasks, t)
	}
	e = rows.Err()
	rows.Close()
	queryCancel()
	if e != nil {
		return 0, ErrBillingUnavailable
	}
	failed := false
	processed := 0
	for _, t := range tasks {
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}
		knownSource := ""
		if t.kind == "subscription" {
			knownSource = t.id
		}
		_, e = p.verifyReceipt(ctx, t.account, t.request, knownSource)
		if e != nil {
			failed = true
		}
		writeCtx, writeCancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
		if t.kind == "subscription" {
			_, e = p.db.ExecContext(writeCtx, `UPDATE billing_subscription_current SET checked_at=clock_timestamp() WHERE platform=$1 AND source_key=$2`, t.platform, t.id)
		} else {
			_, e = p.db.ExecContext(writeCtx, `UPDATE billing_transactions SET checked_at=clock_timestamp() WHERE purchase_id=$1`, t.id)
		}
		writeCancel()
		if e != nil {
			failed = true
		}
		processed++
	}
	if e = p.RetryAcknowledgements(ctx, limit); e != nil {
		failed = true
	}
	if failed {
		return processed, ErrBillingUnavailable
	}
	return processed, nil
}

// RunProviderTasks stops on cancellation. Reports contain only stable safe
// codes, never transaction IDs, receipt bodies, credentials or provider errors.
func (p *Purchases) RunProviderTasks(ctx context.Context, report func(error)) {
	if !p.BillingAvailable(PlatformGooglePlay) && !p.BillingAvailable(PlatformAppStore) {
		return
	}
	if p.billing.TaskPollIntervalS < 30 || p.billing.TaskPollIntervalS > 3600 {
		if report != nil {
			report(ErrBillingUnavailable)
		}
		return
	}
	tick := time.NewTicker(time.Duration(p.billing.TaskPollIntervalS) * time.Second)
	defer tick.Stop()
	for {
		if _, e := p.ProcessProviderTasks(ctx); e != nil && ctx.Err() == nil && report != nil {
			report(ErrBillingUnavailable)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
