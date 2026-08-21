package portal

import (
	"context"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/pkg/media"
)

// Handler returns the public contributor portal HTTP handler mounted at /portal/.
func (m *Manager) Handler() http.Handler {
	mux := http.NewServeMux()
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

func (m *Manager) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := m.accountFromRequest(r)
		if accountID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxAccountIDKey{}, accountID)
		next(w, r.WithContext(ctx))
	}
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

func (m *Manager) accountFromRequest(r *http.Request) string {
	// Prefer Authorization: Bearer <JWT>.
	if h := r.Header.Get("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
		accountID, err := m.auth.ValidateAccessToken(r.Context(), h[7:])
		if err == nil {
			return accountID
		}
	}
	return ""
}

type ctxAccountIDKey struct{}

func accountIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxAccountIDKey{}).(string); ok {
		return v
	}
	return ""
}

func (m *Manager) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/portal/" {
		http.NotFound(w, r)
		return
	}
	accountID := accountIDFromContext(r.Context())
	role, _ := m.ActiveRole(r.Context(), accountID)
	fmt.Fprintf(w, `<!doctype html>
<html><head><title>Knowoff Contributor Portal</title></head><body>
<h1>Contributor Portal</h1>
<p>Active role: %s</p>
<ul>
<li><a href="/portal/apply">Apply for role</a></li>
<li><a href="/portal/submissions">My submissions</a></li>
<li><a href="/portal/challenge">Weekly Nown Challenge</a></li>
</ul>
</body></html>`, template.HTMLEscapeString(string(role)))
}

func (m *Manager) applyForm(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, `<!doctype html>
<html><head><title>Apply for role</title></head><body>
<h1>Apply for Contributor / Curator / Guard role</h1>
<form method="post">
<label>Role <select name="role"><option>contributor</option><option>curator</option><option>guard</option></select></label><br>
<button>Apply</button>
</form>
</body></html>`)
}

func (m *Manager) applyPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	accountID := accountIDFromContext(r.Context())
	if err := m.ApplyForRole(r.Context(), accountID, Role(r.FormValue("role"))); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/portal/", http.StatusSeeOther)
}

func (m *Manager) submissionsList(w http.ResponseWriter, r *http.Request) {
	accountID := accountIDFromContext(r.Context())
	subs, err := m.ListSubmissions(r.Context(), accountID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, `<!doctype html>
<html><head><title>My submissions</title></head><body>
<h1>My submissions</h1>
<ul>`)
	for _, s := range subs {
		fmt.Fprintf(w, `<li>%s [%s] %s</li>`, template.HTMLEscapeString(s.ID), template.HTMLEscapeString(string(s.Status)), template.HTMLEscapeString(s.Content))
	}
	fmt.Fprint(w, `</ul>
<h2>New text submission</h2>
<form method="post" action="/portal/submissions">
<label>Content <input name="content" required></label><br>
<button>Create draft</button>
</form>
</body></html>`)
}

func (m *Manager) submissionCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	accountID := accountIDFromContext(r.Context())
	if _, err := m.CreateDraft(r.Context(), accountID, MediaText, r.FormValue("content")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) submissionSubmit(w http.ResponseWriter, r *http.Request) {
	accountID := accountIDFromContext(r.Context())
	id := r.PathValue("id")
	if err := m.SubmitDraft(r.Context(), accountID, id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) submissionWithdraw(w http.ResponseWriter, r *http.Request) {
	accountID := accountIDFromContext(r.Context())
	id := r.PathValue("id")
	if err := m.WithdrawSubmission(r.Context(), accountID, id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/portal/submissions", http.StatusSeeOther)
}

func (m *Manager) simulateForm(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, `<!doctype html>
<html><head><title>Deal Simulator</title></head><body>
<h1>Deal Simulator</h1>
<form method="post">
<label>Table size <select name="table_size"><option value="4">4</option><option value="6" selected>6</option></select></label><br>
<label>Nown ID <input name="nown_id"></label><br>
<label>Seed <input name="seed" value="42"></label><br>
<button>Simulate</button>
</form>
</body></html>`)
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

	fmt.Fprint(w, `<!doctype html>
<html><head><title>Deal Simulator Result</title></head><body>
<h1>Deal Simulator Result</h1>
<p>Pack: `+template.HTMLEscapeString(pack.Manifest.PackTag)+` | Nown: `+template.HTMLEscapeString(nownID)+` | Table: `+strconv.Itoa(tableSize)+` | Seed: `+strconv.FormatInt(seed, 10)+`</p>
<table border="1"><tr><th>Seat</th><th>Hand (5)</th><th>Draw pile (3)</th></tr>`)
	for i, hand := range result.Hands {
		fmt.Fprintf(w, `<tr><td>%d</td><td>%s</td><td>%s</td></tr>`,
			i+1,
			template.HTMLEscapeString(joinIDs(hand.Cards)),
			template.HTMLEscapeString(joinIDs(hand.DrawPile)),
		)
	}
	fmt.Fprint(w, `</table><p><a href="/portal/simulate">Again</a></p></body></html>`)
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
	accountID := accountIDFromContext(r.Context())
	topic, err := m.ActiveChallengeTopic(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if topic == nil {
		fmt.Fprint(w, `<p>No active Weekly Nown Challenge.</p>`)
		return
	}
	entries, err := m.ListChallengeEntries(r.Context(), topic.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, `<!doctype html>
<html><head><title>Weekly Nown Challenge</title></head><body>
<h1>Weekly Nown Challenge</h1>
<p>Week: %s - %s</p>
<ul>`, topic.WeekStart.Format("2006-01-02"), topic.WeekEnd.Format("2006-01-02"))
	for _, e := range entries {
		if e.AccountID == accountID {
			fmt.Fprintf(w, `<li>%s (yours) — %d votes</li>`, template.HTMLEscapeString(e.Content), e.VoteCount)
		} else {
			fmt.Fprintf(w, `<li>%s — %d votes <form method="post" action="/portal/challenge/vote" style="display:inline"><input type="hidden" name="entry_id" value="%s"><button>Vote</button></form></li>`, template.HTMLEscapeString(e.Content), e.VoteCount, template.HTMLEscapeString(e.ID))
		}
	}
	fmt.Fprint(w, `</ul>
<h2>Submit entry</h2>
<form method="post" action="/portal/challenge/entry">
<input type="hidden" name="topic_id" value="`)
	fmt.Fprint(w, template.HTMLEscapeString(topic.ID))
	fmt.Fprint(w, `">
<label>Content <input name="content" required></label><br>
<button>Submit</button>
</form>
</body></html>`)
}

func (m *Manager) challengeEntryPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	accountID := accountIDFromContext(r.Context())
	topicID := r.FormValue("topic_id")
	content := r.FormValue("content")
	if _, err := m.SubmitChallengeEntry(r.Context(), accountID, topicID, MediaText, content); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/portal/challenge", http.StatusSeeOther)
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
