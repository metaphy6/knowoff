package admin

import (
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/knowoff/knowoff/server/internal/portal"
)

// PortalHandler returns admin routes for the contributor portal mounted at /admin/portal/.
func (m *Manager) PortalHandler(portalMgr *portal.Manager) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /admin/portal/roles/{account_id}/revoke", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := portalMgr.RevokeRole(r.Context(), adminIDFromContext(r.Context()), r.PathValue("account_id"), portal.Role(r.FormValue("role"))); err != nil {
			adminError(w, r, err, "/admin/portal/applications")
			return
		}
		http.Redirect(w, r, "/admin/portal/applications", http.StatusSeeOther)
	})))
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
	http.Redirect(w, r, "/admin/portal/applications", http.StatusSeeOther)
}

func (m *Manager) portalTerms(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	terms, err := pm.ListTermsVersions(r.Context())
	if err != nil {
		http.Error(w, "Terms unavailable.", 500)
		return
	}
	adminPage(w, r, "Contribution terms", "The agreement contributors actually see and accept.", termsBody, map[string]any{"Terms": terms})
}

func (m *Manager) portalTermsCreate(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	activeFrom, err := time.Parse("2006-01-02", r.FormValue("active_from"))
	if err != nil {
		adminError(w, r, err, "/admin/portal/")
		return
	}
	adminID := adminIDFromContext(r.Context())
	if err := pm.CreateTermsVersion(r.Context(), adminID, r.FormValue("version"), r.FormValue("title"), r.FormValue("body"), activeFrom); err != nil {
		adminError(w, r, err, "/admin/portal/")
		return
	}
	http.Redirect(w, r, "/admin/portal/terms", http.StatusSeeOther)
}

func (m *Manager) portalApplications(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	apps, err := pm.ListApplications(r.Context(), "")
	if err != nil {
		http.Error(w, "Applications unavailable.", 500)
		return
	}
	grants, err := pm.ListRoleGrants(r.Context())
	if err != nil {
		http.Error(w, "Roles unavailable.", 500)
		return
	}
	adminPage(w, r, "Roles & applications", "Review the people who want to help build Knowoff.", applicationsBody, map[string]any{"Applications": apps, "Grants": grants})
}

func (m *Manager) portalApplicationApprove(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	app, err := pm.GetApplication(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "application not found", http.StatusNotFound)
		return
	}
	adminID := adminIDFromContext(r.Context())
	if err := pm.ApproveApplication(r.Context(), adminID, app.ID); err != nil {
		adminError(w, r, err, "/admin/portal/")
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
		adminError(w, r, err, "/admin/portal/")
		return
	}
	http.Redirect(w, r, "/admin/portal/applications", http.StatusSeeOther)
}

func (m *Manager) portalSubmissions(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	subs, err := pm.ListSubmissions(r.Context(), "", "")
	if err != nil {
		http.Error(w, "Submissions unavailable.", 500)
		return
	}
	type reviewedSubmission struct {
		portal.Submission
		Revision string
	}
	reviewed := []reviewedSubmission{}
	for _, s := range subs {
		if s.Status != portal.StatusDraft {
			reviewed = append(reviewed, reviewedSubmission{s, portal.ContentRevision(s.Content)})
		}
	}
	adminPage(w, r, "Submission review", "Give a good idea a careful second look.", submissionsReviewBody, map[string]any{"Submissions": reviewed})
}

func (m *Manager) portalSubmissionApprove(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	revision := r.FormValue("revision")
	if raw, err := hex.DecodeString(revision); err != nil || len(raw) != 32 {
		adminError(w, r, errors.New("reviewed content revision required"), "/admin/portal/submissions")
		return
	}
	if r.FormValue("human_reviewed") != "yes" {
		adminError(w, r, errors.New("human review confirmation required"), "/admin/portal/submissions")
		return
	}
	adminID := adminIDFromContext(r.Context())
	if err := pm.DecideSubmission(r.Context(), adminID, r.PathValue("id"), true, "", revision); err != nil {
		adminError(w, r, err, "/admin/portal/")
		return
	}
	http.Redirect(w, r, "/admin/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) portalSubmissionReject(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	adminID := adminIDFromContext(r.Context())
	if err := pm.DecideSubmission(r.Context(), adminID, r.PathValue("id"), false, r.FormValue("reason")); err != nil {
		adminError(w, r, err, "/admin/portal/")
		return
	}
	http.Redirect(w, r, "/admin/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) portalSubmissionPublish(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	http.Error(w, "This operation is not available until its product enforcement or pack-deployment workflow is implemented.", http.StatusNotImplemented)
}

func (m *Manager) portalFreezes(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	freezes, err := pm.ListActiveFreezes(r.Context())
	if err != nil {
		http.Error(w, "Freeze queue unavailable.", 503)
		return
	}
	adminPage(w, r, "Guard enforcement", "Review each independent freeze. Dismissal never clears an existing admin ban.", freezeAdminBody, map[string]any{"Freezes": freezes})
}

func (m *Manager) portalFreezeDismiss(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	if err := pm.DismissFreeze(r.Context(), adminIDFromContext(r.Context()), r.PathValue("id")); err != nil {
		adminError(w, r, err, "/admin/portal/freezes")
		return
	}
	http.Redirect(w, r, "/admin/portal/freezes", http.StatusSeeOther)
}

func (m *Manager) portalFreezeBan(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	var err error
	switch r.FormValue("decision") {
	case "permanent":
		err = pm.ConvertFreezeToBan(r.Context(), adminIDFromContext(r.Context()), r.PathValue("id"), r.FormValue("reason"))
	case "timed":
		var until time.Time
		until, err = time.Parse(time.RFC3339, r.FormValue("until"))
		if err == nil {
			err = pm.ConvertFreezeToTimedBan(r.Context(), adminIDFromContext(r.Context()), r.PathValue("id"), r.FormValue("reason"), until)
		}
	default:
		err = errors.New("choose timed or permanent ban")
	}
	if err != nil {
		adminError(w, r, err, "/admin/portal/freezes")
		return
	}
	http.Redirect(w, r, "/admin/portal/freezes", http.StatusSeeOther)
}

const freezeAdminBody = `{{range .Data.Freezes}}<article class="panel"><h2>Account <code>{{.account_id}}</code></h2><p>Guard <code>{{.frozen_by}}</code> · expires {{.expires_at}}</p><p>{{.reason}}</p><form method="post" action="/admin/portal/freezes/{{.id}}/dismiss"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><button>Dismiss this freeze</button></form><form method="post" action="/admin/portal/freezes/{{.id}}/ban"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><label for="decision-{{.id}}">Final decision</label><select id="decision-{{.id}}" name="decision"><option value="timed">Timed ban</option><option value="permanent">Permanent ban</option></select><label for="until-{{.id}}">Timed ban end, RFC3339 with timezone</label><input id="until-{{.id}}" name="until" placeholder="2026-09-20T12:00:00Z"><label for="reason-{{.id}}">Decision reason</label><textarea id="reason-{{.id}}" name="reason" required></textarea><button>Apply final decision</button></form></article>{{else}}<p>No active freezes.</p>{{end}}`

func (m *Manager) portalChallenge(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	topic, err := pm.CurrentChallengeTopic(r.Context())
	if err != nil {
		http.Error(w, "Challenge unavailable.", 500)
		return
	}
	var entries []portal.ChallengeEntry
	if topic != nil {
		entries, err = pm.ListChallengeReviewEntries(r.Context(), topic.ID)
		if err != nil {
			http.Error(w, "Entries unavailable.", 500)
			return
		}
	}
	subs, err := pm.ListSubmissions(r.Context(), "", "approved")
	if err != nil {
		http.Error(w, "Topic media unavailable.", 500)
		return
	}
	var topics []portal.Submission
	for _, s := range subs {
		if s.MediaType == portal.MediaText {
			topics = append(topics, s)
		}
	}
	adminPage(w, r, "Weekly Nown Challenge", "Schedule the topic, screen responses, and close the week.", challengeAdminBody, map[string]any{"Topic": topic, "Entries": entries, "Topics": topics})
}

func (m *Manager) portalChallengeTopicCreate(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	weekStart, err := time.Parse("2006-01-02", r.FormValue("week_start"))
	if err != nil {
		adminError(w, r, err, "/admin/portal/")
		return
	}
	adminID := adminIDFromContext(r.Context())
	if _, err := pm.CreateChallengeTopic(r.Context(), adminID, weekStart, r.FormValue("nown_media_id")); err != nil {
		adminError(w, r, err, "/admin/portal/")
		return
	}
	http.Redirect(w, r, "/admin/portal/challenge", http.StatusSeeOther)
}

func (m *Manager) portalChallengeClose(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	if r.FormValue("confirm_close") != "yes" {
		adminError(w, r, errors.New("week closure confirmation required"), "/admin/portal/challenge")
		return
	}
	adminID := adminIDFromContext(r.Context())
	if _, err := pm.CloseChallengeWeek(r.Context(), adminID, r.PathValue("id")); err != nil {
		adminError(w, r, err, "/admin/portal/")
		return
	}
	http.Redirect(w, r, "/admin/portal/challenge", http.StatusSeeOther)
}

func (m *Manager) portalChallengeEntryApprove(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	if r.FormValue("human_reviewed") != "yes" {
		adminError(w, r, errors.New("human review confirmation required"), "/admin/portal/challenge")
		return
	}
	adminID := adminIDFromContext(r.Context())
	if err := pm.ApproveChallengeEntry(r.Context(), adminID, r.PathValue("id")); err != nil {
		adminError(w, r, err, "/admin/portal/")
		return
	}
	http.Redirect(w, r, "/admin/portal/challenge", http.StatusSeeOther)
}

func (m *Manager) portalChallengeEntryReject(w http.ResponseWriter, r *http.Request, pm *portal.Manager) {
	adminID := adminIDFromContext(r.Context())
	if err := pm.RejectChallengeEntry(r.Context(), adminID, r.PathValue("id"), r.FormValue("reason")); err != nil {
		adminError(w, r, err, "/admin/portal/")
		return
	}
	http.Redirect(w, r, "/admin/portal/challenge", http.StatusSeeOther)
}
