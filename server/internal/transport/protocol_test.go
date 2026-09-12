package transport_test

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
)

var protocolLimits = v2.Limits{MaxFrameBytes: 65536, MaxHistoryEvents: 1024, MaxHistoryPageEvents: 8, MaxTextBytes: 512, MaxRequestsPerSeat: 512}

// The runtime imports whole Go packages. Removing route assembly alone must not
// leave an executable second game behind for a future caller to reconnect.
func TestRetiredExecutableSurfaceAbsent(t *testing.T) {
	forbidden := map[string]string{
		"game":      "Match NewMatch Specialty PlayerHand PayloadRenderer NewPayloadRenderer MatchResult MatchFinishCallback",
		"lobby":     "Manager NewManager Room NewRoom Deps SeatBinding QueueAssignment",
		"bots":      "Manager NewManager BackfillManager NewBackfillManager BotActor NewBotActor",
		"handler":   "RealtimeHandler HandlerDeps ConnectionState RoomCreateHandler RoomJoinHandler",
		"transport": "Envelope DecodeEnvelope NewEvent NewIntent NewErrorEnvelope IntentIsPhase3 WebSocketHandler",
		"economy":   "MatchGrants MatchGrant MatchGrantInput GrantDailyFirstWin CanQueueQuickPlay RecordQuickPlayMatch CheckCooldown RecordAbandon nextCooldownSeconds",
		"notices":   "Broadcaster broadcastNotice",
	}
	for directory, names := range forbidden {
		files, err := filepath.Glob(filepath.Join("..", directory, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			source, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(source, func(node ast.Node) bool {
				name := ""
				switch n := node.(type) {
				case *ast.FuncDecl:
					name = n.Name.Name
				case *ast.TypeSpec:
					name = n.Name.Name
				}
				if name != "" && strings.Contains(" "+names+" ", " "+name+" ") {
					t.Errorf("retired executable %s remains in %s", name, path)
				}
				return true
			})
		}
	}
}

func protocolAction(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("v2/testdata/contracts.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Kind string          `json:"kind"`
		Code string          `json:"code"`
		Wire json.RawMessage `json:"wire"`
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		if f.Kind == "action" && f.Code == "" {
			return f.Wire
		}
	}
	t.Fatal("missing valid action fixture")
	return nil
}

func TestProtocolStrictRoundTripAndBounds(t *testing.T) {
	raw := protocolAction(t)
	var request v2.ActionRequest
	if err := v2.Decode(raw, &request, protocolLimits); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var second v2.ActionRequest
	if err = v2.Decode(encoded, &second, protocolLimits); err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(second)
	if string(encoded) != string(again) {
		t.Fatal("roundtrip changed authoritative request")
	}
	oversized := append(append([]byte{}, raw...), []byte(strings.Repeat(" ", protocolLimits.MaxFrameBytes))...)
	var ce *v2.ContractError
	if err = v2.Decode(oversized, &second, protocolLimits); !errors.As(err, &ce) || ce.Code != v2.ErrFrameTooLarge {
		t.Fatalf("frame bound: %v", err)
	}
	for _, bad := range [][]byte{nil, []byte(`null`), []byte(`[]`), []byte(`{`), append(append([]byte{}, raw...), []byte(`{}`)...)} {
		if err = v2.Decode(bad, &second, protocolLimits); err == nil {
			t.Fatalf("accepted malformed frame %q", bad)
		}
	}
}

func TestProtocolRefusesLegacyAndSpecialtyFrames(t *testing.T) {
	for _, kind := range []string{"join_room", "queue_quickplay", "play_card", "use_specialty", "view_revealed_hand", "dev_grant_specialty", "dev_force_role", "shuffle", "prefetch_ack", "asset_loaded"} {
		t.Run(kind, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"v": 1, "kind": kind, "payload": map[string]any{"size": 4, "specialty": "reveal"}})
			var request v2.ActionRequest
			if err := v2.Decode(raw, &request, protocolLimits); err == nil {
				t.Fatal("legacy frame was accepted")
			}
		})
	}
	raw := protocolAction(t)
	for _, key := range []string{"specialty", "asset_ref", "signed_url", "embedding", "revealed_hand"} {
		t.Run(key, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal(raw, &data); err != nil {
				t.Fatal(err)
			}
			data[key] = "legacy"
			changed, _ := json.Marshal(data)
			var request v2.ActionRequest
			if err := v2.Decode(changed, &request, protocolLimits); err == nil {
				t.Fatal("retired field was accepted")
			}
		})
	}
}
