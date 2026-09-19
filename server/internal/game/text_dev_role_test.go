package game

import (
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"testing"
)

func TestTextDevRolesPreserveCounts(t *testing.T) {
	for _, size := range []int{4, 6} {
		for _, role := range []string{"random", "nower", "donower"} {
			o := textOptions(gamecontract.ModeMissedTheBriefing, size)
			o.DevRoles = map[int]string{0: role}
			m, err := NewTextMatch(o)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for seat := 0; seat < size; seat++ {
				r := textSnapshot(t, m, seat).Private.Role
				if r == "donower" {
					count++
				}
				if seat == 0 && role != "random" && r != role {
					t.Fatalf("wanted %s got %s", role, r)
				}
			}
			if count != size/2-1 {
				t.Fatalf("count %d", count)
			}
		}
	}
}
func TestTextDevRolesRejectUnsafeAndImpossible(t *testing.T) {
	for _, roles := range []map[int]string{{0: "invalid"}, {-1: "nower"}, {4: "nower"}, {0: "donower", 1: "donower"}, {0: "nower", 1: "nower", 2: "nower", 3: "nower"}} {
		o := textOptions(gamecontract.ModeMissedTheBriefing, 4)
		o.DevRoles = roles
		if _, err := NewTextMatch(o); err == nil {
			t.Fatal("accepted invalid roles", roles)
		}
	}
	for _, env := range []string{"prod", "production"} {
		o := textOptions(gamecontract.ModeMissedTheBriefing, 4)
		o.Config.App.Env = env
		o.DevRoles = map[int]string{0: "nower"}
		if _, err := NewTextMatch(o); err == nil {
			t.Fatal("production override accepted")
		}
	}
	o := textOptions(gamecontract.ModeMissedTheBriefing, 4)
	o.Prototype = false
	o.DevRoles = map[int]string{0: "nower"}
	if _, err := NewTextMatch(o); err == nil {
		t.Fatal("live override accepted")
	}
}
