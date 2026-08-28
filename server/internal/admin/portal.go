package admin

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/knowoff/knowoff/server/internal/portal"
)

// PortalHandler returns admin routes for the contributor portal mounted at /admin/portal/.
func (m *Manager) PortalHandler(portalMgr *portal.Manager) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /admin/portal/", m.requireRole("admin", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalIndex(w, r, portalMgr) })))
	mux.Handle("GET /admin/portal/terms", m.requireRole("admin", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalTerms(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/terms", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalTermsCreate(w, r, portalMgr) })))
	mux.Handle("GET /admin/portal/applications", m.requireRole("admin", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalApplications(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/applications/{id}/approve", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalApplicationApprove(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/applications/{id}/reject", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalApplicationReject(w, r, portalMgr) })))
	mux.Handle("GET /admin/portal/submissions", m.requireRole("admin", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalSubmissions(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/submissions/{id}/approve", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalSubmissionApprove(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/submissions/{id}/reject", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalSubmissionReject(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/submissions/{id}/publish", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalSubmissionPublish(w, r, portalMgr) })))
	mux.Handle("GET /admin/portal/freezes", m.requireRole("admin", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalFreezes(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/freezes/{id}/dismiss", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalFreezeDismiss(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/freezes/{id}/ban", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalFreezeBan(w, r, portalMgr) })))
	mux.Handle("GET /admin/portal/challenge", m.requireRole("admin", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalChallenge(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/challenge/topic", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalChallengeTopicCreate(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/challenge/{id}/close", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalChallengeClose(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/challenge/entries/{id}/approve", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalChallengeEntryApprove(w, r, portalMgr) })))
	mux.Handle("POST /admin/portal/challenge/entries/{id}/reject", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.portalChallengeEntryReject(w, r, portalMgr) })))
	return mux
}

func (m *Manager) portalIndex(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	fmt.Fprint(w, `<!doctype html>
<html><head><title>Portal Admin</title></head><body>
<h1>Portal Administration</h1>
<nav>
<a href="/admin/portal/terms">Terms</a> |
<a href="/admin/portal/applications">Applications</a> |
<a href="/admin/portal/submissions">Submissions</a> |
<a href="/admin/portal/freezes">Guard Freezes</a> |
<a href="/admin/portal/challenge">Challenge</a>
</nav>
</body></html>`)
}

func (m *Manager) portalTerms(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	terms, err := pm.ListTermsVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	csrf := csrfFromContext(r.Context())
	fmt.Fprint(w, `<!doctype html>
<html><head><title>Contribution Terms</title></head><body>
<h1>Contribution Terms Versions</h1>
<table border="1"><tr><th>Version</th><th>Title</th><th>Active From</th></tr>`)
	for _, t := range terms {
		fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td></tr>`,
			template.HTMLEscapeString(t.Version),
			template.HTMLEscapeString(t.Title),
			t.ActiveFrom.Format(time.RFC3339))
	}
	fmt.Fprintf(w, `</table>
<h2>New version</h2>
<form method="post" action="/admin/portal/terms">
<input type="hidden" name="csrf_token" value="%s">
<label>Version <input name="version" required></label><br>
<label>Title <input name="title" required></label><br>
<label>Body <textarea name="body" required></textarea></label><br>
<label>Active from (YYYY-MM-DD) <input name="active_from" required></label><br>
<button>Create</button>
</form>
</body></html>`, template.HTMLEscapeString(csrf))
}

func (m *Manager) portalTermsCreate(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	activeFrom, err := time.Parse("2006-01-02", r.FormValue("active_from"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	adminID := adminIDFromContext(r.Context())
	if err := pm.CreateTermsVersion(r.Context(), adminID, r.FormValue("version"), r.FormValue("title"), r.FormValue("body"), activeFrom); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/terms", http.StatusSeeOther)
}

func (m *Manager) portalApplications(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	apps, err := pm.ListApplications(r.Context(), "pending")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	csrf := csrfFromContext(r.Context())
	fmt.Fprint(w, `<!doctype html>
<html><head><title>Role Applications</title></head><body>
<h1>Pending Role Applications</h1>
<table border="1"><tr><th>Account</th><th>Role</th><th>Applied</th><th>Actions</th></tr>`)
	for _, a := range apps {
		fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td>
<form method="post" action="/admin/portal/applications/%s/approve" style="display:inline"><input type="hidden" name="csrf_token" value="%s"><button>Approve</button></form>
<form method="post" action="/admin/portal/applications/%s/reject" style="display:inline"><input type="hidden" name="csrf_token" value="%s"><input name="reason" placeholder="reason" required><button>Reject</button></form>
</td></tr>`,
			template.HTMLEscapeString(a.AccountID), template.HTMLEscapeString(string(a.Role)),
			a.AppliedAt.Format(time.RFC3339), template.HTMLEscapeString(a.ID),
			template.HTMLEscapeString(csrf), template.HTMLEscapeString(a.ID), template.HTMLEscapeString(csrf))
	}
	fmt.Fprint(w, `</table></body></html>`)
}

func (m *Manager) portalApplicationApprove(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	app, err := pm.GetApplication(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "application not found", http.StatusNotFound)
		return
	}
	adminID := adminIDFromContext(r.Context())
	if err := pm.GrantRole(r.Context(), adminID, app.AccountID, app.Role); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/applications", http.StatusSeeOther)
}

func (m *Manager) portalApplicationReject(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	adminID := adminIDFromContext(r.Context())
	if err := pm.RejectApplication(r.Context(), adminID, r.PathValue("id"), r.FormValue("reason")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/applications", http.StatusSeeOther)
}

func (m *Manager) portalSubmissions(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	subs, err := pm.ListSubmissions(r.Context(), "", "submitted")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	csrf := csrfFromContext(r.Context())
	fmt.Fprint(w, `<!doctype html>
<html><head><title>Submissions</title></head><body>
<h1>Submitted Media</h1>
<table border="1"><tr><th>ID</th><th>Account</th><th>Type</th><th>Content</th><th>Actions</th></tr>`)
	for _, s := range subs {
		fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>
<form method="post" action="/admin/portal/submissions/%s/approve" style="display:inline"><input type="hidden" name="csrf_token" value="%s"><button>Approve</button></form>
<form method="post" action="/admin/portal/submissions/%s/reject" style="display:inline"><input type="hidden" name="csrf_token" value="%s"><button>Reject</button></form>
</td></tr>`,
			template.HTMLEscapeString(s.ID), template.HTMLEscapeString(s.AccountID),
			template.HTMLEscapeString(string(s.MediaType)), template.HTMLEscapeString(s.Content),
			template.HTMLEscapeString(s.ID), template.HTMLEscapeString(csrf),
			template.HTMLEscapeString(s.ID), template.HTMLEscapeString(csrf))
	}
	fmt.Fprint(w, `</table></body></html>`)
}

func (m *Manager) portalSubmissionApprove(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	adminID := adminIDFromContext(r.Context())
	if err := pm.DecideSubmission(r.Context(), adminID, r.PathValue("id"), true, ""); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) portalSubmissionReject(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	adminID := adminIDFromContext(r.Context())
	if err := pm.DecideSubmission(r.Context(), adminID, r.PathValue("id"), false, "rejected in admin console"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) portalSubmissionPublish(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	adminID := adminIDFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := pm.PublishSubmission(r.Context(), adminID, r.PathValue("id"), r.FormValue("pack_tag")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) portalFreezes(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	freezes, err := pm.ListActiveFreezes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	csrf := csrfFromContext(r.Context())
	fmt.Fprint(w, `<!doctype html>
<html><head><title>Guard Freezes</title></head><body>
<h1>Active Guard Freezes</h1>
<table border="1"><tr><th>Account</th><th>Guard</th><th>Reason</th><th>Expires</th><th>Actions</th></tr>`)
	for _, f := range freezes {
		id := fmt.Sprintf("%v", f["id"])
		fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%v</td><td>
<form method="post" action="/admin/portal/freezes/%s/dismiss" style="display:inline"><input type="hidden" name="csrf_token" value="%s"><button>Dismiss</button></form>
<form method="post" action="/admin/portal/freezes/%s/ban" style="display:inline"><input type="hidden" name="csrf_token" value="%s"><button>Ban</button></form>
</td></tr>`,
			template.HTMLEscapeString(fmt.Sprintf("%v", f["account_id"])),
			template.HTMLEscapeString(fmt.Sprintf("%v", f["frozen_by"])),
			template.HTMLEscapeString(fmt.Sprintf("%v", f["reason"])),
			f["expires_at"],
			template.HTMLEscapeString(id), template.HTMLEscapeString(csrf),
			template.HTMLEscapeString(id), template.HTMLEscapeString(csrf))
	}
	fmt.Fprint(w, `</table></body></html>`)
}

func (m *Manager) portalFreezeDismiss(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	adminID := adminIDFromContext(r.Context())
	if err := pm.DismissFreeze(r.Context(), adminID, r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/freezes", http.StatusSeeOther)
}

func (m *Manager) portalFreezeBan(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	adminID := adminIDFromContext(r.Context())
	if err := pm.ConvertFreezeToBan(r.Context(), adminID, r.PathValue("id"), "converted to ban by admin"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/freezes", http.StatusSeeOther)
}

func (m *Manager) portalChallenge(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	topic, err := pm.ActiveChallengeTopic(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	csrf := csrfFromContext(r.Context())
	fmt.Fprint(w, `<!doctype html>
<html><head><title>Challenge Admin</title></head><body>
<h1>Weekly Nown Challenge</h1>`)
	if topic != nil {
		fmt.Fprintf(w, `<p>Active topic: %s (%s - %s)</p>`, topic.ID, topic.WeekStart.Format("2006-01-02"), topic.WeekEnd.Format("2006-01-02"))
		entries, _ := pm.ListChallengeEntries(r.Context(), topic.ID)
		fmt.Fprint(w, `<table border="1"><tr><th>Entry</th><th>Account</th><th>Votes</th><th>Actions</th></tr>`)
		for _, e := range entries {
			fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%d</td><td>
<form method="post" action="/admin/portal/challenge/entries/%s/approve" style="display:inline"><input type="hidden" name="csrf_token" value="%s"><button>Approve</button></form>
<form method="post" action="/admin/portal/challenge/entries/%s/reject" style="display:inline"><input type="hidden" name="csrf_token" value="%s"><button>Reject</button></form>
</td></tr>`,
				template.HTMLEscapeString(e.Content), template.HTMLEscapeString(e.AccountID), e.VoteCount,
				template.HTMLEscapeString(e.ID), template.HTMLEscapeString(csrf),
				template.HTMLEscapeString(e.ID), template.HTMLEscapeString(csrf))
		}
		fmt.Fprintf(w, `</table>
<form method="post" action="/admin/portal/challenge/%s/close"><input type="hidden" name="csrf_token" value="%s"><button>Close week</button></form>`,
			topic.ID, template.HTMLEscapeString(csrf))
	}
	fmt.Fprintf(w, `<h2>New topic</h2>
<form method="post" action="/admin/portal/challenge/topic">
<input type="hidden" name="csrf_token" value="%s">
<label>Week start (YYYY-MM-DD) <input name="week_start" required></label><br>
<label>Nown media ID <input name="nown_media_id" required></label><br>
<button>Create</button>
</form>
</body></html>`, template.HTMLEscapeString(csrf))
}

func (m *Manager) portalChallengeTopicCreate(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	weekStart, err := time.Parse("2006-01-02", r.FormValue("week_start"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	adminID := adminIDFromContext(r.Context())
	if _, err := pm.CreateChallengeTopic(r.Context(), adminID, weekStart, r.FormValue("nown_media_id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/challenge", http.StatusSeeOther)
}

func (m *Manager) portalChallengeClose(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	adminID := adminIDFromContext(r.Context())
	if _, err := pm.CloseChallengeWeek(r.Context(), adminID, r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/challenge", http.StatusSeeOther)
}

func (m *Manager) portalChallengeEntryApprove(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	adminID := adminIDFromContext(r.Context())
	if err := pm.ApproveChallengeEntry(r.Context(), adminID, r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/challenge", http.StatusSeeOther)
}

func (m *Manager) portalChallengeEntryReject(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	adminID := adminIDFromContext(r.Context())
	if err := pm.RejectChallengeEntry(r.Context(), adminID, r.PathValue("id"), "rejected in admin console"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/portal/challenge", http.StatusSeeOther)
}
