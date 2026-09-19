package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/store"
)

func (m *Manager) registerNoinCorrections(mux *http.ServeMux) {
	guarded := m.requireRole("admin", true)(http.HandlerFunc(m.correctNoin))
	mux.Handle("POST /admin/economy/corrections", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		guarded.ServeHTTP(w, r.WithContext(ctx))
	}))
}

func (m *Manager) correctNoin(w http.ResponseWriter, r *http.Request) {
	command, err := readOperatorCommand(r)
	allowed := map[string]bool{"csrf_token": true, "id": true, "kind": true, "target_account_id": true, "amount": true, "source_ledger_id": true, "reason": true}
	for key := range r.PostForm {
		if !allowed[key] {
			err = store.ErrAdminOperation
		}
	}
	if len(r.URL.Query()) != 0 {
		err = store.ErrAdminOperation
	}
	if err != nil {
		http.Error(w, "operator.invalid_or_conflicting", http.StatusBadRequest)
		return
	}
	receipt, err := economy.NewManager(m.db, m.cfg).CorrectNoin(r.Context(), adminIDFromContext(r.Context()), command)
	if err != nil {
		http.Error(w, "operator.invalid_or_conflicting", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/admin/operators/"+receipt.Command.ID, http.StatusSeeOther)
}

const noinCorrectionForms = `{{if .Data.Found}}
<h2>Noin corrections</h2><p>These audited actions adjust this account's Noin wallet. They do not refund platform payments or change entitlements. A spend refund credits the full original amount once.</p>
<div class="split"><section class="panel"><h3>Grant Noin</h3>
<form method="post" action="/admin/economy/corrections"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="id" value="{{.Data.GrantID}}"><input type="hidden" name="kind" value="noin_grant"><input type="hidden" name="target_account_id" value="{{.Data.AccountID}}">
<label>Noin amount<input name="amount" type="number" min="1" max="2147483647" required></label><label>Reason<textarea name="reason" maxlength="500" required></textarea></label><button>Grant Noin and record audit</button></form></section>
<section class="panel"><h3>Refund a Noin spend</h3><form method="post" action="/admin/economy/corrections"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="id" value="{{.Data.RefundID}}"><input type="hidden" name="kind" value="noin_refund"><input type="hidden" name="target_account_id" value="{{.Data.AccountID}}">
<label>Original spend ledger ID<input name="source_ledger_id" inputmode="numeric" pattern="[1-9][0-9]*" required></label><label>Reason<textarea name="reason" maxlength="500" required></textarea></label><button>Refund full Noin spend and record audit</button></form></section></div>{{end}}`
