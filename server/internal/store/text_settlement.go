package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

func (s *TextValueStore) Award(ctx context.Context, a TextAward) (int, error) {
	if err := s.validateProcessOwner(a.Owner); err != nil {
		return 0, err
	}
	a.At = valueTime(a.At)
	if !valueUUID(a.MatchID) || !valueUUID(a.AccountID) || a.At.IsZero() || a.Ordinal < 1 || a.Ordinal > 3 || a.Amount < 0 || (a.Kind != "correct_vote" && a.Kind != "donower_vote_survived") {
		return 0, ErrValueConflict
	}
	credited := 0
	err := s.ownerTransaction(ctx, func(tx *sql.Tx) error {
		m, err := lockTextMatch(ctx, tx, a.MatchID)
		if err != nil {
			return err
		}
		_, bodyHash, hashErr := valueHash(a)
		if hashErr != nil {
			return hashErr
		}
		var previous string
		readErr := tx.QueryRowContext(ctx, `SELECT body_hash,credited FROM text_award_receipts WHERE match_id=$1 AND account_id=$2 AND kind=$3 AND ordinal=$4`, a.MatchID, a.AccountID, a.Kind, a.Ordinal).Scan(&previous, &credited)
		if readErr == nil {
			if previous != bodyHash {
				return ErrValueConflict
			}
			return nil
		}
		if readErr != sql.ErrNoRows {
			return readErr
		}
		if err = checkValueFence(m, a.Owner, a.Epoch, "started"); err != nil {
			return err
		}
		if !m.started.Valid || a.At.Before(m.started.Time) {
			return ErrValueConflict
		}
		if err = valueAccountLock(ctx, tx, a.AccountID); err != nil {
			return err
		}
		pinned := *s
		pinned.tuning = m.record.Policy
		credited, err = pinned.awardTx(ctx, tx, a, m.record.Prototype || !m.record.Contract.Eligibility.Rewards)
		return err
	})
	return credited, err
}
func (s *TextValueStore) awardTx(ctx context.Context, tx *sql.Tx, a TextAward, zero bool) (int, error) {
	_, hash, err := valueHash(a)
	if err != nil {
		return 0, err
	}
	var existing string
	var credited int
	err = tx.QueryRowContext(ctx, `SELECT body_hash,credited FROM text_award_receipts WHERE match_id=$1 AND account_id=$2 AND kind=$3 AND ordinal=$4`, a.MatchID, a.AccountID, a.Kind, a.Ordinal).Scan(&existing, &credited)
	if err == nil {
		if existing != hash {
			return 0, ErrValueConflict
		}
		return credited, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	allowed := map[string]int{"correct_vote": s.tuning.Noin.CorrectVote, "donower_vote_survived": s.tuning.Noin.DonowerVoteSurvived, "match_completed": s.tuning.Noin.MatchCompleted, "nower_win": s.tuning.Noin.NowerWin, "donower_team_win": s.tuning.Noin.DonowerTeamWin, "daily_first_win": s.tuning.Noin.DailyFirstWin}
	// The engine deliberately emits zero for a prototype/no-reward event.
	// Its receipt is still immutable; live events must match the pinned policy.
	if amount, ok := allowed[a.Kind]; !ok || amount != a.Amount && !(zero && a.Amount == 0) {
		return 0, ErrValueConflict
	}
	day := valueDay(a.At)
	credited = a.Amount
	if zero {
		credited = 0
	}
	if a.Kind == "daily_first_win" && !zero {
		var old bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM noin_ledger WHERE account_id=$1 AND server_day=$2 AND event_type='daily_first_win')`, a.AccountID, day).Scan(&old); err != nil {
			return 0, err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO text_first_win_claims(account_id,server_day,match_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, a.AccountID, day, a.MatchID)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n == 0 || old {
			credited = 0
		}
	}
	var ledger any
	if credited > 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, a.AccountID, day); err != nil {
			return 0, err
		}
		var earned int
		if err = tx.QueryRowContext(ctx, `SELECT earned FROM daily_noin_earned WHERE account_id=$1 AND server_day=$2 FOR UPDATE`, a.AccountID, day).Scan(&earned); err != nil {
			return 0, err
		}
		remaining := s.tuning.Noin.DailyEarnCap - earned
		if remaining < credited {
			credited = remaining
		}
		if credited < 0 {
			credited = 0
		}
		if credited > 0 {
			if _, err = tx.ExecContext(ctx, `INSERT INTO noin_wallets(account_id,balance) VALUES($1,$2) ON CONFLICT(account_id) DO UPDATE SET balance=noin_wallets.balance+EXCLUDED.balance,updated_at=now()`, a.AccountID, credited); err != nil {
				return 0, err
			}
			var id int64
			if err = tx.QueryRowContext(ctx, `INSERT INTO noin_ledger(account_id,event_type,amount,reason,payload,server_day) VALUES($1,$2,$3,'text match award',jsonb_build_object('writer','text-v2','match_id',$4::text,'ordinal',$5::int),$6) RETURNING id`, a.AccountID, a.Kind, credited, a.MatchID, a.Ordinal, day).Scan(&id); err != nil {
				return 0, err
			}
			ledger = id
			if _, err = tx.ExecContext(ctx, `UPDATE daily_noin_earned SET earned=earned+$3,updated_at=now() WHERE account_id=$1 AND server_day=$2`, a.AccountID, day, credited); err != nil {
				return 0, err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO text_award_receipts(match_id,account_id,kind,ordinal,body_hash,occurred_at,server_day,requested,credited,ledger_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, a.MatchID, a.AccountID, a.Kind, a.Ordinal, hash, a.At, day, a.Amount, credited, ledger)
	return credited, err
}

func (s *TextValueStore) Finish(ctx context.Context, o TextOutcome) error {
	if err := s.validateProcessOwner(o.Owner); err != nil {
		return err
	}
	o.At = valueTime(o.At)
	if !valueUUID(o.MatchID) || o.At.IsZero() || (o.Kind != "completed" && o.Kind != "scored_low_population") || (o.Kind == "completed" && o.Winner != "nower" && o.Winner != "donower") || (o.Kind == "scored_low_population" && o.Winner != "") {
		return ErrValueConflict
	}
	o.Players = append([]TextPlayerResult(nil), o.Players...)
	sort.Slice(o.Players, func(i, j int) bool { return o.Players[i].Seat < o.Players[j].Seat })
	body, hash, err := valueHash(o)
	if err != nil {
		return err
	}
	return s.ownerTransaction(ctx, func(tx *sql.Tx) error {
		m, err := lockTextMatch(ctx, tx, o.MatchID)
		if err != nil {
			return err
		}
		if m.hash.Valid {
			if m.hash.String != hash {
				return ErrValueConflict
			}
			return nil
		}
		if err = checkValueFence(m, o.Owner, o.Epoch, "started"); err != nil {
			return err
		}
		if !m.started.Valid || o.At.Before(m.started.Time) {
			return ErrValueConflict
		}
		if m.record.Contract.Eligibility.Leaderboard && !m.record.Prototype {
			week, err := ensureValueWeek(ctx, tx, o.At)
			if err != nil {
				return err
			}
			var closed bool
			if err := tx.QueryRowContext(ctx, `SELECT closed FROM leaderboard_weeks WHERE week_id=$1 FOR UPDATE`, week).Scan(&closed); err != nil {
				return err
			}
			if closed {
				return ErrWeekClosed
			}
		}
		admissions, err := matchAdmissions(ctx, tx, o.MatchID)
		if err != nil {
			return err
		}
		if len(o.Players) != len(admissions) {
			return ErrValueConflict
		}
		byAccount := map[string]int{}
		for _, a := range admissions {
			byAccount[a.account] = a.seat
		}
		seen := map[string]bool{}
		donowers := 0
		for i, p := range o.Players {
			seat, ok := byAccount[p.AccountID]
			if !ok || seen[p.AccountID] || p.Seat != seat || p.Seat != i || p.Points < 0 || p.Points > int64(m.record.Policy.Points.CorrectVote*3+m.record.Policy.Points.DonowerTeamWin+m.record.Policy.Points.NowerWinBonus) || p.CorrectVotes < 0 || p.CorrectVotes > 3 || p.VotesCast < 0 || p.VotesCast > 3 || p.Survivals < 0 || p.Survivals > 3 || p.Pokes < 0 || (p.Role != "nower" && p.Role != "donower") {
				return ErrValueConflict
			}
			seen[p.AccountID] = true
			if p.Role == "donower" {
				donowers++
			}
		}
		if donowers != m.record.Policy.Game.DonowersBySize[len(o.Players)] {
			return ErrValueConflict
		}
		_, err = tx.ExecContext(ctx, `UPDATE text_matches SET state=$2,ended_at=$3,outcome=$4,outcome_hash=$5 WHERE id=$1`, o.MatchID, o.Kind, o.At, body, hash)
		if err != nil {
			return err
		}
		for _, a := range admissions {
			if _, err = tx.ExecContext(ctx, `INSERT INTO text_settlements(match_id,account_id,outcome_hash) VALUES($1,$2,$3)`, o.MatchID, a.account, hash); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE text_admissions SET state='released' WHERE match_id=$1 AND state='started'`, o.MatchID)
		return err
	})
}

// SettlePending replays immutable outcomes. Each account's local value effects,
// private notification identity and applied receipt commit in one transaction.
func (s *TextValueStore) SettlePending(ctx context.Context, matchID string) error {
	rows, err := s.db.QueryContext(ctx, `SELECT account_id FROM text_settlements WHERE match_id=$1 AND state='pending' ORDER BY account_id`, matchID)
	if err != nil {
		return err
	}
	var accounts []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		accounts = append(accounts, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if err = s.settle(ctx, matchID, account); err != nil {
			return err
		}
	}
	return nil
}
func valueWeek(at time.Time) time.Time {
	day := valueDay(at)
	weekday := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -weekday)
}
func boolValue(b bool) int {
	if b {
		return 1
	}
	return 0
}
func (s *TextValueStore) settle(ctx context.Context, matchID, account string) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		m, err := lockTextMatch(ctx, tx, matchID)
		if err != nil {
			return err
		}
		if m.state != "completed" && m.state != "scored_low_population" {
			return ErrValueFence
		}
		var state string
		if err = tx.QueryRowContext(ctx, `SELECT state FROM text_settlements WHERE match_id=$1 AND account_id=$2 FOR UPDATE`, matchID, account).Scan(&state); err != nil {
			return err
		}
		if state == "applied" {
			return nil
		}
		var o TextOutcome
		if err = json.Unmarshal(m.outcome, &o); err != nil {
			return err
		}
		var p TextPlayerResult
		found := false
		for _, player := range o.Players {
			if player.AccountID == account {
				p = player
				found = true
				break
			}
		}
		if !found {
			return ErrValueConflict
		}
		pinned := *s
		pinned.tuning = m.record.Policy
		policy := m.record.Policy
		eligible := m.record.Contract.Eligibility.Rewards && !m.record.Prototype
		leaderboard := eligible && m.record.Contract.Eligibility.Leaderboard && len(o.Players) >= policy.Liquidity.LeaderboardMinHumans
		day, week := valueDay(o.At), valueWeek(o.At)
		weekID := week.Format("2006-01-02")
		if leaderboard {
			if _, err = tx.ExecContext(ctx, `INSERT INTO leaderboard_weeks(week_id,start_at,end_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, weekID, week, week.AddDate(0, 0, 7)); err != nil {
				return err
			}
			var closed bool
			if err = tx.QueryRowContext(ctx, `SELECT closed FROM leaderboard_weeks WHERE week_id=$1 FOR UPDATE`, weekID).Scan(&closed); err != nil {
				return err
			}
			if closed {
				return fmt.Errorf("leaderboard week closed with unsettled outcome")
			}
		}
		if err = valueAccountLock(ctx, tx, account); err != nil {
			return err
		}

		// All day buckets precede profile/wallet locks, including capped zero rewards.
		if eligible {
			if _, err = tx.ExecContext(ctx, `INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, account, day); err != nil {
				return err
			}
			var earned int
			if err = tx.QueryRowContext(ctx, `SELECT earned FROM daily_noin_earned WHERE account_id=$1 AND server_day=$2 FOR UPDATE`, account, day).Scan(&earned); err != nil {
				return err
			}
			if leaderboard {
				if _, err = tx.ExecContext(ctx, `INSERT INTO leaderboard_daily_counts(account_id,server_day,count) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, account, day); err != nil {
					return err
				}
				var counted int
				if err = tx.QueryRowContext(ctx, `SELECT count FROM leaderboard_daily_counts WHERE account_id=$1 AND server_day=$2 FOR UPDATE`, account, day).Scan(&counted); err != nil {
					return err
				}
			}
		}
		present := !p.Absent
		points := p.Points
		if !present && o.Kind != "scored_low_population" {
			points = 0
		}
		won := o.Kind == "completed" && p.Role == o.Winner
		xp := 0
		countedMatch := false
		if eligible {
			xp = policy.Progression.XPBase + p.CorrectVotes*policy.Progression.XPPerCorrectVote
			if won {
				xp += policy.Progression.XPWinBonus
			}
			if !present && o.Kind != "scored_low_population" {
				xp = 0
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO profiles(account_id) VALUES($1) ON CONFLICT DO NOTHING`, account); err != nil {
				return err
			}
			var oldXP int
			if err = tx.QueryRowContext(ctx, `SELECT xp FROM profiles WHERE account_id=$1 FOR UPDATE`, account).Scan(&oldXP); err != nil {
				return err
			}
			level := 1
			for i, threshold := range policy.Progression.LevelThresholds {
				if oldXP+xp >= threshold {
					level = i + 1
				}
			}
			_, err = tx.ExecContext(ctx, `UPDATE profiles SET overall_points=overall_points+$2,non_converted_points=non_converted_points+$2,xp=xp+$3,level=$4,matches_played=matches_played+1,matches_won_nower=matches_won_nower+$5,matches_won_donower=matches_won_donower+$6,correct_votes=correct_votes+$7,votes_cast=votes_cast+$8,donower_survivals=donower_survivals+$9,donower_matches=donower_matches+$10,pokes_sent=pokes_sent+$11,updated_at=now() WHERE account_id=$1`, account, points, xp, level, boolValue(won && p.Role == "nower"), boolValue(won && p.Role == "donower"), p.CorrectVotes, p.VotesCast, p.Survivals, boolValue(p.Role == "donower"), p.Pokes)
			if err != nil {
				return err
			}
			if leaderboard {
				if _, err = tx.ExecContext(ctx, `INSERT INTO leaderboard_daily_counts(account_id,server_day,count) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, account, day); err != nil {
					return err
				}
				var counted int
				if err = tx.QueryRowContext(ctx, `SELECT count FROM leaderboard_daily_counts WHERE account_id=$1 AND server_day=$2 FOR UPDATE`, account, day).Scan(&counted); err != nil {
					return err
				}
				if counted < policy.LiveOps.LeaderboardDailyCountedMatches {
					countedMatch = true
					if _, err = tx.ExecContext(ctx, `UPDATE leaderboard_daily_counts SET count=count+1 WHERE account_id=$1 AND server_day=$2`, account, day); err != nil {
						return err
					}
					if _, err = tx.ExecContext(ctx, `INSERT INTO leaderboard_entries(week_id,account_id,points,matches_counted) VALUES($1,$2,$3,1) ON CONFLICT(week_id,account_id) DO UPDATE SET points=leaderboard_entries.points+EXCLUDED.points,matches_counted=leaderboard_entries.matches_counted+1,updated_at=now()`, weekID, account, points); err != nil {
						return err
					}
				}
			}
		}
		// Event awards were already credited at their accepted vote occurrence.
		// Terminal settlement never reconstructs them from aggregate counters.
		awards := []struct {
			kind   string
			amount int
		}{}
		if present {
			awards = append(awards, struct {
				kind   string
				amount int
			}{"match_completed", policy.Noin.MatchCompleted})
		}
		if present && won && len(o.Players) >= policy.Liquidity.NoinMinHumans {
			kind, amount := "nower_win", policy.Noin.NowerWin
			if p.Role == "donower" {
				kind, amount = "donower_team_win", policy.Noin.DonowerTeamWin
			}
			awards = append(awards, struct {
				kind   string
				amount int
			}{kind, amount}, struct {
				kind   string
				amount int
			}{"daily_first_win", policy.Noin.DailyFirstWin})
		}
		for _, a := range awards {
			if _, err = pinned.awardTx(ctx, tx, TextAward{MatchID: matchID, Owner: m.record.Owner, Epoch: m.record.Epoch, AccountID: account, Kind: a.kind, Ordinal: 0, Amount: a.amount, At: o.At}, !eligible); err != nil {
				return err
			}
		}
		private := TextPrivateSettlement{MatchID: matchID, Points: points, XP: xp, LeaderboardCounted: countedMatch, Awards: []TextPrivateAward{}}
		if !eligible {
			private.Points = 0
		}
		rows, err := tx.QueryContext(ctx, `SELECT kind,ordinal,requested,credited FROM text_award_receipts WHERE match_id=$1 AND account_id=$2 ORDER BY kind,ordinal`, matchID, account)
		if err != nil {
			return err
		}
		for rows.Next() {
			var a TextPrivateAward
			if err = rows.Scan(&a.Kind, &a.Ordinal, &a.Requested, &a.Credited); err != nil {
				rows.Close()
				return err
			}
			private.Awards = append(private.Awards, a)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		payload, err := json.Marshal(private)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO text_outbox(match_id,account_id,effect_kind,payload) VALUES($1,$2,'private_settlement',$3) ON CONFLICT DO NOTHING`, matchID, account, payload); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE text_settlements SET state='applied',applied_at=now(),effects=$3 WHERE match_id=$1 AND account_id=$2`, matchID, account, payload); err != nil {
			return err
		}
		if s.beforeCommit != nil {
			return s.beforeCommit()
		}
		return nil
	})
}
