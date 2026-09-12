package economy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
)

// MatchGrantInput describes one player's match result for Noin granting.
type MatchGrantInput struct {
	AccountID   string
	Seat        int
	Role        game.Role
	Won         bool
	Eliminated  bool
	Absent      bool
	CorrectVote bool
}

// MatchGrants returns the per-account Noin grants for a finished match.
// Team-win grants require enough humans. Discreet grants (Donower survival)
// are returned separately so callers can send private events.
func MatchGrants(cfg *config.Config, humanCount int, inputs []MatchGrantInput) []MatchGrant {
	minHumans := cfg.Tuning.Liquidity.NoinMinHumans
	grants := make([]MatchGrant, 0, len(inputs)*2)

	for _, in := range inputs {
		if in.AccountID == "" || in.Absent {
			continue
		}
		// Match completion grant for everyone who finished the match.
		grants = append(grants, MatchGrant{
			AccountID: in.AccountID,
			Seat:      in.Seat,
			Amount:    cfg.Tuning.Noin.MatchCompleted,
			Reason:    "match_completed",
			Discreet:  false,
			EventType: LedgerMatchCompleted,
		})

		// Team-win grants only when enough humans are present.
		if humanCount >= minHumans && in.Won {
			if in.Role == game.RoleNower {
				grants = append(grants, MatchGrant{
					AccountID: in.AccountID,
					Seat:      in.Seat,
					Amount:    cfg.Tuning.Noin.NowerWin,
					Reason:    "nower_team_win",
					Discreet:  false,
					EventType: LedgerNowerWin,
				})
			} else {
				grants = append(grants, MatchGrant{
					AccountID: in.AccountID,
					Seat:      in.Seat,
					Amount:    cfg.Tuning.Noin.DonowerTeamWin,
					Reason:    "donower_team_win",
					Discreet:  false,
					EventType: LedgerDonowerTeamWin,
				})
			}
		}

		// Correct vote grant.
		if in.CorrectVote {
			grants = append(grants, MatchGrant{
				AccountID: in.AccountID,
				Seat:      in.Seat,
				Amount:    cfg.Tuning.Noin.CorrectVote,
				Reason:    "correct_vote",
				Discreet:  false,
				EventType: LedgerCorrectVote,
			})
		}

		// Discreet Donower survival grant.
		if in.Role == game.RoleDonower && in.Won && humanCount >= minHumans {
			grants = append(grants, MatchGrant{
				AccountID: in.AccountID,
				Seat:      in.Seat,
				Amount:    cfg.Tuning.Noin.DonowerVoteSurvived,
				Reason:    "donower_vote_survived",
				Discreet:  true,
				EventType: LedgerDonowerVoteSurvived,
			})
		}
	}

	return grants
}

// MatchGrant is a single Noin credit to award after a match.
type MatchGrant struct {
	AccountID string
	Seat      int
	Amount    int
	Reason    string
	Discreet  bool
	EventType LedgerEventType
}

// GrantDailyFirstWin credits the daily first-win bonus if it has not already
// been earned today.
func (m *Manager) GrantDailyFirstWin(ctx context.Context, accountID string) (int, error) {
	if accountID == "" {
		return 0, nil
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return 0, fmt.Errorf("invalid account id: %w", err)
	}
	amount := m.config.Tuning.Noin.DailyFirstWin
	if amount <= 0 {
		return 0, nil
	}
	return m.Wallet.Grant(ctx, accountID, LedgerDailyFirstWin, amount, "daily_first_win", int64(m.config.Tuning.Noin.DailyEarnCap))
}

// ErrNoRows mirrors sql.ErrNoRows so callers don't need database/sql.
var ErrNoRows = fmt.Errorf("no rows")
