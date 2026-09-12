package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func (m *Manager) registerUserTerms(mux *http.ServeMux) {
	for path, handler := range map[string]http.HandlerFunc{
		"GET /admin/user-terms":  m.userTermsPage,
		"POST /admin/user-terms": m.userTermsPublish,
	} {
		guarded := m.requireRole("admin", true)(handler)
		mux.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 3*store.MaxUserTermsBytes+(8<<10))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			guarded.ServeHTTP(w, r.WithContext(ctx))
		}))
	}
}

type userTermsVersion struct{ Version, Active string }
type userTermsView struct {
	Required, Selected, Body, Active, Next string
	Versions                               []userTermsVersion
}

func (m *Manager) userTermsPage(w http.ResponseWriter, r *http.Request) {
	after := r.URL.Query().Get("after")
	selected := r.URL.Query().Get("version")
	if selected == "" {
		selected = m.cfg.Trust.UserTermsVersion
	}
	if after != "" && !gamecontract.ValidIdentifier(after) || selected != "" && !gamecontract.ValidIdentifier(selected) {
		adminError(w, r, errors.New("invalid version or page cursor"), "/admin/user-terms")
		return
	}
	data := userTermsView{Required: m.cfg.Trust.UserTermsVersion, Selected: selected}
	rows, err := m.db.QueryContext(r.Context(), `SELECT version,active_from FROM user_terms_versions WHERE version>$1 ORDER BY version LIMIT 51`, after)
	if err != nil {
		http.Error(w, "User terms unavailable.", 503)
		return
	}
	for rows.Next() {
		var v userTermsVersion
		var at time.Time
		if err = rows.Scan(&v.Version, &at); err != nil {
			break
		}
		v.Active = at.UTC().Format(time.RFC3339)
		data.Versions = append(data.Versions, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		http.Error(w, "User terms unavailable.", 503)
		return
	}
	if len(data.Versions) > 50 {
		data.Versions = data.Versions[:50]
		data.Next = data.Versions[49].Version
	}
	if selected != "" {
		var at time.Time
		err = m.db.QueryRowContext(r.Context(), `SELECT body,active_from FROM user_terms_versions WHERE version=$1`, selected).Scan(&data.Body, &at)
		if err == nil {
			if len(data.Body) > store.MaxUserTermsBytes || !utf8.ValidString(data.Body) {
				http.Error(w, "User terms unavailable.", 503)
				return
			}
			data.Active = at.UTC().Format(time.RFC3339)
		} else if !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "User terms unavailable.", 503)
			return
		}
	}
	adminPage(w, r, "User terms", "Publish the exact reviewed wording players must accept before authored chat and contributions.", userTermsBody, data)
}

func (m *Manager) userTermsPublish(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		adminError(w, r, errors.New("invalid request"), "/admin/user-terms")
		return
	}
	at, err := time.Parse(time.RFC3339, r.FormValue("active_from"))
	if err == nil {
		err = store.NewTextTrustStore(m.db).PublishTerms(r.Context(), adminIDFromContext(r.Context()), r.FormValue("version"), r.FormValue("body"), at)
	}
	if err != nil {
		adminError(w, r, errors.New("invalid publication: use a unique version, bounded wording and a timestamp with time zone; existing versions cannot change"), "/admin/user-terms")
		return
	}
	http.Redirect(w, r, "/admin/user-terms", http.StatusSeeOther)
}

const userTermsBody = `<p class="notice">Required version: <strong>{{if .Data.Required}}{{.Data.Required}}{{else}}not configured{{end}}</strong>. Publication never accepts terms for a player. Contribution licensing uses its own terms and consent.</p><h2>Publish user terms</h2><p>Versions are permanent. A correction needs a new version. Publishing a version does not change the required version in server configuration.</p><form method="post" action="/admin/user-terms"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label>Version<input name="version" required maxlength="128"></label><label>Reviewed wording<textarea name="body" required maxlength="262144" spellcheck="false"></textarea></label><label>Available from, with time zone<input name="active_from" required placeholder="2026-09-12T00:00:00Z"></label><button>Publish this version</button></form><h2>Published versions</h2><p>Ordered by version. Up to 50 per page.</p><ul class="list">{{range .Data.Versions}}<li><a href="/admin/user-terms?version={{.Version}}">{{.Version}}</a> · Available from {{.Active}}</li>{{else}}<li>No versions on this page.</li>{{end}}</ul>{{if .Data.Next}}<a class="button secondary" href="/admin/user-terms?after={{.Data.Next}}">Next versions</a>{{end}}{{if .Data.Selected}}<h2>{{.Data.Selected}}</h2>{{if .Data.Active}}<p>Available from {{.Data.Active}}</p><div class="terms panel">{{.Data.Body}}</div>{{else}}<p class="empty">This version has not been published.</p>{{end}}{{end}}`
