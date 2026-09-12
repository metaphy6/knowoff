package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/lib/pq"
)

// Every helper uses textValueDB's nonce/name/current_database guard. These
// fixtures never connect to a caller-selected ordinary database.
func textProofRecord(t *testing.T, s *TextValueStore, owner string, mode gamecontract.ModeID, admissions []string) TextMatchRecord {
	t.Helper()
	return TextMatchRecord{Contract: v2.MatchContract{
		ProtocolVersion: 2, MatchID: uuid.NewString(), RoomID: uuid.NewString(),
		ModeID: mode, OriginalSize: 4, RulesVersion: "text-v1", ContentLanguage: "en",
		PackReleaseID: "value-proof", PackSHA256: repeatHash(),
		Tuning:      v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: valuePolicyHash(t, s.tuning)},
		Eligibility: v2.Eligibility{AdmissionID: uuid.NewString(), EntryPath: "quick_play", Rewards: true, Leaderboard: true},
	}, Owner: owner, Epoch: 1, AdmissionIDs: admissions}
}

func textProofMatch(t *testing.T, db *sql.DB, s *TextValueStore, owner string, mode gamecontract.ModeID, accounts []string, at time.Time) (TextMatchRecord, []string) {
	t.Helper()
	if len(accounts) == 0 {
		for i := 0; i < 4; i++ {
			accounts = append(accounts, valueAccount(t, db))
		}
		// The first settlement is a Nower, so its correct-vote and first-win
		// receipts describe a possible four-seat role combination.
		sort.Strings(accounts)
	}
	admissions := make([]string, 4)
	for seat, id := range accounts {
		admissions[seat] = uuid.NewString()
		if err := s.Reserve(t.Context(), TextReservation{ID: admissions[seat], AccountID: id, EntryPath: "quick_play", At: at}); err != nil {
			t.Fatal(err)
		}
	}
	m := textProofRecord(t, s, owner, mode, admissions)
	if err := s.Prepare(t.Context(), m, at); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context(), m.Contract.MatchID, owner, 1, at); err != nil {
		t.Fatal(err)
	}
	return m, accounts
}

func textProofOutcome(m TextMatchRecord, ids []string, at time.Time, winner string) TextOutcome {
	o := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: winner, At: at}
	for seat, id := range ids {
		role := "nower"
		if seat == 3 {
			role = "donower"
		}
		o.Players = append(o.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 20})
	}
	return o
}

func textProofParallel(t *testing.T, count int, fn func(int) error) []error {
	t.Helper()
	start := make(chan struct{})
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); <-start; errs[i] = fn(i) }(i)
	}
	close(start)
	wg.Wait()
	return errs
}

func TestTextAllowanceRacesAcrossModesAndUTC(t *testing.T) {
	db, base := textValueDB(t)
	base.tuning.Economy.FreeDailyQuickplayMatches = 5
	owner, s := ownerReady(t, db, base)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	accounts := []string{valueAccount(t, db), valueAccount(t, db), valueAccount(t, db), valueAccount(t, db)}
	at := time.Date(2026, 9, 12, 23, 59, 58, 0, time.UTC)
	nextDay := at.Add(3 * time.Second)
	var previous TextMatchRecord
	for index, mode := range gamecontract.AllModes() {
		t.Run(string(mode), func(t *testing.T) {
			candidates := make([]string, 20)
			for i := range candidates {
				candidates[i] = uuid.NewString()
			}
			errs := textProofParallel(t, len(candidates), func(i int) error {
				return s.Reserve(ctx, TextReservation{ID: candidates[i], AccountID: accounts[0], EntryPath: "quick_play", At: at})
			})
			winner := ""
			for i, err := range errs {
				if err == nil {
					if winner != "" {
						t.Fatal("two concurrent reservations admitted one account")
					}
					winner = candidates[i]
				} else if !errors.Is(err, ErrValueConflict) && !errors.Is(err, ErrQuotaExhausted) {
					t.Fatal(err)
				}
			}
			if winner == "" {
				t.Fatal("no reservation admitted with allowance remaining")
			}
			admissions := []string{winner}
			for _, id := range accounts[1:] {
				a := TextReservation{ID: uuid.NewString(), AccountID: id, EntryPath: "quick_play", At: at}
				if err := s.Reserve(ctx, a); err != nil {
					t.Fatal(err)
				}
				admissions = append(admissions, a.ID)
			}
			m := textProofRecord(t, s, owner.Token().IncarnationID, mode, admissions)
			if err := s.Prepare(ctx, m, at); err != nil {
				t.Fatal(err)
			}
			for _, err := range textProofParallel(t, 20, func(int) error { return s.Start(ctx, m.Contract.MatchID, m.Owner, 1, at) }) {
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range accounts {
				if n := valueCount(t, db, `SELECT count FROM daily_quickplay_counts WHERE account_id=$1 AND server_day=$2`, id, valueDay(at)); n != int64(index+1) {
					t.Fatalf("mode %s counted %d starts", mode, n)
				}
			}
			if index == 4 {
				previous = m
				for _, err := range textProofParallel(t, 20, func(int) error { return s.Start(ctx, m.Contract.MatchID, m.Owner, 1, nextDay) }) {
					if err != nil {
						t.Fatal("same started identity across UTC", err)
					}
				}
				if n := valueCount(t, db, `SELECT count(*) FROM daily_quickplay_counts WHERE server_day=$1`, valueDay(nextDay)); n != 0 {
					t.Fatal("old Start retry created next-day count", n)
				}
			}
			if err := s.Finish(ctx, textProofOutcome(m, accounts, at.Add(time.Second), "nower")); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, err := range textProofParallel(t, 20, func(int) error {
		return s.Reserve(ctx, TextReservation{ID: uuid.NewString(), AccountID: accounts[0], EntryPath: "quick_play", At: at})
	}) {
		if !errors.Is(err, ErrQuotaExhausted) {
			t.Fatalf("shared five-mode cap: %v", err)
		}
	}
	if err := s.Start(ctx, previous.Contract.MatchID, previous.Owner, 1, nextDay); !errors.Is(err, ErrValueFence) {
		t.Fatal("completed old match restarted", err)
	}
	m, _ := textProofMatch(t, db, s, owner.Token().IncarnationID, gamecontract.ModeTopThat, accounts, nextDay)
	for _, err := range textProofParallel(t, 20, func(int) error { return s.Start(ctx, m.Contract.MatchID, m.Owner, 1, nextDay) }) {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range accounts {
		if n := valueCount(t, db, `SELECT count FROM daily_quickplay_counts WHERE account_id=$1 AND server_day=$2`, id, valueDay(nextDay)); n != 1 {
			t.Fatal("next-day first counter", n)
		}
		if n := valueCount(t, db, `SELECT count FROM daily_quickplay_counts WHERE account_id=$1 AND server_day=$2`, id, valueDay(at)); n != 5 {
			t.Fatal("old day changed", n)
		}
	}
}

var textProofTables = []string{"daily_noin_earned", "daily_quickplay_counts", "leaderboard_daily_counts", "leaderboard_entries", "leaderboard_history", "leaderboard_weeks", "noin_ledger", "noin_wallets", "profiles", "text_admissions", "text_award_receipts", "text_first_win_claims", "text_matches", "text_outbox", "text_settlements"}

func TestTextCompetingModesCannotReuseLastAdmission(t *testing.T) {
	db, base := textValueDB(t)
	owner, s := ownerReady(t, db, base)
	at := time.Date(2026, 9, 12, 23, 59, 59, 0, time.UTC)
	ids, admissions := []string{}, []string{}
	for i := 0; i < 4; i++ {
		id := valueAccount(t, db)
		if _, err := db.Exec(`INSERT INTO daily_quickplay_counts(account_id,server_day,count) VALUES($1,$2,$3)`, id, valueDay(at), s.tuning.Economy.FreeDailyQuickplayMatches-1); err != nil {
			t.Fatal(err)
		}
		a := TextReservation{ID: uuid.NewString(), AccountID: id, EntryPath: "quick_play", At: at}
		if err := s.Reserve(t.Context(), a); err != nil {
			t.Fatal(err)
		}
		ids, admissions = append(ids, id), append(admissions, a.ID)
	}
	var records []TextMatchRecord
	for _, mode := range gamecontract.AllModes() {
		records = append(records, textProofRecord(t, s, owner.Token().IncarnationID, mode, admissions))
	}
	errs := textProofParallel(t, len(records), func(i int) error { return s.Prepare(t.Context(), records[i], at) })
	winner := -1
	for i, err := range errs {
		if err == nil {
			if winner >= 0 {
				t.Fatal("two mode contracts claimed the same admissions")
			}
			winner = i
		} else if !errors.Is(err, ErrValueConflict) {
			t.Fatal(err)
		}
	}
	if winner < 0 {
		t.Fatal("no mode prepared")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_matches`); n != 1 {
		t.Fatal("failed mode preparation retained a match", n)
	}
	m := records[winner]
	for _, err := range textProofParallel(t, 20, func(int) error { return s.Start(t.Context(), m.Contract.MatchID, m.Owner, 1, at) }) {
		if err != nil {
			t.Fatal(err)
		}
	}
	for i, other := range records {
		if i != winner {
			if err := s.Start(t.Context(), other.Contract.MatchID, other.Owner, 1, at); err == nil {
				t.Fatal("losing mode started")
			}
		}
	}
	for _, id := range ids {
		if n := valueCount(t, db, `SELECT count FROM daily_quickplay_counts WHERE account_id=$1 AND server_day=$2`, id, valueDay(at)); n != int64(s.tuning.Economy.FreeDailyQuickplayMatches) {
			t.Fatal("last allowance consumed more than once", n)
		}
		if n := valueCount(t, db, `SELECT count(*) FROM text_admissions WHERE account_id=$1 AND state='started'`, id); n != 1 {
			t.Fatal("active admission identity duplicated", n)
		}
	}
}

func textProofRows(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, table := range textProofTables {
		rows, err := db.QueryContext(t.Context(), `SELECT to_jsonb(t)::text FROM public.`+pq.QuoteIdentifier(table)+` t ORDER BY to_jsonb(t)::text`)
		if err != nil {
			t.Fatal(err)
		}
		out[table] = []string{}
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			out[table] = append(out[table], row)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func textProofSameRows(t *testing.T, want, got map[string][]string) {
	t.Helper()
	for _, table := range textProofTables {
		if !reflect.DeepEqual(want[table], got[table]) {
			t.Errorf("business rows changed in %s (before %d, after %d)", table, len(want[table]), len(got[table]))
		}
	}
}

func textProofTrigger(t *testing.T, db *sql.DB, table, operation, condition, body string) func() {
	t.Helper()
	query := `CREATE FUNCTION text_value_proof_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN ` + body + `; RETURN NEW; END $$; CREATE TRIGGER text_value_proof_fault AFTER ` + operation + ` ON public.` + pq.QuoteIdentifier(table) + ` FOR EACH ROW`
	if condition != "" {
		query += ` WHEN (` + condition + `)`
	}
	query += ` EXECUTE FUNCTION text_value_proof_fault()`
	if _, err := db.ExecContext(t.Context(), query); err != nil {
		t.Fatal(err)
	}
	removed := false
	remove := func() {
		if removed {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := db.ExecContext(ctx, `DROP TRIGGER text_value_proof_fault ON public.`+pq.QuoteIdentifier(table)+`; DROP FUNCTION text_value_proof_fault()`); err != nil {
			t.Error(err)
			return
		}
		removed = true
	}
	t.Cleanup(remove)
	return remove
}

func TestTextDurableWritesRollbackAndReplay(t *testing.T) {
	cases := []struct{ phase, table, operation, condition string }{
		{"award", "daily_noin_earned", "INSERT", ""}, {"award", "daily_noin_earned", "UPDATE", ""},
		{"award", "noin_wallets", "INSERT", ""}, {"award", "noin_ledger", "INSERT", ""}, {"award", "text_award_receipts", "INSERT", ""},
		{"finish", "leaderboard_weeks", "INSERT", ""}, {"finish", "text_matches", "UPDATE", "NEW.state='completed'"},
		{"finish", "text_settlements", "INSERT", ""}, {"finish", "text_admissions", "UPDATE", "NEW.state='released'"},
		{"settle", "daily_noin_earned", "UPDATE", ""}, {"settle", "leaderboard_daily_counts", "INSERT", ""},
		{"settle", "leaderboard_daily_counts", "UPDATE", ""}, {"settle", "profiles", "UPDATE", ""},
		{"settle", "leaderboard_entries", "INSERT", ""}, {"settle", "noin_wallets", "UPDATE", ""},
		{"settle", "noin_ledger", "INSERT", ""}, {"settle", "text_first_win_claims", "INSERT", ""},
		{"settle", "text_award_receipts", "INSERT", ""}, {"settle", "text_outbox", "INSERT", ""},
		{"settle", "text_settlements", "UPDATE", "NEW.state='applied'"},
	}
	for _, tc := range cases {
		t.Run(tc.phase+"/"+tc.table+"/"+tc.operation, func(t *testing.T) {
			db, base := textValueDB(t)
			owner, s := ownerReady(t, db, base)
			at := time.Date(2026, 9, 6, 23, 59, 0, 0, time.UTC)
			m, ids := textProofMatch(t, db, s, owner.Token().IncarnationID, gamecontract.ModeBadBargains, nil, at)
			sorted := append([]string(nil), ids...)
			sort.Strings(sorted)
			target := sorted[0]
			winner := "nower"
			if target == ids[3] {
				winner = "donower"
			}
			o := textProofOutcome(m, ids, at.Add(2*time.Minute), winner)
			a := TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: target, Kind: "correct_vote", Ordinal: 1, Amount: s.tuning.Noin.CorrectVote, At: o.At}
			for i := range o.Players {
				if o.Players[i].AccountID == target {
					o.Players[i].CorrectVotes = 1
				}
			}
			if tc.phase != "award" {
				if _, err := s.Award(t.Context(), a); err != nil {
					t.Fatal(err)
				}
			}
			if tc.phase == "settle" {
				if err := s.Finish(t.Context(), o); err != nil {
					t.Fatal(err)
				}
			}
			before := textProofRows(t, db)
			remove := textProofTrigger(t, db, tc.table, tc.operation, tc.condition, `RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='text value proof fault'`)
			var err error
			switch tc.phase {
			case "award":
				_, err = s.Award(t.Context(), a)
			case "finish":
				err = s.Finish(t.Context(), o)
			case "settle":
				err = s.SettlePending(t.Context(), m.Contract.MatchID)
			}
			var pg *pq.Error
			if !errors.As(err, &pg) || pg.Code != "P0001" || pg.Message != "text value proof fault" {
				t.Fatalf("fault was not reached: %v", err)
			}
			textProofSameRows(t, before, textProofRows(t, db))
			remove()
			if _, err := s.Award(t.Context(), a); err != nil {
				t.Fatal(err)
			}
			if err := s.Finish(t.Context(), o); err != nil {
				t.Fatal(err)
			}
			if err := s.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
				t.Fatal(err)
			}
			after := textProofRows(t, db)
			// A caller can lose an already committed response. A new store instance
			// replays durable settlement without recapturing policy or UTC time.
			restarted := NewTextValueStore(db, base.tuning)
			for i := 0; i < 2; i++ {
				if _, err := s.Award(t.Context(), a); err != nil {
					t.Fatal(err)
				}
				if err := s.Finish(t.Context(), o); err != nil {
					t.Fatal(err)
				}
				if err := restarted.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
					t.Fatal(err)
				}
			}
			textProofSameRows(t, after, textProofRows(t, db))
			if n := valueCount(t, db, `SELECT count(*) FROM text_settlements WHERE match_id=$1 AND state='applied'`, m.Contract.MatchID); n != 4 {
				t.Fatal("missing settlements", n)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM text_outbox WHERE match_id=$1`, m.Contract.MatchID); n != 4 {
				t.Fatal("missing private deliveries", n)
			}
			if n := valueCount(t, db, `SELECT sum(overall_points) FROM profiles`); n != 80 {
				t.Fatal("points missing or repeated", n)
			}
			want := s.tuning.Noin.CorrectVote + s.tuning.Noin.MatchCompleted + s.tuning.Noin.DailyFirstWin
			if winner == "nower" {
				want += s.tuning.Noin.NowerWin
			} else {
				want += s.tuning.Noin.DonowerTeamWin
			}
			if n := valueCount(t, db, `SELECT balance FROM noin_wallets WHERE account_id=$1`, target); n != int64(want) {
				t.Fatalf("wallet %d want%d", n, want)
			}
			if n := valueCount(t, db, `SELECT xp FROM profiles WHERE account_id=$1`, target); n != int64(s.tuning.Progression.XPBase+s.tuning.Progression.XPPerCorrectVote+s.tuning.Progression.XPWinBonus) {
				t.Fatal("XP missing or repeated", n)
			}
			r, err := s.Reconcile(t.Context())
			if err != nil || r.Pending != 0 || r.MissingEffects != 0 || r.LedgerMismatches != 0 || r.WalletMismatches != 0 {
				t.Fatalf("reconcile %+v %v", r, err)
			}
		})
	}
}

func TestTextTransactionRetryPreservesEventAndBound(t *testing.T) {
	for _, tc := range []struct {
		code               string
		failures, attempts int
		success            bool
	}{{"40001", 2, 3, true}, {"40P01", 2, 3, true}, {"40001", 99, 4, false}, {"40P01", 99, 4, false}, {"P0001", 2, 1, false}} {
		t.Run(fmt.Sprintf("%s/%d", tc.code, tc.failures), func(t *testing.T) {
			db, base := textValueDB(t)
			owner, s := ownerReady(t, db, base)
			at := time.Date(2026, 9, 12, 23, 59, 59, 123456789, time.UTC)
			m, ids := textProofMatch(t, db, s, owner.Token().IncarnationID, gamecontract.ModeSecretScale, nil, at)
			if _, err := db.Exec(`CREATE SEQUENCE text_value_proof_attempts`); err != nil {
				t.Fatal(err)
			}
			a := TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Kind: "correct_vote", Ordinal: 1, Amount: s.tuning.Noin.CorrectVote, At: at}
			before := textProofRows(t, db)
			remove := textProofTrigger(t, db, "text_award_receipts", "INSERT", "", fmt.Sprintf(`IF nextval('text_value_proof_attempts') <= %d THEN RAISE EXCEPTION USING ERRCODE='%s', MESSAGE='text value retry proof'; END IF`, tc.failures, tc.code))
			n, err := s.Award(t.Context(), a)
			if tc.success {
				if err != nil || n != a.Amount {
					t.Fatal(n, err)
				}
			} else {
				var pg *pq.Error
				if !errors.As(err, &pg) || string(pg.Code) != tc.code {
					t.Fatal("unexpected retry error", err)
				}
				textProofSameRows(t, before, textProofRows(t, db))
			}
			if n := valueCount(t, db, `SELECT last_value FROM text_value_proof_attempts`); n != int64(tc.attempts) {
				t.Fatal("retry attempt bound", n)
			}
			remove()
			if n, err := s.Award(t.Context(), a); err != nil || n != a.Amount {
				t.Fatal(n, err)
			}
			if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger WHERE account_id=$1`, ids[0]); n != 1 {
				t.Fatal("duplicate retry ledger", n)
			}
			var occurred, day time.Time
			var hash string
			if err := db.QueryRow(`SELECT occurred_at,server_day,body_hash FROM text_award_receipts WHERE match_id=$1`, m.Contract.MatchID).Scan(&occurred, &day, &hash); err != nil {
				t.Fatal(err)
			}
			a.At = valueTime(a.At)
			_, wantHash, err := valueHash(a)
			if err != nil {
				t.Fatal(err)
			}
			if !occurred.Equal(a.At) || !day.Equal(valueDay(at)) || hash != wantHash {
				t.Fatal("retry recaptured event identity or UTC bucket")
			}
		})
	}
}

func TestTextLeaderboardLimitAcrossModesAndMidnight(t *testing.T) {
	db, base := textValueDB(t)
	base.tuning.Economy.FreeDailyQuickplayMatches = 10
	base.tuning.LiveOps.LeaderboardDailyCountedMatches = 2
	owner, s := ownerReady(t, db, base)
	ids := []string{valueAccount(t, db), valueAccount(t, db), valueAccount(t, db), valueAccount(t, db)}
	at := time.Date(2026, 9, 12, 23, 59, 58, 0, time.UTC)
	var matches []TextMatchRecord
	for _, mode := range gamecontract.AllModes() {
		m, _ := textProofMatch(t, db, s, owner.Token().IncarnationID, mode, ids, at)
		if err := s.Finish(t.Context(), textProofOutcome(m, ids, at.Add(time.Second), "nower")); err != nil {
			t.Fatal(err)
		}
		matches = append(matches, m)
	}
	for _, err := range textProofParallel(t, len(matches), func(i int) error { return s.SettlePending(t.Context(), matches[i].Contract.MatchID) }) {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range ids {
		if n := valueCount(t, db, `SELECT count FROM leaderboard_daily_counts WHERE account_id=$1 AND server_day=$2`, id, valueDay(at)); n != 2 {
			t.Fatal("five modes exceeded shared daily leaderboard cap", n)
		}
		if n := valueCount(t, db, `SELECT points FROM leaderboard_entries WHERE account_id=$1`, id); n != 40 {
			t.Fatal("wrong capped leaderboard points", n)
		}
		if n := valueCount(t, db, `SELECT overall_points FROM profiles WHERE account_id=$1`, id); n != 100 {
			t.Fatal("daily board cap reduced career points", n)
		}
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_first_win_claims WHERE account_id=$1`, ids[0]); n != 1 {
		t.Fatal("cross-mode first win duplicated", n)
	}
	next := at.Add(3 * time.Second)
	m, _ := textProofMatch(t, db, s, owner.Token().IncarnationID, gamecontract.ModeMakeRoom, ids, next)
	if err := s.Finish(t.Context(), textProofOutcome(m, ids, next, "nower")); err != nil {
		t.Fatal(err)
	}
	if err := s.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	after := textProofRows(t, db)
	for _, old := range matches {
		if err := s.SettlePending(t.Context(), old.Contract.MatchID); err != nil {
			t.Fatal(err)
		}
	}
	textProofSameRows(t, after, textProofRows(t, db))
	for _, id := range ids {
		if n := valueCount(t, db, `SELECT count FROM leaderboard_daily_counts WHERE account_id=$1 AND server_day=$2`, id, valueDay(next)); n != 1 {
			t.Fatal("new UTC day did not reset board allowance", n)
		}
		if n := valueCount(t, db, `SELECT points FROM leaderboard_entries WHERE account_id=$1`, id); n != 60 {
			t.Fatal("new day did not accumulate same-week points", n)
		}
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_first_win_claims WHERE account_id=$1`, ids[0]); n != 2 {
		t.Fatal("new UTC day missing first win", n)
	}
}
