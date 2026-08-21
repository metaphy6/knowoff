package economy

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EntitlementType identifies an active entitlement.
type EntitlementType string

const (
	EntitlementPlayPass1D    EntitlementType = "play_pass_1d"
	EntitlementPlayPass3D    EntitlementType = "play_pass_3d"
	EntitlementPlayPass7D    EntitlementType = "play_pass_7d"
	EntitlementPremiumMonthly EntitlementType = "premium_monthly"
	EntitlementPremiumYearly  EntitlementType = "premium_yearly"
	EntitlementCustomAvatar   EntitlementType = "custom_avatar"
	EntitlementPokeStyle      EntitlementType = "poke_style"
	EntitlementThemePack      EntitlementType = "theme_pack"
)

// Entitlement represents a single active grant or unlock.
type Entitlement struct {
	Type        EntitlementType
	Value       string
	ActiveUntil *time.Time
}

// Entitlements owns the entitlements table and unlock checks.
type Entitlements struct {
	db *sql.DB
}

// NewEntitlements returns an entitlements manager.
func NewEntitlements(db *sql.DB) *Entitlements {
	return &Entitlements{db: db}
}

// Has returns true if the account has an active entitlement of the given type.
// For time-bounded entitlements, active_until must be in the future.
func (e *Entitlements) Has(ctx context.Context, accountID string, t EntitlementType) (bool, error) {
	var activeUntil sql.NullTime
	err := e.db.QueryRowContext(ctx,
		"SELECT active_until FROM entitlements WHERE account_id = $1 AND entitlement_type = $2",
		accountID, string(t),
	).Scan(&activeUntil)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load entitlement: %w", err)
	}
	if !activeUntil.Valid {
		// Permanent unlock (e.g. custom_avatar).
		return true, nil
	}
	return activeUntil.Time.After(time.Now().UTC()), nil
}

// HasPremium returns true if the account has an active Premium subscription.
func (e *Entitlements) HasPremium(ctx context.Context, accountID string) (bool, error) {
	for _, t := range []EntitlementType{EntitlementPremiumMonthly, EntitlementPremiumYearly} {
		ok, err := e.Has(ctx, accountID, t)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// HasAnyPlayPass returns true if the account has an active Play Pass.
func (e *Entitlements) HasAnyPlayPass(ctx context.Context, accountID string) (bool, error) {
	for _, t := range []EntitlementType{EntitlementPlayPass1D, EntitlementPlayPass3D, EntitlementPlayPass7D} {
		ok, err := e.Has(ctx, accountID, t)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// List returns all active entitlements for an account.
func (e *Entitlements) List(ctx context.Context, accountID string) ([]Entitlement, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT entitlement_type, value, active_until
		 FROM entitlements
		 WHERE account_id = $1 AND (active_until IS NULL OR active_until > now())`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list entitlements: %w", err)
	}
	defer rows.Close()

	var out []Entitlement
	for rows.Next() {
		var t string
		var value sql.NullString
		var activeUntil sql.NullTime
		if err := rows.Scan(&t, &value, &activeUntil); err != nil {
			return nil, fmt.Errorf("scan entitlement: %w", err)
		}
		ent := Entitlement{Type: EntitlementType(t)}
		if value.Valid {
			ent.Value = value.String
		}
		if activeUntil.Valid {
			ent.ActiveUntil = &activeUntil.Time
		}
		out = append(out, ent)
	}
	return out, rows.Err()
}

// GrantPlayPass grants a time-bounded Play Pass by debiting Noin and inserting
// the entitlement. It returns the expiry time.
func (e *Entitlements) GrantPlayPass(ctx context.Context, accountID string, t EntitlementType, price int) (time.Time, error) {
	if _, err := uuid.Parse(accountID); err != nil {
		return time.Time{}, fmt.Errorf("invalid account id: %w", err)
	}

	duration := playPassDuration(t)
	if duration == 0 {
		return time.Time{}, fmt.Errorf("invalid play pass type")
	}

	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return time.Time{}, fmt.Errorf("begin play pass tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE noin_wallets SET balance = balance - $2, updated_at = now()
		 WHERE account_id = $1 AND balance >= $2`,
		accountID, price,
	)
	if err != nil {
		return time.Time{}, fmt.Errorf("debit wallet: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return time.Time{}, fmt.Errorf("insufficient noin")
	}

	day := serverDay(time.Now().UTC())
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO noin_ledger (account_id, event_type, amount, reason, server_day)
		 VALUES ($1, $2, $3, $4, $5)`,
		accountID, string(LedgerSpend), -price, fmt.Sprintf("purchase %s", t), day,
	); err != nil {
		return time.Time{}, fmt.Errorf("insert spend ledger: %w", err)
	}

	activeUntil := time.Now().UTC().Add(duration)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO entitlements (account_id, entitlement_type, value, active_until, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, now(), now())
		 ON CONFLICT (account_id, entitlement_type) DO UPDATE SET
		   value = EXCLUDED.value,
		   active_until = GREATEST(entitlements.active_until, EXCLUDED.active_until),
		   updated_at = now()`,
		accountID, string(t), string(t), activeUntil,
	); err != nil {
		return time.Time{}, fmt.Errorf("insert entitlement: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return time.Time{}, fmt.Errorf("commit play pass: %w", err)
	}
	return activeUntil, nil
}

// GrantUnlock grants a permanent unlock (custom_avatar, poke_style, theme_pack)
// by debiting Noin. It fails if the entitlement already exists.
func (e *Entitlements) GrantUnlock(ctx context.Context, accountID string, t EntitlementType, value string, price int) error {
	if _, err := uuid.Parse(accountID); err != nil {
		return fmt.Errorf("invalid account id: %w", err)
	}

	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("begin unlock tx: %w", err)
	}
	defer tx.Rollback()

	var existing bool
	if err := tx.QueryRowContext(ctx,
		"SELECT true FROM entitlements WHERE account_id = $1 AND entitlement_type = $2",
		accountID, string(t),
	).Scan(&existing); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("check existing entitlement: %w", err)
	}
	if existing {
		return fmt.Errorf("already unlocked")
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE noin_wallets SET balance = balance - $2, updated_at = now()
		 WHERE account_id = $1 AND balance >= $2`,
		accountID, price,
	)
	if err != nil {
		return fmt.Errorf("debit wallet: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("insufficient noin")
	}

	day := serverDay(time.Now().UTC())
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO noin_ledger (account_id, event_type, amount, reason, server_day)
		 VALUES ($1, $2, $3, $4, $5)`,
		accountID, string(LedgerSpend), -price, fmt.Sprintf("unlock %s", t), day,
	); err != nil {
		return fmt.Errorf("insert spend ledger: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO entitlements (account_id, entitlement_type, value, active_until, created_at, updated_at)
		 VALUES ($1, $2, $3, NULL, now(), now())`,
		accountID, string(t), value,
	); err != nil {
		return fmt.Errorf("insert entitlement: %w", err)
	}

	return tx.Commit()
}

// GrantPremium records an active Premium subscription. It is used after a
// platform billing receipt has been verified.
func (e *Entitlements) GrantPremium(ctx context.Context, accountID string, t EntitlementType, activeUntil time.Time) error {
	if t != EntitlementPremiumMonthly && t != EntitlementPremiumYearly {
		return fmt.Errorf("invalid premium type")
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin premium tx: %w", err)
	}
	defer tx.Rollback()

	// A newer premium entitlement always wins; overlapping passes are fine.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM entitlements
		 WHERE account_id = $1 AND entitlement_type IN ('premium_monthly','premium_yearly')
		   AND COALESCE(active_until, 'epoch') < now()`,
		accountID,
	); err != nil {
		return fmt.Errorf("clean expired premium: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO entitlements (account_id, entitlement_type, value, active_until, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, now(), now())
		 ON CONFLICT (account_id, entitlement_type) DO UPDATE SET
		   value = EXCLUDED.value,
		   active_until = GREATEST(entitlements.active_until, EXCLUDED.active_until),
		   updated_at = now()`,
		accountID, string(t), string(t), activeUntil,
	); err != nil {
		return fmt.Errorf("insert premium entitlement: %w", err)
	}

	return tx.Commit()
}

func playPassDuration(t EntitlementType) time.Duration {
	switch t {
	case EntitlementPlayPass1D:
		return 24 * time.Hour
	case EntitlementPlayPass3D:
		return 72 * time.Hour
	case EntitlementPlayPass7D:
		return 168 * time.Hour
	default:
		return 0
	}
}
