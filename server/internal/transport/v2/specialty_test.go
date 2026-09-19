package v2

import (
	"encoding/json"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"testing"
)

func TestSpecialtyActionWireAllModes(t *testing.T) {
	l := Limits{MaxFrameBytes: 65536, MaxHistoryEvents: 500, MaxHistoryPageEvents: 50, MaxTextBytes: 500, MaxRequestsPerSeat: 500}
	for _, mode := range gamecontract.AllModes() {
		for _, kind := range []ActionKind{ActionPass, ActionReveal, ActionFreeCard, ActionShuffle, ActionRevote, ActionViewReveal} {
			r := ActionRequest{Version: 2, RequestID: "specialty", MatchID: "match", ModeID: mode, Round: 1, Turn: 1, Phase: PhasePlay, PhaseID: "phase", ExpectedBoardRevision: 1, Action: Action{Kind: kind}}
			if kind == ActionRevote {
				r.Phase = PhaseKnowoff
			}
			if kind == ActionReveal {
				target := 1
				r.Action.TargetSeat = &target
			}
			raw, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var decoded ActionRequest
			if err = Decode(raw, &decoded, l); err != nil {
				t.Fatal(mode, kind, err)
			}
			r.Action.CopyID = "forbidden"
			raw, err = json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			if err = Decode(raw, &decoded, l); err == nil {
				t.Fatal("contradictoryspecialtyfield", kind)
			}
		}
	}
}

func TestSpecialtyCapabilitiesRequireHeldCardAndRole(t *testing.T) {
	for _, kind := range []ActionKind{ActionPass, ActionReveal, ActionFreeCard, ActionShuffle} {
		s := snapshotFixture(t, "snapshot-nower")
		s.Private.Capabilities = []ActionKind{kind}
		if err := s.Validate(testLimits); err == nil {
			t.Fatal("unheld specialty", kind)
		}
	}
	s := snapshotFixture(t, "snapshot-nower")
	s.Private.Specialty = "shuffle"
	s.Private.Capabilities = []ActionKind{ActionShuffle}
	if err := s.Validate(testLimits); err == nil {
		t.Fatal("wrongrole shuffle")
	}
	s = snapshotFixture(t, "snapshot-nower")
	s.Private.FreeDraws = 1
	if err := s.Validate(testLimits); err == nil {
		t.Fatal("cardplay advertised beforefree draw")
	}
	s = snapshotFixture(t, "snapshot-nower")
	s.Private.Capabilities = []ActionKind{ActionViewReveal}
	if err := s.Validate(testLimits); err == nil {
		t.Fatal("view withouttarget")
	}
}
