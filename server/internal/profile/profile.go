// Package profile manages player profiles, public stats, XP progression, and
// point balances. All values are server-authoritative.
package profile

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/lib/pq"
)

// Profile is the public + owner-only view of a player.
type Profile struct {
	AccountID          string    `json:"account_id"`
	Nickname           string    `json:"nickname"`
	Avatar             string    `json:"avatar"`
	AvatarRevision     int64     `json:"avatar_revision"`
	Level              int       `json:"level"`
	XP                 int       `json:"xp"`
	OverallPoints      int64     `json:"overall_points"`
	NonConvertedPoints int64     `json:"non_converted_points,omitempty"`
	MatchesPlayed      int       `json:"matches_played"`
	MatchesWonNower    int       `json:"matches_won_nower"`
	MatchesWonDonower  int       `json:"matches_won_donower"`
	CorrectVotes       int       `json:"correct_votes"`
	VotesCast          int       `json:"votes_cast"`
	DonowerSurvivals   int       `json:"donower_survivals"`
	DonowerMatches     int       `json:"donower_matches"`
	PokesSent          int       `json:"pokes_sent"`
	WeekWinnerTitles   int       `json:"week_winner_titles"`
	CurrentWeekWinner  bool      `json:"current_week_winner"`
	WeeklyPodiums      int       `json:"weekly_podiums"`
	ContributorCredits []string  `json:"contributor_credits"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Manager is the profile service.
type Manager struct {
	db *sql.DB
}

// NewManager creates a profile manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// Get returns a profile. If owner is false, NonConvertedPoints is omitted.
func (m *Manager) Get(ctx context.Context, accountID string, owner bool) (*Profile, error) {
	row := m.db.QueryRowContext(ctx, `
		SELECT a.id, a.nickname,
 CASE WHEN a.avatar IN ('default','nower','donower','detective','party') THEN a.avatar
 WHEN a.avatar='custom' AND EXISTS(SELECT 1 FROM custom_avatars c WHERE c.account_id=a.id AND c.moderated AND c.revision=a.avatar_revision AND c.revision>0)
 AND EXISTS(SELECT 1 FROM entitlements e WHERE e.account_id=a.id AND e.entitlement_type='custom_avatar' AND e.active_until IS NULL) THEN 'custom' ELSE 'default' END,
 a.avatar_revision,
		       p.level, p.xp, p.overall_points, p.non_converted_points,
		       p.matches_played, p.matches_won_nower, p.matches_won_donower,
		       p.correct_votes, p.votes_cast, p.donower_survivals, p.donower_matches,
		       p.pokes_sent, p.week_winner_titles, p.weekly_podiums,
		       p.contributor_credits, p.updated_at,
		       EXISTS(SELECT 1 FROM challenge_current_winner w WHERE w.singleton AND w.account_id=a.id)
		FROM accounts a JOIN profiles p ON a.id = p.account_id
		WHERE a.id = $1 AND a.deleted_at IS NULL`,
		accountID,
	)
	p := &Profile{}
	if err := row.Scan(
		&p.AccountID, &p.Nickname, &p.Avatar, &p.AvatarRevision,
		&p.Level, &p.XP, &p.OverallPoints, &p.NonConvertedPoints,
		&p.MatchesPlayed, &p.MatchesWonNower, &p.MatchesWonDonower,
		&p.CorrectVotes, &p.VotesCast, &p.DonowerSurvivals, &p.DonowerMatches,
		&p.PokesSent, &p.WeekWinnerTitles, &p.WeeklyPodiums,
		pq.Array(&p.ContributorCredits), &p.UpdatedAt, &p.CurrentWeekWinner,
	); err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}
	if !owner {
		p.NonConvertedPoints = 0
	}
	return p, nil
}

// UpdateNickname changes a player's nickname. The nickname must be unique.
func (m *Manager) UpdateNickname(ctx context.Context, accountID, nickname string) error {
	if err := ValidateNickname(nickname); err != nil {
		return err
	}
	_, err := m.db.ExecContext(ctx,
		"UPDATE accounts SET nickname = $1, updated_at = now() WHERE id = $2",
		nickname, accountID,
	)
	if err != nil {
		return fmt.Errorf("update nickname: %w", err)
	}
	return nil
}

// UpdateAvatar selects only a curated preset and invalidates every earlier
// in-flight upload, including a repeated selection of the same preset.
func (m *Manager) UpdateAvatar(ctx context.Context, accountID, avatar string) error {
	return m.UpdateAvatarAuthorized(ctx, accountID, avatar, nil)
}
func (m *Manager) UpdateAvatarAuthorized(ctx context.Context, accountID, avatar string, authorize func(context.Context, *sql.Tx) error) error {
	allowed := false
	for _, preset := range AvatarPresets {
		if avatar == preset {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("avatar.invalid")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, accountID).Scan(&id); err != nil {
		return err
	}
	if authorize != nil {
		if err = authorize(ctx, tx); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE accounts SET avatar=$2,avatar_revision=avatar_revision+1,updated_at=clock_timestamp() WHERE id=$1 AND deleted_at IS NULL AND banned_at IS NULL AND (suspended_until IS NULL OR suspended_until<=clock_timestamp())`, accountID, avatar)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return fmt.Errorf("avatar.account_unavailable")
	}
	if authorize != nil {
		if err = authorize(ctx, tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AvatarPresets is the free curated avatar gallery.
var AvatarPresets = []string{
	"default", "nower", "donower", "detective", "party",
}

// ValidateNickname checks a nickname against length and basic profanity rules.
func ValidateNickname(nickname string) error {
	if len(nickname) < 2 || len(nickname) > 20 {
		return fmt.Errorf("nickname must be 2-20 characters")
	}
	if containsProfanity(nickname) {
		return fmt.Errorf("nickname contains disallowed language")
	}
	return nil
}

// AddContributorCreditTx appends a credit within the accepted-contribution transaction.
func (m *Manager) AddContributorCreditTx(ctx context.Context, tx *sql.Tx, accountID, credit string) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE profiles SET
		 contributor_credits = array_append(contributor_credits, $2),
		 updated_at = now()
		 WHERE account_id = $1`,
		accountID, credit,
	)
	if err != nil {
		return fmt.Errorf("add contributor credit: %w", err)
	}
	return nil
}

// AddWeekWinnerTitleTx increments the lifetime title count within the canonical weekly close transaction.
func (m *Manager) AddWeekWinnerTitleTx(ctx context.Context, tx *sql.Tx, accountID string) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE profiles SET
		 week_winner_titles = week_winner_titles + 1,
		 updated_at = now()
		 WHERE account_id = $1`,
		accountID,
	)
	if err != nil {
		return fmt.Errorf("add week winner title: %w", err)
	}
	return nil
}

var profanityList = []string{
	"fuck", "shit", "bitch", "asshole", "cunt", "damn", "dick", "pussy",
}

// normalizeNickname strips separators and lowercases for filtering.
func normalizeNickname(n string) string {
	var b strings.Builder
	for _, r := range n {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// containsProfanity reports whether the normalized nickname contains a
// blocked substring. This is a conservative baseline filter; store review
// may require a stronger service.
func containsProfanity(nickname string) bool {
	n := normalizeNickname(nickname)
	for _, w := range profanityList {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}
