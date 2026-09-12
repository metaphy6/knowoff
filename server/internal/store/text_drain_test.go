package store

import (
	"testing"
	"time"
)

func TestTextDrainStatusCountsDurableWorkWithoutApplyingIt(t *testing.T) {
	db, s := textValueDB(t)
	ctx := t.Context()
	now := time.Now().UTC()
	prepared, _ := valuePreparedMatch(t, s, db, now, false)
	active, ids := valueMatch(t, s, db, now, false)
	status, err := s.DrainStatus(ctx)
	if err != nil || status.PreparedMatches != 1 || status.StartedMatches != 1 || status.ReservedAdmissions != 4 || status.PendingSettlements != 0 {
		t.Fatal(status, err)
	}
	result := TextOutcome{MatchID: active.Contract.MatchID, Owner: active.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: now.Add(time.Minute)}
	for seat, id := range ids {
		role := "nower"
		if seat == 3 {
			role = "donower"
		}
		result.Players = append(result.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 20})
	}
	if err = s.Finish(ctx, result); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		status, err = s.DrainStatus(ctx)
		if err != nil || status.PendingSettlements != 4 || status.StartedMatches != 0 || status.Undelivered != 0 {
			t.Fatal(status, err)
		}
		if valueCount(t, db, `SELECT count(*) FROM noin_ledger`) != 0 {
			t.Fatal("status applied pending value")
		}
	}
	if err = s.SettlePending(ctx, active.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	if err = s.CancelPrepared(ctx, prepared.Contract.MatchID, prepared.Owner, 1, now); err != nil {
		t.Fatal(err)
	}
	status, err = s.DrainStatus(ctx)
	if err != nil || status.PendingSettlements != 0 || status.PreparedMatches != 0 || status.ReservedAdmissions != 0 || status.Undelivered != 4 {
		t.Fatal("committed delivery is distinct from unapplied settlement", status, err)
	}
}
