package v2

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/knowoff/knowoff/server/internal/config"
	"gopkg.in/yaml.v3"
)

func TestWorstCaseHistoryPagesAreCompleteAndBounded(t *testing.T) {
	limits := testLimits
	limits.MaxFrameBytes = 4096
	// Three rounds, six seats: every seat draws its reserve and the bounded
	// request budget includes changes of ballot, offer attempts and resolutions.
	// This is synthetic frame stress, not a gameplay/content certification.
	events := make([]PublicAction, 900)
	for i := range events {
		actor := i % 6
		count := 1
		events[i] = PublicAction{Phase: PhasePlay, PhaseID: "phase-a", EventID: fmt.Sprintf("event-%d", i+1), EvidenceSeq: uint64(i + 1), Round: i/300 + 1, Actor: Actor{Kind: "seat", Seat: &actor}, Kind: "draw", Cards: []Card{}, BeforeRevision: uint64(i), AfterRevision: uint64(i + 1), Reason: "player", ServerTimeMS: 1000 + int64(i), Count: &count}
	}
	manifest, pages, err := PaginateHistory("match-a", "snapshot-a", "epoch-a", events, limits)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.TotalEvents != len(events) || manifest.ThroughEvidenceSeq != 900 || manifest.PageCount != len(pages) || len(pages) < 2 {
		t.Fatalf("bad manifest: %+v", manifest)
	}
	assembled, err := AssembleHistory(manifest, pages, "match-a", "snapshot-a", "epoch-a", limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(assembled) != len(events) {
		t.Fatal("history was truncated")
	}
	for i, page := range pages {
		data, _ := json.Marshal(page)
		if len(data) > limits.MaxFrameBytes {
			t.Fatalf("page %d exceeds frame: %d", i, len(data))
		}
		if err := Decode(data, &HistoryPage{}, limits); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AssembleHistory(manifest, pages[:len(pages)-1], "match-a", "snapshot-a", "epoch-a", limits); errorCode(err) != ErrHistoryIntegrity {
		t.Fatalf("missing page: %v", err)
	}
	pages[0].Events[0].Count = nil
	if _, err := AssembleHistory(manifest, pages, "match-a", "snapshot-a", "epoch-a", limits); err == nil {
		t.Fatal("accepted tampered page")
	}
}

func TestConfiguredMaximumHistoryFitsFrames(t *testing.T) {
	base, err := os.ReadFile("../../../../configs/base.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tuning, err := os.ReadFile("../../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(base, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(tuning, &cfg.Tuning); err != nil {
		t.Fatal(err)
	}
	l := Limits{MaxFrameBytes: min(cfg.WebSocket.MaxMessageBytes, cfg.RateLimit.MaxBytesPerFrame), MaxHistoryEvents: cfg.Tuning.Contract.MaxHistoryEvents, MaxHistoryPageEvents: cfg.Tuning.Contract.MaxHistoryPageEvents, MaxTextBytes: cfg.Tuning.Contract.MaxTextBytes, MaxRequestsPerSeat: cfg.Tuning.Contract.MaxRequestsPerSeat}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	events := make([]PublicAction, l.MaxHistoryEvents)
	actor, target, count := 0, 1, 1
	for i := range events {
		cards := make([]Card, 6)
		for j := range cards {
			cards[j] = Card{CopyID: CopyID(fmt.Sprintf("copy-%d-%d", i, j)), Content: TextContent{ContentRef: ContentRef{ContentID: ContentID(fmt.Sprintf("text-%d", j)), Revision: 1}, Text: strings.Repeat("x", l.MaxTextBytes)}}
		}
		e := PublicAction{Phase: PhasePlay, PhaseID: "phase-a", EventID: fmt.Sprintf("event-%d", i+1), EvidenceSeq: uint64(i + 1), Round: i*3/len(events) + 1, Actor: Actor{Kind: "seat", Seat: &actor}, Cards: []Card{}, BeforeRevision: uint64(i), AfterRevision: uint64(i + 1), Reason: "player", ServerTimeMS: int64(i + 1)}
		switch i % 6 {
		case 0:
			e.Kind = "seed"
			e.Phase = PhaseRoundStart
			e.Actor = Actor{Kind: "system"}
			e.Reason = "seed"
			e.Cards = cards
		case 1:
			e.Kind = "draw"
			e.Count = &count
		case 2:
			e.Kind = "auto_pass"
			e.Reason = "timeout"
			e.Cards = cards[:1]
		case 3:
			e.Kind = "offer"
			e.OfferID = fmt.Sprintf("offer-%d", i)
			e.Cards = cards[:2]
			e.TargetSeat = &target
			deadline := int64(i + 10000)
			e.DeadlineMS = &deadline
		case 4:
			e.Kind = "resolve_offer"
			e.Phase = PhaseTradeResponse
			e.OfferID = fmt.Sprintf("offer-%d", i-1)
			e.Resolution = "refuse"
			e.Cards = cards[:2]
		case 5:
			e.Kind = "vote"
			e.Phase = PhaseKnowoff
			e.TargetSeat = &target
		}
		events[i] = e
	}
	manifest, pages, err := PaginateHistory("match-six-seats", "snapshot-final", "epoch-new", events, l)
	if err != nil {
		t.Fatal(err)
	}
	all, err := AssembleHistory(manifest, pages, "match-six-seats", "snapshot-final", "epoch-new", l)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != l.MaxHistoryEvents {
		t.Fatal("configured evidence budget truncated")
	}
	maxBytes := 0
	for _, page := range pages {
		encoded, _ := json.Marshal(page)
		maxBytes = max(maxBytes, len(encoded))
	}
	t.Logf("configured maximum: %d events, %d pages, largest frame %d/%d bytes", len(all), len(pages), maxBytes, l.MaxFrameBytes)
}

func TestHistoryLimitsAndMixedSnapshots(t *testing.T) {
	limits := testLimits
	limits.MaxHistoryEvents = 1
	limits.MaxHistoryPageEvents = 1
	actor, count := 0, 1
	event := PublicAction{Phase: PhasePlay, PhaseID: "phase-a", EventID: "event-a", EvidenceSeq: 1, Round: 1, Actor: Actor{Kind: "seat", Seat: &actor}, Kind: "draw", Cards: []Card{}, BeforeRevision: 0, AfterRevision: 1, Reason: "player", ServerTimeMS: 1000, Count: &count}
	if _, _, err := PaginateHistory("match-a", "snapshot-a", "epoch-a", []PublicAction{event, event}, limits); errorCode(err) != ErrHistoryLimit {
		t.Fatalf("event cap: %v", err)
	}
	manifest, pages, err := PaginateHistory("match-a", "snapshot-a", "epoch-a", []PublicAction{event}, limits)
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []struct{ match, snapshot, epoch string }{{"match-b", "snapshot-a", "epoch-a"}, {"match-a", "snapshot-b", "epoch-a"}, {"match-a", "snapshot-a", "epoch-b"}} {
		if _, err := AssembleHistory(manifest, pages, identity.match, identity.snapshot, identity.epoch, limits); errorCode(err) != ErrHistoryIntegrity {
			t.Fatalf("mixed identity: %v", err)
		}
	}
}

func TestHistoryCanonicalJSON(t *testing.T) {
	encoded, err := canonicalJSON(map[string]any{"z": "<>&\u2028\u2029\\u2028", "a": []int{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"a\":[1,2],\"z\":\"<>&\u2028\u2029\\\\u2028\"}"
	if string(encoded) != want {
		t.Fatalf("canonical bytes: %q != %q", encoded, want)
	}
}
