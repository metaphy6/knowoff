package media

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"sort"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

var ErrTextInfeasible = errors.New("text setup is infeasible")
var ErrTextSearchBudget = errors.New("text setup search budget exhausted")

func textTuningValid(t TextDealTuning) bool {
	return t.HandSize > 0 && t.ReserveSize >= 0 && t.MinHigh > 0 && t.MinDistant > 0 && t.MinHigh+t.MinDistant <= t.HandSize && t.MaxSearchNodes > 0
}

// Deal selects a complete distinct schedule before a bounded constraint search
// over each seat's actual retained hand+reserve. No role or entitlement enters
// the procedure. Any failure returns no partial allocation for admission to use.
func (s *TextSnapshot) Deal(mode gamecontract.ModeID, size int, tuning TextDealTuning, randomness TextRandomness) (TextDeal, error) {
	if s == nil || !textHasMode(s.bundle.Manifest.Modes, mode) || (size != 4 && size != 6) || !textTuningValid(tuning) {
		return TextDeal{}, ErrTextInfeasible
	}
	var schedule []TextNown
	var pool []TextCard
	for _, n := range s.bundle.Nowns {
		if textHasMode(n.Modes, mode) {
			schedule = append(schedule, n)
		}
	}
	for _, c := range s.bundle.Cards {
		if textHasMode(c.Modes, mode) {
			pool = append(pool, c)
		}
	}
	if len(schedule) < size/2 || len(pool) < tuning.HandSize+tuning.ReserveSize {
		return TextDeal{}, ErrTextInfeasible
	}
	sort.Slice(schedule, func(i, j int) bool { return schedule[i].ID < schedule[j].ID })
	sort.Slice(pool, func(i, j int) bool { return pool[i].ID < pool[j].ID })
	rngSchedule := rand.New(rand.NewSource(randomness.Schedule))
	rngSchedule.Shuffle(len(schedule), func(i, j int) { schedule[i], schedule[j] = schedule[j], schedule[i] })
	schedule = schedule[:size/2]
	result := TextDeal{ReleaseID: s.bundle.Manifest.ReleaseID, Language: s.bundle.Manifest.Language, RulesVersion: s.bundle.Manifest.RulesVersion, SnapshotSHA256: s.hash, Nowns: schedule, Hands: make([]TextHand, size), SystemSeeds: make([][]TextCard, len(schedule))}
	// This branch uses only mode/size, the complete eligible pool, and System.
	// It intentionally does not filter by Nown relation or player allocations.
	seedCount := 0
	switch mode {
	case gamecontract.ModeMakeRoom:
		seedCount = 3
	case gamecontract.ModeBadBargains:
		seedCount = size
	case gamecontract.ModeTopThat:
		seedCount = 1
	}
	rngSystem := rand.New(rand.NewSource(randomness.System))
	for round := range result.SystemSeeds {
		seeds := append([]TextCard(nil), pool...)
		rngSystem.Shuffle(len(seeds), func(i, j int) { seeds[i], seeds[j] = seeds[j], seeds[i] })
		seen := map[string]bool{}
		for _, card := range seeds {
			if len(result.SystemSeeds[round]) == seedCount {
				break
			}
			if seen[card.Text] {
				continue
			}
			seen[card.Text] = true
			result.SystemSeeds[round] = append(result.SystemSeeds[round], card)
		}
		if len(result.SystemSeeds[round]) != seedCount {
			return TextDeal{}, ErrTextInfeasible
		}
	}
	rngHands := rand.New(rand.NewSource(randomness.Hands))
	nodes := 0
	for seat := range result.Hands {
		shuffled := append([]TextCard(nil), pool...)
		rngHands.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		retained, err := s.textRetained(mode, schedule, shuffled, tuning, &nodes)
		if err != nil {
			return TextDeal{}, err
		}
		// The first hand meets the opening relation minimum; all scheduled Nowns
		// are checked against the complete retained eight, including the reserve.
		chosen := map[string]bool{}
		var hand, reserve []TextCard
		for _, requirement := range []struct {
			band  string
			count int
		}{{"high", tuning.MinHigh}, {"distant", tuning.MinDistant}} {
			count := 0
			for _, card := range retained {
				if count == requirement.count {
					break
				}
				if s.bands[textPair(mode, schedule[0].ID, card.ID)] == requirement.band {
					hand = append(hand, card)
					chosen[card.ID] = true
					count++
				}
			}
		}
		for _, card := range retained {
			if chosen[card.ID] {
				continue
			}
			if len(hand) < tuning.HandSize {
				hand = append(hand, card)
			} else {
				reserve = append(reserve, card)
			}
		}
		rngHands.Shuffle(len(hand), func(i, j int) { hand[i], hand[j] = hand[j], hand[i] })
		rngHands.Shuffle(len(reserve), func(i, j int) { reserve[i], reserve[j] = reserve[j], reserve[i] })
		if err := s.CheckTextCoverage(mode, schedule, append(append([]TextCard(nil), hand...), reserve...), tuning); err != nil {
			return TextDeal{}, err
		}
		result.Hands[seat] = TextHand{Cards: hand, Reserve: reserve}
	}
	return textClone(result), nil
}

func (s *TextSnapshot) textRetained(mode gamecontract.ModeID, schedule []TextNown, pool []TextCard, t TextDealTuning, nodes *int) ([]TextCard, error) {
	budget := t.HandSize + t.ReserveSize
	dimensions := 2 * len(schedule)
	contribution := make([][]int, len(pool))
	suffix := make([][]int, len(pool)+1)
	suffix[len(pool)] = make([]int, dimensions)
	for i := len(pool) - 1; i >= 0; i-- {
		contribution[i] = make([]int, dimensions)
		suffix[i] = append([]int(nil), suffix[i+1]...)
		for n, prompt := range schedule {
			band := s.bands[textPair(mode, prompt.ID, pool[i].ID)]
			d := -1
			if band == "high" {
				d = n * 2
			} else if band == "distant" {
				d = n*2 + 1
			}
			if d >= 0 {
				contribution[i][d] = 1
				suffix[i][d]++
			}
		}
	}
	counts := make([]int, dimensions)
	var selected []TextCard
	exhausted := false
	var search func(int) bool
	search = func(index int) bool {
		*nodes++
		if *nodes > t.MaxSearchNodes {
			exhausted = true
			return false
		}
		if len(selected) > budget || len(selected)+len(pool)-index < budget {
			return false
		}
		for d, count := range counts {
			need := t.MinHigh
			if d%2 == 1 {
				need = t.MinDistant
			}
			if count+suffix[index][d] < need {
				return false
			}
			if len(selected) == budget && count < need {
				return false
			}
		}
		if len(selected) == budget {
			return true
		}
		if index == len(pool) {
			return false
		}
		selected = append(selected, pool[index])
		for d, v := range contribution[index] {
			counts[d] += v
		}
		if search(index + 1) {
			return true
		}
		selected = selected[:len(selected)-1]
		for d, v := range contribution[index] {
			counts[d] -= v
		}
		if exhausted {
			return false
		}
		return search(index + 1)
	}
	if search(0) {
		return selected, nil
	}
	if exhausted {
		return nil, ErrTextSearchBudget
	}
	return nil, ErrTextInfeasible
}

// CheckTextCoverage audits actual retained records, rejecting forged revisions,
// duplicate content within a seat and missing reviewed relations. It does not
// assess action correctness or claim that depleted future hands retain bands.
func (s *TextSnapshot) CheckTextCoverage(mode gamecontract.ModeID, nowns []TextNown, cards []TextCard, tuning TextDealTuning) error {
	if s == nil || !textTuningValid(tuning) || !textHasMode(s.bundle.Manifest.Modes, mode) || len(nowns) == 0 {
		return ErrTextInfeasible
	}
	seen := map[string]bool{}
	for _, card := range cards {
		index, ok := s.cards[card.ID]
		if !ok || seen[card.ID] || !textHasMode(card.Modes, mode) || !reflect.DeepEqual(card, s.bundle.Cards[index]) {
			return fmt.Errorf("invalid retained card")
		}
		seen[card.ID] = true
	}
	seen = map[string]bool{}
	for _, nown := range nowns {
		index, ok := s.nowns[nown.ID]
		if !ok || seen[nown.ID] || !textHasMode(nown.Modes, mode) || !reflect.DeepEqual(nown, s.bundle.Nowns[index]) {
			return fmt.Errorf("invalid schedule record")
		}
		seen[nown.ID] = true
		high, distant := 0, 0
		for _, card := range cards {
			switch s.bands[textPair(mode, nown.ID, card.ID)] {
			case "high":
				high++
			case "distant":
				distant++
			case "chaos":
			default:
				return fmt.Errorf("missing retained relation")
			}
		}
		if high < tuning.MinHigh || distant < tuning.MinDistant {
			return ErrTextInfeasible
		}
	}
	return nil
}
