package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/knowoff/knowoff/server/internal/portal"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func (m *Manager) registerTextContent(mux *http.ServeMux) {
	if m.textContent == nil {
		var screen func(context.Context, string) error
		if s := portal.NewTextScreener(m.cfg.Moderation.ContentScreening); s != nil {
			screen = s.ScreenText
		}
		m.textContent = store.NewTextReleaseStore(m.db, m.cfg.Tuning, screen)
	}
	routes := map[string]http.HandlerFunc{
		"GET /admin/text":                         m.textContentPage,
		"GET /admin/text/inputs/{id}":             m.textInputExport,
		"POST /admin/text/capture":                m.textInputCapture,
		"POST /admin/text/publish":                m.textPublish,
		"POST /admin/text/releases/{id}/activate": m.textActivate,
		"POST /admin/text/releases/{id}/withdraw": m.textWithdraw,
		"POST /admin/text/archive/{operation}":    m.textArchive,
	}
	for path, handler := range routes {
		guarded := m.requireRole("admin", true)(handler)
		mux.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Bound the encoded form before authentication can parse a CSRF field.
			limit := m.cfg.Tuning.TextCatalog.MaxBundleBytes
			if limit < 1 || limit > 1<<30 {
				http.Error(w, "Text content limits are unavailable.", 503)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 3*limit+(64<<10))
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			guarded.ServeHTTP(w, r.WithContext(ctx))
		}))
	}
}

type textReleaseView struct {
	ID, Language, Rules, Class, Key, Hash string
	Withdrawn, Active                     bool
}
type textInputView struct{ ID, Kind, SourceID, Text string }
type textContentView struct {
	Releases                   []textReleaseView
	Inputs                     []textInputView
	Phase                      string
	Expected, Copied, Verified int64
}

func (m *Manager) textContentPage(w http.ResponseWriter, r *http.Request) {
	data := textContentView{Phase: "not started"}
	rows, err := m.db.QueryContext(r.Context(), `SELECT r.release_id,r.language,r.rules_version,r.access_class,r.entitlement_key,r.snapshot_sha256,r.withdrawn_at IS NOT NULL,EXISTS(SELECT 1 FROM text_active_releases a WHERE a.release_id=r.release_id) FROM text_releases r ORDER BY r.published_at DESC,r.release_id LIMIT 100`)
	if err != nil {
		http.Error(w, "Text releases unavailable.", 500)
		return
	}
	for rows.Next() {
		var v textReleaseView
		if err = rows.Scan(&v.ID, &v.Language, &v.Rules, &v.Class, &v.Key, &v.Hash, &v.Withdrawn, &v.Active); err != nil {
			break
		}
		data.Releases = append(data.Releases, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		http.Error(w, "Text releases unavailable.", 500)
		return
	}
	rows, err = m.db.QueryContext(r.Context(), `SELECT id,source_kind,source_id,text_content FROM text_accepted_inputs ORDER BY id LIMIT 100`)
	if err != nil {
		http.Error(w, "Accepted inputs unavailable.", 500)
		return
	}
	for rows.Next() {
		var v textInputView
		if err = rows.Scan(&v.ID, &v.Kind, &v.SourceID, &v.Text); err != nil {
			break
		}
		data.Inputs = append(data.Inputs, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		http.Error(w, "Accepted inputs unavailable.", 500)
		return
	}
	err = m.db.QueryRowContext(r.Context(), `SELECT phase,expected_count,copy_count,verify_count FROM text_archive_progress WHERE job_id=1`).Scan(&data.Phase, &data.Expected, &data.Copied, &data.Verified)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, "Archive status unavailable.", 500)
		return
	}
	adminPage(w, r, "Text releases", "Capture accepted wording, publish certified bundles, and choose what new matches may use.", textContentBody, data)
}
func (m *Manager) textInputCapture(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		adminError(w, r, errors.New("invalid request"), "/admin/text")
		return
	}
	if _, err := m.textContent.CaptureAccepted(r.Context(), adminIDFromContext(r.Context()), r.FormValue("source_kind"), r.FormValue("source_id")); err != nil {
		adminError(w, r, err, "/admin/text")
		return
	}
	http.Redirect(w, r, "/admin/text", http.StatusSeeOther)
}
func (m *Manager) textInputExport(w http.ResponseWriter, r *http.Request) {
	var text string
	var provenance json.RawMessage
	if err := m.db.QueryRowContext(r.Context(), `SELECT text_content,provenance FROM text_accepted_inputs WHERE id=$1`, r.PathValue("id")).Scan(&text, &provenance); err != nil {
		http.Error(w, "Accepted input unavailable.", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="accepted-text.json"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := json.NewEncoder(w).Encode(struct {
		Text       string          `json:"text"`
		Provenance json.RawMessage `json:"provenance"`
	}{text, provenance}); err != nil {
		return
	}
}
func (m *Manager) textPublish(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		adminError(w, r, errors.New("invalid request"), "/admin/text")
		return
	}
	t := m.cfg.Tuning
	limits := media.TextLimits{MaxTextBytes: t.Contract.MaxTextBytes, MaxRecords: t.TextCatalog.MaxRecords, MaxFileBytes: t.TextCatalog.MaxFileBytes, MaxBundleBytes: t.TextCatalog.MaxBundleBytes}
	bundle, err := media.DecodeTextBundle([]byte(r.FormValue("bundle")), limits)
	var snap *media.TextSnapshot
	if err == nil {
		snap, err = media.NewTextSnapshot(bundle, limits)
	}
	if err == nil {
		err = m.textContent.Publish(r.Context(), adminIDFromContext(r.Context()), snap, store.TextPackAccess{Class: r.FormValue("access_class"), EntitlementKey: r.FormValue("entitlement_key")})
	}
	if err != nil {
		adminError(w, r, errors.New("invalid certified bundle or unavailable accepted source"), "/admin/text")
		return
	}
	http.Redirect(w, r, "/admin/text", http.StatusSeeOther)
}
func (m *Manager) textActivate(w http.ResponseWriter, r *http.Request) {
	if err := m.textContent.Activate(r.Context(), adminIDFromContext(r.Context()), r.PathValue("id")); err != nil {
		adminError(w, r, err, "/admin/text")
		return
	}
	http.Redirect(w, r, "/admin/text", http.StatusSeeOther)
}
func (m *Manager) textWithdraw(w http.ResponseWriter, r *http.Request) {
	if err := m.textContent.Takedown(r.Context(), adminIDFromContext(r.Context()), r.PathValue("id"), r.FormValue("reason")); err != nil {
		adminError(w, r, err, "/admin/text")
		return
	}
	http.Redirect(w, r, "/admin/text", http.StatusSeeOther)
}
func (m *Manager) textArchive(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(r.FormValue("limit"))
	if err != nil || limit < 1 || limit > 1000 {
		adminError(w, r, fmt.Errorf("invalid batch size: use 1 to 1000"), "/admin/text")
		return
	}
	archive := store.NewTextArchiveStore(m.db)
	admin := adminIDFromContext(r.Context())
	switch r.PathValue("operation") {
	case "start":
		err = archive.StartAs(r.Context(), admin, limit)
	case "batch":
		_, err = archive.BatchAs(r.Context(), admin, limit)
	default:
		err = errors.New("invalid archive operation")
	}
	if err != nil {
		adminError(w, r, err, "/admin/text")
		return
	}
	http.Redirect(w, r, "/admin/text", http.StatusSeeOther)
}

const textContentBody = `<h2>Publish a certified bundle</h2><p>Publication stores an immutable release. Activation separately selects it for new matches. Existing matches keep their pinned wording.</p><form method="post" action="/admin/text/publish"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label>Certified bundle JSON<textarea name="bundle" required spellcheck="false"></textarea></label><label>Access<select name="access_class"><option value="core">Free core</option><option value="featured">Free featured</option><option value="theme">Owned theme</option></select></label><label>Theme entitlement key (theme only)<input name="entitlement_key"></label><button>Validate & publish</button></form><h2>Latest 100 releases</h2>{{range .Data.Releases}}<article class="panel"><h3>{{.ID}}</h3><p>{{.Language}} · {{.Rules}} · {{.Class}} {{.Key}}</p><p class="small">Snapshot <code>{{.Hash}}</code></p>{{if .Withdrawn}}<p class="status">Withdrawn</p>{{else}}{{if .Active}}<p class="status">Active for new matches</p>{{else}}<form method="post" action="/admin/text/releases/{{.ID}}/activate"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><button>Activate for new matches</button></form>{{end}}<form method="post" action="/admin/text/releases/{{.ID}}/withdraw"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><label>Withdrawal reason<input name="reason" maxlength="1024" required></label><button class="danger">Withdraw release</button></form>{{end}}</article>{{else}}<p class="empty">No certified text release has been published.</p>{{end}}<h2>Capture accepted text</h2><p>Rescreen an approved contribution and retain its exact wording, original consent, attribution and human decision. This never grants another approval reward.</p><form method="post" action="/admin/text/capture"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label>Source<select name="source_kind"><option value="portal_submission">Contributor submission</option><option value="challenge_entry">Challenge entry</option></select></label><label>Source ID<input name="source_id" required></label><button>Rescreen & capture</button></form><h2>Latest 100 captured inputs</h2>{{range .Data.Inputs}}<article class="panel"><p class="preview">{{.Text}}</p><p class="small">{{.Kind}} · {{.SourceID}}</p><a href="/admin/text/inputs/{{.ID}}">Download exact text and provenance</a></article>{{else}}<p class="empty">No accepted input has been captured.</p>{{end}}<h2>Legacy archive</h2><p>Status: <strong>{{.Data.Phase}}</strong> · {{.Data.Copied}} copied · {{.Data.Verified}} verified · {{.Data.Expected}} expected.</p><p>Starting captures the source inventory and holds contribution writes until copy and verification finish. Each batch resumes saved progress. Archived image records remain historical records.</p>{{if eq .Data.Phase "not started"}}<form method="post" action="/admin/text/archive/start"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label>Records per page<input type="number" name="limit" min="1" max="1000" value="100" required></label><button>Capture archive inventory</button></form>{{else if ne .Data.Phase "complete"}}<form method="post" action="/admin/text/archive/batch"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label>Records in this batch<input type="number" name="limit" min="1" max="1000" value="100" required></label><button>Run next batch</button></form>{{end}}`
