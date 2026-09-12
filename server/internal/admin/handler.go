package admin

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/notices"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/internal/webui"
)

// Handler returns the admin console HTTP handler mounted at /admin/.
func (m *Manager) Handler(noticesMgr *notices.Manager) http.Handler {
	mux := http.NewServeMux()
	m.registerOperations(mux)
	m.registerUserTerms(mux)
	mux.HandleFunc("GET /admin/login", m.loginForm)
	mux.HandleFunc("POST /admin/login", m.loginPost)
	mux.Handle("POST /admin/logout", m.requireRole("admin", true)(http.HandlerFunc(m.logoutPost)))
	mux.Handle("GET /admin/", m.requireRole("admin", false)(http.HandlerFunc(m.dashboard)))
	mux.Handle("GET /admin/notices", m.requireRole("admin", false)(http.HandlerFunc(m.noticesList)))
	mux.Handle("POST /admin/notices", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.noticesCreate(w, r, noticesMgr) })))
	mux.Handle("POST /admin/notices/{id}/withdraw", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.noticesWithdraw(w, r, noticesMgr) })))
	mux.Handle("POST /admin/avatars/{account_id}/takedown", m.requireRole("admin", true)(http.HandlerFunc(m.avatarTakedown)))
	return mux
}

// RequireAdmin protects an additional admin handler, including CSRF on unsafe methods.
func (m *Manager) RequireAdmin(next http.Handler) http.Handler {
	return m.requireRole("admin", true)(next)
}

func (m *Manager) requireRole(role string, _ bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionID, csrfToken := SessionFromRequest(r)
			adminID, adminRole, storedCSRF, err := m.sessionDetails(r.Context(), sessionID)
			if err != nil {
				ClearSessionCookie(w, m.secureCookie(r))
				http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
				return
			}
			if adminRole != role {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			// Every unsafe method is guarded even if a new route omits the old flag.
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
				if csrfToken == "" || subtle.ConstantTimeCompare([]byte(storedCSRF), []byte(csrfToken)) != 1 {
					http.Error(w, "invalid csrf token", http.StatusForbidden)
					return
				}
			}
			ctx := context.WithValue(r.Context(), ctxAdminIDKey{}, adminID)
			ctx = context.WithValue(ctx, ctxCSRFKey{}, storedCSRF)
			ctx = store.WithAdminAuthorization(ctx, adminID, func(ctx context.Context, tx *sql.Tx) (string, error) {
				return m.AuthorizeSessionTx(ctx, tx, sessionID, csrfToken)
			})
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (m *Manager) secureCookie(r *http.Request) bool {
	return r.TLS != nil || m.cfg.App.Env == "prod"
}

type ctxAdminIDKey struct{}
type ctxCSRFKey struct{}

func csrfFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxCSRFKey{}).(string); ok {
		return v
	}
	return ""
}

func adminIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxAdminIDKey{}).(string); ok {
		return v
	}
	return ""
}

func (m *Manager) loginForm(w http.ResponseWriter, r *http.Request) {
	token, err := randomHex(32)
	if err != nil {
		http.Error(w, "login unavailable", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "knowoff_admin_login_csrf", Value: token, Path: "/admin/", HttpOnly: true, Secure: m.secureCookie(r), SameSite: http.SameSiteStrictMode, MaxAge: 600})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	webui.Render(w, http.StatusOK, webui.Page{Title: "Admin login", Intro: "The control room. Authorized crew only.", CSRF: token}, `<section class="panel"><form method="post" action="/admin/login"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label>Email<input name="email" type="email" autocomplete="username" required></label><label>Password<input name="password" type="password" autocomplete="current-password" required></label><label>Authenticator code<input name="totp" inputmode="numeric" pattern="[0-9]{6}" autocomplete="one-time-code" required></label><button>Sign in</button></form></section>`)
}

func (m *Manager) loginPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	loginCookie, err := r.Cookie("knowoff_admin_login_csrf")
	supplied := r.PostForm.Get("csrf_token")
	if err != nil || supplied == "" || subtle.ConstantTimeCompare([]byte(loginCookie.Value), []byte(supplied)) != 1 {
		slog.Warn("admin login rejected", "reason", "csrf")
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	email := r.FormValue("email")
	password := r.FormValue("password")
	code := r.FormValue("totp")

	if !m.AllowLogin(email) {
		http.Error(w, "too many login attempts", http.StatusTooManyRequests)
		return
	}

	a, err := m.Authenticate(r.Context(), email, password)
	if err != nil {
		slog.Warn("admin login rejected", "reason", "credentials")
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if !VerifyTOTP(a.TOTPSecret, code) {
		slog.Warn("admin login rejected", "reason", "credentials")
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	sessionID, _, _, err := m.CreateSessionForEpoch(r.Context(), a.ID, a.SessionEpoch)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	SetSessionCookie(w, sessionID, time.Now().UTC().Add(time.Duration(m.cfg.Security.AdminSessionTTLH)*time.Hour), m.secureCookie(r))
	http.SetCookie(w, &http.Cookie{Name: "knowoff_admin_login_csrf", Path: "/admin/", HttpOnly: true, Secure: m.secureCookie(r), SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/admin/", http.StatusSeeOther)
}

func (m *Manager) logoutPost(w http.ResponseWriter, r *http.Request) {
	sessionID, _ := SessionFromRequest(r)
	if sessionID != "" {
		if err := m.DestroySession(r.Context(), sessionID); err != nil {
			http.Error(w, "logout unavailable", http.StatusInternalServerError)
			return
		}
	}
	ClearSessionCookie(w, m.secureCookie(r))
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (m *Manager) dashboard(w http.ResponseWriter, r *http.Request) {
	adminPage(w, r, "Operations", "Keep the game welcoming, the content sharp, and every decision traceable.", `<div class="split"><section><h2 style="margin-top:0">Community & content</h2><ul class="list"><li><a href="/admin/portal/applications">Review role applications and grants</a></li><li><a href="/admin/portal/submissions">Review contributor submissions</a></li><li><a href="/admin/portal/challenge">Schedule and screen the Weekly Nown Challenge</a></li><li><a href="/admin/portal/terms">Version contribution terms</a></li></ul></section><section><h2 style="margin-top:0">Live operations</h2><ul class="list"><li><a href="/admin/notices">Compose system notices</a></li><li><a href="/admin/reports">Review player reports</a></li><li><a href="/admin/feedback">Triage feedback</a></li><li><a href="/admin/economy">Look up wallets and entitlements</a></li></ul></section></div><p class="notice">Pack deployment, Guard enforcement, grants/refunds and leaderboard operations remain under construction. Only the available workspaces above are enabled.</p>`, nil)
}

func (m *Manager) noticesList(w http.ResponseWriter, r *http.Request) {
	rows, err := m.db.QueryContext(r.Context(), `SELECT id,type,title,body,published_at,withdrawn_at FROM system_notices ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, "Notices unavailable.", 500)
		return
	}
	defer rows.Close()
	type item struct {
		ID, Type, Title, Body, Published string
		Withdrawn                        bool
	}
	var items []item
	for rows.Next() {
		var v item
		var title, body []byte
		var published, withdrawn sql.NullTime
		if err = rows.Scan(&v.ID, &v.Type, &title, &body, &published, &withdrawn); err != nil {
			http.Error(w, "Notices unavailable.", 500)
			return
		}
		v.Title = extractLocale(title, "en")
		v.Body = extractLocale(body, "en")
		v.Published = "Unscheduled"
		if published.Valid {
			v.Published = published.Time.Format(time.RFC3339)
		}
		v.Withdrawn = withdrawn.Valid
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		http.Error(w, "Notices unavailable.", 500)
		return
	}
	adminPage(w, r, "System notices", "Tell players what is happening before it happens.", `<div class="split"><section><h2 style="margin-top:0">Compose a notice</h2><form method="post" action="/admin/notices"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label for="type">Notice type</label><select id="type" name="type"><option>announcement</option><option>maintenance</option><option>downtime</option></select><label for="title">Title (English)</label><input id="title" name="title_en" required><label for="body">Message (English)</label><textarea id="body" name="body_en" required></textarea><label for="publish">Publish at (UTC, optional)</label><input id="publish" name="published_at" type="datetime-local"><label for="maintenance">Maintenance start (UTC, if applicable)</label><input id="maintenance" name="maintenance_start" type="datetime-local"><label for="duration">Maintenance duration (minutes)</label><input id="duration" name="duration_min" type="number" min="1" value="30"><button>Create notice</button></form></section><section><h2 style="margin-top:0">Notice history</h2>{{range .Data}}<article class="panel"><div class="row spread"><h3>{{.Title}}</h3><span class="status">{{if .Withdrawn}}Withdrawn{{else}}{{.Type}}{{end}}</span></div><p>{{.Body}}</p><p class="small">Publish: {{.Published}}</p>{{if not .Withdrawn}}<form method="post" action="/admin/notices/{{.ID}}/withdraw"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><button class="secondary">Withdraw notice</button></form>{{end}}</article>{{else}}<p class="empty">No notices yet. Announcements and maintenance messages will appear here.</p>{{end}}</section></div>`, items)
}

func (m *Manager) noticesCreate(w http.ResponseWriter, r *http.Request, nm *notices.Manager) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	n := notices.Notice{
		Type: notices.NoticeType(r.FormValue("type")),
		Title: map[string]string{
			"en": r.FormValue("title_en"),
		},
		Body: map[string]string{
			"en": r.FormValue("body_en"),
		},
	}
	for _, field := range []struct {
		name string
		dest **time.Time
	}{{"published_at", &n.PublishedAt}, {"maintenance_start", &n.MaintenanceStart}} {
		if v := r.FormValue(field.name); v != "" {
			parsed, err := time.Parse("2006-01-02T15:04", v)
			if err != nil {
				adminError(w, r, fmt.Errorf("invalid %s: use the date and time control (UTC)", field.name), "/admin/notices")
				return
			}
			*field.dest = &parsed
		}
	}
	if v := r.FormValue("duration_min"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil || d <= 0 || int64(d) > int64((1<<63-1)/time.Minute) {
			adminError(w, r, fmt.Errorf("invalid maintenance duration: use positive whole minutes"), "/admin/notices")
			return
		}
		n.MaintenanceDurationMin = d
	}

	adminID := adminIDFromContext(r.Context())
	if adminID != "" {
		if id, err := uuid.Parse(adminID); err == nil {
			n.CreatedBy = &id
		}
	}

	if nm == nil {
		nm = notices.NewManager(m.db, m.cfg, nil)
	}
	if _, err := nm.CreateNotice(r.Context(), n); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/notices", http.StatusSeeOther)
}

func (m *Manager) noticesWithdraw(w http.ResponseWriter, r *http.Request, nm *notices.Manager) {
	id := r.PathValue("id")
	uid, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if nm == nil {
		nm = notices.NewManager(m.db, m.cfg, nil)
	}
	if err := nm.WithdrawNotice(r.Context(), uid, adminIDFromContext(r.Context())); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/notices", http.StatusSeeOther)
}

func (m *Manager) avatarTakedown(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("account_id")
	if err := m.TakedownAvatar(r.Context(), adminIDFromContext(r.Context()), accountID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/", http.StatusSeeOther)
}

func extractLocale(data []byte, locale string) string {
	if len(data) == 0 {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return string(data)
	}
	if v, ok := m[locale]; ok {
		return v
	}
	for _, v := range m {
		return v
	}
	return ""
}
