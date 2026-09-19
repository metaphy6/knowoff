package media

import (
	"encoding/json"
	"fmt"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

const TextActionAlgorithm = "authoritative-full-schedule-v1"
const TextActionScope = "sampled-full-schedule-actions-with-bounded-branch-witnesses"

// TextActionEvidence is privileged executable evidence, distinct from human
// actions.json attestations and small exhaustive engine fixtures. No player DTO
// contains it. Full tuning and ordered requests/clocks are retained for replay.
type TextActionEvidence struct {
	SchemaVersion  int              `json:"schema_version"`
	Algorithm      string           `json:"algorithm"`
	Scope          string           `json:"scope"`
	SnapshotSHA256 string           `json:"snapshot_sha256"`
	RulesVersion   string           `json:"rules_version"`
	Language       string           `json:"language"`
	TuningSHA256   string           `json:"tuning_sha256"`
	Tuning         json.RawMessage  `json:"tuning"`
	Samples        int              `json:"samples"`
	Seed           int64            `json:"seed"`
	Cells          []TextActionCell `json:"cells"`
}
type TextActionCell struct {
	Mode      gamecontract.ModeID `json:"mode"`
	TableSize int                 `json:"table_size"`
	Witnesses []TextActionWitness `json:"witnesses"`
}
type TextActionWitness struct {
	Seed          int64            `json:"seed"`
	Scenario      string           `json:"scenario"`
	Rounds        int              `json:"rounds"`
	HistorySHA256 string           `json:"history_sha256"`
	Steps         []TextActionStep `json:"steps"`
}
type TextActionStep struct {
	AtMS    int64           `json:"at_ms"`
	Seat    int             `json:"seat"`
	Request json.RawMessage `json:"request,omitempty"`
}

// DecodeTextActionEvidence bounds bytes and work before engine replay. Structural
// validity alone is not approval: pkg/textcert must reproduce all witnesses.
func DecodeTextActionEvidence(raw []byte, snapshot *TextSnapshot, maxBytes int64, maxSteps int) (TextActionEvidence, error) {
	var e TextActionEvidence
	if snapshot == nil || maxBytes < 1 || maxSteps < 1 || int64(len(raw)) > maxBytes {
		return e, fmt.Errorf("action evidence budget invalid")
	}
	if err := textStrictJSON(raw, &e); err != nil {
		return e, err
	}
	m := snapshot.Manifest()
	if e.SchemaVersion != TextSchemaVersion || e.Algorithm != TextActionAlgorithm || e.Scope != TextActionScope || e.SnapshotSHA256 != snapshot.SHA256() || e.RulesVersion != m.RulesVersion || e.Language != m.Language || !textHashValid(e.TuningSHA256) || ContentHash(e.Tuning) != e.TuningSHA256 || e.Samples < 1 || e.Samples > maxSteps || e.Seed == 0 || len(e.Cells) != 2*len(m.Modes) {
		return e, fmt.Errorf("action evidence identity/coverage invalid")
	}
	seen := map[string]bool{}
	count := 0
	for _, c := range e.Cells {
		key := fmt.Sprintf("%s/%d", c.Mode, c.TableSize)
		if !textHasMode(m.Modes, c.Mode) || (c.TableSize != 4 && c.TableSize != 6) || seen[key] || e.Samples > maxSteps/2 || len(c.Witnesses) != e.Samples*2 {
			return e, fmt.Errorf("action cell coverage invalid")
		}
		seen[key] = true
		for i, w := range c.Witnesses {
			scenario := "actions"
			if i%2 == 1 {
				scenario = "timeouts"
			}
			if w.Seed == 0 || w.Scenario != scenario || w.Rounds != c.TableSize/2 || !textHashValid(w.HistorySHA256) || len(w.Steps) == 0 || len(w.Steps) > maxSteps-count {
				return e, fmt.Errorf("action witness coverage/budget invalid")
			}
			count += len(w.Steps)
			var last int64
			for _, step := range w.Steps {
				if step.AtMS <= 0 || step.AtMS < last || step.Seat < -1 || step.Seat >= c.TableSize || (step.Seat == -1) != (len(step.Request) == 0) {
					return e, fmt.Errorf("invalid action replay step")
				}
				last = step.AtMS
			}
		}
	}
	return e, nil
}
