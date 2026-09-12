package media

import (
	"errors"
	"reflect"
	"testing"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func TestTextDealRetainedScheduleAndNeutralSeeds(t *testing.T) {
	for _, language := range []string{"en", "tr", "ar"} {
		for _, mode := range gamecontract.AllModes() {
			for _, size := range []int{4, 6} {
				t.Run(language+"/"+string(mode)+"/"+string(rune('0'+size)), func(t *testing.T) {
					snapshot, err := NewTextSnapshot(textFixtureBundle(t, language), textFixtureLimits())
					if err != nil {
						t.Fatal(err)
					}
					randomness := TextRandomness{Schedule: 17, Hands: 31, System: 59}
					deal, err := snapshot.Deal(mode, size, textFixtureTuning(), randomness)
					if err != nil {
						t.Fatal(err)
					}
					if deal.ReleaseID != snapshot.Manifest().ReleaseID || deal.Language != language || deal.RulesVersion != "text-v2" || deal.SnapshotSHA256 != snapshot.SHA256() {
						t.Fatal("deal lost immutable contract pins")
					}
					wantRounds := size / 2
					if len(deal.Nowns) != wantRounds || len(deal.Hands) != size || len(deal.SystemSeeds) != wantRounds {
						t.Fatal("partial schedule or hand setup")
					}
					seen := map[string]bool{}
					for _, n := range deal.Nowns {
						if seen[n.ID] {
							t.Fatal("repeated scheduled Nown")
						}
						seen[n.ID] = true
					}
					for _, hand := range deal.Hands {
						if len(hand.Cards) != 5 || len(hand.Reserve) != 3 {
							t.Fatal("hand/reserve budget changed")
						}
						all := append(append([]TextCard(nil), hand.Cards...), hand.Reserve...)
						if err := snapshot.CheckTextCoverage(mode, deal.Nowns, all, textFixtureTuning()); err != nil {
							t.Fatal("actual retained coverage:", err)
						}
						if err := snapshot.CheckTextCoverage(mode, deal.Nowns[:1], hand.Cards, textFixtureTuning()); err != nil {
							t.Fatal("opening hand coverage:", err)
						}
					}
					for _, seeds := range deal.SystemSeeds {
						want := 0
						switch mode {
						case gamecontract.ModeMakeRoom:
							want = 3
						case gamecontract.ModeBadBargains:
							want = size
						case gamecontract.ModeTopThat:
							want = 1
						}
						if len(seeds) != want {
							t.Fatal("wrong system seed count")
						}
						seen := map[string]bool{}
						for _, seed := range seeds {
							if seen[seed.Text] {
								t.Fatal("duplicate wording in neutral board")
							}
							seen[seed.Text] = true
						}
					}
					replayed, err := snapshot.Deal(mode, size, textFixtureTuning(), randomness)
					if err != nil || !reflect.DeepEqual(deal, replayed) {
						t.Fatal("non-deterministic fixture replay")
					}
					randomness.Schedule++
					randomness.Hands++
					changed, err := snapshot.Deal(mode, size, textFixtureTuning(), randomness)
					if err != nil || !reflect.DeepEqual(deal.SystemSeeds, changed.SystemSeeds) {
						t.Fatal("public seeds depend on schedule/hand RNG")
					}
					// Roles are deliberately absent from every dealer input. The engine can
					// assign any valid role permutation to these identical seat budgets.
					deal.Hands[0].Cards[0].Text = "modified copy"
					original, _ := snapshot.Card(deal.Hands[0].Cards[0].ID)
					if original.Text == "modified copy" {
						t.Fatal("deal mutated pinned words")
					}
				})
			}
		}
	}
}

func TestTextDealRejectsIncompleteAndInfeasibleSetup(t *testing.T) {
	for _, name := range []string{"short_schedule", "disjoint_retained_requirements", "search_budget", "wrong_mode", "invalid_size"} {
		t.Run(name, func(t *testing.T) {
			b := textFixtureBundle(t, "en")
			tuning := textFixtureTuning()
			mode := gamecontract.ModeMissedTheBriefing
			size := 6
			switch name {
			case "short_schedule":
				removed := b.Nowns[0].ID
				b.Nowns = b.Nowns[1:]
				var relations []TextSuitability
				for _, r := range b.Suitability {
					if r.NownID != removed {
						relations = append(relations, r)
					}
				}
				b.Suitability = relations
			case "disjoint_retained_requirements":
				for i := range b.Suitability {
					r := &b.Suitability[i]
					if r.Mode != mode {
						continue
					}
					r.Band = "chaos"
					var group int
					switch r.NownID {
					case "situation-0":
						group = 0
					case "situation-1":
						group = 1
					default:
						group = 2
					}
					for c := 0; c < 4; c++ {
						id := b.Cards[group*4+c].ID
						if r.CardID == id {
							r.Band = "high"
							if c >= 2 {
								r.Band = "distant"
							}
						}
					}
				}
			case "search_budget":
				tuning.MaxSearchNodes = 1
			case "wrong_mode":
				mode = "unknown"
			case "invalid_size":
				size = 5
			}
			sealed, err := SealTextBundle(b, textFixtureLimits())
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := NewTextSnapshot(sealed, textFixtureLimits())
			if err != nil {
				t.Fatal(err)
			}
			deal, err := snapshot.Deal(mode, size, tuning, TextRandomness{1, 2, 3})
			if err == nil {
				t.Fatal("infeasible setup accepted")
			}
			if len(deal.Hands) != 0 || len(deal.Nowns) != 0 {
				t.Fatal("failed setup leaked a partial deal")
			}
			if name == "search_budget" && !errors.Is(err, ErrTextSearchBudget) {
				t.Fatal("search exhaustion must differ from proved infeasibility")
			}
		})
	}
}

func TestTextCoverageRejectsForgedAndDuplicateRetainedCards(t *testing.T) {
	snapshot, _ := NewTextSnapshot(textFixtureBundle(t, "en"), textFixtureLimits())
	deal, err := snapshot.Deal(gamecontract.ModeMissedTheBriefing, 6, textFixtureTuning(), TextRandomness{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"duplicate", "forged_revision", "forged_wording", "wrong_pool"} {
		t.Run(name, func(t *testing.T) {
			cards := append(append([]TextCard(nil), deal.Hands[0].Cards...), deal.Hands[0].Reserve...)
			switch name {
			case "duplicate":
				cards[1] = cards[0]
			case "forged_revision":
				cards[0].Revision++
			case "forged_wording":
				cards[0].Text = "forged"
			case "wrong_pool":
				cards[0].Pool = "item"
			}
			if err := snapshot.CheckTextCoverage(gamecontract.ModeMissedTheBriefing, deal.Nowns, cards, textFixtureTuning()); err == nil {
				t.Fatal("invalid retained card accepted")
			}
		})
	}
}
