package media

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// DealingTuning is imported from the config package shape so this package
// can be used independently (e.g. by tools/mediapack).
type DealingTuning struct {
	BandHigh          float64
	BandLow           float64
	MinHighPerNown    int
	MinDistantPerNown int
}

// DefaultDealingTuning returns the v1 dealing thresholds from tuning.yaml.
func DefaultDealingTuning() DealingTuning {
	return DealingTuning{
		BandHigh:          0.55,
		BandLow:           0.30,
		MinHighPerNown:    2,
		MinDistantPerNown: 2,
	}
}

// Cosine returns the cosine similarity between two vectors. Vectors must be
// non-empty and of equal length; the function returns -1 for invalid input.
func Cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return -1
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return -1
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// BandFor classifies a cosine similarity value into a relevance band.
func BandFor(score float64, dealing DealingTuning) string {
	switch {
	case score >= dealing.BandHigh:
		return "high"
	case score >= dealing.BandLow:
		return "distant"
	default:
		return "chaos"
	}
}

// BuildCandidates computes per-Nown card candidate lists for each band.
func BuildCandidates(media []*MediaItem, cards []*CardItem, dealing DealingTuning) map[string]*BandCandidates {
	candidates := make(map[string]*BandCandidates, len(media))
	for _, n := range media {
		bc := &BandCandidates{}
		for _, c := range cards {
			score := Cosine(n.Embedding, c.Embedding)
			switch BandFor(score, dealing) {
			case "high":
				bc.High = append(bc.High, c.ID)
			case "distant":
				bc.Distant = append(bc.Distant, c.ID)
			default:
				bc.Chaos = append(bc.Chaos, c.ID)
			}
		}
		candidates[n.ID] = bc
	}
	return candidates
}

// Hand is the dealt cards for one player.
type Hand struct {
	Cards     []string // IDs in hand (played + unplayed order not enforced here)
	DrawPile  []string // IDs in draw pile
	Specialty string   // optional specialty marker for tests; not persisted
}

// DealResult contains the dealt hands for a match.
type DealResult struct {
	NownIDs []string
	Hands   []Hand
	Seed    int64
}

// Dealer deals hands for a match from a loaded pack.
type Dealer struct {
	Pack    *Pack
	Dealing DealingTuning
	Hand    HandTuning
}

// NewDealer creates a Dealer from a loaded pack and tuning values.
func NewDealer(pack *Pack, dealing DealingTuning) *Dealer {
	return &Dealer{
		Pack:    pack,
		Dealing: dealing,
		Hand:    HandTuning{Size: 5, DrawPile: 3},
	}
}

// HandTuning is the shape of the hand config used for dealing.
type HandTuning struct {
	Size     int
	DrawPile int
}

// Deal produces one hand per player for the given table size and Nown
// schedule. It is deterministic for a given rng source.
func (d *Dealer) Deal(tableSize int, nownIDs []string, rng *rand.Rand) (*DealResult, error) {
	if d.Pack == nil {
		return nil, fmt.Errorf("no pack loaded")
	}
	playerCount := tableSize
	if playerCount == 0 {
		return nil, fmt.Errorf("nown schedule is empty")
	}

	cardPool := make([]*CardItem, len(d.Pack.Cards))
	copy(cardPool, d.Pack.Cards)

	hands := make([]Hand, playerCount)
	for i := range hands {
		hands[i] = Hand{
			Cards:    make([]string, 0, d.Hand.Size),
			DrawPile: make([]string, 0, d.Hand.DrawPile),
		}
	}

	for _, nownID := range nownIDs {
		cands, ok := d.Pack.Candidates[nownID]
		if !ok {
			return nil, fmt.Errorf("nown %s not found in pack", nownID)
		}
		for p := 0; p < playerCount; p++ {
			allocated := map[string]bool{}
			for _, id := range hands[p].Cards {
				allocated[id] = true
			}
			for _, id := range hands[p].DrawPile {
				allocated[id] = true
			}

			pick := func(list []string, count int) ([]string, bool) {
				shuffled := make([]string, len(list))
				copy(shuffled, list)
				rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
				var out []string
				for _, id := range shuffled {
					if allocated[id] {
						continue
					}
					out = append(out, id)
					allocated[id] = true
					if len(out) == count {
						break
					}
				}
				return out, len(out) == count
			}

			high, ok := pick(cands.High, d.Dealing.MinHighPerNown)
			if !ok {
				return nil, fmt.Errorf("cannot satisfy high requirement for nown %s player %d", nownID, p)
			}
			distant, ok := pick(cands.Distant, d.Dealing.MinDistantPerNown)
			if !ok {
				return nil, fmt.Errorf("cannot satisfy distant requirement for nown %s player %d", nownID, p)
			}

			remaining := d.Hand.Size - len(high) - len(distant)
			var allCands []string
			allCands = append(allCands, cands.High...)
			allCands = append(allCands, cands.Distant...)
			allCands = append(allCands, cands.Chaos...)
			fill, ok := pick(allCands, remaining)
			if !ok {
				return nil, fmt.Errorf("cannot fill hand for nown %s player %d", nownID, p)
			}

			combined := append(high, distant...)
			combined = append(combined, fill...)
			for _, id := range combined {
				if len(hands[p].Cards) < d.Hand.Size {
					hands[p].Cards = append(hands[p].Cards, id)
				} else if len(hands[p].DrawPile) < d.Hand.DrawPile {
					hands[p].DrawPile = append(hands[p].DrawPile, id)
				}
			}
		}
	}

	// Fill any remaining draw-pile slots from the full pool.
	for p := range hands {
		allocated := map[string]bool{}
		for _, id := range hands[p].Cards {
			allocated[id] = true
		}
		for _, id := range hands[p].DrawPile {
			allocated[id] = true
		}
		var pool []string
		for _, c := range d.Pack.Cards {
			pool = append(pool, c.ID)
		}
		rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		for _, id := range pool {
			if allocated[id] {
				continue
			}
			if len(hands[p].DrawPile) >= d.Hand.DrawPile {
				break
			}
			hands[p].DrawPile = append(hands[p].DrawPile, id)
		}
	}

	return &DealResult{
		NownIDs: nownIDs,
		Hands:   hands,
	}, nil
}

// DealFeasible runs Monte Carlo deals and reports how many succeeded.
func DealFeasible(d *Dealer, tableSize int, nownIDs []string, rounds int, seed int64) (success int, total int, err error) {
	for i := 0; i < rounds; i++ {
		rng := rand.New(rand.NewSource(seed + int64(i)))
		_, err := d.Deal(tableSize, nownIDs, rng)
		if err != nil {
			return 0, 0, err
		}
		success++
		total++
	}
	return success, total, nil
}

// MediaByID returns a Nown by id from the pack, or nil if absent.
func (p *Pack) MediaByID(id string) *MediaItem {
	for _, n := range p.Media {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// CardByID returns a card by id from the pack, or nil if absent.
func (p *Pack) CardByID(id string) *CardItem {
	for _, c := range p.Cards {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// CardReachable returns true if every card appears in at least one band list.
func CardReachable(pack *Pack) ([]string, bool) {
	reachable := map[string]bool{}
	for _, cands := range pack.Candidates {
		for _, id := range cands.High {
			reachable[id] = true
		}
		for _, id := range cands.Distant {
			reachable[id] = true
		}
		for _, id := range cands.Chaos {
			reachable[id] = true
		}
	}

	var missing []string
	for _, c := range pack.Cards {
		if !reachable[c.ID] {
			missing = append(missing, c.ID)
		}
	}
	sort.Strings(missing)
	return missing, len(missing) == 0
}
