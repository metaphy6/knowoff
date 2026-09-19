package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAccountSanctionIndependentExactLiftAndRetry(t *testing.T) {
	db, _ := textValueDB(t)
	target := valueAccount(t, db)
	adminAccount := valueAccount(t, db)
	actor := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, adminAccount); err != nil {
		t.Fatal(err)
	}
	ctx := WithAdminAuthorization(t.Context(), actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
	for _, hash := range []string{"known-a", "known-b"} {
		if _, err := db.Exec(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2)`, target, hash); err != nil {
			t.Fatal(err)
		}
	}
	s := NewAccountSanctionStore(db)
	revocations := 0
	revoke := func(ctx context.Context, tx *sql.Tx, account string) error {
		revocations++
		_, err := tx.ExecContext(ctx, `UPDATE accounts SET session_epoch=session_epoch+1 WHERE id=$1`, account)
		return err
	}
	first := AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target, Reason: "Independent direct sanction"}
	for i := 0; i < 2; i++ {
		r, err := s.Apply(ctx, actor, first, revoke)
		if err != nil || r.Status != "pending" {
			t.Fatal(r, err)
		}
	}
	if revocations != 1 {
		t.Fatal("retry revoked twice", revocations)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM account_sanction_installations WHERE operation_id=$1`, first.ID); n != 2 {
		t.Fatal("incomplete captured installations", n)
	}
	until := time.Now().Add(time.Hour)
	second := AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target, Reason: "Another independent sanction", Until: &until}
	if _, err := s.Apply(ctx, actor, second, revoke); err != nil {
		t.Fatal(err)
	}
	lift := AdminOperationCommand{ID: uuid.NewString(), Kind: "sanction_lift", TargetAccountID: target, PriorSanctionID: first.ID, Reason: "Exact reviewed lift"}
	for i := 0; i < 2; i++ {
		if _, err := s.Apply(ctx, actor, lift, revoke); err != nil {
			t.Fatal(err)
		}
	}
	var active bool
	if err := db.QueryRow(`SELECT direct_account_sanction_active($1)`, target).Scan(&active); err != nil || !active {
		t.Fatal("lift cleared unrelated sanction", active, err)
	}
	lift.ID = uuid.NewString()
	lift.PriorSanctionID = second.ID
	if _, err := s.Apply(ctx, actor, lift, revoke); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT direct_account_sanction_active($1) OR installation_sanction_active('known-a')`, target).Scan(&active); err != nil || active {
		t.Fatal("lift failed", active, err)
	}
	if n := valueCount(t, db, `SELECT session_epoch FROM accounts WHERE id=$1`, target); n != 2 {
		t.Fatal("lift changed epoch", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger`); n != 0 {
		t.Fatal("sanction changed value", n)
	}
	if _, err := db.Exec(`DELETE FROM account_sanction_lifts`); err == nil {
		t.Fatal("sanction provenance mutable")
	}
}

func TestAccountSanctionCapturePaginationAuditRollbackAndPendingDelivery(t *testing.T) {
	db, _ := textValueDB(t)
	target := valueAccount(t, db)
	actor := uuid.NewString()
	adminAccount := valueAccount(t, db)
	if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, adminAccount); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 205; i++ {
		if _, err := db.Exec(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2)`, target, fmt.Sprintf("installation-%03d", i)); err != nil {
			t.Fatal(err)
		}
	}
	ctx := WithAdminAuthorization(t.Context(), actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
	s := NewAccountSanctionStore(db)
	command := AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target, Reason: "Complete retained capture"}
	revoke := func(ctx context.Context, tx *sql.Tx, account string) error {
		_, err := tx.ExecContext(ctx, `UPDATE accounts SET session_epoch=session_epoch+1 WHERE id=$1`, account)
		return err
	}
	if _, err := db.Exec(`CREATE FUNCTION refuse_sanction_capture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture capture failure'; END $$; CREATE TRIGGER refuse_capture BEFORE INSERT ON account_sanction_installations FOR EACH ROW EXECUTE FUNCTION refuse_sanction_capture()`); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.Apply(ctx, actor, command, revoke)
	if err == nil || receipt.Command.ID != "" {
		t.Fatal("failed transaction returned a receipt", receipt.Command.ID, err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM admin_operation_decisions`); n != 0 {
		t.Fatal("failure retained decision", n)
	}
	if _, err = db.Exec(`DROP TRIGGER refuse_capture ON account_sanction_installations; DROP FUNCTION refuse_sanction_capture()`); err != nil {
		t.Fatal(err)
	}
	receipt, err = s.Apply(ctx, actor, command, revoke)
	if err != nil || receipt.Status != "pending" {
		t.Fatal(receipt.Status, err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM account_sanction_installations`); n != 205 {
		t.Fatal("silently truncated installation capture", n)
	}
	if err = s.ResumePending(ctx, 20, func(context.Context, string) error { return errors.New("socket cleanup failed") }); err == nil {
		t.Fatal("failed delivery returned success")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM account_sanction_deliveries`); n != 0 {
		t.Fatal("failed delivery retained completion")
	}
	if _, err = db.Exec(`CREATE FUNCTION refuse_sanction_delivery() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='sanction_live_delivery' THEN RAISE EXCEPTION 'fixture delivery audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER refuse_delivery BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION refuse_sanction_delivery()`); err != nil {
		t.Fatal(err)
	}
	delivered := 0
	deliver := func(context.Context, string) error { delivered++; return nil }
	if err = s.ResumePending(ctx, 20, deliver); err == nil {
		t.Fatal("audit failure accepted delivery")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM admin_operation_results`); n != 0 {
		t.Fatal("delivery audit failure left result")
	}
	if _, err = db.Exec(`DROP TRIGGER refuse_delivery ON admin_audit_log; DROP FUNCTION refuse_sanction_delivery()`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = s.ResumePending(t.Context(), 20, deliver); err != nil {
			t.Fatal(err)
		}
	}
	receipt, err = NewAdminOperationStore(db).Get(t.Context(), command.ID)
	if err != nil || receipt.Status != "applied" || delivered != 2 {
		t.Fatal(receipt.Status, delivered, err)
	}
	if n := valueCount(t, db, `SELECT session_epoch FROM accounts WHERE id=$1`, target); n != 1 {
		t.Fatal("delivery replay bumped epoch", n)
	}
}

func TestAccountSanctionDeliveryReplayKeepsOriginalOutcomeAfterLift(t *testing.T) {
	db, _ := textValueDB(t)
	target, admin := valueAccount(t, db), valueAccount(t, db)
	actor := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, admin); err != nil {
		t.Fatal(err)
	}
	ctx := WithAdminAuthorization(t.Context(), actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
	s := NewAccountSanctionStore(db)
	c := AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target, Reason: "retained delivery"}
	noop := func(context.Context, *sql.Tx, string) error { return nil }
	if _, err := s.Apply(ctx, actor, c, noop); err != nil {
		t.Fatal(err)
	}
	deliver := func(context.Context, string) error { return nil }
	if err := s.DeliverPending(ctx, c.ID, deliver); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, actor, AdminOperationCommand{ID: uuid.NewString(), Kind: "sanction_lift", TargetAccountID: target, PriorSanctionID: c.ID, Reason: "exact lift"}, noop); err != nil {
		t.Fatal(err)
	}
	if err := s.DeliverPending(ctx, c.ID, deliver); err != nil {
		t.Fatal("historical delivery replay changed outcome after lift", err)
	}
}
