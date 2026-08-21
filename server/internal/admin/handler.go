package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/notices"
)

// Handler returns the admin console HTTP handler mounted at /admin/.
func (m *Manager) Handler(noticesMgr *notices.Manager) http.Handler {
	mux := http.NewServeMux()
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

func (m *Manager) requireRole(role string, checkCSRF bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionID, csrfToken := SessionFromRequest(r)
			if checkCSRF && r.Method != http.MethodGet && r.Method != http.MethodHead {
				if csrfToken == "" {
					http.Error(w, "missing csrf token", http.StatusForbidden)
					return
				}
			}
			adminID, adminRole, err := m.ValidateSession(r.Context(), sessionID, csrfToken)
			if err != nil {
				ClearSessionCookie(w)
				http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
				return
			}
			if adminRole != role {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			ctx := context.WithValue(r.Context(), ctxAdminIDKey{}, adminID)
			ctx = context.WithValue(ctx, ctxCSRFKey{}, csrfToken)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
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
	fmt.Fprint(w, `<!doctype html>
<html><head><title>Knowoff Admin</title></head><body>
<h1>Admin Login</h1>
<form method="post" action="/admin/login">
<label>Email <input type="email" name="email" required></label><br>
<label>Password <input type="password" name="password" required></label><br>
<label>TOTP code <input type="text" name="totp" required pattern="[0-9]{6}"></label><br>
<button>Login</button>
</form>
</body></html>`)
}

func (m *Manager) loginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
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
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if !VerifyTOTP(a.TOTPSecret, code) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	sessionID, csrfToken, _, err := m.CreateSession(r.Context(), a.ID)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	SetSessionCookie(w, sessionID, time.Now().UTC().Add(time.Duration(m.cfg.Security.AdminSessionTTLH)*time.Hour))
	w.Header().Set("X-CSRF-Token", csrfToken)
	http.Redirect(w, r, "/admin/", http.StatusSeeOther)
}

func (m *Manager) logoutPost(w http.ResponseWriter, r *http.Request) {
	sessionID, _ := SessionFromRequest(r)
	if sessionID != "" {
		_ = m.DestroySession(r.Context(), sessionID)
	}
	ClearSessionCookie(w)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (m *Manager) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var wallets, reportsCount, feedbackCount, avatars int
	_ = m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM noin_wallets`).Scan(&wallets)
	_ = m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports`).Scan(&reportsCount)
	_ = m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedback`).Scan(&feedbackCount)
	_ = m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_avatars`).Scan(&avatars)

	fmt.Fprintf(w, `<!doctype html>
<html><head><title>Knowoff Admin Dashboard</title></head><body>
<h1>Dashboard</h1>
<ul>
<li>Wallets: %d</li>
<li>Reports: %d</li>
<li>Feedback: %d</li>
<li>Custom avatars: %d</li>
</ul>
<nav>
<a href="/admin/notices">Notices</a>
</nav>
</body></html>`, wallets, reportsCount, feedbackCount, avatars)
}

func (m *Manager) noticesList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, type, title, body, published_at, withdrawn_at, maintenance_start, maintenance_duration_min
		 FROM system_notices ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	csrf := csrfFromContext(ctx)
	fmt.Fprint(w, `<!doctype html>
<html><head><title>System Notices</title></head><body>
<h1>System Notices</h1>
<table border="1"><tr><th>Type</th><th>Published</th><th>Withdrawn</th><th>Title (en)</th><th>Actions</th></tr>`)
	for rows.Next() {
		var id uuid.UUID
		var nType string
		var title, body []byte
		var published, withdrawn, maintenanceStart sql.NullTime
		var durationMin sql.NullInt32
		if err := rows.Scan(&id, &nType, &title, &body, &published, &withdrawn, &maintenanceStart, &durationMin); err != nil {
			continue
		}
		titleEn := extractLocale(title, "en")
		publishedStr := "no"
		if published.Valid {
			publishedStr = published.Time.Format(time.RFC3339)
		}
		withdrawnStr := "no"
		if withdrawn.Valid {
			withdrawnStr = withdrawn.Time.Format(time.RFC3339)
		}
		fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>
<form method="post" action="/admin/notices/%s/withdraw" style="display:inline">
<input type="hidden" name="csrf_token" value="%s">
<button>Withdraw</button>
</form></td></tr>`,
			template.HTMLEscapeString(nType),
			template.HTMLEscapeString(publishedStr),
			template.HTMLEscapeString(withdrawnStr),
			template.HTMLEscapeString(titleEn),
			id.String(),
			template.HTMLEscapeString(csrf))
	}
	fmt.Fprintf(w, `</table>
<h2>Create notice</h2>
<form method="post" action="/admin/notices">
<input type="hidden" name="csrf_token" value="%s">
<label>Type <select name="type"><option>announcement</option><option>maintenance</option><option>downtime</option></select></label><br>
<label>Title (en) <input name="title_en" required></label><br>
<label>Body (en) <input name="body_en" required></label><br>
<label>Publish at (optional, RFC3339) <input name="published_at" type="datetime-local"></label><br>
<label>Maintenance start (for maintenance) <input name="maintenance_start" type="datetime-local"></label><br>
<label>Duration min <input name="duration_min" type="number" value="30"></label><br>
<button>Create</button>
</form>
</body></html>`, template.HTMLEscapeString(csrf))
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
	if v := r.FormValue("published_at"); v != "" {
		if t, err := time.Parse("2006-01-02T15:04", v); err == nil {
			n.PublishedAt = &t
		}
	}
	if v := r.FormValue("maintenance_start"); v != "" {
		if t, err := time.Parse("2006-01-02T15:04", v); err == nil {
			n.MaintenanceStart = &t
		}
	}
	if v := r.FormValue("duration_min"); v != "" {
		if d, err := time.ParseDuration(v + "m"); err == nil {
			min := int(d.Minutes())
			n.MaintenanceDurationMin = min
		}
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
	if err := nm.WithdrawNotice(r.Context(), uid); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/notices", http.StatusSeeOther)
}

func (m *Manager) avatarTakedown(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("account_id")
	before := map[string]any{"account_id": accountID}
	if err := m.TakedownAvatar(r.Context(), accountID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = m.LogAction(r.Context(), adminIDFromContext(r.Context()), "avatar_takedown", "custom_avatar", accountID, before, map[string]any{"status": "removed"})
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
