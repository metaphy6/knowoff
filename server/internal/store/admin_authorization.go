package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
)

var ErrAdminRequired = errors.New("admin_required")

type adminAuthorizationKey struct{}
type adminAuthorization struct {
	actor     string
	authorize func(context.Context, *sql.Tx) (string, error)
}

// WithAdminAuthorization binds an initiating browser's exact session to its
// expected actor. The callback must perform SQL checks only on the supplied
// transaction, without remote work or acquiring other accounts after role locks.
// A missing binding is reserved for trusted internal callers; they still pass
// canonical account/status/role checks in LockAdminTx.
func WithAdminAuthorization(ctx context.Context, actor string, authorize func(context.Context, *sql.Tx) (string, error)) context.Context {
	return context.WithValue(ctx, adminAuthorizationKey{}, adminAuthorization{actor: actor, authorize: authorize})
}

// HasAdminAuthorization distinguishes a browser-bound request from an explicit
// trusted system call. Presence never grants authority: even an invalid or empty
// binding returns true and must be checked by LockAdminTx.
func HasAdminAuthorization(ctx context.Context) bool {
	_, bound := ctx.Value(adminAuthorizationKey{}).(adminAuthorization)
	return bound
}

// LockAdminTx serializes an admin mutation with account, role and initiating
// session revocation. Supply all target accounts before taking dependent locks.
// Existing community crown/topic locks precede this function. Call it again
// after later domain-lock waits, before returning success or committing effects,
// to reject a session that naturally expired during those waits. That recheck
// must use only accounts already locked by the initial call.
func LockAdminTx(ctx context.Context, tx *sql.Tx, actor string, allowedRoles []string, targets ...string) error {
	proof, bound := ctx.Value(adminAuthorizationKey{}).(adminAuthorization)
	if tx == nil || !valueUUID(actor) || len(allowedRoles) == 0 || bound && (proof.actor != actor || proof.authorize == nil) {
		return ErrAdminRequired
	}
	for _, id := range targets {
		if !valueUUID(id) {
			return ErrAdminRequired
		}
	}
	var account string
	if err := tx.QueryRowContext(ctx, `SELECT account_id FROM admin_accounts WHERE id=$1`, actor).Scan(&account); err != nil {
		return err
	}
	accounts := append(append([]string(nil), targets...), account)
	slices.Sort(accounts)
	accounts = slices.Compact(accounts)
	for _, id := range accounts {
		if err := LockValueAccount(ctx, tx, id); err != nil {
			return err
		}
	}
	var lockedAccount, role string
	if err := tx.QueryRowContext(ctx, `SELECT account_id,role FROM admin_accounts WHERE id=$1 FOR SHARE`, actor).Scan(&lockedAccount, &role); err != nil {
		return err
	}
	if lockedAccount != account || !slices.Contains(allowedRoles, role) {
		return ErrAdminRequired
	}
	if bound {
		verified, err := proof.authorize(ctx, tx)
		if err != nil {
			return err
		}
		if verified != actor {
			return ErrAdminRequired
		}
	}
	// Keep wall-clock status validation after every authority lock wait, including
	// the exact-session callback. Guard freezes deliberately do not revoke admin.
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT deleted_at IS NULL AND banned_at IS NULL AND (suspended_until IS NULL OR suspended_until<=clock_timestamp()) FROM accounts WHERE id=$1`, account).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrAdminRequired
	}
	return nil
}
