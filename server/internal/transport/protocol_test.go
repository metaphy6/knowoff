package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeEnvelope_Valid(t *testing.T) {
	data := []byte(`{"v":1,"kind":"ready","payload":{"v":true}}`)
	env, err := DecodeEnvelope(data, 1024)
	if err != nil {
		t.Fatalf("decode valid envelope: %v", err)
	}
	if env.Version != 1 {
		t.Fatalf("version: %d", env.Version)
	}
	if env.Kind != "ready" {
		t.Fatalf("kind: %s", env.Kind)
	}
	if env.Payload["v"] != true {
		t.Fatalf("payload: %v", env.Payload)
	}
}

func TestDecodeEnvelope_VersionMismatch(t *testing.T) {
	data := []byte(`{"v":99,"kind":"ready"}`)
	if _, err := DecodeEnvelope(data, 1024); err == nil {
		t.Fatal("expected error for bad version")
	}
}

func TestDecodeEnvelope_TooLarge(t *testing.T) {
	data := []byte(`{"v":1,"kind":"ready","payload":{"x":"` + strings.Repeat("a", 2048) + `"}}`)
	if _, err := DecodeEnvelope(data, 1024); err == nil {
		t.Fatal("expected error for oversized frame")
	}
}

func TestDecodeEnvelope_Malformed(t *testing.T) {
	cases := [][]byte{
		[]byte(`not json`),
		[]byte(`{"v":1}`),
		[]byte(`[]`),
	}
	for _, c := range cases {
		if _, err := DecodeEnvelope(c, 1024); err == nil {
			t.Fatalf("expected error for %s", c)
		}
	}
}

func TestNewErrorEnvelope(t *testing.T) {
	env := NewErrorEnvelope("intent.rejected", map[string]any{"reason": "late"}, "play_card")
	if env.Kind != EventError {
		t.Fatal("expected error event")
	}
	params, ok := env.Payload["params"].(map[string]any)
	if !ok {
		t.Fatal("expected params map")
	}
	if params["reply_to"] != "play_card" {
		t.Fatalf("reply_to: %v", params["reply_to"])
	}
}

func TestIntentIsPhase3(t *testing.T) {
	for _, in := range []string{IntentJoinRoom, IntentPlayCard, IntentUseSpecialty,
		IntentDrawCards, IntentCastVote, IntentQuickChat, IntentReady, IntentPoke} {
		if !IntentIsPhase3(in) {
			t.Fatalf("expected %s to be phase 3", in)
		}
	}
	for _, in := range []string{IntentReportMedia, IntentConvertPoints, IntentQueueQuickPlay, "nonsense"} {
		if IntentIsPhase3(in) {
			t.Fatalf("expected %s not to be phase 3", in)
		}
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	env := NewEvent(EventTurnStarted, map[string]any{"seat": 2, "deadline": "2026-08-20T00:00:00Z"})
	b, err := env.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeEnvelope(b, 4096)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Kind != EventTurnStarted {
		t.Fatalf("kind mismatch: %s", decoded.Kind)
	}
	// JSON numbers decode as float64.
	if decoded.Payload["seat"] != float64(2) {
		t.Fatalf("seat mismatch: %v", decoded.Payload["seat"])
	}
}

func TestDecodeEnvelope_NilPayload(t *testing.T) {
	data := []byte(`{"v":1,"kind":"ready"}`)
	env, err := DecodeEnvelope(data, 1024)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Payload == nil {
		t.Fatal("expected empty payload map")
	}
}

// BenchmarkDecodeEnvelope is intentionally small; it keeps the codec honest
// about allocation cost and is part of the load-test baseline.
func BenchmarkDecodeEnvelope(b *testing.B) {
	data, _ := json.Marshal(NewEvent(EventPlayRevealed, map[string]any{
		"seat": 0, "card_id": "card-1", "card_type": "image",
	}))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeEnvelope(data, 4096)
	}
}
