package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var errBootstrapChanged = errors.New("installation mapping changed")

func validInstallation(hash string) bool {
	if hash == "" || len(hash) > 256 || !utf8.ValidString(hash) || strings.HasPrefix(hash, developmentDevicePrefix) {
		return false
	}
	for _, r := range hash {
		if r <= 32 || r == 127 {
			return false
		}
	}
	return true
}

// Account locks always precede installation locks. A mapping race rolls back
// the candidate transaction before acquiring the winning account's row.
func lockInstallation(ctx context.Context, tx *sql.Tx, hash string) error {
	if !validInstallation(hash) {
		return fmt.Errorf("installation required")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO auth_installations(device_hash) VALUES($1) ON CONFLICT DO NOTHING`, hash); err != nil {
		return err
	}
	var got string
	return tx.QueryRowContext(ctx, `SELECT device_hash FROM auth_installations WHERE device_hash=$1 FOR UPDATE`, hash).Scan(&got)
}

func installationAllowed(ctx context.Context, q accountSessionQuerier, hash, purpose string) error {
	if purpose == "development" {
		return nil
	}
	if !validInstallation(hash) {
		return fmt.Errorf("installation binding required")
	}
	var blocked bool
	if err := q.QueryRowContext(ctx, `SELECT installation_sanction_active($1)`, hash).Scan(&blocked); err != nil {
		return err
	}
	if blocked {
		return fmt.Errorf("installation unavailable")
	}
	return nil
}

func linkInstallationTx(ctx context.Context, tx *sql.Tx, account, hash, purpose string) error {
	if purpose == "development" {
		return nil
	}
	if err := lockInstallation(ctx, tx, hash); err != nil {
		return err
	}
	if err := installationAllowed(ctx, tx, hash, purpose); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2) ON CONFLICT(account_id,device_hash) DO UPDATE SET last_seen_at=clock_timestamp()`, account, hash)
	return err
}

func bootstrapAccount(ctx context.Context, q accountSessionQuerier, hash string) (string, error) {
	var canonical string
	err := q.QueryRowContext(ctx, `SELECT account_id FROM auth_installation_bootstrap WHERE device_hash=$1`, hash).Scan(&canonical)
	if err == nil {
		return canonical, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var count int
	var legacy sql.NullString
	err = q.QueryRowContext(ctx, `SELECT count(*),min(account_id::text) FROM (SELECT DISTINCT account_id FROM device_tokens WHERE device_hash=$1 LIMIT 2) owners`, hash).Scan(&count, &legacy)
	if err != nil {
		return "", err
	}
	if count > 1 {
		return "", fmt.Errorf("ambiguous legacy installation")
	}
	return legacy.String, nil
}

func (m *Manager) authenticateInstallation(ctx context.Context, hash string) (*TokenPair, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if !validInstallation(hash) {
		return nil, fmt.Errorf("device_hash required")
	}
	for attempt := 0; attempt < 8; attempt++ {
		known, err := bootstrapAccount(ctx, m.db, hash)
		if err != nil {
			return nil, err
		}
		pair, err := m.bootstrapInstallationTx(ctx, hash, known)
		if errors.Is(err, errBootstrapChanged) {
			continue
		}
		return pair, err
	}
	return nil, fmt.Errorf("installation busy")
}

func (m *Manager) bootstrapInstallationTx(ctx context.Context, hash, known string) (*TokenPair, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	account := known
	if account == "" {
		account = uuid.NewString()
		if _, err = tx.ExecContext(ctx, `INSERT INTO accounts(id,nickname) VALUES($1,$2)`, account, randomNickname()); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO profiles(account_id) VALUES($1)`, account); err != nil {
			return nil, err
		}
	}
	purpose, epoch, err := m.accountSession(ctx, tx, account, true)
	if err != nil {
		return nil, err
	}
	if err = lockInstallation(ctx, tx, hash); err != nil {
		return nil, err
	}
	current, err := bootstrapAccount(ctx, tx, hash)
	if err != nil {
		return nil, err
	}
	if current != known {
		return nil, errBootstrapChanged
	}
	if err = installationAllowed(ctx, tx, hash, purpose); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO auth_installation_bootstrap(device_hash,account_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, hash, account); err != nil {
		return nil, err
	}
	if err = linkInstallationTx(ctx, tx, account, hash, purpose); err != nil {
		return nil, err
	}
	pair, err := m.signTokens(account, hash, purpose, epoch)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return pair, nil
}

// BindInstallation upgrades a legacy unbound refresh credential in place. Its
// retained issuance receipt recovers a lost committed response without creating
// another account, and cannot be replayed with a different installation.
func (m *Manager) BindInstallation(ctx context.Context, refreshToken, hash string) (*TokenPair, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	claims, err := m.parseToken(refreshToken, TokenRefresh)
	if err != nil {
		return nil, err
	}
	if claims.Purpose != "player" || !validInstallation(hash) || claims.DeviceHash != "" && claims.DeviceHash != hash {
		return nil, fmt.Errorf("invalid installation binding")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	purpose, epoch, err := m.accountSession(ctx, tx, claims.AccountID, true)
	if err != nil || purpose != claims.Purpose || epoch != claims.SessionEpoch {
		return nil, fmt.Errorf("account unavailable")
	}
	if err = linkInstallationTx(ctx, tx, claims.AccountID, hash, purpose); err != nil {
		return nil, err
	}
	var storedAccount, storedHash, configHash, accessID, refreshID string
	var storedEpoch int64
	var at time.Time
	err = tx.QueryRowContext(ctx, `SELECT account_id,device_hash,session_epoch,issued_at,access_id,refresh_id,issuance_config_hash FROM auth_installation_rotations WHERE old_refresh_id=$1`, claims.ID).Scan(&storedAccount, &storedHash, &storedEpoch, &at, &accessID, &refreshID, &configHash)
	if errors.Is(err, sql.ErrNoRows) {
		result, e := tx.ExecContext(ctx, `INSERT INTO auth_revocations(token_id,expires_at) VALUES($1,$2) ON CONFLICT DO NOTHING`, claims.ID, claims.ExpiresAt.Time)
		if e != nil {
			return nil, e
		}
		if n, e := result.RowsAffected(); e != nil || n != 1 {
			return nil, fmt.Errorf("token revoked")
		}
		at = time.Now().UTC().Truncate(time.Second)
		accessID = uuid.NewString()
		refreshID = uuid.NewString()
		configHash = m.oauthIssuanceConfigHash()
		_, err = tx.ExecContext(ctx, `INSERT INTO auth_installation_rotations(old_refresh_id,account_id,device_hash,session_epoch,issued_at,access_id,refresh_id,issuance_config_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, claims.ID, claims.AccountID, hash, epoch, at, accessID, refreshID, configHash)
	} else if err == nil && (storedAccount != claims.AccountID || storedHash != hash || storedEpoch != epoch || configHash != m.oauthIssuanceConfigHash()) {
		return nil, fmt.Errorf("binding conflict")
	}
	if err != nil {
		return nil, err
	}
	var revoked bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auth_revocations WHERE token_id IN ($1,$2))`, accessID, refreshID).Scan(&revoked); err != nil {
		return nil, err
	}
	if revoked || !at.Add(m.refreshTTL).After(time.Now()) {
		return nil, fmt.Errorf("binding receipt closed")
	}
	if _, err := m.parseToken(refreshToken, TokenRefresh); err != nil {
		return nil, err
	}
	pair, err := m.signTokensAt(claims.AccountID, hash, purpose, epoch, at, accessID, refreshID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return pair, nil
}

// AccessBinding contains only verified signed identity, never client assertions.
type AccessBinding struct {
	AccountID, DeviceHash, TokenID, Purpose string
	ExpiresAt                               time.Time
	SessionEpoch                            int64
}

func (m *Manager) ValidateAccessBinding(ctx context.Context, token string) (AccessBinding, error) {
	account, err := m.ValidateAccessToken(ctx, token)
	if err != nil {
		return AccessBinding{}, err
	}
	claims, err := m.parseToken(token, TokenAccess)
	if err != nil {
		return AccessBinding{}, err
	}
	return AccessBinding{AccountID: account, DeviceHash: claims.DeviceHash, SessionEpoch: claims.SessionEpoch, TokenID: claims.ID, Purpose: claims.Purpose, ExpiresAt: claims.ExpiresAt.Time}, nil
}

func (m *Manager) ValidateAccessBindingTx(ctx context.Context, tx *sql.Tx, token string) (AccessBinding, error) {
	account, err := m.ValidateAccessTokenTx(ctx, tx, token)
	if err != nil {
		return AccessBinding{}, err
	}
	claims, err := m.parseToken(token, TokenAccess)
	if err != nil {
		return AccessBinding{}, err
	}
	return AccessBinding{AccountID: account, DeviceHash: claims.DeviceHash, SessionEpoch: claims.SessionEpoch, TokenID: claims.ID, Purpose: claims.Purpose, ExpiresAt: claims.ExpiresAt.Time}, nil
}
