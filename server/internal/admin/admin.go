// Package admin implements the admin console backend: account creation,
// password + TOTP authentication, session management, RBAC, audit logging,
// and login rate limiting.
package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/lib/pq"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "knowoff_admin_session"
	loginWindow       = 5 * time.Minute
	maxLoginsPerEmail = 5
)

// Manager owns admin accounts, sessions, and the audit trail.
type Manager struct {
	db    *sql.DB
	cfg   *config.Config
	redis *store.RedisClient

	loginMu       sync.Mutex
	loginAttempts map[string][]time.Time
}

// Admin is a row from admin_accounts.
type Admin struct {
	ID         string
	AccountID  string
	Email      string
	Role       string
	TOTPSecret string
}

// NewManager returns an admin console manager.
func NewManager(db *sql.DB, cfg *config.Config, redis *store.RedisClient) *Manager {
	return &Manager{
		db:            db,
		cfg:           cfg,
		redis:         redis,
		loginAttempts: make(map[string][]time.Time),
	}
}

// CreateAdmin creates an admin record linked to an existing player account.
func (m *Manager) CreateAdmin(ctx context.Context, accountID, email, password, role string) error {
	if _, err := uuid.Parse(accountID); err != nil {
		return fmt.Errorf("invalid account id: %w", err)
	}
	if email == "" || password == "" {
		return fmt.Errorf("email and password required")
	}
	if role == "" {
		role = "admin"
	}

	cost := m.cfg.Security.BcryptCost
	if cost < bcrypt.MinCost {
		cost = bcrypt.DefaultCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	secret, _, err := m.GenerateTOTPSecret(email)
	if err != nil {
		return fmt.Errorf("generate totp: %w", err)
	}

	_, err = m.db.ExecContext(ctx,
		`INSERT INTO admin_accounts (account_id, email, password_hash, totp_secret, backup_codes, role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now(), now())`,
		accountID, email, string(hash), secret, pq.Array([]string{}), role,
	)
	if err != nil {
		return fmt.Errorf("insert admin account: %w", err)
	}
	return nil
}

// Authenticate checks email + password and returns the admin row.
func (m *Manager) Authenticate(ctx context.Context, email, password string) (*Admin, error) {
	var hash string
	var a Admin
	err := m.db.QueryRowContext(ctx,
		`SELECT id, account_id, email, role, password_hash, totp_secret FROM admin_accounts WHERE email = $1`,
		email,
	).Scan(&a.ID, &a.AccountID, &a.Email, &a.Role, &hash, &a.TOTPSecret)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, fmt.Errorf("lookup admin: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return &a, nil
}

// GenerateTOTPSecret creates a new TOTP secret for the given account name.
func (m *Manager) GenerateTOTPSecret(accountName string) (secret, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      m.cfg.Security.AdminTOTPIssuer,
		AccountName: accountName,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// VerifyTOTP validates a TOTP code against a secret.
func VerifyTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

// TOTPSecretForEmail returns an existing admin's stored secret plus a fresh
// otpauth:// URL built from it, for dev/ops tooling (e.g. the seed-admin CLI
// command) that needs to print something scannable without minting a new,
// unrelated secret via GenerateTOTPSecret.
func (m *Manager) TOTPSecretForEmail(ctx context.Context, email string) (secret, otpauthURL string, err error) {
	if err := m.db.QueryRowContext(ctx,
		`SELECT totp_secret FROM admin_accounts WHERE email = $1`, email,
	).Scan(&secret); err != nil {
		return "", "", fmt.Errorf("lookup totp secret: %w", err)
	}
	label := fmt.Sprintf("%s:%s", m.cfg.Security.AdminTOTPIssuer, email)
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", m.cfg.Security.AdminTOTPIssuer)
	otpauthURL = fmt.Sprintf("otpauth://totp/%s?%s", url.PathEscape(label), v.Encode())
	return secret, otpauthURL, nil
}

// CreateSession issues a new admin session and CSRF token.
func (m *Manager) CreateSession(ctx context.Context, adminID string) (sessionID, csrfToken, cookieValue string, err error) {
	sessionID = uuid.NewString()
	csrfToken, err = randomHex(32)
	if err != nil {
		return "", "", "", fmt.Errorf("generate csrf: %w", err)
	}
	ttl := time.Duration(m.cfg.Security.AdminSessionTTLH) * time.Hour
	expiresAt := time.Now().UTC().Add(ttl)
	_, err = m.db.ExecContext(ctx,
		`INSERT INTO admin_sessions (id, admin_id, csrf_token, expires_at, last_activity)
		 VALUES ($1, $2, $3, $4, now())`,
		sessionID, adminID, csrfToken, expiresAt,
	)
	if err != nil {
		return "", "", "", fmt.Errorf("insert session: %w", err)
	}
	return sessionID, csrfToken, sessionID, nil
}

// sessionDetails authenticates a cookie independently of request method. The
// server's CSRF token is returned for rendering forms, never taken from a GET.
func (m *Manager) sessionDetails(ctx context.Context, sessionID string) (adminID, role, csrf string, err error) {
	if sessionID == "" {
		return "", "", "", fmt.Errorf("missing session")
	}
	err = m.db.QueryRowContext(ctx,
		`SELECT a.id, a.role, s.csrf_token FROM admin_sessions s
   JOIN admin_accounts a ON a.id = s.admin_id
   WHERE s.id = $1 AND s.expires_at > now()`, sessionID).Scan(&adminID, &role, &csrf)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid session: %w", err)
	}
	return adminID, role, csrf, nil
}

// ValidateSession authenticates a session and verifies an unsafe request's CSRF.
func (m *Manager) ValidateSession(ctx context.Context, sessionID, csrfToken string) (adminID, role string, err error) {
	adminID, role, storedCSRF, err := m.sessionDetails(ctx, sessionID)
	if err != nil {
		return "", "", err
	}
	if csrfToken == "" || subtle.ConstantTimeCompare([]byte(storedCSRF), []byte(csrfToken)) != 1 {
		return "", "", fmt.Errorf("invalid csrf token")
	}
	return adminID, role, nil
}

// DestroySession removes a session from the database.
func (m *Manager) DestroySession(ctx context.Context, sessionID string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id = $1`, sessionID)
	return err
}

// LogAction writes an append-only row to admin_audit_log.
func (m *Manager) LogAction(ctx context.Context, adminID, action, targetType, targetID string, before, after map[string]any) error {
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	var adminUUID interface{}
	if adminID != "" {
		if id, err := uuid.Parse(adminID); err == nil {
			adminUUID = id
		}
	}
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO admin_audit_log (admin_id, action, target_type, target_id, before_state, after_state, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())`,
		adminUUID, action, targetType, targetID, b, a,
	)
	if err != nil {
		return fmt.Errorf("audit log: %w", err)
	}
	return nil
}

// AllowLogin implements a sliding-window rate limit on login attempts per email.
func (m *Manager) AllowLogin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	now := time.Now().UTC()
	cutoff := now.Add(-loginWindow)

	if m.redis != nil {
		key := fmt.Sprintf("admin:login:%s", email)
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		ok, err := m.redis.AllowIntent(ctx, "admin:"+email, loginWindow, maxLoginsPerEmail)
		_ = key
		if err == nil {
			return ok
		}
		// Fall back to memory on Redis error.
	}

	m.loginMu.Lock()
	defer m.loginMu.Unlock()
	attempts := m.loginAttempts[email]
	var fresh []time.Time
	for _, t := range attempts {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	if len(fresh) >= maxLoginsPerEmail {
		m.loginAttempts[email] = fresh
		return false
	}
	m.loginAttempts[email] = append(fresh, now)
	return true
}

// SessionFromRequest extracts the admin session id and CSRF token from a request.
func SessionFromRequest(r *http.Request) (sessionID, csrfToken string) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		sessionID = cookie.Value
	}
	csrfToken = r.Header.Get("X-CSRF-Token")
	if csrfToken == "" {
		csrfToken = r.PostFormValue("csrf_token")
	}
	return sessionID, csrfToken
}

// SetSessionCookie writes the admin session cookie.
func SetSessionCookie(w http.ResponseWriter, value string, expires time.Time, secure ...bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Secure:   len(secure) > 0 && secure[0],
		Value:    value,
		Path:     "/admin/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  expires,
	})
}

// ClearSessionCookie removes the admin session cookie.
func ClearSessionCookie(w http.ResponseWriter, secure ...bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Secure:   len(secure) > 0 && secure[0],
		Value:    "",
		Path:     "/admin/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// TakedownAvatar removes a custom avatar and its entitlement.
func (m *Manager) TakedownAvatar(ctx context.Context, accountID string) error {
	if _, err := uuid.Parse(accountID); err != nil {
		return fmt.Errorf("invalid account id: %w", err)
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin takedown tx: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM custom_avatars WHERE account_id = $1`, accountID); err != nil {
		return fmt.Errorf("delete avatar: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM entitlements WHERE account_id = $1 AND entitlement_type = 'custom_avatar'`,
		accountID); err != nil {
		return fmt.Errorf("delete entitlement: %w", err)
	}
	return tx.Commit()
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
