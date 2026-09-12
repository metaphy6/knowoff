package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/reports"
	"net/http"
)

func (m *Manager) registerOperations(mux *http.ServeMux) {
	m.registerTextContent(mux)
	mux.Handle("GET /admin/report-cases/{id}", m.requireRole("admin", false)(http.HandlerFunc(m.caseReports)))
	resolve := m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := reports.NewManager(m.db).ResolveCase(r.Context(), adminIDFromContext(r.Context()), r.PathValue("id"), r.FormValue("decision"), r.FormValue("reason"), m.textContent.TakedownTx); err != nil {
			adminError(w, r, err, "/admin/reports")
			return
		}
		http.Redirect(w, r, "/admin/reports", http.StatusSeeOther)
	}))
	mux.Handle("POST /admin/report-cases/{id}/resolve", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, reports.MaxBodyBytes)
		resolve.ServeHTTP(w, r)
	}))
	mux.Handle("GET /admin/reports", m.requireRole("admin", false)(http.HandlerFunc(m.reportsList)))
	mux.Handle("GET /admin/feedback", m.requireRole("admin", false)(http.HandlerFunc(m.feedbackList)))
	mux.Handle("GET /admin/economy", m.requireRole("admin", false)(http.HandlerFunc(m.economyLookup)))
	for _, kind := range []string{"reports", "feedback", "cases"} {
		destination := "/admin/" + kind
		if kind == "cases" {
			destination = "/admin/reports"
		}
		mux.Handle("POST /admin/"+kind+"/{id}/status", m.requireRole("admin", true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := m.Triage(r.Context(), adminIDFromContext(r.Context()), kind, r.PathValue("id"), r.FormValue("status")); err != nil {
				adminError(w, r, err, destination)
				return
			}
			http.Redirect(w, r, destination, http.StatusSeeOther)
		})))
	}
}

// Triage updates only a queue label. It cannot ban, freeze, refund or remove an
// asset. The decision and its before/after audit either both commit or neither.
func (m *Manager) Triage(ctx context.Context, adminID, kind, id, status string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("invalid case id")
	}
	var selectSQL, updateSQL string
	switch kind {
	case "cases":
		if status != "new" && status != "in_review" && status != "resolved" {
			return fmt.Errorf("invalid case status")
		}
		selectSQL = `SELECT status FROM report_cases WHERE id=$1 AND resolution IS NULL FOR UPDATE`
		updateSQL = `UPDATE report_cases SET status=$2,updated_at=now() WHERE id=$1`
	case "reports":
		var caseID sql.NullString
		if err := m.db.QueryRowContext(ctx, `SELECT case_id FROM reports WHERE id=$1`, id).Scan(&caseID); err != nil {
			return fmt.Errorf("case unavailable")
		}
		if caseID.Valid {
			return m.Triage(ctx, adminID, "cases", caseID.String, status)
		}
		if status != "new" && status != "in_review" && status != "resolved" {
			return fmt.Errorf("invalid report status")
		}
		selectSQL = `SELECT status FROM reports WHERE id=$1 FOR UPDATE`
		updateSQL = `UPDATE reports SET status=$2,updated_at=now() WHERE id=$1`
	case "feedback":
		if status != "new" && status != "seen" && status != "done" {
			return fmt.Errorf("invalid feedback status")
		}
		selectSQL = `SELECT status FROM feedback WHERE id=$1 FOR UPDATE`
		updateSQL = `UPDATE feedback SET status=$2,updated_at=now() WHERE id=$1`
	default:
		return fmt.Errorf("invalid queue")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = reports.LockAdmin(ctx, tx, adminID); err != nil {
		return err
	}
	var before string
	if err = tx.QueryRowContext(ctx, selectSQL, id).Scan(&before); err != nil {
		return fmt.Errorf("case unavailable")
	}
	if before == status {
		return reports.LockAdmin(ctx, tx, adminID)
	}
	if _, err = tx.ExecContext(ctx, updateSQL, id, status); err != nil {
		return err
	}
	if kind == "cases" {
		if _, err = tx.ExecContext(ctx, `UPDATE reports SET status=$2,updated_at=now() WHERE case_id=$1`, id, status); err != nil {
			return err
		}
	}
	b, _ := json.Marshal(map[string]string{"status": before})
	a, _ := json.Marshal(map[string]string{"status": status})
	if _, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log(admin_id,action,target_type,target_id,before_state,after_state) VALUES($1,$2,$3,$4,$5,$6)`, adminID, "triage_status", kind, id, b, a); err != nil {
		return err
	}
	if err = reports.LockAdmin(ctx, tx, adminID); err != nil {
		return err
	}
	return tx.Commit()
}
func (m *Manager) reportsList(w http.ResponseWriter, r *http.Request) {
	rows, next, err := reports.NewManager(m.db).ListCaseQueue(r.Context(), r.URL.Query().Get("queue"), r.URL.Query().Get("after"), 100)
	if err != nil {
		http.Error(w, "Reports unavailable.", 400)
		return
	}
	adminPage(w, r, "Reports", "One case per reported target, with the original reports retained.", `<p><a href="/admin/reports?queue=conduct">Conduct queue</a> · <a href="/admin/reports?queue=curation">Curation queue</a> · <a href="/admin/reports">All cases</a></p>{{range .Data.Rows}}<article class="panel"><h2>{{.Reason}}</h2><p>{{.Reports}} reports · {{.Kind}} · {{.Status}}</p><p>{{.Description}}</p>{{if .TargetAccount}}<p>Account <code>{{.TargetAccount}}</code></p>{{end}}{{if .TargetMediaID}}<p>Legacy media <code>{{.TargetMediaID}}</code></p>{{end}}{{if .TextTarget}}<p class="preview">{{.Text}}</p><p>{{.TextTarget.Language}} · {{.TextTarget.ModeID}} · {{.TextTarget.RulesVersion}}</p><p>Release <code>{{.TextTarget.PackReleaseID}}</code>, content <code>{{.TextTarget.ContentRef.ContentID}}</code> revision {{.TextTarget.ContentRef.Revision}}</p><p>Text SHA256 <code>{{.TextTarget.TextSHA256}}</code></p>{{end}}<p><a href="/admin/report-cases/{{.ID}}">Read reports</a></p>{{if .Resolution}}<p>Decision: {{.Resolution}} — {{.ResolutionReason}}</p>{{else}}<form method="post" action="/admin/cases/{{.ID}}/status"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><label>Queue status<select name="status"><option>new</option><option>in_review</option><option>resolved</option></select></label><button>Update label</button></form><form method="post" action="/admin/report-cases/{{.ID}}/resolve"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><label>Decision<select name="decision"><option value="dismissed">Dismiss after review</option>{{if .TextTarget}}<option value="release_takedown">Withdraw this release from new matches</option>{{end}}</select></label><label>Reason<textarea name="reason" maxlength="1024" required></textarea></label>{{if .TextTarget}}<p>Withdrawal keeps the immutable release and begun matches intact.</p>{{end}}<button>Record decision</button></form>{{end}}</article>{{else}}<p>No reports yet.</p>{{end}}{{if .Data.Next}}<a class="button" href="/admin/reports?queue={{.Data.Queue}}&amp;after={{.Data.Next}}">Older cases</a>{{end}}`, map[string]any{"Rows": rows, "Next": next, "Queue": r.URL.Query().Get("queue")})
}
func (m *Manager) caseReports(w http.ResponseWriter, r *http.Request) {
	rows, next, err := reports.NewManager(m.db).ListReportsPage(r.Context(), "", r.URL.Query().Get("after"), r.PathValue("id"), 100)
	if err != nil {
		http.Error(w, "Case unavailable.", 400)
		return
	}
	adminPage(w, r, "Case reports", "Original player observations.", `<p><a href="/admin/reports">All cases</a></p>{{range .Data.Rows}}<article class="panel"><h2>{{.Reason}}</h2><p class="preview">{{.Description}}</p><p>{{.CreatedAt.Format "02 Jan 2006 15:04 UTC"}}</p></article>{{else}}<p>No reports.</p>{{end}}{{if .Data.Next}}<a class="button" href="?after={{.Data.Next}}">Older reports</a>{{end}}`, map[string]any{"Rows": rows, "Next": next})
}
func (m *Manager) feedbackList(w http.ResponseWriter, r *http.Request) {
	rows, next, err := reports.NewManager(m.db).ListFeedbackPage(r.Context(), "", r.URL.Query().Get("after"), 100)
	if err != nil {
		http.Error(w, "Feedback unavailable. Please try again.", 500)
		return
	}
	adminPage(w, r, "Feedback", "The bug reports, bright ideas and wonderfully specific requests.", `{{range .Data.Rows}}<article class="panel"><div class="row spread"><h2 style="margin-top:0">{{if .Title}}{{.Title}}{{else}}{{.Type}}{{end}}</h2><span class="status">{{.Status}}</span></div><p class="preview">{{.Message}}</p><p class="small">{{.Type}} · {{.CreatedAt.Format "02 Jan 2006 15:04 UTC"}}</p>{{if .ContextSnapshot}}<details><summary>Submitted app context</summary><dl>{{range $key,$value:=.ContextSnapshot}}<dt>{{$key}}</dt><dd>{{$value}}</dd>{{end}}</dl></details>{{end}}<form method="post" action="/admin/feedback/{{.ID}}/status"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><label for="status-{{.ID}}">Feedback status</label><select id="status-{{.ID}}" name="status"><option {{if eq .Status "new"}}selected{{end}}>new</option><option {{if eq .Status "seen"}}selected{{end}}>seen</option><option {{if eq .Status "done"}}selected{{end}}>done</option></select><button class="secondary">Update feedback</button></form></article>{{else}}<p class="empty">No feedback yet. Players can send ideas and bugs from the game.</p>{{end}}{{if .Data.Next}}<a class="button" href="?after={{.Data.Next}}">Older feedback</a>{{end}}`, map[string]any{"Rows": rows, "Next": next})
}

type ledgerRow struct {
	Type, Reason string
	Amount       int
	Created      string
}
type entitlementRow struct{ Type, Value, Until string }

func (m *Manager) economyLookup(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"AccountID": r.URL.Query().Get("account_id")}
	id := r.URL.Query().Get("account_id")
	if id != "" {
		if _, err := uuid.Parse(id); err != nil {
			adminError(w, r, fmt.Errorf("invalid account id"), "/admin/economy")
			return
		}
		var nickname string
		var balance int64
		err := m.db.QueryRowContext(r.Context(), `SELECT a.nickname,COALESCE(w.balance,0) FROM accounts a LEFT JOIN noin_wallets w ON w.account_id=a.id WHERE a.id=$1`, id).Scan(&nickname, &balance)
		if err == sql.ErrNoRows {
			data["Missing"] = true
		} else if err != nil {
			http.Error(w, "Account lookup unavailable.", 500)
			return
		} else {
			data["Found"] = true
			data["Nickname"] = nickname
			data["Balance"] = balance
			rows, err := m.db.QueryContext(r.Context(), `SELECT event_type,amount,reason,created_at::text FROM noin_ledger WHERE account_id=$1 ORDER BY created_at DESC,id DESC LIMIT 100`, id)
			if err != nil {
				http.Error(w, "Ledger unavailable.", 500)
				return
			}
			var ledger []ledgerRow
			for rows.Next() {
				var v ledgerRow
				if err = rows.Scan(&v.Type, &v.Amount, &v.Reason, &v.Created); err != nil {
					rows.Close()
					http.Error(w, "Ledger unavailable.", 500)
					return
				}
				ledger = append(ledger, v)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				http.Error(w, "Ledger unavailable.", 500)
				return
			}
			data["Ledger"] = ledger
			rows, err = m.db.QueryContext(r.Context(), `SELECT entitlement_type,COALESCE(value,''),COALESCE(active_until::text,'Permanent') FROM entitlements WHERE account_id=$1 ORDER BY entitlement_type`, id)
			if err != nil {
				http.Error(w, "Entitlements unavailable.", 500)
				return
			}
			var grants []entitlementRow
			for rows.Next() {
				var v entitlementRow
				if err = rows.Scan(&v.Type, &v.Value, &v.Until); err != nil {
					rows.Close()
					http.Error(w, "Entitlements unavailable.", 500)
					return
				}
				grants = append(grants, v)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				http.Error(w, "Entitlements unavailable.", 500)
				return
			}
			data["Entitlements"] = grants
		}
	}
	adminPage(w, r, "Accounts & economy", "Inspect the server's wallet, ledger and entitlement records.", `<form method="get" action="/admin/economy"><label for="account">Account ID</label><input id="account" name="account_id" value="{{.Data.AccountID}}" required><button>Look up account</button></form>{{if .Data.Missing}}<p class="notice">No account matches that ID.</p>{{end}}{{if .Data.Found}}<h2>{{.Data.Nickname}}</h2><p>Wallet balance: <strong>{{.Data.Balance}} Noin</strong></p><h2>Entitlements</h2><div class="table-scroll"><table><tr><th>Type</th><th>Value</th><th>Active until</th></tr>{{range .Data.Entitlements}}<tr><td>{{.Type}}</td><td>{{.Value}}</td><td>{{.Until}}</td></tr>{{else}}<tr><td colspan="3">No entitlements.</td></tr>{{end}}</table></div><h2>Latest 100 ledger entries</h2><div class="table-scroll"><table><tr><th>Event</th><th>Noin</th><th>Reason</th><th>Created</th></tr>{{range .Data.Ledger}}<tr><td>{{.Type}}</td><td>{{.Amount}}</td><td>{{.Reason}}</td><td>{{.Created}}</td></tr>{{else}}<tr><td colspan="4">No wallet activity.</td></tr>{{end}}</table></div>{{end}}<p class="muted">This lookup is read-only. Grants and refunds need their own audited product workflows.</p>`, data)
}
