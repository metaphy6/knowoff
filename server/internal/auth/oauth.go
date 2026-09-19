package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

var (
	ErrOAuthInvalid     = errors.New("oauth.invalid")
	ErrOAuthPending     = errors.New("oauth.pending")
	ErrOAuthConflict    = errors.New("oauth.conflict")
	ErrOAuthUnlinked    = errors.New("oauth.restore_unlinked")
	ErrOAuthUnavailable = errors.New("oauth.provider_unavailable")
	ErrOAuthRateLimited = errors.New("oauth.rate_limited")
)

// Linking requires the current player credential. Restoration is an explicit
// separate intent: an invalid or expired credential never falls back to it.
type OAuthStartRequest struct {
	DeviceHash  string `json:"device_hash"`
	Provider    string `json:"provider"`
	Intent      string `json:"intent"`
	AccessToken string `json:"-"`
	Principal   string `json:"-"`
}
type OAuthStart struct {
	URL              string    `json:"url"`
	FlowID           string    `json:"flow_id"`
	CompletionSecret string    `json:"completion_secret"`
	ExpiresAt        time.Time `json:"expires_at"`
}

var graphVersionPattern = regexp.MustCompile(`^v[1-9][0-9]?\.[0-9]{1,2}$`)

func configuredOAuth(c OAuthProviderConfig, provider string) bool {
	u, err := url.Parse(c.RedirectURL)
	return err == nil && c.ClientID != "" && c.ClientSecret != "" && len(c.ClientID) <= 512 && len(c.ClientSecret) <= 4096 && len(c.RedirectURL) <= 2048 && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == "" && u.Path == "/api/auth/oauth/callback" && len(u.Query()) == 1 && len(u.Query()["provider"]) == 1 && u.Query().Get("provider") == provider && (provider != "facebook" || graphVersionPattern.MatchString(c.GraphVersion))
}

// BeginOAuth retains only hashes of browser/app proof secrets. The short-lived
// PKCE verifier stays private in Postgres until its one permitted exchange.
func (m *Manager) BeginOAuth(ctx context.Context, req OAuthStartRequest) (*OAuthStart, error) {
	cfg := m.oauthCfgs[req.Provider]
	if cfg == nil {
		return nil, ErrOAuthUnavailable
	}
	if !validInstallation(req.DeviceHash) || (req.Intent != "link" && req.Intent != "restore") || req.Principal == "" || len(req.Principal) > 256 || (req.Intent == "restore" && req.AccessToken != "") {
		return nil, ErrOAuthInvalid
	}
	state, err := randomCodeVerifier()
	if err != nil {
		return nil, err
	}
	secret, err := randomCodeVerifier()
	if err != nil {
		return nil, err
	}
	nonce, err := randomCodeVerifier()
	if err != nil {
		return nil, err
	}
	verifier, err := randomCodeVerifier()
	if err != nil {
		return nil, err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var account any
	var epoch any
	var initiatingToken any
	principal := "address:" + req.Principal
	if req.Intent == "link" {
		id, e := m.ValidateAccessTokenTx(ctx, tx, req.AccessToken)
		if e != nil {
			return nil, ErrOAuthInvalid
		}
		purpose, generation, e := m.accountSession(ctx, tx, id, false)
		if e != nil || purpose != "player" {
			return nil, ErrOAuthInvalid
		}
		claims, e := m.parseToken(req.AccessToken, TokenAccess)
		if e != nil || claims.DeviceHash != req.DeviceHash {
			return nil, ErrOAuthInvalid
		}
		initiatingToken = claims.ID
		account, epoch = id, generation
		principal = "account:" + id
	}
	if err := installationAllowed(ctx, tx, req.DeviceHash, "player"); err != nil {
		return nil, ErrOAuthInvalid
	}
	// A bounded global flow budget also bounds unauthenticated restoration state.
	// Account locks precede this lock, matching callback/result issuance ordering.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(69428041)`); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM oauth_flows WHERE id IN (SELECT id FROM oauth_flows WHERE expires_at<clock_timestamp()-interval '24 hours' ORDER BY expires_at,id LIMIT 100)`); err != nil {
		return nil, err
	}
	var total, recent int
	if err = tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER (WHERE requester_hash=$1 AND created_at>clock_timestamp()-interval '1 hour') FROM oauth_flows`, codeChallenge(principal)).Scan(&total, &recent); err != nil {
		return nil, err
	}
	if total >= 10000 || recent >= 10 {
		return nil, ErrOAuthRateLimited
	}
	flow := &OAuthStart{FlowID: uuid.NewString(), CompletionSecret: secret}
	if err = tx.QueryRowContext(ctx, `INSERT INTO oauth_flows(id,provider,intent,state_hash,completion_hash,nonce_hash,code_verifier,requester_hash,account_id,session_epoch,initiating_token_id,device_hash,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,statement_timestamp(),statement_timestamp()+interval '10 minutes') RETURNING expires_at`, flow.FlowID, req.Provider, req.Intent, codeChallenge(state), codeChallenge(secret), codeChallenge(nonce), verifier, codeChallenge(principal), account, epoch, initiatingToken, req.DeviceHash).Scan(&flow.ExpiresAt); err != nil {
		return nil, err
	}
	options := []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("prompt", "select_account")}
	if req.Provider == "google" {
		options = append(options, oauth2.SetAuthURLParam("nonce", nonce), oauth2.SetAuthURLParam("code_challenge", codeChallenge(verifier)), oauth2.SetAuthURLParam("code_challenge_method", "S256"))
	}
	flow.URL = cfg.AuthCodeURL(state, options...)
	if req.Intent == "link" {
		if _, err = m.ValidateAccessTokenTx(ctx, tx, req.AccessToken); err != nil {
			return nil, ErrOAuthInvalid
		}
	} else if err = installationAllowed(ctx, tx, req.DeviceHash, "player"); err != nil {
		return nil, ErrOAuthInvalid
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return flow, nil
}

// CompleteOAuth consumes browser state before contacting a provider. A crashed
// exchange cannot replay; starting a new flow is safe and never creates an account.
// Player credentials are available only through the separate app-held secret.
func (m *Manager) CompleteOAuth(ctx context.Context, provider, state, code string) error {
	if m.oauthCfgs[provider] == nil || len(state) != 43 || len(code) < 1 || len(code) > 4096 {
		return ErrOAuthInvalid
	}
	var id, intent, verifier, nonce string
	var account sql.NullString
	var generation sql.NullInt64
	err := m.db.QueryRowContext(ctx, `WITH claim AS (SELECT id,code_verifier FROM oauth_flows WHERE state_hash=$1 AND provider=$2 AND status='pending' AND expires_at>clock_timestamp() FOR UPDATE) UPDATE oauth_flows f SET status='exchanging',code_verifier='' FROM claim WHERE f.id=claim.id RETURNING f.id,f.intent,f.account_id,f.session_epoch,f.nonce_hash,claim.code_verifier`, codeChallenge(state), provider).Scan(&id, &intent, &account, &generation, &nonce, &verifier)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOAuthInvalid
	}
	if err != nil {
		return err
	}
	subject, email, err := m.verifyOAuthCode(ctx, provider, code, verifier, nonce)
	if err == nil {
		err = m.completeOAuthIdentity(ctx, id, intent, account, generation, provider, subject, email)
	}
	if err != nil {
		safe := ErrOAuthInvalid
		for _, candidate := range []error{ErrOAuthConflict, ErrOAuthUnlinked, ErrOAuthUnavailable} {
			if errors.Is(err, candidate) {
				safe = candidate
			}
		}
		// Callback receipt is already consumed if persistence or request cancellation
		// prevents this best-effort diagnostic; result still cannot mint credentials.
		_, _ = m.db.ExecContext(ctx, `UPDATE oauth_flows SET status='failed',error_code=$2 WHERE id=$1 AND status='exchanging'`, id, safe.Error())
		return safe
	}
	return nil
}

func (m *Manager) completeOAuthIdentity(ctx context.Context, id, intent string, account sql.NullString, generation sql.NullInt64, provider, subject, email string) error {
	if !validOAuthIdentity(provider, subject, email) {
		return ErrOAuthInvalid
	}
	if intent == "restore" {
		owner, err := m.FindOAuthAccount(ctx, provider, subject)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrOAuthUnlinked
		}
		if err != nil {
			return err
		}
		account = sql.NullString{String: owner, Valid: true}
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	purpose, epoch, err := m.accountSession(ctx, tx, account.String, true)
	if err != nil || purpose != "player" || (intent == "link" && epoch != generation.Int64) {
		return ErrOAuthInvalid
	}
	var status string
	var installation sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT status,device_hash FROM oauth_flows WHERE id=$1 AND expires_at>clock_timestamp() AND NOT EXISTS (SELECT 1 FROM auth_revocations r WHERE r.token_id=oauth_flows.initiating_token_id) FOR UPDATE`, id).Scan(&status, &installation); err != nil || status != "exchanging" {
		return ErrOAuthInvalid
	}
	if err = linkInstallationTx(ctx, tx, account.String, installation.String, purpose); err != nil {
		return ErrOAuthInvalid
	}
	if intent == "restore" {
		var owner string
		if err = tx.QueryRowContext(ctx, `SELECT account_id FROM oauth_links WHERE provider=$1 AND provider_subject=$2`, provider, subject).Scan(&owner); err != nil || owner != account.String {
			return ErrOAuthUnlinked
		}
	} else if err = linkOAuthTx(ctx, tx, account.String, provider, subject, email); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE oauth_flows SET status='completed',account_id=$2,session_epoch=$3 WHERE id=$1`, id, account.String, epoch); err != nil {
		return err
	}
	if err = freshOAuthFlow(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// OAuthResult replays one issuance receipt, never generating a fresh pair on
// retries. Revocation, refresh consumption and expiry all close that receipt.
func (m *Manager) OAuthResult(ctx context.Context, id, secret string) (*TokenPair, error) {
	if _, err := uuid.Parse(id); err != nil || len(secret) != 43 {
		return nil, ErrOAuthInvalid
	}
	var account sql.NullString
	var status, code string
	err := m.db.QueryRowContext(ctx, `SELECT account_id,status,error_code FROM oauth_flows WHERE id=$1 AND completion_hash=$2 AND expires_at>clock_timestamp()`, id, codeChallenge(secret)).Scan(&account, &status, &code)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOAuthInvalid
	}
	if err != nil {
		return nil, err
	}
	if status == "pending" || status == "exchanging" {
		return nil, ErrOAuthPending
	}
	if status == "failed" {
		switch code {
		case ErrOAuthConflict.Error():
			return nil, ErrOAuthConflict
		case ErrOAuthUnlinked.Error():
			return nil, ErrOAuthUnlinked
		case ErrOAuthUnavailable.Error():
			return nil, ErrOAuthUnavailable
		}
		return nil, ErrOAuthInvalid
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	purpose, epoch, err := m.accountSession(ctx, tx, account.String, true)
	if err != nil || purpose != "player" {
		return nil, ErrOAuthInvalid
	}
	var bound int64
	var at sql.NullTime
	var accessID, refreshID, configHash, installation sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT session_epoch,issued_at,access_id,refresh_id,issuance_config_hash,device_hash FROM oauth_flows WHERE id=$1 AND completion_hash=$2 AND status='completed' AND expires_at>clock_timestamp() AND NOT EXISTS (SELECT 1 FROM auth_revocations r WHERE r.token_id=oauth_flows.initiating_token_id) FOR UPDATE`, id, codeChallenge(secret)).Scan(&bound, &at, &accessID, &refreshID, &configHash, &installation)
	if err != nil || bound != epoch {
		return nil, ErrOAuthInvalid
	}
	if installation.Valid {
		if err = linkInstallationTx(ctx, tx, account.String, installation.String, purpose); err != nil {
			return nil, ErrOAuthInvalid
		}
	} else if !at.Valid {
		return nil, ErrOAuthInvalid
	}
	if at.Valid && subtle.ConstantTimeCompare([]byte(configHash.String), []byte(m.oauthIssuanceConfigHash())) != 1 {
		return nil, ErrOAuthInvalid
	}
	if !at.Valid {
		at = sql.NullTime{Time: time.Now().UTC().Truncate(time.Second), Valid: true}
		accessID = sql.NullString{String: uuid.NewString(), Valid: true}
		refreshID = sql.NullString{String: uuid.NewString(), Valid: true}
		if _, err = tx.ExecContext(ctx, `UPDATE oauth_flows SET issued_at=$2,access_id=$3,refresh_id=$4,issuance_config_hash=$5 WHERE id=$1`, id, at.Time, accessID.String, refreshID.String, m.oauthIssuanceConfigHash()); err != nil {
			return nil, err
		}
	}
	var revoked bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auth_revocations WHERE token_id=$1 OR (token_id=$2 AND NOT ($3 AND EXISTS(SELECT 1 FROM auth_installation_rotations b WHERE b.old_refresh_id=$2 AND b.account_id=$4 AND b.session_epoch=$5))))`, accessID.String, refreshID.String, !installation.Valid, account.String, epoch).Scan(&revoked); err != nil {
		return nil, err
	}
	if revoked || !at.Time.Add(m.accessTTL).After(time.Now()) {
		return nil, ErrOAuthInvalid
	}
	pair, err := m.signTokensAt(account.String, installation.String, purpose, epoch, at.Time, accessID.String, refreshID.String)
	if err != nil {
		return nil, err
	}
	if err = freshOAuthFlow(ctx, tx, id); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return pair, nil
}

func randomCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func codeChallenge(value string) string {
	sum := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Only fixed public error codes may cross the HTTP boundary.
func OAuthErrorCode(err error) string {
	for _, e := range []error{ErrOAuthInvalid, ErrOAuthPending, ErrOAuthConflict, ErrOAuthUnlinked, ErrOAuthUnavailable, ErrOAuthRateLimited} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	return ErrOAuthUnavailable.Error()
}

// A receipt is closed if signing parameters change across a restart. Reusing
// its JTIs for different claims would create a second credential issuance.
func (m *Manager) oauthIssuanceConfigHash() string {
	data, _ := json.Marshal([]any{m.issuer, m.audience, m.accessTTL, m.refreshTTL})
	digest := hmac.New(sha256.New, m.signingKey)
	digest.Write(data)
	return base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}

// Recheck after installation and identity writes: their locks can outlive the
// flow even though the initial locked flow read was valid.
func freshOAuthFlow(ctx context.Context, tx *sql.Tx, id string) error {
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT expires_at>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM auth_revocations r WHERE r.token_id=oauth_flows.initiating_token_id) FROM oauth_flows WHERE id=$1`, id).Scan(&allowed); err != nil || !allowed {
		return ErrOAuthInvalid
	}
	return nil
}
