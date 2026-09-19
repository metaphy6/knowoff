package portal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"github.com/knowoff/knowoff/server/internal/webui"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	portalSessionCookie = "knowoff_portal_session"
	portalLoginCookie   = "knowoff_portal_login"
	portalSessionTTL    = 8 * time.Hour
	portalPairingTTL    = 5 * time.Minute
)

type ctxPortalCSRFKey struct{}

func csrfFromContext(ctx context.Context) string {
	value, _ := ctx.Value(ctxPortalCSRFKey{}).(string)
	return value
}
func portalRandom(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func portalHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func (m *Manager) secureBrowserCookie(r *http.Request) bool {
	return r.TLS != nil || (m.cfg != nil && m.cfg.App.Env == "prod")
}
func (m *Manager) browserCookie(w http.ResponseWriter, r *http.Request, name, value string, ttl time.Duration) {
	cookie := &http.Cookie{Name: name, Value: value, Path: "/portal/", HttpOnly: true, Secure: m.secureBrowserCookie(r), SameSite: http.SameSiteStrictMode}
	if ttl <= 0 {
		cookie.MaxAge = -1
	} else {
		cookie.Expires = time.Now().Add(ttl)
		cookie.MaxAge = int(ttl.Seconds())
	}
	http.SetCookie(w, cookie)
}
func portalHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func portalCSRF(r *http.Request) string {
	if token := r.Header.Get("X-CSRF-Token"); token != "" {
		return token
	}
	return r.PostFormValue("csrf_token")
}
func validPortalCSRF(stored, supplied string) bool {
	return stored != "" && supplied != "" && subtle.ConstantTimeCompare([]byte(stored), []byte(supplied)) == 1
}

// Authorize every browser request against live account and Guard state, not
// role claims cached in a cookie. Role/workflow checks remain in domain methods.
func (m *Manager) portalAccountAllowed(ctx context.Context, account string) bool {
	var allowed bool
	err := m.db.QueryRowContext(ctx, `SELECT banned_at IS NULL AND deleted_at IS NULL AND (suspended_until IS NULL OR suspended_until<=now())
  AND NOT direct_account_sanction_active(a.id)
	  AND NOT EXISTS (SELECT 1 FROM guard_freezes f WHERE f.account_id=a.id
  AND f.expires_at>now() AND f.dismissed_at IS NULL AND f.converted_to_ban_at IS NULL)
  FROM accounts a WHERE a.id=$1`, account).Scan(&allowed)
	return err == nil && allowed
}

func (m *Manager) accountFromRequest(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || m.auth == nil {
		return ""
	}
	account, err := m.auth.ValidateAccessToken(r.Context(), parts[1])
	if err != nil {
		return ""
	}
	return account
}

func (m *Manager) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		portalHeaders(w)
		var account, csrf string
		var proof portalCredential
		if r.Header.Get("Authorization") != "" {
			account = m.accountFromRequest(r)
			proof = portalCredential{account: account, bearer: strings.Fields(r.Header.Get("Authorization")), auth: m.auth}
			if account == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		} else {
			cookie, err := r.Cookie(portalSessionCookie)
			if err == nil {
				err = m.db.QueryRowContext(r.Context(), `SELECT account_id,csrf_token FROM portal_browser_sessions WHERE token_hash=$1 AND expires_at>clock_timestamp() AND device_hash IS NOT NULL AND NOT installation_sanction_active(device_hash)`, portalHash(cookie.Value)).Scan(&account, &csrf)
				if err != nil {
					account = ""
				}
			}
			if account == "" {
				m.browserCookie(w, r, portalSessionCookie, "", 0)
				if r.Method == http.MethodGet || r.Method == http.MethodHead {
					http.Redirect(w, r, "/portal/login", http.StatusSeeOther)
				} else {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
				}
				return
			}
			proof = portalCredential{account: account, sessionHash: portalHash(cookie.Value), csrf: csrf}
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !validPortalCSRF(csrf, portalCSRF(r)) {
				http.Error(w, "invalid csrf token", http.StatusForbidden)
				return
			}
		}
		if !m.portalAccountAllowed(r.Context(), account) {
			http.Error(w, "account unavailable", http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), ctxAccountIDKey{}, account)
		ctx = context.WithValue(ctx, ctxPortalCSRFKey{}, csrf)
		ctx = context.WithValue(ctx, portalCredentialKey{}, proof)
		next(w, r.WithContext(ctx))
	}
}

// Limits are keyed by hashes of the authenticated account or the direct peer
// address. Untrusted forwarding headers cannot reset the budget.
func (m *Manager) allowPortalLogin(ctx context.Context, principal string, limit int) bool {
	_, err := m.db.ExecContext(ctx, `DELETE FROM portal_login_limits WHERE expires_at<now()`)
	if err != nil {
		return false
	}
	var attempts int
	err = m.db.QueryRowContext(ctx, `INSERT INTO portal_login_limits(principal_hash,attempts,expires_at)
  VALUES($1,1,now()+interval '5 minutes') ON CONFLICT(principal_hash) DO UPDATE
  SET attempts=portal_login_limits.attempts+1 RETURNING attempts`, portalHash(principal)).Scan(&attempts)
	return err == nil && attempts <= limit
}

func (m *Manager) loginForm(w http.ResponseWriter, r *http.Request) {
	portalHeaders(w)
	var code, csrf string
	if cookie, err := r.Cookie(portalLoginCookie); err == nil {
		_ = m.db.QueryRowContext(r.Context(), `SELECT pairing_code,csrf_token FROM portal_login_requests WHERE browser_hash=$1 AND expires_at>now()`, portalHash(cookie.Value)).Scan(&code, &csrf)
	}
	if code == "" {
		peer, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			peer = r.RemoteAddr
		}
		if !m.allowPortalLogin(r.Context(), "browser:"+peer, 10) {
			http.Error(w, "too many login attempts; try again later", http.StatusTooManyRequests)
			return
		}
		if _, err := m.db.ExecContext(r.Context(), `DELETE FROM portal_login_requests WHERE expires_at<=now()`); err != nil {
			http.Error(w, "login unavailable", 500)
			return
		}
		var active int
		if err := m.db.QueryRowContext(r.Context(), `SELECT count(*) FROM portal_login_requests`).Scan(&active); err != nil || active >= 10000 {
			http.Error(w, "login busy; try again later", 503)
			return
		}
		nonce, err := portalRandom(32)
		if err != nil {
			http.Error(w, "login unavailable", 500)
			return
		}
		csrf, err = portalRandom(32)
		if err != nil {
			http.Error(w, "login unavailable", 500)
			return
		}
		codeBytes := make([]byte, 5)
		if _, err = rand.Read(codeBytes); err != nil {
			http.Error(w, "login unavailable", 500)
			return
		}
		code = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(codeBytes)
		_, err = m.db.ExecContext(r.Context(), `INSERT INTO portal_login_requests(browser_hash,pairing_code,csrf_token,expires_at) VALUES($1,$2,$3,$4)`, portalHash(nonce), code, csrf, time.Now().Add(portalPairingTTL))
		if err != nil {
			http.Error(w, "login unavailable; reload to retry", 500)
			return
		}
		m.browserCookie(w, r, portalLoginCookie, nonce, portalPairingTTL)
	}
	shown := code[:4] + "-" + code[4:]
	webui.Render(w, http.StatusOK, webui.Page{Title: "Connect your player account", Intro: "Same player. A new backstage pass.", CSRF: csrf, Data: shown}, `<section class="panel"><h2>A code for this browser</h2><p>In Knowoff, open your profile and choose Connect contributor portal. Enter this code only for this browser.</p><p class="stat"><strong data-pairing-code="{{.Data}}">{{.Data}}</strong></p><p>This code expires in five minutes. After approving it in the game, continue here.</p><form method="post" action="/portal/session"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><button>Continue</button></form></section>`)
}

// ConnectHandler approves the browser's pairing request using an already
// verified player access token. It never accepts cookie authentication.
func (m *Manager) ConnectHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		portalHeaders(w)
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		account := m.accountFromRequest(r)
		if account == "" {
			http.Error(w, "unauthorized", 401)
			return
		}
		if !m.portalAccountAllowed(r.Context(), account) {
			http.Error(w, "account unavailable", 403)
			return
		}
		if !m.allowPortalLogin(r.Context(), "account:"+account, 5) {
			http.Error(w, "too many code attempts; try again later", 429)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var input struct {
			Code string `json:"code"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "invalid code", 400)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "invalid code", 400)
			return
		}
		code := strings.ToUpper(strings.TrimSpace(input.Code))
		code = strings.NewReplacer("-", "", " ", "").Replace(code)
		if len(code) != 8 || strings.ContainsAny(code, "0189") {
			http.Error(w, "invalid code", 400)
			return
		}
		if _, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(code); err != nil {
			http.Error(w, "invalid code", 400)
			return
		}
		tx, err := m.db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "connection unavailable", 500)
			return
		}
		defer tx.Rollback()
		// Initial authentication is a routing precheck. Bind this derived session
		// only after validating the same credential under the account lock.
		binding, err := m.auth.ValidateAccessBindingTx(r.Context(), tx, strings.Fields(r.Header.Get("Authorization"))[1])
		if err != nil || binding.AccountID != account {
			http.Error(w, "unauthorized", 401)
			return
		}
		if err = portalActorAllowedTx(r.Context(), tx, account, m.now()); err != nil {
			http.Error(w, "account unavailable", 403)
			return
		}
		result, err := tx.ExecContext(r.Context(), `UPDATE portal_login_requests SET account_id=$1,device_hash=$3 WHERE pairing_code=$2 AND account_id IS NULL AND expires_at>clock_timestamp()`, account, code, binding.DeviceHash)
		if err != nil {
			http.Error(w, "connection unavailable", 500)
			return
		}
		count, err := result.RowsAffected()
		if err != nil || count != 1 {
			slog.Warn("portal pairing rejected", "reason", "invalid or expired code")
			http.Error(w, "code invalid, expired, or already approved", 400)
			return
		}
		if verified, checkErr := m.auth.ValidateAccessTokenTx(r.Context(), tx, strings.Fields(r.Header.Get("Authorization"))[1]); checkErr != nil || verified != account {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "connection unavailable", 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (m *Manager) loginContinue(w http.ResponseWriter, r *http.Request) {
	portalHeaders(w)
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	cookie, err := r.Cookie(portalLoginCookie)
	if err != nil {
		http.Error(w, "invalid login browser", 403)
		return
	}
	var csrf string
	var account, installation sql.NullString
	err = m.db.QueryRowContext(r.Context(), `SELECT csrf_token,account_id,device_hash FROM portal_login_requests WHERE browser_hash=$1 AND expires_at>now()`, portalHash(cookie.Value)).Scan(&csrf, &account, &installation)
	if err != nil || !validPortalCSRF(csrf, portalCSRF(r)) {
		http.Error(w, "login expired or invalid; return to login", 403)
		return
	}
	if !account.Valid {
		webui.Render(w, http.StatusConflict, webui.Page{Title: "One more step", Intro: "Approve this browser in your Knowoff profile, then continue.", CSRF: csrf}, `<section class="panel"><form method="post" action="/portal/session"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><button>Try Continue again</button></form><p><a href="/portal/login">View your code</a></p></section>`)
		return
	}
	if !m.portalAccountAllowed(r.Context(), account.String) {
		http.Error(w, "account unavailable", 403)
		return
	}
	token, err := portalRandom(32)
	if err != nil {
		http.Error(w, "login unavailable", 500)
		return
	}
	sessionCSRF, err := portalRandom(32)
	if err != nil {
		http.Error(w, "login unavailable", 500)
		return
	}
	tx, err := m.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "login unavailable", 500)
		return
	}
	defer tx.Rollback()
	// Account precedes pairing/session rows, matching final session revocation.
	// Revocation deletes approved requests under this lock, so an old approval
	// cannot produce a fresh browser session after its credential epoch is closed.
	if err = m.authorizePortalWriteTx(r.Context(), tx, account.String, "", false); err != nil {
		http.Error(w, "account unavailable", 403)
		return
	}
	if !installation.Valid || lockPortalInstallation(r.Context(), tx, installation.String) != nil {
		http.Error(w, "installation unavailable", 403)
		return
	}
	result, err := tx.ExecContext(r.Context(), `DELETE FROM portal_login_requests WHERE browser_hash=$1 AND account_id=$2 AND csrf_token=$3 AND device_hash=$4 AND expires_at>clock_timestamp()`, portalHash(cookie.Value), account.String, csrf, installation.String)
	if err != nil {
		http.Error(w, "login unavailable", 500)
		return
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		http.Error(w, "login already used", 403)
		return
	}
	if previous, err := r.Cookie(portalSessionCookie); err == nil {
		if _, err = tx.ExecContext(r.Context(), `DELETE FROM portal_browser_sessions WHERE token_hash=$1`, portalHash(previous.Value)); err != nil {
			http.Error(w, "login unavailable", 500)
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM portal_browser_sessions WHERE account_id=$1 AND expires_at<=clock_timestamp()`, account.String); err != nil {
		http.Error(w, "login unavailable", 500)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_browser_sessions(token_hash,account_id,csrf_token,expires_at,device_hash) VALUES($1,$2,$3,$4,$5)`, portalHash(token), account.String, sessionCSRF, time.Now().Add(portalSessionTTL), installation.String); err != nil {
		http.Error(w, "login unavailable", 500)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "login unavailable", 500)
		return
	}
	m.browserCookie(w, r, portalLoginCookie, "", 0)
	m.browserCookie(w, r, portalSessionCookie, token, portalSessionTTL)
	http.Redirect(w, r, "/portal/", http.StatusSeeOther)
}

func (m *Manager) logoutPost(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(portalSessionCookie); err == nil {
		if _, err = m.db.ExecContext(r.Context(), `DELETE FROM portal_browser_sessions WHERE token_hash=$1`, portalHash(cookie.Value)); err != nil {
			http.Error(w, "logout unavailable", 500)
			return
		}
	}
	m.browserCookie(w, r, portalSessionCookie, "", 0)
	http.Redirect(w, r, "/portal/login", http.StatusSeeOther)
}
