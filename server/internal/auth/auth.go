// Package auth manages sessions: anonymous device accounts, JWT issuance and
// revocation, and OAuth linking. All decisions are server-side; clients only
// present tokens.
package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// TokenKind distinguishes access and refresh tokens.
type TokenKind string

const (
	TokenAccess  TokenKind = "access"
	TokenRefresh TokenKind = "refresh"
)

// Claims is the JWT claim set used for Knowoff sessions.
type Claims struct {
	jwt.RegisteredClaims
	AccountID    string    `json:"aid"`
	DeviceHash   string    `json:"dvh"`
	Kind         TokenKind `json:"knd"`
	Purpose      string    `json:"purpose,omitempty"`
	SessionEpoch int64     `json:"session_epoch,omitempty"`
}

// Manager is the auth service.
type Manager struct {
	development atomic.Bool
	db          *sql.DB
	signingKey  []byte
	issuer      string
	audience    string
	accessTTL   time.Duration
	refreshTTL  time.Duration
	oauth       OAuthProviders
	oauthHTTP   *http.Client
	oauthCfgs   map[string]*oauth2.Config
}

// OAuthProviderConfig holds OAuth client credentials for one provider.
type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	GraphVersion string
}

// OAuthProviders is the set of supported OAuth providers.
type OAuthProviders struct {
	Google   OAuthProviderConfig
	Facebook OAuthProviderConfig
}

// NewManager creates an auth manager.
func NewManager(db *sql.DB, signingKey []byte, issuer, audience string, accessTTL, refreshTTL time.Duration, oauth OAuthProviders) *Manager {
	m := &Manager{
		db:         db,
		signingKey: signingKey,
		issuer:     issuer,
		audience:   audience,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		oauth:      oauth,
		oauthHTTP:  &http.Client{},
		oauthCfgs:  make(map[string]*oauth2.Config),
	}
	if configuredOAuth(oauth.Google, "google") {
		m.oauthCfgs["google"] = &oauth2.Config{
			ClientID:     oauth.Google.ClientID,
			ClientSecret: oauth.Google.ClientSecret,
			RedirectURL:  oauth.Google.RedirectURL,
			Scopes:       []string{"openid", "email"},
			Endpoint:     google.Endpoint,
		}
	}
	if configuredOAuth(oauth.Facebook, "facebook") {
		m.oauthCfgs["facebook"] = &oauth2.Config{
			ClientID:     oauth.Facebook.ClientID,
			ClientSecret: oauth.Facebook.ClientSecret,
			RedirectURL:  oauth.Facebook.RedirectURL,
			Scopes:       []string{"email"},
			Endpoint:     oauth2.Endpoint{AuthURL: "https://www.facebook.com/" + oauth.Facebook.GraphVersion + "/dialog/oauth", TokenURL: "https://graph.facebook.com/" + oauth.Facebook.GraphVersion + "/oauth/access_token", AuthStyle: oauth2.AuthStyleInParams},
		}
	}
	return m
}

// CreateAnonymousAccount creates a fresh account for a device fingerprint and
// returns access and refresh tokens. The account has a random nickname.
func (m *Manager) CreateAnonymousAccount(ctx context.Context, deviceHash string) (*TokenPair, error) {
	return m.authenticateInstallation(ctx, deviceHash)
}

// AuthenticateDevice returns tokens for an existing device-linked account, or
// creates one if the device has not been seen before.
func (m *Manager) AuthenticateDevice(ctx context.Context, deviceHash string) (*TokenPair, error) {
	return m.authenticateInstallation(ctx, deviceHash)
}

// TokenPair contains the JWT access and refresh tokens plus metadata.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	AccountID    string    `json:"account_id"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (m *Manager) issueTokens(ctx context.Context, accountID, deviceHash string) (*TokenPair, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	purpose, epoch, err := m.accountSession(ctx, tx, accountID, true)
	if err != nil {
		return nil, err
	}
	if deviceHash != "" {
		if err := linkInstallationTx(ctx, tx, accountID, deviceHash, purpose); err != nil {
			return nil, err
		}
	}
	pair, err := m.signTokens(accountID, deviceHash, purpose, epoch)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return pair, nil
}

// Called only while the account row is held through commit, so a revoked
// refresh token cannot obtain credentials for a later session generation.
func (m *Manager) signTokens(accountID, deviceHash, purpose string, epoch int64) (*TokenPair, error) {
	return m.signTokensAt(accountID, deviceHash, purpose, epoch, time.Now().UTC(), uuid.NewString(), uuid.NewString())
}

func (m *Manager) signTokensAt(accountID, deviceHash, purpose string, epoch int64, now time.Time, accessID, refreshID string) (*TokenPair, error) {
	accessClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        accessID,
			Subject:   accountID,
			Issuer:    m.issuer,
			Audience:  jwt.ClaimStrings{m.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
		},
		AccountID:    accountID,
		DeviceHash:   deviceHash,
		Kind:         TokenAccess,
		Purpose:      purpose,
		SessionEpoch: epoch,
	}
	refreshClaims := accessClaims
	refreshClaims.ID = refreshID
	refreshClaims.ExpiresAt = jwt.NewNumericDate(now.Add(m.refreshTTL))
	refreshClaims.Kind = TokenRefresh

	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(m.signingKey)
	if err != nil {
		return nil, fmt.Errorf("sign access: %w", err)
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(m.signingKey)
	if err != nil {
		return nil, fmt.Errorf("sign refresh: %w", err)
	}
	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		AccountID:    accountID,
		ExpiresAt:    now.Add(m.accessTTL),
	}, nil
}

// ValidateAccessToken parses and validates an access token, returning the
// account ID. Revoked tokens are rejected.
func (m *Manager) ValidateAccessToken(ctx context.Context, token string) (string, error) {
	claims, err := m.parseToken(token, TokenAccess)
	if err != nil {
		return "", err
	}
	revoked, err := m.isRevoked(ctx, claims.ID)
	if err != nil {
		return "", fmt.Errorf("revocation check: %w", err)
	}
	if revoked {
		return "", fmt.Errorf("token revoked")
	}
	purpose, epoch, err := m.accountSession(ctx, m.db, claims.AccountID, false)
	if err != nil || purpose != claims.Purpose || epoch != claims.SessionEpoch {
		return "", fmt.Errorf("account unavailable")
	}
	if err := installationAllowed(ctx, m.db, claims.DeviceHash, purpose); err != nil {
		return "", err
	}
	return claims.AccountID, nil
}

// Refresh uses a refresh token to issue a new access token pair, revoking the
// used refresh token.
func (m *Manager) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := m.parseToken(refreshToken, TokenRefresh)
	if err != nil {
		return nil, err
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
	if err := linkInstallationTx(ctx, tx, claims.AccountID, claims.DeviceHash, purpose); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO auth_revocations(token_id,expires_at) VALUES($1,$2) ON CONFLICT DO NOTHING`, claims.ID, claims.ExpiresAt.Time)
	if err != nil {
		return nil, fmt.Errorf("revoke refresh: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return nil, fmt.Errorf("token revoked")
	}
	if _, err := m.parseToken(refreshToken, TokenRefresh); err != nil {
		return nil, err
	}
	pair, err := m.signTokens(claims.AccountID, claims.DeviceHash, purpose, epoch)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return pair, nil
}

// RevokeAccount revokes all tokens for an account by deleting device links and
// marking the account banned. Live connections must drop when their next token
// validation fails.
func (m *Manager) RevokeAccount(ctx context.Context, accountID string) error {
	if _, err := m.db.ExecContext(ctx, "DELETE FROM device_tokens WHERE account_id = $1", accountID); err != nil {
		return fmt.Errorf("delete device tokens: %w", err)
	}
	if _, err := m.db.ExecContext(ctx, "UPDATE accounts SET banned_at = now() WHERE id = $1", accountID); err != nil {
		return fmt.Errorf("mark account banned: %w", err)
	}
	return nil
}

func (m *Manager) parseToken(token string, kind TokenKind) (*Claims, error) {
	claims := &Claims{}
	t, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (interface{}, error) {
		return m.signingKey, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithAudience(m.audience), jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !t.Valid || claims.Kind != kind {
		return nil, fmt.Errorf("invalid token")
	}
	if claims.Purpose == "" {
		claims.Purpose = "player"
	}
	if claims.ID == "" || claims.Subject != claims.AccountID || claims.AccountID == "" || claims.SessionEpoch < 0 || (claims.Purpose != "player" && (claims.Purpose != "development" || !m.development.Load())) {
		return nil, fmt.Errorf("invalid token purpose or identity")
	}
	return claims, nil
}

// HashDevice hashes a raw device identifier so it is never stored in plain text.
func HashDevice(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomNickname() string {
	return "Player " + uuid.NewString()[:6]
}
