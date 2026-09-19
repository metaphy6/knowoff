package lobby

import (
	"context"
	"errors"
)

var (
	ErrTextDevUnavailable  = errors.New("dev.unavailable")
	ErrTextDevRoleInvalid  = errors.New("dev.role_invalid")
	ErrTextDevRoleConflict = errors.New("dev.role_conflict")
)

func (m *TextManager) devEnabled() bool {
	return m.deps.Prototype != nil && m.deps.Prototype.Manifest().Synthetic && m.deps.Config.App.Env != "prod" && m.deps.Config.App.Env != "production"
}

// DevRole changes only the authenticated member's next-match preference.
// It survives reconnect/rematch within this room and never mutates active roles.
func (m *TextManager) DevRole(ctx context.Context, p *TextPeer, role string) error {
	r, unlock, err := m.lockRuntime(ctx, p)
	if err != nil {
		return err
	}
	defer unlock()
	if !m.devEnabled() {
		return ErrTextDevUnavailable
	}
	if role != "random" && role != "nower" && role != "donower" {
		return ErrTextDevRoleInvalid
	}
	_, member := m.member(r, p.AccountID)
	donowers, nowers := 0, 0
	for _, s := range r.seats {
		candidate := s.devRole
		if s == member {
			candidate = role
		}
		if candidate == "donower" {
			donowers++
		}
		if candidate == "nower" {
			nowers++
		}
	}
	if donowers > r.settings.Size/2-1 || nowers > r.settings.Size-(r.settings.Size/2-1) {
		return ErrTextDevRoleConflict
	}
	member.devRole = role
	return m.devRoleFrame(r, p)
}
func (m *TextManager) devRoleFrame(r *textRoom, p *TextPeer) error {
	if !m.devEnabled() {
		return nil
	}
	_, member := m.member(r, p.AccountID)
	if member == nil {
		return ErrTextMembership
	}
	role := member.devRole
	if role == "" {
		role = "random"
	}
	return m.emit(p, "dev_role", "", map[string]string{"role": role})
}

// DevSpecialty replaces only the caller's held specialty in a private match.
// It grants ownership, never an action or an exemption from use restrictions.
func (m *TextManager) DevSpecialty(ctx context.Context, p *TextPeer, specialty string) error {
	r, unlock, err := m.lockRuntime(ctx, p)
	if err != nil {
		return err
	}
	defer unlock()
	if !m.devEnabled() {
		return ErrTextDevUnavailable
	}
	if r.match == nil {
		return ErrTextMembership
	}
	seat, _ := m.member(r, p.AccountID)
	if err := r.match.DevGrantSpecialty(ctx, seat, specialty); err != nil {
		return err
	}
	return m.snapshot(ctx, r, seat, p)
}
