package v2

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/knowoff/knowoff/server/internal/transport"
)

var testLimits = Limits{MaxFrameBytes: 65536, MaxHistoryEvents: 1024, MaxHistoryPageEvents: 32, MaxTextBytes: 512, MaxRequestsPerSeat: 512}

type fixture struct {
	Name string          `json:"name"`
	Kind string          `json:"kind"`
	Code ErrorCode       `json:"code"`
	Wire json.RawMessage `json:"wire"`
}

func fixtures(t *testing.T) []fixture {
	t.Helper()
	b, err := os.ReadFile("testdata/contracts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []fixture
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func TestGoldenContracts(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.Name, func(t *testing.T) {
			var dst Validatable
			switch f.Kind {
			case "action":
				dst = &ActionRequest{}
			case "match":
				dst = &MatchContract{}
			case "lobby":
				dst = &LobbyState{}
			case "snapshot":
				dst = &Snapshot{}
			case "history_page":
				dst = &HistoryPage{}
			case "error":
				dst = &ErrorEvent{}
			default:
				t.Fatalf("unknown fixture kind: %s", f.Kind)
			}
			err := Decode(f.Wire, dst, testLimits)
			if f.Code != "" {
				var ce *ContractError
				if !errors.As(err, &ce) || ce.Code != f.Code {
					t.Fatalf("got %v, want %s", err, f.Code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(dst)
			if err != nil {
				t.Fatal(err)
			}
			var original, roundtrip any
			if err := json.Unmarshal(f.Wire, &original); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &roundtrip); err != nil {
				t.Fatal(err)
			}
			a, _ := json.Marshal(original)
			b, _ := json.Marshal(roundtrip)
			if string(a) != string(b) {
				t.Fatalf("wire drift\nwant %s\ngot  %s", a, b)
			}
		})
	}
}

func TestStrictJSONAndFrameBounds(t *testing.T) {
	valid := `{"v":2,"request_id":"req-a","match_id":"match-a","mode_id":"missed_the_briefing","round":1,"turn":1,"phase":"play","phase_id":"phase-a","expected_board_revision":0,"action":{"kind":"respond","copy_id":"copy-a"}}`
	for _, raw := range []string{
		strings.Replace(valid, `"v":2`, `"v":2,"v":2`, 1),
		strings.Replace(valid, `"kind":"respond"`, `"kind":"respond","kind":"respond"`, 1),
		strings.Replace(valid, `"expected_board_revision":0,`, ``, 1),
		strings.Replace(valid, `"expected_board_revision":0`, `"expected_board_revision":null`, 1),
		strings.Replace(valid, `"round":1`, `"round":1.0`, 1),
		strings.Replace(valid, `"copy_id":"copy-a"`, `"copy_id":"copy-a","role":"nower"`, 1),
		strings.Replace(valid, `"copy_id":"copy-a"`, `"copy_id":"copy-a","offer_id":""`, 1),
		valid + `{}`, `null`, `[]`,
	} {
		if err := Decode([]byte(raw), &ActionRequest{}, testLimits); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	limits := testLimits
	limits.MaxFrameBytes = len(valid) - 1
	limits.MaxTextBytes = 128
	if err := Decode([]byte(valid), &ActionRequest{}, limits); errorCode(err) != ErrFrameTooLarge {
		t.Fatalf("frame bound: %v", err)
	}
	if transport.ProtocolVersion != 1 {
		t.Fatal("Phase 1 must not activate live v2")
	}
	if _, err := transport.DecodeEnvelope([]byte(valid), testLimits.MaxFrameBytes); err == nil {
		t.Fatal("legacy decoder accepted v2")
	}
}

func TestRequestBudgetPreservesRetries(t *testing.T) {
	if err := CheckRequestBudget(testLimits.MaxRequestsPerSeat, false, testLimits); errorCode(err) != ErrRequestLimit {
		t.Fatalf("new request over bound: %v", err)
	}
	if err := CheckRequestBudget(testLimits.MaxRequestsPerSeat, true, testLimits); err != nil {
		t.Fatalf("identical retry at bound: %v", err)
	}
	if err := CheckRequestBudget(testLimits.MaxRequestsPerSeat-1, false, testLimits); err != nil {
		t.Fatal(err)
	}
}

func TestRecipientSequencesAndReconnect(t *testing.T) {
	current := Cursor{StreamEpoch: "epoch-a", RecipientSeq: 5, EvidenceSeq: 3}
	for _, tc := range []struct {
		name string
		next Cursor
		want ErrorCode
	}{
		{"private delivery", Cursor{"epoch-a", 6, 3}, ""},
		{"public evidence", Cursor{"epoch-a", 6, 4}, ""},
		{"duplicate", Cursor{"epoch-a", 5, 3}, ErrDuplicateEvent},
		{"gap", Cursor{"epoch-a", 7, 4}, ErrSequenceGap},
		{"evidence gap", Cursor{"epoch-a", 6, 5}, ErrSequenceGap},
		{"evidence regression", Cursor{"epoch-a", 6, 2}, ErrStaleEvidence},
		{"old epoch", Cursor{"epoch-old", 6, 4}, ErrStaleStream},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorCode(CheckNext(current, tc.next)); got != tc.want {
				t.Fatalf("%s != %s", got, tc.want)
			}
		})
	}
}

func TestRequestIdentityAndStaleContext(t *testing.T) {
	var req ActionRequest
	for _, f := range fixtures(t) {
		if f.Name == "respond" {
			if err := Decode(f.Wire, &req, testLimits); err != nil {
				t.Fatal(err)
			}
		}
	}
	hash, err := RequestHash(req)
	if err != nil {
		t.Fatal(err)
	}
	record := RequestRecord{MatchID: req.MatchID, Seat: 0, RequestID: req.RequestID, BodyHash: hash, ResultEventID: "event-a"}
	if duplicate, err := CheckRequestReuse(record, req, 0); err != nil || !duplicate {
		t.Fatalf("duplicate: %v %v", duplicate, err)
	}
	req.Action.CopyID = "different-copy"
	if _, err := CheckRequestReuse(record, req, 0); errorCode(err) != ErrRequestConflict {
		t.Fatalf("conflict: %v", err)
	}
	if duplicate, err := CheckRequestReuse(record, req, 1); err != nil || duplicate {
		t.Fatalf("different recipient scope: %v %v", duplicate, err)
	}
	context := ActionContext{MatchID: req.MatchID, ModeID: req.ModeID, Round: req.Round, Turn: req.Turn, Phase: req.Phase, PhaseID: req.PhaseID, BoardRevision: 1, DeadlineMS: 2000, NowMS: 1999}
	if errorCode(CheckActionContext(req, context)) != ErrStaleRevision {
		t.Fatal("accepted stale board")
	}
	context.BoardRevision = req.ExpectedBoardRevision
	if err := CheckActionContext(req, context); err != nil {
		t.Fatal(err)
	}
	context.NowMS = context.DeadlineMS
	if errorCode(CheckActionContext(req, context)) != ErrDeadlineExpired {
		t.Fatal("accepted action at deadline")
	}
	context.NowMS = 1999
	context.PhaseID = "new-phase"
	if errorCode(CheckActionContext(req, context)) != ErrStalePhase {
		t.Fatal("accepted old phase identity")
	}
}

func errorCode(err error) ErrorCode {
	var ce *ContractError
	if errors.As(err, &ce) {
		return ce.Code
	}
	if err != nil {
		return "unexpected_error"
	}
	return ""
}
