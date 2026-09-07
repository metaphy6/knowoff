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

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/lib/pq"
)

// Profile is the public + owner-only view of a player.
type Profile struct {
	AccountID          string    `json:"account_id"`
	Nickname           string    `json:"nickname"`
	Avatar             string    `json:"avatar"`
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
	WeeklyPodiums      int       `json:"weekly_podiums"`
	ContributorCredits []string  `json:"contributor_credits"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Manager is the profile service.
type Manager struct {
	db   *sql.DB
	prog config.ProgressionTuning
}

// NewManager creates a profile manager.
func NewManager(db *sql.DB, prog config.ProgressionTuning) *Manager {
	return &Manager{db: db, prog: prog}
}

// Get returns a profile. If owner is false, NonConvertedPoints is omitted.
func (m *Manager) Get(ctx context.Context, accountID string, owner bool) (*Profile, error) {
	row := m.db.QueryRowContext(ctx, `
		SELECT a.id, a.nickname, a.avatar,
		       p.level, p.xp, p.overall_points, p.non_converted_points,
		       p.matches_played, p.matches_won_nower, p.matches_won_donower,
		       p.correct_votes, p.votes_cast, p.donower_survivals, p.donower_matches,
		       p.pokes_sent, p.week_winner_titles, p.weekly_podiums,
		       p.contributor_credits, p.updated_at
		FROM accounts a JOIN profiles p ON a.id = p.account_id
		WHERE a.id = $1 AND a.deleted_at IS NULL`,
		accountID,
	)
	p := &Profile{}
	if err := row.Scan(
		&p.AccountID, &p.Nickname, &p.Avatar,
		&p.Level, &p.XP, &p.OverallPoints, &p.NonConvertedPoints,
		&p.MatchesPlayed, &p.MatchesWonNower, &p.MatchesWonDonower,
		&p.CorrectVotes, &p.VotesCast, &p.DonowerSurvivals, &p.DonowerMatches,
		&p.PokesSent, &p.WeekWinnerTitles, &p.WeeklyPodiums,
		pq.Array(&p.ContributorCredits), &p.UpdatedAt,
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

// UpdateAvatar changes the avatar reference.
func (m *Manager) UpdateAvatar(ctx context.Context, accountID, avatar string) error {
	_, err := m.db.ExecContext(ctx,
		"UPDATE accounts SET avatar = $1, updated_at = now() WHERE id = $2",
		avatar, accountID,
	)
	if err != nil {
		return fmt.Errorf("update avatar: %w", err)
	}
	return nil
}

// ApplyMatchResult updates profile stats from a finished match. This is called
// by the game loop; the nightly job reconciles from the audit stream.
func (m *Manager) ApplyMatchResult(ctx context.Context, accountID string, nower, won bool, correctVotes, pokes int, points int64) error {
	wonNower := nower && won
	wonDonower := !nower && won
	deltaXP := m.xpForMatch(won, correctVotes)
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin apply match result: %w", err)
	}
	defer tx.Rollback()

	level := 0
	if deltaXP > 0 {
		var currentXP int
		if err := tx.QueryRowContext(ctx,
			"SELECT xp FROM profiles WHERE account_id = $1 FOR UPDATE", accountID,
		).Scan(&currentXP); err != nil {
			return fmt.Errorf("load xp: %w", err)
		}
		level = levelForXP(m.prog.LevelThresholds, currentXP+deltaXP)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE profiles SET
			matches_played = matches_played + 1,
			matches_won_nower = matches_won_nower + $2,
			matches_won_donower = matches_won_donower + $3,
			correct_votes = correct_votes + $4,
			pokes_sent = pokes_sent + $5,
			donower_matches = donower_matches + $6,
			donower_survivals = donower_survivals + $7,
			overall_points = overall_points + $8,
			non_converted_points = non_converted_points + $8,
			xp = xp + $9,
			level = CASE WHEN $9 > 0 THEN $10 ELSE level END,
			updated_at = now()
		WHERE account_id = $1`,
		accountID,
		boolInt(wonNower),
		boolInt(wonDonower),
		correctVotes,
		pokes,
		boolInt(!nower),
		boolInt(!nower && won),
		points,
		deltaXP,
		level,
	)
	if err != nil {
		return fmt.Errorf("apply match result: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit apply match result: %w", err)
	}
	return nil
}

// ConvertPoints converts Non-Converted Points to Noin. It is one-way and atomic.
func (m *Manager) ConvertPoints(ctx context.Context, accountID string, points int64) (int64, error) {
	if points <= 0 {
		return 0, fmt.Errorf("points must be positive")
	}
	if points%100 != 0 {
		return 0, fmt.Errorf("points must be a multiple of 100")
	}
	noin := points / 100
	res, err := m.db.ExecContext(ctx, `
		UPDATE profiles SET
			non_converted_points = non_converted_points - $2,
			updated_at = now()
		WHERE account_id = $1 AND non_converted_points >= $2`,
		accountID, points,
	)
	if err != nil {
		return 0, fmt.Errorf("convert points: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, fmt.Errorf("insufficient non-converted points")
	}
	return noin, nil
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

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (m *Manager) xpForMatch(won bool, correctVotes int) int {
	xp := m.prog.XPBase + correctVotes*m.prog.XPPerCorrectVote
	if won {
		xp += m.prog.XPWinBonus
	}
	return xp
}

func levelForXP(thresholds []int, xp int) int {
	level := 1
	for i, th := range thresholds {
		if xp >= th {
			level = i + 1
		}
	}
	return level
}

// AddContributorCredit appends a credit to the profile's contributor_credits array.
func (m *Manager) AddContributorCredit(ctx context.Context, accountID, credit string) error {
	_, err := m.db.ExecContext(ctx,
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

// AddContributorCreditTx is the transaction-scoped variant of AddContributorCredit.
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

// AddWeekWinnerTitle increments the week_winner_titles counter on a profile.
func (m *Manager) AddWeekWinnerTitle(ctx context.Context, accountID string) error {
	_, err := m.db.ExecContext(ctx,
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

// AddWeekWinnerTitleTx is the transaction-scoped variant of AddWeekWinnerTitle.
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

// EnsureProfile creates an account and profile if missing. Used in tests and
// migration backfills.
func (m *Manager) EnsureProfile(ctx context.Context, accountID, nickname string) error {
	if _, err := uuid.Parse(accountID); err != nil {
		return fmt.Errorf("invalid account id: %w", err)
	}
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO accounts (id, nickname) VALUES ($1, $2)
		 ON CONFLICT (id) DO UPDATE SET updated_at = now()`,
		accountID, nickname,
	)
	if err != nil {
		return fmt.Errorf("ensure account: %w", err)
	}
	_, err = m.db.ExecContext(ctx,
		`INSERT INTO profiles (account_id) VALUES ($1)
		 ON CONFLICT (account_id) DO NOTHING`,
		accountID,
	)
	if err != nil {
		return fmt.Errorf("ensure profile: %w", err)
	}
	return nil
}
