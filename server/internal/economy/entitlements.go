package economy

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

// EntitlementType identifies an active entitlement.
type EntitlementType string

const (
	EntitlementPlayPass1D     EntitlementType = "play_pass_1d"
	EntitlementPlayPass3D     EntitlementType = "play_pass_3d"
	EntitlementPlayPass7D     EntitlementType = "play_pass_7d"
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
		return time.Time{}, err
	}
	duration := playPassDuration(t)
	if duration == 0 || price <= 0 {
		return time.Time{}, fmt.Errorf("invalid play pass purchase")
	}
	now := time.Now().UTC()
	until := now.Add(duration)
	err := store.WithValueTransaction(ctx, e.db, func(tx *sql.Tx) error {
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return err
		}
		if err := debitTx(ctx, tx, accountID, price, fmt.Sprintf("purchase %s", t), serverDay(now)); err != nil {
			return err
		}
		return grantTimedEntitlement(ctx, tx, accountID, t, until)
	})
	if err != nil {
		return time.Time{}, err
	}
	return until, nil
}

// GrantUnlock atomically purchases a permanent cosmetic entitlement.
func (e *Entitlements) GrantUnlock(ctx context.Context, accountID string, t EntitlementType, value string, price int) error {
	if _, err := uuid.Parse(accountID); err != nil {
		return err
	}
	if price <= 0 || (t != EntitlementCustomAvatar && t != EntitlementPokeStyle && t != EntitlementThemePack) {
		return fmt.Errorf("invalid unlock purchase")
	}
	day := serverDay(time.Now().UTC())
	return store.WithValueTransaction(ctx, e.db, func(tx *sql.Tx) error {
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entitlements WHERE account_id=$1 AND entitlement_type=$2)`, accountID, string(t)).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("already unlocked")
		}
		if err := debitTx(ctx, tx, accountID, price, fmt.Sprintf("unlock %s", t), day); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,$2,$3)`, accountID, string(t), value)
		return err
	})
}

// GrantPremium accepts only a server-verified subscription expiry.
func (e *Entitlements) GrantPremium(ctx context.Context, accountID string, t EntitlementType, activeUntil time.Time) error {
	if t != EntitlementPremiumMonthly && t != EntitlementPremiumYearly {
		return fmt.Errorf("invalid premium type")
	}
	if !activeUntil.After(time.Now().UTC()) {
		return fmt.Errorf("expired premium")
	}
	return store.WithValueTransaction(ctx, e.db, func(tx *sql.Tx) error {
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return err
		}
		return grantTimedEntitlement(ctx, tx, accountID, t, activeUntil)
	})
}
func grantTimedEntitlement(ctx context.Context, tx *sql.Tx, accountID string, t EntitlementType, until time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO entitlements(account_id,entitlement_type,value,active_until) VALUES($1,$2,$2,$3)
 ON CONFLICT(account_id,entitlement_type) DO UPDATE SET value=EXCLUDED.value,active_until=GREATEST(entitlements.active_until,EXCLUDED.active_until),updated_at=now()`, accountID, string(t), until)
	return err
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
