package portal

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/pkg/media"
)

// Handler returns the public contributor portal HTTP handler mounted at /portal/.
func (m *Manager) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /portal/submissions/{id}/edit", m.requireAuth(m.submissionEdit))
	mux.HandleFunc("GET /portal/login", m.loginForm)
	mux.HandleFunc("POST /portal/session", m.loginContinue)
	mux.HandleFunc("POST /portal/logout", m.requireAuth(m.logoutPost))
	mux.HandleFunc("GET /portal/", m.requireAuth(m.index))
	mux.HandleFunc("GET /portal/apply", m.requireAuth(m.applyForm))
	mux.HandleFunc("POST /portal/apply", m.requireAuth(m.applyPost))
	mux.HandleFunc("GET /portal/submissions", m.requireAuth(m.submissionsList))
	mux.HandleFunc("POST /portal/submissions", m.requireAuth(m.submissionCreate))
	mux.HandleFunc("POST /portal/submissions/{id}/submit", m.requireAuth(m.submissionSubmit))
	mux.HandleFunc("POST /portal/submissions/{id}/withdraw", m.requireAuth(m.submissionWithdraw))
	mux.HandleFunc("GET /portal/simulate", m.requireRole(RoleCurator, m.simulateForm))
	mux.HandleFunc("POST /portal/simulate", m.requireRole(RoleCurator, m.simulateRun))
	mux.HandleFunc("GET /portal/challenge", m.requireAuth(m.challengeView))
	mux.HandleFunc("POST /portal/challenge/entry", m.requireAuth(m.challengeEntryPost))
	mux.HandleFunc("POST /portal/challenge/vote", m.requireAuth(m.challengeVotePost))
	return mux
}

func (m *Manager) requireRole(role Role, next http.HandlerFunc) http.HandlerFunc {
	return m.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		accountID := accountIDFromContext(r.Context())
		has, err := m.HasRole(r.Context(), accountID, role)
		if err != nil || !has {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

type ctxAccountIDKey struct{}

func accountIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxAccountIDKey{}).(string); ok {
		return v
	}
	return ""
}

func (m *Manager) index(w http.ResponseWriter, r *http.Request) {
	id := accountIDFromContext(r.Context())
	role, err := m.ActiveRole(r.Context(), id)
	if err != nil {
		http.Error(w, "Studio unavailable. Please try again.", 500)
		return
	}
	subs, err := m.ListSubmissions(r.Context(), id, "")
	if err != nil {
		http.Error(w, "Studio unavailable. Please try again.", 500)
		return
	}
	canSimulate, err := m.HasRole(r.Context(), id, RoleCurator)
	if err != nil {
		http.Error(w, "Roles unavailable.", 500)
		return
	}
	portalPage(w, r, "Contributor studio", "Your ideas, a little editorial discipline, and a suspicious amount of personality.", overviewBody, map[string]any{"Role": role, "Submissions": len(subs), "CanSimulate": canSimulate})
}

func (m *Manager) applyForm(w http.ResponseWriter, r *http.Request) {
	id := accountIDFromContext(r.Context())
	p, err := m.profile.Get(r.Context(), id, true)
	if err != nil {
		http.Error(w, "Profile unavailable. Please try again.", 500)
		return
	}
	role, err := m.ActiveRole(r.Context(), id)
	if err != nil {
		http.Error(w, "Roles unavailable.", 500)
		return
	}
	apps, err := m.ListOwnApplications(r.Context(), id)
	if err != nil {
		http.Error(w, "Applications unavailable.", 500)
		return
	}
	portalPage(w, r, "Roles & applications", "Find your place at the table behind the table.", applicationBody, map[string]any{"Role": role, "Level": p.Level, "MinLevel": m.cfg.Tuning.Portal.MinAccountLevelToApply, "Eligible": p.Level >= m.cfg.Tuning.Portal.MinAccountLevelToApply, "Applications": apps})
}

func (m *Manager) applyPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	accountID := accountIDFromContext(r.Context())
	if err := m.ApplyForRole(r.Context(), accountID, Role(r.FormValue("role"))); err != nil {
		portalError(w, r, err, "/portal/apply")
		return
	}
	http.Redirect(w, r, "/portal/apply", http.StatusSeeOther)
}

func (m *Manager) submissionsList(w http.ResponseWriter, r *http.Request) {
	id := accountIDFromContext(r.Context())
	subs, err := m.ListSubmissions(r.Context(), id, "")
	if err != nil {
		http.Error(w, "Submissions unavailable.", 500)
		return
	}
	terms, err := m.CurrentTerms(r.Context())
	if err != nil {
		http.Error(w, "Contribution terms are not available yet.", 503)
		return
	}
	can, err := m.HasRole(r.Context(), id, RoleContributor)
	if err != nil {
		http.Error(w, "Roles unavailable.", 500)
		return
	}
	portalPage(w, r, "My submissions", "Good ideas can stay drafts. Great ones make it into review.", submissionsBody, map[string]any{"Submissions": subs, "Terms": terms, "CanContribute": can, "MaxLength": m.MaxTextBytes()})
}

func (m *Manager) submissionCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	accountID := accountIDFromContext(r.Context())
	if _, err := m.CreateDraft(r.Context(), accountID, MediaText, r.FormValue("content"), ContributionConsent{Version: r.FormValue("terms_version"), Accepted: r.FormValue("terms_accepted") == "yes"}); err != nil {
		portalError(w, r, err, "/portal/submissions")
		return
	}
	http.Redirect(w, r, "/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) submissionSubmit(w http.ResponseWriter, r *http.Request) {
	accountID := accountIDFromContext(r.Context())
	id := r.PathValue("id")
	if err := m.SubmitDraft(r.Context(), accountID, id); err != nil {
		portalError(w, r, err, "/portal/submissions")
		return
	}
	http.Redirect(w, r, "/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) submissionWithdraw(w http.ResponseWriter, r *http.Request) {
	accountID := accountIDFromContext(r.Context())
	id := r.PathValue("id")
	if err := m.WithdrawSubmission(r.Context(), accountID, id); err != nil {
		portalError(w, r, err, "/portal/submissions")
		return
	}
	http.Redirect(w, r, "/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) simulateForm(w http.ResponseWriter, r *http.Request) {
	portalPage(w, r, "Deal simulator", "Test a Nown with the same dealer the game uses.", simulatorBody, nil)
}

func (m *Manager) simulateRun(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	tableSize, err := strconv.Atoi(r.FormValue("table_size"))
	if err != nil || (tableSize != 4 && tableSize != 6) {
		http.Error(w, "invalid table size", http.StatusBadRequest)
		return
	}
	nownID := r.FormValue("nown_id")
	if nownID == "" {
		http.Error(w, "nown id required", http.StatusBadRequest)
		return
	}
	seed, err := strconv.ParseInt(r.FormValue("seed"), 10, 64)
	if err != nil {
		seed = 42
	}

	if m.media == nil {
		http.Error(w, "The media pack is not available yet.", http.StatusServiceUnavailable)
		return
	}
	pack := m.media.Active()
	if pack == nil {
		http.Error(w, "no media pack loaded", http.StatusServiceUnavailable)
		return
	}
	if pack.MediaByID(nownID) == nil {
		http.Error(w, "nown not found in active pack", http.StatusNotFound)
		return
	}

	dealing := media.DealingTuning{
		BandHigh:          m.cfg.Tuning.Dealing.BandHigh,
		BandLow:           m.cfg.Tuning.Dealing.BandLow,
		MinHighPerNown:    m.cfg.Tuning.Dealing.MinHighPerNown,
		MinDistantPerNown: m.cfg.Tuning.Dealing.MinDistantPerNown,
	}
	dealer := media.NewDealer(pack, dealing)
	dealer.Hand = media.HandTuning{Size: m.cfg.Tuning.Hand.Size, DrawPile: m.cfg.Tuning.Hand.DrawPile}
	rng := rand.New(rand.NewSource(seed))
	result, err := dealer.Deal(tableSize, []string{nownID}, rng)
	if err != nil {
		http.Error(w, fmt.Sprintf("deal failed: %v", err), http.StatusBadRequest)
		return
	}

	type seatHand struct {
		Seat        int
		Cards, Draw string
	}
	var hands []seatHand
	for i, hand := range result.Hands {
		hands = append(hands, seatHand{i + 1, joinIDs(hand.Cards), joinIDs(hand.DrawPile)})
	}
	portalPage(w, r, "Deal simulator result", "A deterministic deal from the active pack.", `<p>Pack <code>{{.Data.Pack}}</code> · Nown <code>{{.Data.Nown}}</code> · Seed {{.Data.Seed}}</p><div class="table-scroll"><table><thead><tr><th>Seat</th><th>Hand card IDs</th><th>Draw pile IDs</th></tr></thead><tbody>{{range .Data.Hands}}<tr><td>{{.Seat}}</td><td><code>{{.Cards}}</code></td><td><code>{{.Draw}}</code></td></tr>{{end}}</tbody></table></div><a class="button secondary" href="/portal/simulate">Deal another hand</a>`, map[string]any{"Pack": pack.Manifest.PackTag, "Nown": nownID, "Seed": seed, "Hands": hands})

}

func joinIDs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	out := ids[0]
	for _, id := range ids[1:] {
		out += ", " + id
	}
	return out
}

func (m *Manager) challengeView(w http.ResponseWriter, r *http.Request) {
	snapshot, err := m.ChallengeSnapshot(r.Context(), accountIDFromContext(r.Context()))
	if err != nil {
		http.Error(w, "Challenge unavailable. Please try again.", 500)
		return
	}
	if snapshot != nil {
		if entries, ok := snapshot["entries"].([]map[string]any); ok {
			for _, e := range entries {
				e["is_own"] = e["account_id"] == accountIDFromContext(r.Context())
			}
		}
	}
	terms, err := m.CurrentTerms(r.Context())
	if err != nil {
		http.Error(w, "Contribution terms unavailable.", 503)
		return
	}
	portalPage(w, r, "Weekly Nown Challenge", "One topic. One response each. A whole week to argue.", challengeBody, map[string]any{"Snapshot": snapshot, "Terms": terms, "MaxLength": m.MaxTextBytes()})
}

func (m *Manager) challengeEntryPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	accountID := accountIDFromContext(r.Context())
	topicID := r.FormValue("topic_id")
	content := r.FormValue("content")
	if _, err := m.SubmitChallengeEntry(r.Context(), accountID, topicID, MediaText, content, ContributionConsent{Version: r.FormValue("terms_version"), Accepted: r.FormValue("terms_accepted") == "yes"}); err != nil {
		portalError(w, r, err, "/portal/challenge")
		return
	}
	http.Redirect(w, r, "/portal/challenge", http.StatusSeeOther)
}

func (m *Manager) challengeVotePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	accountID := accountIDFromContext(r.Context())
	entryID := r.FormValue("entry_id")
	var topicID string
	if err := m.db.QueryRowContext(r.Context(), "SELECT topic_id FROM challenge_entries WHERE id = $1", entryID).Scan(&topicID); err != nil {
		http.Error(w, "entry not found", http.StatusNotFound)
		return
	}
	if err := m.VoteChallengeEntry(r.Context(), accountID, topicID, entryID); err != nil {
		portalError(w, r, err, "/portal/challenge")
		return
	}
	http.Redirect(w, r, "/portal/challenge", http.StatusSeeOther)
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func (m *Manager) submissionEdit(w http.ResponseWriter, r *http.Request) {
	if err := m.EditDraft(r.Context(), accountIDFromContext(r.Context()), r.PathValue("id"), r.FormValue("content")); err != nil {
		portalError(w, r, err, "/portal/submissions")
		return
	}
	http.Redirect(w, r, "/portal/submissions", http.StatusSeeOther)
}
