package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func TestPayloadPinnedWordingSurvivesReplacementReconnectAndVerdict(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				opts := textOptions(mode, size)
				now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
				opts.Now = func() time.Time { return now }
				original := map[v2.ContentRef]string{}
				remember := func(id string, revision uint64, wording string) {
					original[v2.ContentRef{ContentID: v2.ContentID(id), Revision: revision}] = wording
				}
				for _, n := range opts.Deal.Nowns {
					remember(n.ID, n.Revision, n.Text)
				}
				for _, h := range opts.Deal.Hands {
					for _, c := range h.Cards {
						remember(c.ID, c.Revision, c.Text)
					}
					for _, c := range h.Reserve {
						remember(c.ID, c.Revision, c.Text)
					}
				}
				for _, seeds := range opts.Deal.SystemSeeds {
					for _, c := range seeds {
						remember(c.ID, c.Revision, c.Text)
					}
				}
				m, err := NewTextMatch(opts)
				if err != nil {
					t.Fatal(err)
				}
				contract := m.Contract()
				// Simulate the renderer's old same-ID wording-drift defect at the
				// engine boundary. A replacement release supplies revised input;
				// the retained match must own its original copies and full schedule.
				for i := range opts.Deal.Nowns {
					opts.Deal.Nowns[i].Text = "replacement prompt"
					opts.Deal.Nowns[i].Revision++
				}
				for i := range opts.Deal.Hands {
					for j := range opts.Deal.Hands[i].Cards {
						opts.Deal.Hands[i].Cards[j].Text = "replacement card"
						opts.Deal.Hands[i].Cards[j].Revision++
					}
					for j := range opts.Deal.Hands[i].Reserve {
						opts.Deal.Hands[i].Reserve[j].Text = "replacement reserve"
						opts.Deal.Hands[i].Reserve[j].Revision++
					}
				}
				for i := range opts.Deal.SystemSeeds {
					for j := range opts.Deal.SystemSeeds[i] {
						opts.Deal.SystemSeeds[i][j].Text = fmt.Sprintf("replacement seed %d %d", i, j)
						opts.Deal.SystemSeeds[i][j].Revision++
					}
				}
				opts.Deal.ReleaseID = "replacement-release"
				opts.Deal.SnapshotSHA256 = strings.Repeat("b", 64)
				opts.Contract.PackReleaseID, opts.Contract.PackSHA256 = opts.Deal.ReleaseID, opts.Deal.SnapshotSHA256
				opts.Contract.MatchID = "00000000-0000-4000-8000-000000000002"
				fresh, err := NewTextMatch(opts)
				if err != nil {
					t.Fatal(err)
				}
				if got := textSnapshot(t, fresh, 0); got.Private.Hand[0].Content.Text != "replacement card" || got.Private.Hand[0].Content.Revision != 2 {
					t.Fatal("replacement fixture did not change new match")
				}
				seenHistory, seenSecret, seenVerdict := false, false, false
				for step := 0; step < 80; step++ {
					for seat := 0; seat < size; seat++ {
						before := textSnapshot(t, m, seat)
						if err := m.ResetStream(seat); err != nil {
							t.Fatal(err)
						}
						after := textSnapshot(t, m, seat)
						if before.Cursor.StreamEpoch == after.Cursor.StreamEpoch || after.Cursor.RecipientSeq != 1 || after.Contract != contract || before.DeadlineMS != after.DeadlineMS || !reflect.DeepEqual(before.History, after.History) || !reflect.DeepEqual(before.Private, after.Private) {
							t.Fatal("reconnect changed pinned state or failed to replace stream")
						}
						check := func(c v2.TextContent) {
							if wording, ok := original[c.ContentRef]; !ok || wording != c.Text {
								t.Fatal("revised or unpinned wording reached begun match", c.ContentRef)
							}
						}
						if after.Private.Nown != nil {
							check(*after.Private.Nown)
							seenSecret = true
						}
						for _, c := range after.Private.Hand {
							check(c.Content)
						}
						for _, c := range after.Board.Cards {
							check(c.Card.Content)
						}
						for _, e := range after.History {
							for _, c := range e.Cards {
								check(c.Content)
								seenHistory = true
							}
						}
						for _, n := range after.VerdictNowns {
							check(n.Content)
							seenVerdict = true
						}
					}
					s := textSnapshot(t, m, 0)
					if s.Phase == v2.PhaseVerdict {
						if len(s.VerdictNowns) != size/2 || !seenHistory || !seenSecret || !seenVerdict {
							t.Fatal("pinning proof omitted a scheduled round or delivery surface")
						}
						if err := m.CheckConservation(); err != nil {
							t.Fatal(err)
						}
						return
					}
					// Catch one Donower on the first six-seat ballot so all three
					// scheduled prompts really begin before the final verdict.
					if size == 6 && s.Round == 1 && s.Phase == v2.PhaseKnowoff {
						target := -1
						for seat := 0; seat < size; seat++ {
							if textSnapshot(t, m, seat).Private.Role == "donower" {
								target = seat
								break
							}
						}
						if target < 0 {
							t.Fatal("missing fixture Donower")
						}
						for seat := 0; seat < size; seat++ {
							choice := target
							if seat == target {
								choice = (target + 1) % size
							}
							view := textSnapshot(t, m, seat)
							textApply(t, m, view, fmt.Sprintf("pin-vote-%d", seat), v2.Action{Kind: v2.ActionVote, TargetSeat: &choice})
						}
						textReadyAll(t, m, "pin-ready")
					}
					_, deadline := m.Clock()
					now = deadline
					if _, err := m.Advance(context.Background(), now); err != nil {
						t.Fatal(err)
					}
				}
				t.Fatal("pinning proof did not finish")
			})
		}
	}
}

func TestPayloadEveryModeSeatSeesOnlyAuthorizedText(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				m, _ := textFixture(t, mode, size)
				for seat := 0; seat < size; seat++ {
					s := textSnapshot(t, m, seat)
					raw, err := json.Marshal(s)
					if err != nil {
						t.Fatal(err)
					}
					wire := string(raw)
					if err = s.Validate(m.Limits()); err != nil {
						t.Fatal(err)
					}
					if s.Private.Role == "donower" && s.Private.Nown != nil {
						t.Fatal("Donower received secret Nown")
					}
					for other := 0; other < size; other++ {
						if other != seat && strings.Contains(wire, fmt.Sprintf("private-seat-%d-card-", other)) {
							t.Fatal("another hand leaked")
						}
						if s.Seats[other].RevealedRole != "" {
							t.Fatal("premature public role")
						}
					}
					for round := 1; round < size/2; round++ {
						if strings.Contains(wire, fmt.Sprintf("secret-round-%d", round)) {
							t.Fatal("future Nown leaked")
						}
					}
					for _, forbidden := range []string{"signed_url", "asset_ref", "embedding", "candidates_by_nown", "specialty", "revealed_hand", "prefetch"} {
						if strings.Contains(wire, forbidden) {
							t.Fatalf("retired payload member %s", forbidden)
						}
					}
				}
			})
		}
	}
}

func TestPayloadNeverPrefetchesReserveCopies(t *testing.T) {
	// Retain the privacy regression across the complete three-round six-seat
	// clock fixture; elimination and current history are covered in the companion
	// lifecycle tests, while the wire must never include reserve card bodies.
	for _, mode := range gamecontract.AllModes() {
		m, _ := textFixture(t, mode, 6)
		for seat := 0; seat < 6; seat++ {
			raw, err := json.Marshal(textSnapshot(t, m, seat))
			if err != nil {
				t.Fatal(err)
			}
			for owner := 0; owner < 6; owner++ {
				for index := 5; index < 8; index++ {
					if strings.Contains(string(raw), fmt.Sprintf("private-seat-%d-card-%d", owner, index)) {
						t.Fatal("reserve body was prefetched")
					}
				}
			}
		}
	}
}
