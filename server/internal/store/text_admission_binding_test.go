package store

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestTextAdmissionBindingRequiredAndFresh(t *testing.T) {
	for _, variant := range []string{"missing", "duplicate", "wrong_account", "expired", "revoked", "epoch", "captured", "clean"} {
		t.Run(variant, func(t *testing.T) {
			db, base := textValueDB(t)
			record, ids := valuePreparedMatch(t, base, db, time.Now(), false)
			bindings := make([]TextAdmissionBinding, len(ids))
			for i, id := range ids {
				bindings[i] = TextAdmissionBinding{AccountID: id, DeviceHash: uuid.NewString(), SessionEpoch: 0, TokenID: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour), Purpose: "player"}
				if _, err := db.Exec(`INSERT INTO auth_installations(device_hash) VALUES($1)`, bindings[i].DeviceHash); err != nil {
					t.Fatal(err)
				}
			}
			switch variant {
			case "missing":
				bindings = nil
			case "duplicate":
				bindings[1] = bindings[0]
			case "wrong_account":
				bindings[0].AccountID = uuid.NewString()
			case "expired":
				bindings[0].ExpiresAt = time.Now().Add(-time.Second)
			case "revoked":
				if _, err := db.Exec(`INSERT INTO auth_revocations(token_id,expires_at) VALUES($1,$2)`, bindings[0].TokenID, bindings[0].ExpiresAt); err != nil {
					t.Fatal(err)
				}
			case "epoch":
				bindings[0].SessionEpoch++
			case "captured", "clean":
				target := valueAccount(t, db)
				actor := uuid.NewString()
				admin := valueAccount(t, db)
				if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, admin); err != nil {
					t.Fatal(err)
				}
				hash := bindings[0].DeviceHash
				if variant == "clean" {
					hash = "other-historical-install"
				}
				if _, err := db.Exec(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2),($3,$2)`, target, hash, ids[0]); err != nil {
					t.Fatal(err)
				}
				ctx := WithAdminAuthorization(t.Context(), actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
				if _, err := NewAccountSanctionStore(db).Apply(ctx, actor, AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target, Reason: "captured peer installation"}, func(context.Context, *sql.Tx, string) error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			ctx := WithTextAdmissionBindings(t.Context(), bindings)
			s := base.RequireAdmissionBindings()
			err := s.Start(ctx, record.Contract.MatchID, record.Owner, record.Epoch, time.Now())
			if variant == "clean" {
				if err != nil {
					t.Fatal("unrelated clean installation denied", err)
				}
				return
			}
			if err == nil {
				t.Fatal("untrusted binding admitted", variant)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM daily_quickplay_counts`); n != 0 {
				t.Fatal("refusal spent quota", n)
			}
		})
	}
}

func waitAdmissionLock(t *testing.T, ctx context.Context, db *sql.DB, fragment string) {
	t.Helper()
	for {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1`, "%"+fragment+"%").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("lock barrier absent", fragment, ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
}

func TestTextAdmissionAndSharedInstallationSanctionBothCommitOrders(t *testing.T) {
	for _, startFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "sanction_first", true: "start_first"}[startFirst], func(t *testing.T) {
			db, base := textValueDB(t)
			record, ids := valuePreparedMatch(t, base, db, time.Now(), false)
			s := base.RequireAdmissionBindings()
			bindings := make([]TextAdmissionBinding, len(ids))
			for i, id := range ids {
				bindings[i] = TextAdmissionBinding{AccountID: id, DeviceHash: uuid.NewString(), TokenID: uuid.NewString(), Purpose: "player", ExpiresAt: time.Now().Add(time.Hour)}
				if _, err := db.Exec(`INSERT INTO auth_installations(device_hash) VALUES($1)`, bindings[i].DeviceHash); err != nil {
					t.Fatal(err)
				}
			}
			target, admin := valueAccount(t, db), valueAccount(t, db)
			actor := uuid.NewString()
			if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, admin); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2)`, target, bindings[0].DeviceHash); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			barrier, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer barrier.Rollback()
			if startFirst {
				_, err = barrier.ExecContext(ctx, `LOCK TABLE daily_quickplay_counts IN SHARE MODE`)
			} else {
				_, err = barrier.ExecContext(ctx, `SELECT device_hash FROM auth_installations WHERE device_hash=$1 FOR UPDATE`, bindings[0].DeviceHash)
			}
			if err != nil {
				t.Fatal(err)
			}
			startDone, sanctionDone := make(chan error, 1), make(chan error, 1)
			start := func() {
				startDone <- s.Start(WithTextAdmissionBindings(ctx, bindings), record.Contract.MatchID, record.Owner, record.Epoch, time.Now())
			}
			sanction := func() {
				authority := WithAdminAuthorization(ctx, actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
				_, e := NewAccountSanctionStore(db).Apply(authority, actor, AdminOperationCommand{ID: uuid.NewString(), Kind: "account_sanction", TargetAccountID: target, Reason: "ordered installation sanction"}, func(context.Context, *sql.Tx, string) error { return nil })
				sanctionDone <- e
			}
			if startFirst {
				go start()
				waitAdmissionLock(t, ctx, db, "INSERT INTO daily_quickplay_counts")
				go sanction()
				waitAdmissionLock(t, ctx, db, "auth_installations")
			} else {
				go sanction()
				waitAdmissionLock(t, ctx, db, "auth_installations")
				go start()
				waitAdmissionLock(t, ctx, db, "SELECT device_hash FROM auth_installations")
			}
			if err = barrier.Commit(); err != nil {
				t.Fatal(err)
			}
			startErr := <-startDone
			if err = <-sanctionDone; err != nil {
				t.Fatal(err)
			}
			if startFirst {
				if startErr != nil {
					t.Fatal("earlier start failed", startErr)
				}
				if err = s.Start(context.Background(), record.Contract.MatchID, record.Owner, record.Epoch, time.Now()); err != nil {
					t.Fatal("committed Start retry reauthorized", err)
				}
			} else {
				if startErr == nil {
					t.Fatal("later start escaped captured installation sanction")
				}
				if n := valueCount(t, db, `SELECT count(*) FROM daily_quickplay_counts`); n != 0 {
					t.Fatal("refused Start spent", n)
				}
			}
		})
	}
}

func TestTextAdmissionExpiryWhileBonusCaptureWaitsRollsBackAllValue(t *testing.T) {
	db, base := textValueDB(t)
	record, ids := valuePreparedMatch(t, base, db, time.Now(), false)
	bindings := make([]TextAdmissionBinding, len(ids))
	expiry := time.Now().Add(2 * time.Second)
	for i, id := range ids {
		bindings[i] = TextAdmissionBinding{AccountID: id, DeviceHash: uuid.NewString(), TokenID: uuid.NewString(), Purpose: "player", ExpiresAt: expiry}
		if _, err := db.Exec(`INSERT INTO auth_installations(device_hash) VALUES($1)`, bindings[i].DeviceHash); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	barrier, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback()
	if _, err = barrier.ExecContext(ctx, `LOCK TABLE text_bonus_eligibility IN SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- base.RequireAdmissionBindings().Start(WithTextAdmissionBindings(ctx, bindings), record.Contract.MatchID, record.Owner, record.Epoch, time.Now())
	}()
	waitAdmissionLock(t, ctx, db, "INSERT INTO text_bonus_eligibility")
	time.Sleep(time.Until(expiry) + 20*time.Millisecond)
	if err = barrier.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("expired binding committed Start after bonus wait")
	}
	for _, query := range []string{`SELECT count(*) FROM daily_quickplay_counts`, `SELECT count(*) FROM text_bonus_eligibility`, `SELECT count(*) FROM text_matches WHERE state='started'`} {
		if n := valueCount(t, db, query); n != 0 {
			t.Fatal("expired Start retained value", query, n)
		}
	}
}
