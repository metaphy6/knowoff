package economy

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSubscriptionTaskMigrationPreservesPendingWork(t *testing.T) {
	db := setupPurchasesTestDB(t)
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('billing_subscription_tasks') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("source acknowledgement schema missing", err)
	}
	account := newAccount(t, db)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	up, down := subscriptionTaskMigrationFile(t, "up"), subscriptionTaskMigrationFile(t, "down")
	if _, err = tx.Exec(down); err != nil {
		t.Fatal("empty source-task down", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i, state := range []string{"pending", "done", "canceled"} {
		id := uuid.NewString()
		key := []string{"pending-source", "done-source", "canceled-source"}[i]
		if _, err = tx.Exec(`INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id,raw_receipt) VALUES($1,$2,'google_play','premium',$3,'{"synthetic":true}')`, id, account, key); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO billing_transactions(purchase_id,platform,provider_key,original_key,account_id,application,environment,product_id,product_kind,quantity,noin_amount,state,purchased_at,observed_at) VALUES($1,'google_play',$2,$2,$3,'example.knowoff','Production','premium','premium_monthly',1,0,'purchased',$4,$4)`, id, key, account, now); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO billing_subscription_sources(platform,source_key,account_id,application,environment,initial_purchase_id) VALUES('google_play',$1,$2,'example.knowoff','Production',$3)`, key, account, id); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO billing_provider_tasks(purchase_id,request,proof,state,attempts,updated_at) VALUES($1,'{"synthetic_request":true}','{"synthetic_proof":true}',$2,4,$3)`, id, state, now); err != nil {
			t.Fatal(err)
		}
	}
	before := subscriptionLegacyFingerprint(t, tx, account)
	if _, err = tx.Exec(`UPDATE billing_provider_tasks SET attempts=-1 WHERE state='pending'`); err != nil {
		t.Fatal(err)
	}
	subscriptionRefuse(t, tx, up)
	if err = tx.QueryRow(`SELECT to_regclass('billing_subscription_tasks') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("failed import left a partial schema", err)
	}
	if _, err = tx.Exec(`UPDATE billing_provider_tasks SET attempts=4 WHERE state='pending'`); err != nil {
		t.Fatal(err)
	}
	var oldTasks string
	if err = tx.QueryRow(`SELECT md5(jsonb_agg(to_jsonb(t) ORDER BY purchase_id)::text) FROM billing_provider_tasks t`).Scan(&oldTasks); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(up); err != nil {
		t.Fatal(err)
	}
	if after := subscriptionLegacyFingerprint(t, tx, account); after != before {
		t.Fatal("source-task migration changed old value/receipts")
	}
	var retainedTasks string
	if err = tx.QueryRow(`SELECT md5(jsonb_agg(to_jsonb(t) ORDER BY purchase_id)::text) FROM billing_provider_tasks t`).Scan(&retainedTasks); err != nil || retainedTasks != oldTasks {
		t.Fatal("old task bytes changed", err)
	}
	var equal bool
	if err = tx.QueryRow(`SELECT bool_and((t.request,t.proof,t.state,t.attempts,t.updated_at)=(old.request,old.proof,old.state,old.attempts,old.updated_at)) FROM billing_subscription_tasks t JOIN billing_subscription_sources s USING(platform,source_key) JOIN billing_provider_tasks old ON old.purchase_id=s.initial_purchase_id`).Scan(&equal); err != nil || !equal {
		t.Fatal("task import changed retained work", err)
	}
	for _, q := range []string{
		`UPDATE billing_subscription_tasks SET state='pending' WHERE state IN ('done','canceled')`,
		`UPDATE billing_subscription_tasks SET source_key='other'`,
		`UPDATE billing_subscription_tasks SET attempts=0`,
		`DELETE FROM billing_subscription_tasks`,
		`INSERT INTO billing_subscription_tasks(platform,source_key,request,proof) VALUES('google_play','unknown','{}','{}')`,
		`INSERT INTO billing_subscription_tasks(platform,source_key,request,proof) VALUES('google_play','pending-source','{}','{}')`,
		down,
	} {
		subscriptionRefuse(t, tx, q)
	}
	if _, err = tx.Exec(`UPDATE billing_subscription_tasks SET state='done',attempts=attempts+1,updated_at=clock_timestamp() WHERE source_key='pending-source'`); err != nil {
		t.Fatal("pending completion refused", err)
	}
	var count int
	if err = tx.QueryRow(`SELECT count(*) FROM pg_indexes WHERE indexname='billing_subscription_tasks_pending'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("pending index missing", err)
	}
}

func subscriptionTaskMigrationFile(t *testing.T, direction string) string {
	t.Helper()
	b, err := os.ReadFile("../../migrations/000022_subscription_tasks." + direction + ".sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
