package lobby

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func testDeps() Deps {
	return Deps{
		Config: &config.Config{
			Tuning: config.TuningConfig{
				Game: config.GameTuning{
					RoomSizes:       []int{4, 6},
					DonowersBySize:  map[int]int{4: 1, 6: 2},
					VotesBySize:     map[int]int{4: 2, 6: 3},
					ReconnectGraceS: 1,
					MinConnected:    3,
				},
			},
		},
		Pack:    &media.Pack{},
		Manager: media.NewManager(nil),
		NodeID:  "test-node",
	}
}

func TestManager_CreateRoom_ValidSizes(t *testing.T) {
	m := NewManager(testDeps())
	for _, size := range []int{4, 6} {
		r, err := m.CreateRoom(size)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if r.Size != size {
			t.Fatalf("size %d: got %d", size, r.Size)
		}
		if len(r.Code) != 6 {
			t.Fatalf("expected 6-char code, got %q", r.Code)
		}
	}
}

func TestManager_CreateRoom_InvalidSize(t *testing.T) {
	m := NewManager(testDeps())
	if _, err := m.CreateRoom(5); err == nil {
		t.Fatal("expected error for size 5")
	}
}

func TestManager_RoomByCode(t *testing.T) {
	m := NewManager(testDeps())
	r, _ := m.CreateRoom(4)
	if m.RoomByCode(r.Code) != r {
		t.Fatal("lookup by code failed")
	}
	if m.RoomByID(r.ID) != r {
		t.Fatal("lookup by id failed")
	}
}

func TestManager_RoomNodeAffinity(t *testing.T) {
	redisAddr := os.Getenv("KNOWOFF_TEST_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	client := store.NewRedisClient(redisAddr, "", 0)
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("redis not available (%v); run python3 xops/test/tests-lints.py for disposable services", err)
	}
	defer client.Close()

	deps := testDeps()
	deps.Redis = client
	m := NewManager(deps)
	r, _ := m.CreateRoom(4)

	node, err := client.RoomNode(context.Background(), r.ID)
	if err != nil {
		t.Fatalf("room node lookup: %v", err)
	}
	if node != deps.NodeID {
		t.Fatalf("expected node %q, got %q", deps.NodeID, node)
	}

	m.DestroyRoom(r.ID)
	_, err = client.RoomNode(context.Background(), r.ID)
	if err == nil {
		t.Fatal("expected room mapping removed after destroy")
	}
}

func TestRoom_SessionTokenAndReconnect(t *testing.T) {
	r := NewRoom("r1", "AAAAAA", 4, 0, false, testDeps())
	seat, token, ok := r.ClaimSeat("", false)
	if !ok || seat != 0 || token == "" {
		t.Fatalf("unexpected claim result seat=%d token=%q ok=%v", seat, token, ok)
	}
	if r.SessionToken(0) != token {
		t.Fatal("session token mismatch")
	}
	if _, ok := r.ReclaimSeat("bad"); ok {
		t.Fatal("bad token should not reclaim")
	}
	if s, ok := r.ReclaimSeat(token); !ok || s != 0 {
		t.Fatalf("reclaim failed s=%d ok=%v", s, ok)
	}
}

func TestRoom_GraceExpiryMarksAbsent(t *testing.T) {
	r := NewRoom("r1", "AAAAAA", 4, 0, false, testDeps())
	seat, _, _ := r.ClaimSeat("", false)
	r.SetConnection(seat, nil)

	// Wait for the 1-second grace.
	time.Sleep(1500 * time.Millisecond)

	// After grace, the binding should still exist but the seat is disconnected.
	if r.Connection(seat) != nil {
		t.Fatal("expected disconnected after grace")
	}
}

// Dev-only role forcing (dev_force_role): the room remembers a seat's chosen
// role so a later match start can apply it. Only valid roles stick, and the
// override can be cleared by forcing an empty role.
func TestRoom_DevRoleOverrideStoresAndClears(t *testing.T) {
	r := NewRoom("r1", "AAAAAA", 4, 0, false, testDeps())
	seat, _, _ := r.ClaimSeat("", false)

	if err := r.SetDevRoleOverride(seat, "donower"); err != nil {
		t.Fatalf("set override: %v", err)
	}
	if got := r.DevRoleOverride(seat); got != "donower" {
		t.Fatalf("override = %q, want donower", got)
	}
	if err := r.SetDevRoleOverride(seat, "bogus"); err == nil {
		t.Fatal("expected an unknown role to be rejected")
	}
	if err := r.SetDevRoleOverride(seat, ""); err != nil {
		t.Fatalf("clear override: %v", err)
	}
	if got := r.DevRoleOverride(seat); got != "" {
		t.Fatalf("cleared override = %q, want empty", got)
	}
}
