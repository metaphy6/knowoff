package portal

import (
	"context"
	"database/sql"
	"errors"
)

type portalCredentialKey struct{}
type portalCredential struct {
	account, sessionHash, csrf string
	bearer                     []string
	auth                       AuthClient
}

// Called after every participating account is locked. Registry installation
// locks follow accounts and precede derived credential rows.
func lockPortalInstallation(ctx context.Context, tx *sql.Tx, hash string) error {
	if hash == "" {
		return errors.New("installation unavailable")
	}
	var found string
	if err := tx.QueryRowContext(ctx, `SELECT device_hash FROM auth_installations WHERE device_hash=$1 FOR UPDATE`, hash).Scan(&found); err != nil {
		return err
	}
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT installation_sanction_active($1)`, hash).Scan(&active); err != nil {
		return err
	}
	if active {
		return errors.New("installation unavailable")
	}
	return nil
}

func checkPortalCredentialTx(ctx context.Context, tx *sql.Tx, account string) error {
	proof, present := ctx.Value(portalCredentialKey{}).(portalCredential)
	if !present {
		return nil
	}
	if proof.account != account {
		return errors.New("portal credential unavailable")
	}
	denied := errors.New("portal credential unavailable")
	if len(proof.bearer) == 2 && proof.auth != nil {
		verified, err := proof.auth.ValidateAccessTokenTx(ctx, tx, proof.bearer[1])
		if err != nil || verified != account {
			return denied
		}
		return nil
	}
	var installation string
	if err := tx.QueryRowContext(ctx, `SELECT device_hash FROM portal_browser_sessions WHERE token_hash=$1 AND account_id=$2`, proof.sessionHash, account).Scan(&installation); err != nil {
		return denied
	}
	if err := lockPortalInstallation(ctx, tx, installation); err != nil {
		return denied
	}
	var csrf string
	if err := tx.QueryRowContext(ctx, `SELECT csrf_token FROM portal_browser_sessions WHERE token_hash=$1 AND account_id=$2 FOR SHARE`, proof.sessionHash, account).Scan(&csrf); err != nil || !validPortalCSRF(csrf, proof.csrf) {
		return denied
	}
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() AND device_hash=$3 AND NOT installation_sanction_active(device_hash) FROM portal_browser_sessions WHERE token_hash=$1 AND account_id=$2`, proof.sessionHash, account, installation).Scan(&allowed); err != nil || !allowed {
		return denied
	}
	return nil
}
