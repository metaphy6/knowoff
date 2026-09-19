package lobby

import (
	"encoding/json"
	"errors"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"testing"
)

func TestTextDevRoleNextMatchAndReconnect(t *testing.T) {
	m, _, _, settings := textManagerFixture(t)
	peers, r := textReadyRoom(t, m, settings)
	if err := m.DevRole(t.Context(), peers[0], "donower"); err != nil {
		t.Fatal(err)
	}
	if err := m.DevRole(t.Context(), peers[1], "donower"); err == nil {
		t.Fatal("conflict accepted")
	}
	if err := m.Start(t.Context(), peers[0]); err != nil {
		t.Fatal(err)
	}
	before, err := r.match.Snapshot(0)
	if err != nil {
		t.Fatal(err)
	}
	if before.Private.Role != "donower" {
		t.Fatal("role ignored")
	}
	if err := m.DevRole(t.Context(), peers[0], "nower"); err != nil {
		t.Fatal(err)
	}
	after, err := r.match.Snapshot(0)
	if err != nil {
		t.Fatal(err)
	}
	if after.Private.Role != "donower" {
		t.Fatal("active role mutated")
	}
	replacement, err := m.Open(t.Context(), peers[0].AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Join(t.Context(), replacement, r.code); err != nil {
		t.Fatal(err)
	}
	found := false
	for len(replacement.frames) > 0 {
		f := <-replacement.frames
		if f.Type == "dev_role" {
			var p map[string]string
			if err = json.Unmarshal(f.Payload, &p); err != nil {
				t.Fatal(err)
			}
			found = p["role"] == "nower"
		}
	}
	if !found {
		t.Fatal("reconnect preference missing")
	}
	if err = m.DevRole(t.Context(), peers[0], "random"); err == nil {
		t.Fatal("replaced connection changed preference")
	}
}
func TestTextDevRoleGates(t *testing.T) {
	for _, gate := range []string{"live", "prod", "production"} {
		m, _, _, s := textManagerFixture(t)
		p, _ := textReadyRoom(t, m, s)
		if gate == "live" {
			m.deps.Prototype = nil
		} else {
			m.deps.Config.App.Env = gate
		}
		if err := m.DevRole(t.Context(), p[0], "donower"); err == nil {
			t.Fatal("unsafe override", gate)
		}
	}
	m, _, _, s := textManagerFixture(t)
	p, _ := textReadyRoom(t, m, s)
	if err := m.DevRole(t.Context(), p[0], "other"); err == nil {
		t.Fatal("unknown role")
	}
	for _, peer := range p[:3] {
		if err := m.DevRole(t.Context(), peer, "nower"); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.DevRole(t.Context(), p[3], "nower"); err == nil {
		t.Fatal("all nower accepted")
	}
}

func TestTextDevSpecialtyOwnSeatAndGates(t *testing.T) {
	m, _, _, settings := textManagerFixture(t)
	peers, r := textReadyRoom(t, m, settings)
	if err := m.DevSpecialty(t.Context(), peers[0], "pass"); err == nil {
		t.Fatal("grant outside match accepted")
	}
	if err := m.Start(t.Context(), peers[0]); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pass", "reveal", "one_more_free_card", "shuffle", "revote", ""} {
		otherBefore, err := r.match.Snapshot(1)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.DevSpecialty(t.Context(), peers[0], name); err != nil {
			t.Fatal(err)
		}
		own, err := r.match.Snapshot(0)
		if err != nil {
			t.Fatal(err)
		}
		other, err := r.match.Snapshot(1)
		if err != nil {
			t.Fatal(err)
		}
		if own.Private.Specialty != name || other.Private.Specialty != otherBefore.Private.Specialty {
			t.Fatal("grant changed wrong ownership")
		}
	}
	if err := m.DevSpecialty(t.Context(), peers[0], "unknown"); err == nil {
		t.Fatal("unknown specialty accepted")
	}
	for _, environment := range []string{"prod", "production"} {
		m.deps.Config.App.Env = environment
		if err := m.DevSpecialty(t.Context(), peers[0], "pass"); err == nil {
			t.Fatal("production grant accepted")
		}
	}
	m.deps.Config.App.Env = "local"
	m.deps.Prototype = nil
	if err := m.DevSpecialty(t.Context(), peers[0], "pass"); err == nil {
		t.Fatal("live grant accepted")
	}
}

func TestTextDevRoleSizeChangeConflictIsExplicit(t *testing.T) {
	m, _, _, settings := textManagerFixture(t)
	settings.Size = 6
	peers, room := textReadyRoom(t, m, settings)
	for _, peer := range peers[:2] {
		if err := m.DevRole(t.Context(), peer, "donower"); err != nil {
			t.Fatal(err)
		}
	}
	for _, peer := range peers[4:] {
		if err := m.Leave(t.Context(), peer); err != nil {
			t.Fatal(err)
		}
	}
	settings.Size = 4
	if err := m.Settings(t.Context(), peers[0], room.settingsRevision, settings); err != nil {
		t.Fatal(err)
	}
	for _, peer := range peers[:4] {
		if err := m.Ready(t.Context(), peer, v2.ReadyAcknowledgement{SettingsRevision: room.settingsRevision, MembershipRevision: room.membershipRevision}); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Start(t.Context(), peers[0]); !errors.Is(err, ErrTextDevRoleConflict) {
		t.Fatalf("wanted explicit role conflict; got %v", err)
	}
	if room.match != nil {
		t.Fatal("conflicted role setup started a match")
	}
}
