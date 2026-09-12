package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func adminRoomDecision(t *testing.T, db *sql.DB, owner *TextOwner, room, kind, target string, accounts []string) AdminOperationReceipt {
	t.Helper()
	actor, account := uuid.NewString(), valueAccount(t, db)
	if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, account); err != nil {
		t.Fatal(err)
	}
	ctx := WithAdminAuthorization(t.Context(), actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
	token := owner.Token()
	c := AdminOperationCommand{ID: uuid.NewString(), Kind: kind, TargetAccountID: target, RoomID: room, OwnerID: token.IncarnationID, OwnerGeneration: token.Generation, Reason: "Disposable room operation fixture"}
	r, err := NewAdminOperationStore(db).Decide(ctx, actor, c, accounts)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAdminRoomWaitingReleaseAndAuditAreAtomic(t *testing.T) {
	for _, kind := range []string{"room_close", "room_kick"} {
		t.Run(kind, func(t *testing.T) {
			db, values := textValueDB(t)
			owner, values := ownerReady(t, db, values)
			room := uuid.NewString()
			accounts := []string{valueAccount(t, db), valueAccount(t, db)}
			reservations := map[string]string{}
			for _, account := range accounts {
				reservation := uuid.NewString()
				reservations[account] = reservation
				if err := values.Reserve(t.Context(), TextReservation{ID: reservation, AccountID: account, EntryPath: "local", At: time.Now()}); err != nil {
					t.Fatal(err)
				}
			}
			target := ""
			if kind == "room_kick" {
				target = accounts[0]
			}
			r := adminRoomDecision(t, db, owner, room, kind, target, accounts)
			request := AdminRoomApplication{OperationID: r.Command.ID, RoomID: room, Reservations: reservations, Effect: "release", At: time.Now()}
			if _, err := db.Exec(`CREATE FUNCTION refuse_room_completion_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='operator_applied' THEN RAISE EXCEPTION 'fixture audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER refuse_room_completion_audit BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION refuse_room_completion_audit()`); err != nil {
				t.Fatal(err)
			}
			if _, err := values.ApplyRoomOperation(t.Context(), request); err == nil {
				t.Fatal("audit failure accepted room delivery")
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_admissions WHERE state='reserved'`); n != 2 {
				t.Fatal("audit failure released admission", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM admin_operation_results`); n != 0 {
				t.Fatal("audit failure left completion")
			}
			if _, err := db.Exec(`DROP TRIGGER refuse_room_completion_audit ON admin_audit_log; DROP FUNCTION refuse_room_completion_audit()`); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				got, err := values.ApplyRoomOperation(t.Context(), request)
				if err != nil || got.Status != "applied" {
					t.Fatal("room replay", got, err)
				}
			}
			want := int64(2)
			if kind == "room_kick" {
				want = 1
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_admissions WHERE state='released'`); n != want {
				t.Fatal("wrong admission effects", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger`); n != 0 {
				t.Fatal("room release fabricated value", n)
			}
			request.RoomID = uuid.NewString()
			if _, err := values.ApplyRoomOperation(t.Context(), request); err == nil {
				t.Fatal("replayed decision into replacement room")
			}
		})
	}
}

func TestAdminRoomCloseJoinsPreparedOrStartedCompensation(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared", true: "started"}[started], func(t *testing.T) {
			db, values := textValueDB(t)
			owner, values := ownerReady(t, db, values)
			at := time.Now().UTC()
			match, accounts := ownerMatch(t, owner, values, db, at, started)
			if started {
				if _, err := values.Award(t.Context(), TextAward{MatchID: match.Contract.MatchID, Owner: match.Owner, Epoch: 1, AccountID: accounts[0], Kind: "correct_vote", Ordinal: 1, Amount: values.tuning.Noin.CorrectVote, At: at}); err != nil {
					t.Fatal(err)
				}
			}
			r := adminRoomDecision(t, db, owner, match.Contract.RoomID, "room_close", "", accounts)
			request := AdminRoomApplication{OperationID: r.Command.ID, RoomID: match.Contract.RoomID, MatchID: match.Contract.MatchID, Effect: "close", At: at.Add(time.Second)}
			before := adminRoomValueFingerprint(t, db)
			if _, err := db.Exec(`CREATE FUNCTION refuse_room_completion_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='operator_applied' THEN RAISE EXCEPTION 'fixture audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER refuse_room_completion_audit BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION refuse_room_completion_audit()`); err != nil {
				t.Fatal(err)
			}
			if _, err := values.ApplyRoomOperation(t.Context(), request); err == nil {
				t.Fatal("close audit failure accepted")
			}
			if after := adminRoomValueFingerprint(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("failed close changed quota, awards, terminal, or private outbox", before, after)
			}
			receipt, err := NewAdminOperationStore(db).Get(t.Context(), r.Command.ID)
			if err != nil || receipt.Status != "pending" {
				t.Fatal(receipt, err)
			}
			if _, err = db.Exec(`DROP TRIGGER refuse_room_completion_audit ON admin_audit_log; DROP FUNCTION refuse_room_completion_audit()`); err != nil {
				t.Fatal(err)
			}

			for i := 0; i < 2; i++ {
				got, err := values.ApplyRoomOperation(t.Context(), request)
				if err != nil || got.Status != "applied" {
					t.Fatal("close replay", got, err)
				}
			}
			if n := valueCount(t, db, `SELECT COALESCE(sum(count),0) FROM daily_quickplay_counts`); n != 0 {
				t.Fatal("quota not compensated exactly once", n)
			}
			want := int64(0)
			state := "cancelled"
			if started {
				want = int64(values.tuning.Noin.CorrectVote)
				state = "interrupted"
			}
			if n := valueCount(t, db, `SELECT COALESCE(sum(amount),0) FROM noin_ledger`); n != want {
				t.Fatal("instant award changed", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_matches WHERE state=$1`, state); n != 1 {
				t.Fatal("wrong durable terminal state", state, n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM admin_operation_results`); n != 1 {
				t.Fatal("duplicate completion", n)
			}
		})
	}
}

// Whole-row hashes cover every relevant durable effect, including timestamps and
// immutable outcome bytes; equal row counts alone cannot prove atomic rollback.
func adminRoomValueFingerprint(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, table := range []string{"text_matches", "text_admissions", "daily_quickplay_counts", "text_award_receipts", "text_settlements", "text_outbox", "noin_wallets", "noin_ledger", "profiles"} {
		var hash string
		if err := db.QueryRow(fmt.Sprintf(`SELECT md5(COALESCE(string_agg(row_to_json(r)::text,',' ORDER BY row_to_json(r)::text),'')) FROM %s r`, table)).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		result[table] = hash
	}
	return result
}
func TestAdminRoomTerminalPrecedenceAndKickAcknowledgement(t *testing.T) {
	for _, kind := range []string{"room_close", "room_kick"} {
		t.Run(kind, func(t *testing.T) {
			db, values := textValueDB(t)
			owner, values := ownerReady(t, db, values)
			at := time.Now().UTC()
			match, accounts := ownerMatch(t, owner, values, db, at, true)
			target := ""
			effect := "close"
			if kind == "room_kick" {
				target = accounts[0]
				effect = "kick"
			}
			decision := adminRoomDecision(t, db, owner, match.Contract.RoomID, kind, target, accounts)
			if kind == "room_close" {
				out := TextOutcome{MatchID: match.Contract.MatchID, Owner: match.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: at}
				for seat, id := range accounts {
					role := "nower"
					if seat == 0 {
						role = "donower"
					}
					out.Players = append(out.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 10})
				}
				if err := values.Finish(t.Context(), out); err != nil {
					t.Fatal(err)
				}
				if err := values.SettlePending(t.Context(), match.Contract.MatchID); err != nil {
					t.Fatal(err)
				}
			}
			before := adminRoomValueFingerprint(t, db)
			request := AdminRoomApplication{OperationID: decision.Command.ID, RoomID: match.Contract.RoomID, MatchID: match.Contract.MatchID, Effect: effect, At: at.Add(time.Second)}
			var wg sync.WaitGroup
			errs := make(chan error, 4)
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); _, err := values.ApplyRoomOperation(t.Context(), request); errs <- err }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if after := adminRoomValueFingerprint(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("acknowledgement rewrote value", before, after)
			}
			receipt, err := NewAdminOperationStore(db).Get(t.Context(), decision.Command.ID)
			if err != nil {
				t.Fatal(err)
			}
			var result AdminOperationResult
			if json.Unmarshal(receipt.Result, &result) != nil {
				t.Fatal("bad result")
			}
			want := "applied"
			if kind == "room_close" {
				want = "obsolete"
			}
			if receipt.Status != want {
				t.Fatal(receipt)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM admin_operation_results`); n != 1 {
				t.Fatal(n)
			}
		})
	}
}
func TestAdminRoomRejectsWrongCapturedAccountsAndLostGeneration(t *testing.T) {
	db, values := textValueDB(t)
	owner, values := ownerReady(t, db, values)
	at := time.Now().UTC()
	match, accounts := ownerMatch(t, owner, values, db, at, true)
	wrong := append([]string(nil), accounts...)
	wrong[0] = valueAccount(t, db)
	decision := adminRoomDecision(t, db, owner, match.Contract.RoomID, "room_close", "", wrong)
	before := adminRoomValueFingerprint(t, db)
	request := AdminRoomApplication{OperationID: decision.Command.ID, RoomID: match.Contract.RoomID, MatchID: match.Contract.MatchID, Effect: "close", At: at}
	if _, err := values.ApplyRoomOperation(t.Context(), request); err == nil {
		t.Fatal("changed captured accounts accepted")
	}
	if after := adminRoomValueFingerprint(t, db); !reflect.DeepEqual(before, after) {
		t.Fatal("mismatched capture changed value")
	}
	if err := owner.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := values.ApplyRoomOperation(t.Context(), request); err == nil {
		t.Fatal("lost physical owner accepted")
	}
	next, err := AcquireTextOwner(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release(context.Background())
	replacement, err := values.WithOwner(next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = replacement.ApplyRoomOperation(t.Context(), request); err == nil {
		t.Fatal("replacement owner adopted old operation")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM admin_operation_results`); n != 0 {
		t.Fatal("fenced operation completed", n)
	}
}

func TestAdminRoomRecoveryRequiresConfirmedLostAndCompletedCompensation(t *testing.T) {
	db, values := textValueDB(t)
	owner, values := ownerReady(t, db, values)
	at := time.Now().UTC()
	match, accounts := ownerMatch(t, owner, values, db, at, true)
	award := TextAward{MatchID: match.Contract.MatchID, Owner: match.Owner, Epoch: 1, AccountID: accounts[0], Kind: "correct_vote", Ordinal: 1, Amount: values.tuning.Noin.CorrectVote, At: at}
	if _, err := values.Award(t.Context(), award); err != nil {
		t.Fatal(err)
	}
	receipt := adminRoomDecision(t, db, owner, match.Contract.RoomID, "room_close", "", accounts)
	type recovery interface {
		RecoverRoomOperations(context.Context, int) (int, error)
	}
	current, ok := any(values).(recovery)
	if !ok {
		t.Fatal("room operation recovery is missing")
	}
	if n, err := current.RecoverRoomOperations(t.Context(), 1); err != nil || n != 0 {
		t.Fatal("live owner incorrectly declared obsolete", n, err)
	}
	if err := owner.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	next, err := AcquireTextOwner(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release(context.Background())
	replacement, err := values.WithOwner(next)
	if err != nil {
		t.Fatal(err)
	}
	recovered := any(replacement).(recovery)
	if _, err = recovered.RecoverRoomOperations(t.Context(), 1); err == nil {
		t.Fatal("operation completed before predecessor compensation")
	}
	batch, err := next.RecoverLostOwners(t.Context(), replacement, 100)
	if err != nil || !batch.Done {
		t.Fatal(batch, err)
	}
	before := adminRoomValueFingerprint(t, db)
	if _, err = db.Exec(`CREATE FUNCTION refuse_room_recovery_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='operator_obsolete' THEN RAISE EXCEPTION 'fixture audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER refuse_room_recovery_audit BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION refuse_room_recovery_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err = recovered.RecoverRoomOperations(t.Context(), 1); err == nil {
		t.Fatal("recovery ignored audit failure")
	}
	pending, err := NewAdminOperationStore(db).Get(t.Context(), receipt.Command.ID)
	if err != nil || pending.Status != "pending" {
		t.Fatal(pending, err)
	}
	if _, err = db.Exec(`DROP TRIGGER refuse_room_recovery_audit ON admin_audit_log; DROP FUNCTION refuse_room_recovery_audit()`); err != nil {
		t.Fatal(err)
	}
	if n, err := recovered.RecoverRoomOperations(t.Context(), 1); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if n, err := recovered.RecoverRoomOperations(t.Context(), 1); err != nil || n != 0 {
		t.Fatal("recovery replay", n, err)
	}
	completed, err := NewAdminOperationStore(db).Get(t.Context(), receipt.Command.ID)
	if err != nil || completed.Status != "obsolete" {
		t.Fatal(completed, err)
	}
	if after := adminRoomValueFingerprint(t, db); !reflect.DeepEqual(before, after) {
		t.Fatal("receipt recovery rewrote recovered value")
	}
	if n := valueCount(t, db, `SELECT sum(amount) FROM noin_ledger`); n != int64(values.tuning.Noin.CorrectVote) {
		t.Fatal("recovery changed earned award", n)
	}
}

func TestAdminRoomDecisionProbeWaitsForCommitOrRollback(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(fmt.Sprintf("commit_%t", commit), func(t *testing.T) {
			db, values := textValueDB(t)
			owner, _ := ownerReady(t, db, values)
			accounts := []string{valueAccount(t, db)}
			base := adminRoomDecision(t, db, owner, uuid.NewString(), "room_close", "", accounts)
			command := base.Command
			command.ID = uuid.NewString()
			source := NewAdminOperationStore(db)
			resolver, ok := any(source).(interface {
				ResolveRoomDecision(context.Context, string, AdminOperationCommand, []string) (AdminOperationReceipt, error)
			})
			if !ok {
				t.Fatal("serialized decision probe missing")
			}
			hold, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer hold.Rollback()
			if _, err = hold.Exec(`SELECT pg_advisory_xact_lock(27,278)`); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`CREATE FUNCTION block_room_decision_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(27,278); RETURN NEW; END $$; CREATE TRIGGER block_room_decision_commit AFTER INSERT ON admin_operation_decisions FOR EACH ROW EXECUTE FUNCTION block_room_decision_commit()`); err != nil {
				t.Fatal(err)
			}
			original, cancel := context.WithCancel(t.Context())
			defer cancel()
			original = WithAdminAuthorization(original, base.ActorID, func(context.Context, *sql.Tx) (string, error) { return base.ActorID, nil })
			decisionDone := make(chan error, 1)
			go func() { _, err := source.Decide(original, base.ActorID, command, accounts); decisionDone <- err }()
			deadline := time.Now().Add(2 * time.Second)
			for {
				var waiting bool
				if err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND classid=27 AND objid=278 AND NOT granted)`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("original decision did not reach commit barrier")
				}
				time.Sleep(time.Millisecond)
			}
			type probeResult struct {
				receipt AdminOperationReceipt
				err     error
			}
			done := make(chan probeResult, 1)
			go func() {
				r, e := resolver.ResolveRoomDecision(t.Context(), base.ActorID, command, accounts)
				done <- probeResult{r, e}
			}()
			select {
			case r := <-done:
				t.Fatal("probe crossed uncommitted decision", r.err)
			case <-time.After(30 * time.Millisecond):
			}
			if !commit {
				cancel()
				if err = <-decisionDone; err == nil {
					t.Fatal("canceled decision committed")
				}
			}
			if err = hold.Commit(); err != nil {
				t.Fatal(err)
			}
			if commit {
				if err = <-decisionDone; err != nil {
					t.Fatal(err)
				}
			}
			result := <-done
			if commit {
				if result.err != nil || result.receipt.Command.ID != command.ID || result.receipt.Status != "pending" {
					t.Fatal(result)
				}
			} else {
				if result.err != sql.ErrNoRows {
					t.Fatal("rollback did not prove absence", result.err)
				}
			}
		})
	}
}

func TestAdminRoomDecisionProbeRequiresExactCapturedRequestAndNeverDecides(t *testing.T) {
	db, values := textValueDB(t)
	owner, _ := ownerReady(t, db, values)
	accounts := []string{valueAccount(t, db), valueAccount(t, db)}
	prior := adminRoomDecision(t, db, owner, uuid.NewString(), "room_close", "", accounts)
	source := NewAdminOperationStore(db)
	for _, field := range []string{"actor", "reason", "room", "owner", "generation", "accounts"} {
		t.Run(field, func(t *testing.T) {
			actor, command, affected := prior.ActorID, prior.Command, append([]string(nil), accounts...)
			switch field {
			case "actor":
				actor = uuid.NewString()
			case "reason":
				command.Reason = "different reviewed reason"
			case "room":
				command.RoomID = uuid.NewString()
			case "owner":
				command.OwnerID = uuid.NewString()
			case "generation":
				command.OwnerGeneration++
			case "accounts":
				affected = affected[:1]
			}
			if _, err := source.ResolveRoomDecision(t.Context(), actor, command, affected); !errors.Is(err, ErrAdminOperation) {
				t.Fatal("mismatched captured request confirmed", err)
			}
		})
	}
	bound := WithAdminAuthorization(t.Context(), prior.ActorID, func(context.Context, *sql.Tx) (string, error) { return prior.ActorID, nil })
	if _, err := source.ResolveRoomDecision(bound, prior.ActorID, prior.Command, accounts); !errors.Is(err, ErrAdminRequired) {
		t.Fatal("browser used trusted resolution", err)
	}
	missing := prior.Command
	missing.ID = uuid.NewString()
	if _, err := source.ResolveRoomDecision(t.Context(), prior.ActorID, missing, accounts); err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if count := valueCount(t, db, `SELECT count(*) FROM admin_operation_decisions`); count != 1 {
		t.Fatal("probe created a decision", count)
	}
	if count := valueCount(t, db, `SELECT count(*) FROM admin_operation_results`); count != 0 {
		t.Fatal("probe created an effect", count)
	}
}

func TestAdminRoomRecoveredReceiptSurvivesDeletedParticipant(t *testing.T) {
	db, values := textValueDB(t)
	owner, values := ownerReady(t, db, values)
	at := time.Now().UTC()
	match, accounts := ownerMatch(t, owner, values, db, at, true)
	prior := adminRoomDecision(t, db, owner, match.Contract.RoomID, "room_close", "", accounts)
	if err := owner.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	next, err := AcquireTextOwner(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release(context.Background())
	bound, err := values.WithOwner(next)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := next.RecoverLostOwners(t.Context(), bound, 100)
	if err != nil || !batch.Done {
		t.Fatal(batch, err)
	}
	// The match compensation is already committed. A later soft deletion must
	// not prevent its immutable operator receipt from completing at startup.
	if _, err = db.Exec(`UPDATE accounts SET deleted_at=clock_timestamp() WHERE id=$1`, accounts[0]); err != nil {
		t.Fatal(err)
	}
	before := adminRoomValueFingerprint(t, db)
	if n, err := bound.RecoverRoomOperations(t.Context(), 100); err != nil || n != 1 {
		t.Fatal("retained deleted participant stalled receipt recovery", n, err)
	}
	if n, err := bound.RecoverRoomOperations(t.Context(), 100); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	got, err := NewAdminOperationStore(db).Get(t.Context(), prior.Command.ID)
	if err != nil || got.Status != "obsolete" {
		t.Fatal(got, err)
	}
	if after := adminRoomValueFingerprint(t, db); !reflect.DeepEqual(before, after) {
		t.Fatal("receipt changed compensated value")
	}
}
