package economy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
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
	var active bool
	err := e.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM entitlements WHERE account_id=$1 AND entitlement_type=$2 AND (active_until IS NULL OR active_until>now()))
		 OR EXISTS(SELECT 1 FROM named_entitlement_items WHERE account_id=$1 AND entitlement_type=$2)`,
		accountID, string(t),
	).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("load entitlement: %w", err)
	}
	return active, nil
}

// HasValue checks one named benefit. Legacy rows remain readable without being
// overwritten when the account acquires another theme or poke style.
func (e *Entitlements) HasValue(ctx context.Context, accountID string, t EntitlementType, value string) (bool, error) {
	if (t != EntitlementThemePack && t != EntitlementPokeStyle) || !gamecontract.ValidIdentifier(value) {
		return false, fmt.Errorf("invalid named entitlement")
	}
	var active bool
	err := e.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entitlements WHERE account_id=$1 AND entitlement_type=$2 AND value=$3 AND (active_until IS NULL OR active_until>now()))
	 OR EXISTS(SELECT 1 FROM named_entitlement_items WHERE account_id=$1 AND entitlement_type=$2 AND value=$3)`, accountID, string(t), value).Scan(&active)
	return active, err
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
		 WHERE account_id = $1 AND (active_until IS NULL OR active_until > now())
		 UNION SELECT entitlement_type,value,NULL FROM named_entitlement_items WHERE account_id=$1
		 ORDER BY entitlement_type,value`,
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
	named := t == EntitlementPokeStyle || t == EntitlementThemePack
	if named && !gamecontract.ValidIdentifier(value) {
		return fmt.Errorf("invalid named entitlement")
	}
	day := serverDay(time.Now().UTC())
	return store.WithValueTransaction(ctx, e.db, func(tx *sql.Tx) error {
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return err
		}
		var exists bool
		if named {
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entitlements WHERE account_id=$1 AND entitlement_type=$2 AND value=$3 AND (active_until IS NULL OR active_until>now()))
			 OR EXISTS(SELECT 1 FROM named_entitlement_items WHERE account_id=$1 AND entitlement_type=$2 AND value=$3)`, accountID, string(t), value).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return nil
			}
			if err := debitTx(ctx, tx, accountID, price, fmt.Sprintf("unlock %s", t), day); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO named_entitlement_items(account_id,entitlement_type,value,source_id) VALUES($1,$2,$3,$4)`, accountID, string(t), value, uuid.NewString())
			return err
		}
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

var ErrAvatarPurchaseUnavailable = errors.New("store.catalog_unavailable")
var ErrAvatarPurchaseAuth = errors.New("auth.required")

// PurchaseCustomAvatar is the interactive paid upload unlock. The SQL-only
// credential callback runs under the account lock and again after value writes.
// Permanent ownership is idempotent even while the provider is unavailable.
func (e *Entitlements) PurchaseCustomAvatar(ctx context.Context, account string, price int, available bool, authorize func(context.Context, *sql.Tx) error) error {
	if authorize == nil {
		return ErrAvatarPurchaseAuth
	}
	return store.WithValueTransaction(ctx, e.db, func(tx *sql.Tx) error {
		if err := store.LockValueAccount(ctx, tx, account); err != nil {
			return err
		}
		if err := authorize(ctx, tx); err != nil {
			return ErrAvatarPurchaseAuth
		}
		var owned bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entitlements WHERE account_id=$1 AND entitlement_type='custom_avatar' AND active_until IS NULL)`, account).Scan(&owned); err != nil {
			return err
		}
		if owned {
			return nil
		}
		if !available || price <= 0 {
			return ErrAvatarPurchaseUnavailable
		}
		if err := debitTx(ctx, tx, account, price, "unlock custom_avatar", serverDay(time.Now().UTC())); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'custom_avatar','')`, account); err != nil {
			return err
		}
		if err := authorize(ctx, tx); err != nil {
			return ErrAvatarPurchaseAuth
		}
		return nil
	})
}
