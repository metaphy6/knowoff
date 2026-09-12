// The production backfill package is retired. These tests retain the old bot
// boundary's substantive safety checks against the shared offline text engine.
package bots_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
	"gopkg.in/yaml.v3"
)

func offlineTextMatch(t *testing.T, mode gamecontract.ModeID, size int) (*game.TextMatch, *time.Time) {
	t.Helper()
	raw, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.WebSocket.MaxMessageBytes = 65536
	if err := yaml.Unmarshal(raw, &cfg.Tuning); err != nil {
		t.Fatal(err)
	}
	snapshot, err := media.LoadTextPack("../../pkg/media/testdata/text-en", media.TextLimits{MaxTextBytes: cfg.Tuning.Contract.MaxTextBytes, MaxRecords: cfg.Tuning.TextCatalog.MaxRecords, MaxFileBytes: cfg.Tuning.TextCatalog.MaxFileBytes, MaxBundleBytes: cfg.Tuning.TextCatalog.MaxBundleBytes})
	if err != nil {
		t.Fatal(err)
	}
	tuning := cfg.Tuning
	deal, err := snapshot.Deal(mode, size, media.TextDealTuning{HandSize: tuning.Hand.Size, ReserveSize: tuning.Hand.DrawPile, MinHigh: tuning.Dealing.MinHighPerNown, MinDistant: tuning.Dealing.MinDistantPerNown, MaxSearchNodes: tuning.TextCatalog.MaxSearchNodes}, media.TextRandomness{Schedule: 1, Hands: 2, System: 3})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := tuning.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	contract := v2.MatchContract{ProtocolVersion: 2, MatchID: uuid.NewString(), RoomID: uuid.NewString(), ModeID: mode, OriginalSize: size, ContentLanguage: deal.Language, PackReleaseID: deal.ReleaseID, PackSHA256: deal.SnapshotSHA256, RulesVersion: deal.RulesVersion, Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: hash}, Eligibility: v2.Eligibility{AdmissionID: uuid.NewString(), EntryPath: "quick_play"}}
	m, err := game.NewTextMatch(game.TextOptions{Config: cfg, Contract: contract, Deal: deal, Prototype: true, Now: func() time.Time { return now }, Seed: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := m.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	return m, &now
}

func TestOfflineSimulationNeverTakesOverDisconnectedHuman(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				m, now := offlineTextMatch(t, mode, size)
				initial, err := m.Snapshot(0)
				if err != nil {
					t.Fatal(err)
				}
				*now = time.UnixMilli(initial.DeadlineMS)
				if _, err := m.Advance(t.Context(), *now); err != nil {
					t.Fatal(err)
				}
				turn, err := m.Snapshot(0)
				if err != nil || turn.CurrentSeat == nil {
					t.Fatal("turn unavailable", err)
				}
				seat := *turn.CurrentSeat
				before, err := m.Snapshot(seat)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := m.SetConnected(t.Context(), seat, false); err != nil {
					t.Fatal(err)
				}
				after, err := m.Snapshot(seat)
				if err != nil {
					t.Fatal(err)
				}
				if after.Seats[seat].Connected || len(after.Seats) != size || len(after.Private.Hand) != len(before.Private.Hand)-1 || after.Private.ReserveCount != before.Private.ReserveCount {
					t.Fatal("disconnect replaced or played human hand")
				}
				if len(after.History) != len(before.History)+1 {
					t.Fatal("disconnect must emit exactly its required pass")
				}
				for _, event := range after.History[len(before.History):] {
					if event.Kind != "auto_pass" || event.Reason != "disconnect" {
						t.Fatal("disconnect produced bot action", event)
					}
				}
				// The engine has no autonomous actor goroutine. Only an explicit owner clock
				// advances state, and it never silently reconnects this human.
				_, deadline := m.Clock()
				*now = deadline
				if _, err := m.Advance(t.Context(), *now); err != nil {
					t.Fatal(err)
				}
				later, err := m.Snapshot(seat)
				if err != nil || later.Seats[seat].Connected {
					t.Fatal("clock silently replaced human", err)
				}
				if err := m.CheckConservation(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
