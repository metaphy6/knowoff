package store

import (
	"context"
	"database/sql"
	"sort"
	"time"
)

// TextAdmissionBinding is server-verified connection authority. It is never a
// request payload or persisted match content; accepted-work recovery needs no JWT.
type TextAdmissionBinding struct {
	AccountID, DeviceHash, TokenID, Purpose string
	SessionEpoch                            int64
	ExpiresAt                               time.Time
}
type textAdmissionBindingsKey struct{}

func WithTextAdmissionBindings(ctx context.Context, bindings []TextAdmissionBinding) context.Context {
	return context.WithValue(ctx, textAdmissionBindingsKey{}, append([]TextAdmissionBinding(nil), bindings...))
}
func (s *TextValueStore) RequireAdmissionBindings() *TextValueStore {
	copy := *s
	copy.requireAdmissionBindings = true
	return &copy
}

// All participating accounts are already locked. No account lock may follow
// these sorted distinct installation locks; sanction capture uses the same order.
func (s *TextValueStore) checkAdmissionBindings(ctx context.Context, tx *sql.Tx, accounts []string, prototype bool) error {
	if !s.requireAdmissionBindings {
		return nil
	}
	bindings, ok := ctx.Value(textAdmissionBindingsKey{}).([]TextAdmissionBinding)
	if !ok || len(bindings) != len(accounts) {
		return ErrTextTrust
	}
	byAccount := make(map[string]TextAdmissionBinding, len(bindings))
	hashes := map[string]bool{}
	for _, b := range bindings {
		if _, duplicate := byAccount[b.AccountID]; duplicate || b.TokenID == "" || b.ExpiresAt.IsZero() {
			return ErrTextTrust
		}
		byAccount[b.AccountID] = b
		if b.Purpose == "development" && prototype {
			continue
		}
		if b.Purpose != "player" || b.DeviceHash == "" {
			return ErrTextTrust
		}
		hashes[b.DeviceHash] = true
	}
	for _, account := range accounts {
		if _, ok := byAccount[account]; !ok {
			return ErrTextTrust
		}
	}
	ordered := make([]string, 0, len(hashes))
	for hash := range hashes {
		ordered = append(ordered, hash)
	}
	sort.Strings(ordered)
	for _, hash := range ordered {
		var found string
		if err := tx.QueryRowContext(ctx, `SELECT device_hash FROM auth_installations WHERE device_hash=$1 FOR UPDATE`, hash).Scan(&found); err != nil {
			return ErrTextTrust
		}
	}
	for _, account := range accounts {
		b := byAccount[account]
		var allowed bool
		if err := tx.QueryRowContext(ctx, `SELECT auth_purpose=$2 AND session_epoch=$3 AND $4::timestamptz>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM auth_revocations WHERE token_id=$5) AND NOT direct_account_sanction_active(id) AND ($6::boolean OR NOT installation_sanction_active($7)) FROM accounts WHERE id=$1`, account, b.Purpose, b.SessionEpoch, b.ExpiresAt, b.TokenID, prototype && b.Purpose == "development", b.DeviceHash).Scan(&allowed); err != nil || !allowed {
			return ErrTextTrust
		}
	}
	return nil
}
