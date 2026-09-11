package media

import (
	"fmt"
	"strings"
)

// Certification holds the outcome of the certification gate.
type Certification struct {
	Passed  bool
	Errors  []string
	Details CertificationDetails
}

// CertificationDetails reports per-check diagnostics.
type CertificationDetails struct {
	ManifestOK       bool
	BandCoverageOK   bool
	CardReachableOK  bool
	MonteCarlo4OK    bool
	MonteCarlo6OK    bool
	EmbeddingModel   string
	CoverageFailures []BandCoverageFailure
}

// BandCoverageFailure reports a Nown that lacks candidates in a band.
type BandCoverageFailure struct {
	NownID string
	Band   string
	Need   int
	Have   int
}

// Certify runs the full certification gate against a pack.
func Certify(pack *Pack, dealing DealingTuning, hand HandTuning) Certification {
	c := Certification{
		Passed: true,
		Errors: nil,
		Details: CertificationDetails{
			EmbeddingModel: pack.Manifest.EmbeddingModel,
		},
	}

	if err := ValidateManifest(&pack.Manifest); err != nil {
		c.Errors = append(c.Errors, err.Error())
		c.Passed = false
	} else {
		c.Details.ManifestOK = true
	}
	if err := ValidatePackContent(pack); err != nil {
		c.Errors = append(c.Errors, err.Error())
		c.Passed = false
	}

	// Ensure every Nown has enough high/distant candidates for a 6-player table.
	for _, n := range pack.Media {
		cands := pack.Candidates[n.ID]
		needHigh := dealing.MinHighPerNown * 6
		needDistant := dealing.MinDistantPerNown * 6
		if len(cands.High) < needHigh {
			c.Details.CoverageFailures = append(c.Details.CoverageFailures, BandCoverageFailure{
				NownID: n.ID,
				Band:   "high",
				Need:   needHigh,
				Have:   len(cands.High),
			})
		}
		if len(cands.Distant) < needDistant {
			c.Details.CoverageFailures = append(c.Details.CoverageFailures, BandCoverageFailure{
				NownID: n.ID,
				Band:   "distant",
				Need:   needDistant,
				Have:   len(cands.Distant),
			})
		}
	}
	c.Details.BandCoverageOK = len(c.Details.CoverageFailures) == 0
	if !c.Details.BandCoverageOK {
		c.Passed = false
		for _, f := range c.Details.CoverageFailures {
			c.Errors = append(c.Errors, fmt.Sprintf("band coverage: nown %s needs %d %s cards, has %d", f.NownID, f.Need, f.Band, f.Have))
		}
	}

	if missing, ok := CardReachable(pack); !ok {
		c.Details.CardReachableOK = false
		c.Passed = false
		c.Errors = append(c.Errors, fmt.Sprintf("unreachable cards: %s", strings.Join(missing, ", ")))
	} else {
		c.Details.CardReachableOK = true
	}

	dealer := &Dealer{Pack: pack, Dealing: dealing, Hand: hand}

	success4, _, err := DealFeasible(dealer, 4, nownSchedule(pack, 4, 42), 50, 1)
	if err != nil {
		c.Errors = append(c.Errors, fmt.Sprintf("monte carlo 4-player: %v", err))
		c.Passed = false
	} else {
		c.Details.MonteCarlo4OK = success4 == 50
		if !c.Details.MonteCarlo4OK {
			c.Passed = false
			c.Errors = append(c.Errors, fmt.Sprintf("monte carlo 4-player: %d/50 succeeded", success4))
		}
	}

	success6, _, err := DealFeasible(dealer, 6, nownSchedule(pack, 6, 43), 50, 1)
	if err != nil {
		c.Errors = append(c.Errors, fmt.Sprintf("monte carlo 6-player: %v", err))
		c.Passed = false
	} else {
		c.Details.MonteCarlo6OK = success6 == 50
		if !c.Details.MonteCarlo6OK {
			c.Passed = false
			c.Errors = append(c.Errors, fmt.Sprintf("monte carlo 6-player: %d/50 succeeded", success6))
		}
	}

	return c
}

// nownSchedule returns a deterministic schedule of Nown IDs for a table size.
// The schedule uses as many Nowns as rounds the table would play.
func nownSchedule(pack *Pack, size int, seed int) []string {
	maxRounds := 2
	if size == 6 {
		maxRounds = 3
	}
	if maxRounds > len(pack.Media) {
		maxRounds = len(pack.Media)
	}
	var ids []string
	for i := 0; i < maxRounds; i++ {
		ids = append(ids, pack.Media[(i+seed)%len(pack.Media)].ID)
	}
	return ids
}
