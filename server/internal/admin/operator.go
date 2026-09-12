package admin

import (
	"context"
	"fmt"
	"html"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

// A concrete product hook must capture its immutable target under the proper
// runtime/account locks, then call the store's exact-session decision boundary.
// An absent hook exposes history only and rejects all new decisions.
type OperatorHooks struct {
	Decide func(context.Context, string, store.AdminOperationCommand) (store.AdminOperationReceipt, error)
}

// OperatorHandler is mounted exclusively on the internal administrator listener.
// Recording a decision returns its explicit pending/applied status; no request
// or UI completion is evidence that a wallet or live room was already changed.
func (m *Manager) OperatorHandler(hooks OperatorHooks) http.Handler {
	s := store.NewAdminOperationStore(m.db)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/operators", func(w http.ResponseWriter, r *http.Request) {
		receipts, err := s.List(r.Context(), 50)
		if err != nil {
			http.Error(w, "operator.unavailable", 503)
			return
		}
		var b strings.Builder
		b.WriteString(`<p>Noin corrections are internal wallet adjustments. They do not refund platform payments or change entitlements.</p><ul class="list">`)
		for _, receipt := range receipts {
			fmt.Fprintf(&b, `<li><a href="/admin/operators/%s">%s</a> — %s — %s</li>`, html.EscapeString(receipt.Command.ID), html.EscapeString(receipt.Command.ID), html.EscapeString(receipt.Command.Kind), html.EscapeString(receipt.Status))
		}
		b.WriteString(`</ul>`)
		if hooks.Decide != nil {
			fmt.Fprintf(&b, `<form method="post" action="/admin/operators"><input type="hidden" name="csrf_token" value="%s"><input type="hidden" name="id" value="%s"><label>Action<select name="kind"><option value="room_kick">Kick from this room</option><option value="room_close">Close this room</option><option value="account_sanction">Account sanction</option><option value="sanction_lift">Lift this sanction</option><option value="noin_grant">Noin grant</option><option value="noin_refund">Refund one Noin spend</option></select></label>`, html.EscapeString(csrfFromContext(r.Context())), uuid.NewString())
			for _, field := range []string{"target_account_id", "room_id", "owner_id", "owner_generation", "source_ledger_id", "amount", "prior_sanction_id", "until"} {
				fmt.Fprintf(&b, `<label>%s<input name="%s"></label>`, field, field)
			}
			b.WriteString(`<label>Reason<textarea name="reason" required maxlength="500"></textarea></label><button type="submit">Record reviewed decision</button></form>`)
		} else {
			b.WriteString(`<p>New operator decisions are unavailable until their product delivery is configured.</p>`)
		}
		adminPage(w, r, "Operator decisions", "Review decisions and their durable delivery status.", b.String(), nil)
	})
	mux.HandleFunc("GET /admin/operators/{id}", func(w http.ResponseWriter, r *http.Request) {
		receipt, err := s.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, "operator.unavailable", 404)
			return
		}
		var body strings.Builder
		body.WriteString(`<dl>`)
		field := func(label, value string) {
			if value != "" {
				fmt.Fprintf(&body, `<dt>%s</dt><dd>%s</dd>`, html.EscapeString(label), html.EscapeString(value))
			}
		}
		field("Request", receipt.Command.ID)
		field("Action", receipt.Command.Kind)
		field("Status", receipt.Status)
		field("Administrator", receipt.ActorID)
		field("Account", receipt.Command.TargetAccountID)
		field("Affected accounts", strings.Join(receipt.AffectedAccounts, ", "))
		field("Room", receipt.Command.RoomID)
		field("Process incarnation", receipt.Command.OwnerID)
		if receipt.Command.OwnerGeneration > 0 {
			field("Process generation", strconv.FormatInt(receipt.Command.OwnerGeneration, 10))
		}
		if receipt.Command.Amount > 0 {
			field("Noin amount", strconv.FormatInt(receipt.Command.Amount, 10))
		}
		if receipt.Command.SourceLedgerID > 0 {
			field("Original spend", strconv.FormatInt(receipt.Command.SourceLedgerID, 10))
		}
		field("Original sanction", receipt.Command.PriorSanctionID)
		if receipt.Command.Until != nil {
			field("Sanction until", receipt.Command.Until.Format(time.RFC3339))
		}
		field("Reason", receipt.Command.Reason)
		field("Recorded at", receipt.CreatedAt.Format(time.RFC3339))
		if receipt.CompletedAt != nil {
			field("Completed at", receipt.CompletedAt.Format(time.RFC3339))
		}
		body.WriteString(`</dl><p>A pending decision has not completed delivery.</p><a href="/admin/operators">Decision history</a>`)
		adminPage(w, r, "Operator decision", "Immutable decision and delivery receipt.", body.String(), nil)
	})
	mux.HandleFunc("POST /admin/operators", func(w http.ResponseWriter, r *http.Request) {
		if hooks.Decide == nil {
			http.Error(w, "operator.unavailable", 503)
			return
		}
		command, err := readOperatorCommand(r)
		if err != nil {
			http.Error(w, "operator.invalid", 400)
			return
		}
		receipt, err := hooks.Decide(r.Context(), adminIDFromContext(r.Context()), command)
		if err != nil {
			http.Error(w, "operator.refused", 409)
			return
		}
		if receipt.Command.ID != command.ID {
			http.Error(w, "operator.unavailable", 503)
			return
		}
		http.Redirect(w, r, "/admin/operators/"+receipt.Command.ID, http.StatusSeeOther)
	})
	guarded := m.RequireAdmin(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		guarded.ServeHTTP(w, r.WithContext(ctx))
	})
}

func readOperatorCommand(r *http.Request) (store.AdminOperationCommand, error) {
	var c store.AdminOperationCommand
	typeName, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typeName != "application/x-www-form-urlencoded" {
		return c, store.ErrAdminOperation
	}
	if err = r.ParseForm(); err != nil {
		return c, err
	}
	allowed := map[string]bool{"csrf_token": true, "id": true, "kind": true, "target_account_id": true, "room_id": true, "owner_id": true, "owner_generation": true, "source_ledger_id": true, "amount": true, "prior_sanction_id": true, "until": true, "reason": true}
	for k, v := range r.PostForm {
		if !allowed[k] || len(v) != 1 {
			return c, store.ErrAdminOperation
		}
	}
	c.ID = r.PostForm.Get("id")
	c.Kind = r.PostForm.Get("kind")
	c.Reason = r.PostForm.Get("reason")
	c.TargetAccountID = r.PostForm.Get("target_account_id")
	c.RoomID = r.PostForm.Get("room_id")
	c.OwnerID = r.PostForm.Get("owner_id")
	c.PriorSanctionID = r.PostForm.Get("prior_sanction_id")
	for name, target := range map[string]*int64{"owner_generation": &c.OwnerGeneration, "source_ledger_id": &c.SourceLedgerID, "amount": &c.Amount} {
		if value := r.PostForm.Get(name); value != "" {
			*target, err = strconv.ParseInt(value, 10, 64)
			if err != nil || *target < 1 {
				return c, store.ErrAdminOperation
			}
		}
	}
	if value := r.PostForm.Get("until"); value != "" {
		at, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return c, err
		}
		c.Until = &at
	}
	parsed, err := uuid.Parse(c.ID)
	if err != nil || parsed == uuid.Nil || parsed.String() != c.ID || strings.TrimSpace(c.Reason) == "" {
		return c, store.ErrAdminOperation
	}
	switch c.Kind {
	case "room_kick", "room_close", "account_sanction", "sanction_lift", "noin_grant", "noin_refund":
	default:
		return c, store.ErrAdminOperation
	}
	return c, nil
}
