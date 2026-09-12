package portal

import "net/http"

func (m *Manager) guardForm(w http.ResponseWriter, r *http.Request) {
	rows, err := m.db.QueryContext(r.Context(), `SELECT account_id,reason,frozen_at,expires_at FROM guard_freezes WHERE guard_account_id=$1 AND dismissed_at IS NULL AND converted_to_ban_at IS NULL AND expires_at>$2 ORDER BY frozen_at DESC,id LIMIT 100`, accountIDFromContext(r.Context()), m.now())
	if err != nil {
		http.Error(w, "Guard history unavailable.", 503)
		return
	}
	defer rows.Close()
	history := []map[string]any{}
	for rows.Next() {
		var target, reason string
		var at, until any
		if err = rows.Scan(&target, &reason, &at, &until); err != nil {
			http.Error(w, "Guard history unavailable.", 503)
			return
		}
		history = append(history, map[string]any{"Target": target, "Reason": reason, "FrozenAt": at, "ExpiresAt": until})
	}
	if rows.Err() != nil {
		http.Error(w, "Guard history unavailable.", 503)
		return
	}
	rows.Close()
	// Guard review needs the reported conduct, not the reporter's identity.
	// Bound both rows and displayed excerpts; original report bytes stay intact.
	cases, err := m.db.QueryContext(r.Context(), `SELECT id,target_account_id,LEFT(reason,$1),LEFT(COALESCE(description,''),$1),status FROM reports WHERE report_type='conduct' AND status IN ('new','in_review') AND target_account_id IS NOT NULL ORDER BY created_at,id LIMIT 100`, m.MaxTextBytes())
	if err != nil {
		http.Error(w, "Conduct queue unavailable.", 503)
		return
	}
	defer cases.Close()
	review := []map[string]any{}
	for cases.Next() {
		var id, target, reason, description, status string
		if err = cases.Scan(&id, &target, &reason, &description, &status); err != nil {
			http.Error(w, "Conduct queue unavailable.", 503)
			return
		}
		review = append(review, map[string]any{"ID": id, "Target": target, "Reason": reason, "Description": description, "Status": status})
	}
	if cases.Err() != nil {
		http.Error(w, "Conduct queue unavailable.", 503)
		return
	}
	portalPage(w, r, "Guard review", "A temporary pause for review. Administrators make the final decision.", guardBody, map[string]any{"Hours": m.cfg.Tuning.Portal.GuardFreezeMaxH, "History": history, "Cases": review})
}
func (m *Manager) guardFreezePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form.", 400)
		return
	}
	if err := m.FreezeAccount(r.Context(), accountIDFromContext(r.Context()), r.FormValue("account_id"), r.FormValue("reason")); err != nil {
		portalError(w, r, err, "/portal/guard")
		return
	}
	http.Redirect(w, r, "/portal/guard", http.StatusSeeOther)
}

const guardBody = `<section><h2>Open conduct cases</h2><p>Oldest 100 open cases. Text excerpts are bounded; reporter identities remain private. Administrators manage final case outcomes.</p>{{range .Data.Cases}}<article class="panel" data-guard-case><h3>{{.Reason}}</h3><p>{{.Description}}</p><p>Account <code>{{.Target}}</code> · {{.Status}}</p><p>Case <code>{{.ID}}</code></p></article>{{else}}<p>No open conduct cases.</p>{{end}}</section><div class="split"><section><h2>Pause an account for review</h2><p>A freeze blocks new matchmaking and portal work for up to {{.Data.Hours}} hours. It expires automatically unless an administrator acts. Other Guards' decisions remain independent.</p><form method="post" action="/portal/guard/freeze"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label for="target">Account ID from the report</label><input id="target" name="account_id" required><label for="reason">Reason for review</label><textarea id="reason" name="reason" required></textarea><button>Freeze for review</button></form></section><section><h2>Your active freezes</h2>{{range .Data.History}}<article class="panel"><p><code>{{.Target}}</code></p><p>{{.Reason}}</p><p>Expires {{.ExpiresAt}}</p></article>{{else}}<p>No active freezes.</p>{{end}}</section></div>`
