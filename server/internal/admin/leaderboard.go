package admin

import (
	"context"
	"database/sql"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/leaderboard"
	"github.com/knowoff/knowoff/server/internal/store"
)

func (m *Manager) registerLeaderboard(mux *http.ServeMux) {
	wrap := func(mutate bool, h http.HandlerFunc) http.Handler {
		guarded := m.requireRole("admin", mutate)(h)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			guarded.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	mux.Handle("GET /admin/leaderboard", wrap(false, m.leaderboardPage))
	mux.Handle("GET /admin/leaderboard/decisions/{id}", wrap(false, m.leaderboardReceipt))
	mux.Handle("POST /admin/leaderboard/decisions", wrap(true, m.leaderboardDecision))
}
func (m *Manager) leaderboardDecision(w http.ResponseWriter, r *http.Request) {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/x-www-form-urlencoded" || r.URL.RawQuery != "" {
		http.Error(w, "leaderboard.invalid", 400)
		return
	}
	if err = r.ParseForm(); err != nil {
		http.Error(w, "leaderboard.invalid", 400)
		return
	}
	allowed := map[string]bool{"csrf_token": true, "id": true, "kind": true, "week_id": true, "target_account_id": true, "prior_decision_id": true, "reason": true}
	for k, v := range r.PostForm {
		if !allowed[k] || len(v) != 1 {
			http.Error(w, "leaderboard.invalid", 400)
			return
		}
	}
	c := store.LeaderboardAdminCommand{ID: r.PostForm.Get("id"), Kind: r.PostForm.Get("kind"), WeekID: r.PostForm.Get("week_id"), TargetAccountID: r.PostForm.Get("target_account_id"), PriorDecisionID: r.PostForm.Get("prior_decision_id"), Reason: r.PostForm.Get("reason")}
	receipt, err := store.NewLeaderboardAdminStore(m.db).Decide(r.Context(), adminIDFromContext(r.Context()), c)
	if err != nil {
		http.Error(w, "leaderboard.refused", 409)
		return
	}
	http.Redirect(w, r, "/admin/leaderboard/decisions/"+receipt.Command.ID, http.StatusSeeOther)
}
func (m *Manager) leaderboardReceipt(w http.ResponseWriter, r *http.Request) {
	receipt, err := store.NewLeaderboardAdminStore(m.db).Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "leaderboard.unavailable", 404)
		return
	}
	adminPage(w, r, "Leaderboard decision", "The recorded decision and its current completion status.", `<dl><dt>Request</dt><dd>{{.Data.Command.ID}}</dd><dt>Action</dt><dd>{{.Data.Command.Kind}}</dd><dt>Status</dt><dd>{{.Data.Status}}</dd><dt>Administrator</dt><dd>{{.Data.ActorID}}</dd><dt>Week</dt><dd>{{.Data.Command.WeekID}}</dd><dt>Account</dt><dd>{{.Data.Command.TargetAccountID}}</dd><dt>Reason</dt><dd>{{.Data.Command.Reason}}</dd></dl>{{if eq .Data.Status "pending"}}<p class="notice">Closing has been accepted. Existing matches and their settlements must finish before the immutable history is saved. The server retries this automatically.</p>{{end}}<a href="/admin/leaderboard?week_id={{.Data.Command.WeekID}}">Week standings &amp; history</a>`, receipt)
}

type leaderboardAdminRow struct {
	AccountID, PriorID, Kind, RequestID string
	Points                              int64
	Matches                             int
}
type leaderboardAdminData struct {
	Week, CloseID   string
	Weeks           []string
	Top             []leaderboard.Ranking
	Rows            []leaderboardAdminRow
	Decisions       []store.LeaderboardAdminReceipt
	Closed, Closing bool
	Previous, Next  int
}

func (m *Manager) leaderboardPage(w http.ResponseWriter, r *http.Request) {
	data := leaderboardAdminData{Week: r.URL.Query().Get("week_id"), CloseID: uuid.NewString()}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 1000000 {
			http.Error(w, "leaderboard.invalid", 400)
			return
		}
	}
	rows, err := m.db.QueryContext(r.Context(), `SELECT week_id FROM leaderboard_weeks ORDER BY week_id DESC LIMIT 50 OFFSET $1`, offset)
	if err != nil {
		http.Error(w, "leaderboard.unavailable", 503)
		return
	}
	for rows.Next() {
		var week string
		if err = rows.Scan(&week); err != nil {
			break
		}
		data.Weeks = append(data.Weeks, week)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		http.Error(w, "leaderboard.unavailable", 503)
		return
	}
	data.Next = offset + 50
	data.Previous = max(0, offset-50)
	if data.Week == "" && len(data.Weeks) > 0 {
		data.Week = data.Weeks[0]
	}
	if data.Week != "" {
		at, parseErr := time.Parse("2006-01-02", data.Week)
		if parseErr != nil || leaderboard.WeekID(at) != data.Week {
			http.Error(w, "leaderboard.invalid", 400)
			return
		}
		var closing sql.NullTime
		if err = m.db.QueryRowContext(r.Context(), `SELECT closed,closing_at FROM leaderboard_weeks WHERE week_id=$1`, data.Week).Scan(&data.Closed, &closing); err != nil {
			http.Error(w, "leaderboard.unavailable", 404)
			return
		}
		data.Closing = closing.Valid
		data.Top, _, err = leaderboard.NewManager(m.db).Get(r.Context(), data.Week, 100, "")
		if err != nil {
			http.Error(w, "leaderboard.unavailable", 503)
			return
		}
		// Exact account lookup makes controls available beyond the bounded first page.
		account := r.URL.Query().Get("account_id")
		if account != "" {
			if _, err = uuid.Parse(account); err != nil {
				http.Error(w, "leaderboard.invalid", 400)
				return
			}
		}
		rows, err = m.db.QueryContext(r.Context(), `SELECT e.account_id,e.points,e.matches_counted,COALESCE(d.id::text,''),COALESCE(d.kind,'reinstate') FROM leaderboard_entries e LEFT JOIN LATERAL (SELECT id,kind FROM leaderboard_admin_decisions WHERE week_id=e.week_id AND target_account_id=e.account_id ORDER BY revision DESC LIMIT 1)d ON true WHERE e.week_id=$1 AND ($2='' OR e.account_id::text=$2) ORDER BY e.points DESC,e.account_id LIMIT 100`, data.Week, account)
		if err != nil {
			http.Error(w, "leaderboard.unavailable", 503)
			return
		}
		for rows.Next() {
			var row leaderboardAdminRow
			if err = rows.Scan(&row.AccountID, &row.Points, &row.Matches, &row.PriorID, &row.Kind); err != nil {
				break
			}
			row.RequestID = uuid.NewString()
			data.Rows = append(data.Rows, row)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			http.Error(w, "leaderboard.unavailable", 503)
			return
		}
		rows, err = m.db.QueryContext(r.Context(), `SELECT id FROM leaderboard_admin_decisions WHERE week_id=$1 ORDER BY created_at DESC,id LIMIT 50 OFFSET $2`, data.Week, offset)
		if err != nil {
			http.Error(w, "leaderboard.unavailable", 503)
			return
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				break
			}
			ids = append(ids, id)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			http.Error(w, "leaderboard.unavailable", 503)
			return
		}
		for _, id := range ids {
			receipt, e := store.NewLeaderboardAdminStore(m.db).Get(r.Context(), id)
			if e != nil {
				http.Error(w, "leaderboard.unavailable", 503)
				return
			}
			data.Decisions = append(data.Decisions, receipt)
		}
	}
	adminPage(w, r, "Weekly leaderboard", "Review standings, eligibility decisions and immutable weekly history.", leaderboardAdminBody, data)
}

const leaderboardAdminBody = `<form method="get"><label>Week<select name="week_id">{{range .Data.Weeks}}<option value="{{.}}" {{if eq . $.Data.Week}}selected{{end}}>{{.}}</option>{{end}}</select></label><label>Account lookup<input name="account_id"></label><button>Inspect</button></form>{{if .Data.Week}}<h2>{{.Data.Week}} · {{if .Data.Closed}}Closed history{{else if .Data.Closing}}Closing{{else}}Open{{end}}</h2><p>Exclusions apply to this week before closing begins. Raw match points, daily limits and rewards are preserved. Reinstating includes accumulated week points. Closing and closed weeks cannot change eligibility.</p><h3>Eligible top 100</h3><table><tr><th>Rank</th><th>Account</th><th>Points</th></tr>{{range .Data.Top}}<tr><td>{{.Rank}}</td><td>{{.AccountID}}</td><td>{{.Points}}</td></tr>{{else}}<tr><td colspan="3">No eligible standings.</td></tr>{{end}}</table><h3>Raw points &amp; eligibility</h3><p>First 100 accounts by points. Use exact account lookup for any other account.</p>{{range .Data.Rows}}<article class="panel"><p>{{.AccountID}} · {{.Points}} points · {{.Matches}} counted matches · {{if eq .Kind "exclude"}}Excluded{{else}}Included{{end}}</p>{{if not $.Data.Closing}}{{if not $.Data.Closed}}<form method="post" action="/admin/leaderboard/decisions"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><input type="hidden" name="id" value="{{.RequestID}}"><input type="hidden" name="week_id" value="{{$.Data.Week}}"><input type="hidden" name="target_account_id" value="{{.AccountID}}"><input type="hidden" name="prior_decision_id" value="{{.PriorID}}"><input type="hidden" name="kind" value="{{if eq .Kind "exclude"}}reinstate{{else}}exclude{{end}}"><label>Reason<textarea name="reason" required maxlength="500"></textarea></label><button>{{if eq .Kind "exclude"}}Reinstate{{else}}Exclude{{end}} this week</button></form>{{end}}{{end}}</article>{{end}}<h3>Close or re-run</h3><p>Only an ended week can close. Re-running returns its existing immutable history or resumes accepted pending work.</p><form method="post" action="/admin/leaderboard/decisions"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="id" value="{{.Data.CloseID}}"><input type="hidden" name="kind" value="close"><input type="hidden" name="week_id" value="{{.Data.Week}}"><label>Reason<textarea name="reason" required maxlength="500"></textarea></label><button>Close or resume week</button></form><h3>Decision history</h3>{{range .Data.Decisions}}<p><a href="/admin/leaderboard/decisions/{{.Command.ID}}">{{.Command.Kind}} · {{.Status}}</a> · {{.Command.Reason}}</p>{{else}}<p>No operator decisions.</p>{{end}}{{end}}<p><a href="?week_id={{.Data.Week}}&amp;offset={{.Data.Previous}}">Previous history page</a> · <a href="?week_id={{.Data.Week}}&amp;offset={{.Data.Next}}">Next history page</a></p>`
