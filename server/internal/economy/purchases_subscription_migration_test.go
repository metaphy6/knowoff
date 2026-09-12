package economy

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSubscriptionMigrationPreservesReceiptsAndAccess(t *testing.T) {
	db := setupPurchasesTestDB(t)
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('billing_subscription_sources') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("subscription source schema missing", err)
	}
	up := subscriptionMigrationFile(t, "up")
	down := subscriptionMigrationFile(t, "down")
	account := newAccount(t, db)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(down); err != nil {
		t.Fatal("empty down", err)
	}
	// Retained migration19 fixtures, not freshly verified platform receipts.
	now := time.Now().UTC().Truncate(time.Microsecond)
	monthly, yearly := now.Add(24*time.Hour), now.Add(48*time.Hour)
	for i, kind := range []string{"premium_monthly", "premium_yearly", "premium_monthly"} {
		purchase, key := uuid.NewString(), fmt.Sprintf("retained-%d", i)
		expiry := monthly
		if kind == "premium_yearly" {
			expiry = yearly
		}
		if _, err = tx.Exec(`INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id,raw_receipt,verified_at) VALUES($1,$2,'app_store',$3,$4,'{"synthetic":true}', $5)`, purchase, account, kind, key, now); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO billing_transactions(purchase_id,platform,provider_key,original_key,account_id,application,environment,product_id,product_kind,quantity,noin_amount,state,purchased_at,observed_at,expires_at,granted_at) VALUES($1,'app_store',$2,'retained-original',$3,'example.knowoff','Production',$4,$4,1,0,'purchased',$5,$5,$6,$5)`, purchase, key, account, kind, now, expiry); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(`INSERT INTO entitlements(account_id,entitlement_type,value,active_until) VALUES($1,'premium_monthly','retained-monthly',$2),($1,'premium_yearly','retained-yearly',$3);`, account, monthly, yearly); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO noin_wallets(account_id,balance) VALUES($1,47)`, account); err != nil {
		t.Fatal(err)
	}
	before := subscriptionLegacyFingerprint(t, tx, account)
	if _, err = tx.Exec(up); err != nil {
		t.Fatal(err)
	}
	if after := subscriptionLegacyFingerprint(t, tx, account); before != after {
		t.Fatal("migration changed retained receipts/access/wallet")
	}
	var gotMonthly, gotYearly time.Time
	var provenance string
	var imports, observations int
	if err = tx.QueryRow(`SELECT o.monthly_until,o.yearly_until,o.provenance FROM billing_subscription_current c JOIN billing_subscription_observations o ON (o.platform,o.source_key,o.id)=(c.platform,c.source_key,c.observation_id) WHERE c.source_key='retained-original'`).Scan(&gotMonthly, &gotYearly, &provenance); err != nil || !gotMonthly.Equal(monthly) || !gotYearly.Equal(yearly) || provenance != "migration19" {
		t.Fatal("old coverage was reinterpreted", gotMonthly, gotYearly, provenance, err)
	}
	if err = tx.QueryRow(`SELECT count(*) FROM billing_subscription_imports`).Scan(&imports); err != nil || imports != 3 {
		t.Fatal("missing retained receipt mapping", imports, err)
	}
	if err = tx.QueryRow(`SELECT count(*) FROM billing_subscription_observations WHERE provenance='provider'`).Scan(&observations); err != nil || observations != 0 {
		t.Fatal("migration fabricated provider evidence", observations, err)
	}
	for _, query := range []string{
		`UPDATE billing_subscription_sources SET account_id='00000000-0000-4000-8000-000000000001'`,
		`DELETE FROM billing_subscription_sources`,
		`UPDATE billing_subscription_observations SET evidence='{}'`,
		`DELETE FROM billing_subscription_observations`,
		`UPDATE billing_subscription_imports SET row_sha256=repeat('0',64)`,
		down,
	} {
		subscriptionRefuse(t, tx, query)
	}
	if after := subscriptionLegacyFingerprint(t, tx, account); before != after {
		t.Fatal("refusal changed retained data")
	}
}

func TestSubscriptionSchemaBindsProjectionAndReplacement(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account, other := newAccount(t, db), newAccount(t, db)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, key := range []string{"a", "b", "c", "other"} {
		owner := account
		if key == "other" {
			owner = other
		}
		purchase := uuid.NewString()
		if _, err = tx.Exec(`INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id) VALUES($1,$2,'google_play','premium',$3)`, purchase, owner, key); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO billing_subscription_sources(platform,source_key,account_id,application,environment,initial_purchase_id) VALUES('google_play',$1,$2,'example.knowoff','Production',$3)`, key, owner, purchase); err != nil {
			t.Fatal(err)
		}
	}
	insertEdge := `INSERT INTO billing_subscription_replacements(platform,predecessor_key,successor_key,account_id,application,environment) VALUES('google_play','a','b','` + account + `','example.knowoff','Production')`
	if _, err = tx.Exec(insertEdge); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		insertEdge,
		`INSERT INTO billing_subscription_replacements(platform,predecessor_key,successor_key,account_id,application,environment) VALUES('google_play','b','a','` + account + `','example.knowoff','Production')`,
		`INSERT INTO billing_subscription_replacements(platform,predecessor_key,successor_key,account_id,application,environment) VALUES('google_play','a','c','` + account + `','example.knowoff','Production')`,
		`INSERT INTO billing_subscription_replacements(platform,predecessor_key,successor_key,account_id,application,environment) VALUES('google_play','b','other','` + account + `','example.knowoff','Production')`,
		`UPDATE billing_subscription_replacements SET successor_key='c'`,
		`DELETE FROM billing_subscription_replacements`,
		`INSERT INTO billing_subscription_current(platform,source_key,observation_id,verified_at) VALUES('google_play','a',999999,now())`,
		`INSERT INTO billing_subscription_observations(platform,source_key,evidence_sha256,provenance,state,evidence,observed_at) VALUES('google_play','a',repeat('0',64),'provider','purchased','{}',now())`,
	} {
		subscriptionRefuse(t, tx, query)
	}
	var indexes int
	if err = tx.QueryRow(`SELECT count(*) FROM pg_indexes WHERE indexname IN ('billing_subscription_poll','billing_subscription_account','billing_subscription_observation_identity')`).Scan(&indexes); err != nil || indexes != 3 {
		t.Fatal("missing bounded-work/source indexes", indexes, err)
	}
}

func TestSubscriptionSourceRejectsWrongReceiptOwner(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account, other := newAccount(t, db), newAccount(t, db)
	purchase := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id) VALUES($1,$2,'google_play','premium','binding')`, purchase, account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO billing_subscription_sources(platform,source_key,account_id,application,environment,initial_purchase_id) VALUES('google_play','binding',$1,'example.knowoff','Production',$2)`, other, purchase); err == nil {
		t.Fatal("source attached another account's receipt")
	}
}

func TestSubscriptionProjectionIsSourceScopedAndMonotonic(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, key := range []string{"a", "b"} {
		purchase := uuid.NewString()
		if _, err = tx.Exec(`INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id) VALUES($1,$2,'google_play','premium',$3)`, purchase, account, key); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO billing_subscription_sources(platform,source_key,account_id,application,environment,initial_purchase_id) VALUES('google_play',$1,$2,'example.knowoff','Production',$3)`, key, account, purchase); err != nil {
			t.Fatal(err)
		}
	}
	ids := []int64{}
	for i, key := range []string{"a", "a", "b"} {
		var id int64
		at := now.Add(time.Duration(i) * time.Second)
		if err = tx.QueryRow(`INSERT INTO billing_subscription_observations(platform,source_key,evidence_sha256,provenance,state,product_id,product_kind,transaction_key,purchased_at,observed_at,monthly_until,evidence) VALUES('google_play',$1,$2,'provider','purchased','premium','premium_monthly',$1,$3,$4,$5,'{"synthetic":true}') RETURNING id`, key, fmt.Sprintf("%064d", i), now, at, now.Add(time.Hour)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if _, err = tx.Exec(`INSERT INTO billing_subscription_current(platform,source_key,observation_id,verified_at) VALUES('google_play','a',$1,$2)`, ids[0], now); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE billing_subscription_current SET observation_id=$1,verified_at=$2 WHERE source_key='a'`, ids[1], now.Add(time.Second)); err != nil {
		t.Fatal("new current state refused", err)
	}
	subscriptionRefuse(t, tx, fmt.Sprintf(`UPDATE billing_subscription_current SET observation_id=%d WHERE source_key='a'`, ids[0]))
	subscriptionRefuse(t, tx, fmt.Sprintf(`UPDATE billing_subscription_current SET observation_id=%d WHERE source_key='a'`, ids[2]))
	subscriptionRefuse(t, tx, `DELETE FROM billing_subscription_current`)
	var current int64
	if err = tx.QueryRow(`SELECT observation_id FROM billing_subscription_current WHERE source_key='a'`).Scan(&current); err != nil || current != ids[1] {
		t.Fatal("rejected projection changed authority", current, err)
	}
}

func TestSubscriptionReplacementCyclesSerialize(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	for _, key := range []string{"a", "b"} {
		purchase := uuid.NewString()
		if _, err := db.Exec(`INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id) VALUES($1,$2,'google_play','premium',$3)`, purchase, account, key); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO billing_subscription_sources(platform,source_key,account_id,application,environment,initial_purchase_id) VALUES('google_play',$1,$2,'example.knowoff','Production',$3)`, key, account, purchase); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, pair := range [][2]string{{"a", "b"}, {"b", "a"}} {
		wg.Add(1)
		go func(pair [2]string) {
			defer wg.Done()
			<-start
			_, err := db.ExecContext(ctx, `INSERT INTO billing_subscription_replacements(platform,predecessor_key,successor_key,account_id,application,environment) VALUES('google_play',$1,$2,$3,'example.knowoff','Production')`, pair[0], pair[1], account)
			errs <- err
		}(pair)
	}
	close(start)
	wg.Wait()
	close(errs)
	succeeded := 0
	for err := range errs {
		if err == nil {
			succeeded++
		}
	}
	if ctx.Err() != nil || succeeded != 1 {
		t.Fatal("competing replacement cycle not bounded/serialized", succeeded, ctx.Err())
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM billing_subscription_replacements`).Scan(&count); err != nil || count != 1 {
		t.Fatal("cycle committed", count, err)
	}
}

func TestSubscriptionProjectionRecheckPreventsLateChangedProof(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account, purchase := newAccount(t, db), uuid.NewString()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id) VALUES($1,$2,'google_play','premium','recheck')`, purchase, account); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO billing_subscription_sources(platform,source_key,account_id,application,environment,initial_purchase_id) VALUES('google_play','recheck',$1,'example.knowoff','Production',$2)`, account, purchase); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	ids := []int64{}
	for i, state := range []string{"paused", "on_hold"} {
		var id int64
		if err = tx.QueryRow(`INSERT INTO billing_subscription_observations(platform,source_key,evidence_sha256,provenance,state,product_id,product_kind,transaction_key,purchased_at,observed_at,evidence) VALUES('google_play','recheck',$1,'provider',$2,'premium','premium_monthly','recheck',$3,$4,'{"synthetic":true}') RETURNING id`, fmt.Sprintf("%064d", i), state, now, now.Add(time.Duration(i)*time.Second)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if _, err = tx.Exec(`INSERT INTO billing_subscription_current(platform,source_key,observation_id,verified_at) VALUES('google_play','recheck',$1,$2)`, ids[0], now); err != nil {
		t.Fatal("separate successful verification clock missing", err)
	}
	// An identical successful recheck at t+2 reuses immutable evidence first seen
	// at t. A delayed changed response observed at t+1 must not overwrite it.
	if _, err = tx.Exec(`UPDATE billing_subscription_current SET verified_at=$1 WHERE source_key='recheck'`, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	subscriptionRefuse(t, tx, fmt.Sprintf(`UPDATE billing_subscription_current SET observation_id=%d,verified_at='%s' WHERE source_key='recheck'`, ids[1], now.Add(time.Second).Format(time.RFC3339Nano)))
	// Failed polling only advances checked_at. It neither changes verification
	// authority nor prevents a subsequently completed newer successful response.
	if _, err = tx.Exec(`UPDATE billing_subscription_current SET checked_at=$1 WHERE source_key='recheck'`, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE billing_subscription_current SET observation_id=$1,verified_at=$2 WHERE source_key='recheck'`, ids[1], now.Add(3*time.Second)); err != nil {
		t.Fatal("failed poll became authority", err)
	}
	// Google may return the same earlier state after recovery: evidence replay
	// is immutable, while a genuinely newer successful observation advances it.
	if _, err = tx.Exec(`UPDATE billing_subscription_current SET observation_id=$1,verified_at=$2 WHERE source_key='recheck'`, ids[0], now.Add(5*time.Second)); err != nil {
		t.Fatal("newer repeated state refused", err)
	}
}

func subscriptionMigrationFile(t *testing.T, direction string) string {
	t.Helper()
	b, err := os.ReadFile("../../migrations/000021_subscription_sources." + direction + ".sql")
	if err != nil {
		t.Fatal(err)
	}
	if direction == "down" {
		return subscriptionTaskMigrationFile(t, "down") + "\n" + string(b)
	}
	return string(b)
}
func subscriptionRefuse(t *testing.T, tx *sql.Tx, query string) {
	t.Helper()
	if _, err := tx.Exec(`SAVEPOINT subscription_refusal`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(query); err == nil {
		t.Fatal("unsafe subscription write accepted", query)
	}
	if _, err := tx.Exec(`ROLLBACK TO SAVEPOINT subscription_refusal`); err != nil {
		t.Fatal(err)
	}
}
func subscriptionLegacyFingerprint(t *testing.T, tx *sql.Tx, account string) string {
	t.Helper()
	var result string
	if err := tx.QueryRow(`SELECT md5(jsonb_build_array(
	(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM store_purchases p WHERE account_id=$1),
	(SELECT jsonb_agg(to_jsonb(b) ORDER BY purchase_id) FROM billing_transactions b WHERE account_id=$1),
	(SELECT jsonb_agg(to_jsonb(e) ORDER BY entitlement_type) FROM entitlements e WHERE account_id=$1),
	(SELECT to_jsonb(w) FROM noin_wallets w WHERE account_id=$1),
	(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM noin_ledger l WHERE account_id=$1))::text)`, account).Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
