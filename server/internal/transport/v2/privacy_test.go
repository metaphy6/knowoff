package v2

import (
	"encoding/json"
	"testing"
)

func snapshotFixture(t *testing.T, name string) Snapshot {
	t.Helper()
	for _, f := range fixtures(t) {
		if f.Name == name {
			var s Snapshot
			if err := Decode(f.Wire, &s, testLimits); err != nil {
				t.Fatal(err)
			}
			return s
		}
	}
	t.Fatalf("missing fixture %s", name)
	return Snapshot{}
}

func TestSnapshotCurrentActorCapabilities(t *testing.T) {
	s := snapshotFixture(t, "snapshot-nower")
	other := 1
	s.CurrentSeat = &other
	if errorCode(s.Validate(testLimits)) != ErrUnauthorized {
		t.Fatal("non-current recipient received turn capabilities")
	}
	s.Private.Capabilities = []ActionKind{ActionPoke, ActionChat}
	if err := s.Validate(testLimits); err != nil {
		t.Fatal(err)
	}
	s.Seats[other].Connected = false
	if err := s.Validate(testLimits); err == nil {
		t.Fatal("disconnected seat retained current turn")
	}
}

func TestPendingOfferHasPublicEvidenceForEveryRecipient(t *testing.T) {
	s := snapshotFixture(t, "snapshot-pending-offer")
	s.History = s.History[:len(s.History)-1]
	s.Cursor.EvidenceSeq = uint64(len(s.History))
	if err := s.Validate(testLimits); errorCode(err) != ErrHistoryIntegrity {
		t.Fatalf("missing public offered card: %v", err)
	}
}

func TestPublicDrawCannotExposeCardsEvenWithValidPageHash(t *testing.T) {
	actor, count := 0, 1
	e := PublicAction{Phase: PhasePlay, PhaseID: "phase-a", EventID: "event-a", EvidenceSeq: 1, Round: 1, Actor: Actor{Kind: "seat", Seat: &actor}, Kind: "draw", Cards: []Card{{CopyID: "copy-a", Content: TextContent{ContentRef: ContentRef{ContentID: "content-a", Revision: 1}, Text: "Private reserve"}}}, BeforeRevision: 0, AfterRevision: 1, Reason: "player", ServerTimeMS: 1000, Count: &count}
	page := HistoryPage{Version: 2, MatchID: "match-a", SnapshotID: "snapshot-a", StreamEpoch: "epoch-a", Index: 0, FromEvidenceSeq: 1, ThroughEvidenceSeq: 1, Events: []PublicAction{e}, SHA256: eventsHash([]PublicAction{e})}
	if err := page.Validate(testLimits); errorCode(err) != ErrMalformed {
		t.Fatalf("private draw cards passed public schema: %v", err)
	}
}

func TestRecipientDecodeNeverRetainsPriorNown(t *testing.T) {
	s := snapshotFixture(t, "snapshot-nower")
	for _, f := range fixtures(t) {
		if f.Name == "snapshot-donower" {
			if err := Decode(f.Wire, &s, testLimits); err != nil {
				t.Fatal(err)
			}
		}
	}
	if s.Private.Nown != nil {
		t.Fatal("Nower prompt retained when decoding Donower snapshot into reused value")
	}
	encoded, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if _, exists := raw["private"].(map[string]any)["nown"]; exists {
		t.Fatal("absent prompt serialized")
	}
}

func TestBallotSnapshotRejectsIllegalVotesAndReadiness(t *testing.T) {
	s := snapshotFixture(t, "snapshot-nower")
	s.Phase = PhaseRunoff
	s.CurrentSeat = nil
	s.Private.Capabilities = []ActionKind{ActionVote, ActionReady}
	s.ReadySeats = []int{0}
	s.Ballot = &BallotState{Kind: PhaseRunoff, Candidates: []int{1, 2}, Votes: []BallotVote{{Seat: 0, TargetSeat: 1}}}
	if err := s.Validate(testLimits); err != nil {
		t.Fatal(err)
	}
	s.Ballot.Votes[0].TargetSeat = 3
	if err := s.Validate(testLimits); err == nil {
		t.Fatal("runoff accepted target outside tied candidates")
	}
	s.Ballot.Votes[0].TargetSeat = 1
	s.ReadySeats = []int{0, 0}
	if err := s.Validate(testLimits); err == nil {
		t.Fatal("duplicate Ready seat accepted")
	}
	s.ReadySeats = []int{0}
	s.Ballot = nil
	if err := s.Validate(testLimits); err == nil {
		t.Fatal("ballot snapshot omitted live votes")
	}
}

func TestPagedSnapshotKeepsInlineContextChecks(t *testing.T) {
	s := snapshotFixture(t, "snapshot-nower")
	actor, count := 5, 1
	e := PublicAction{EventID: "event-a", EvidenceSeq: 1, Round: 1, Phase: PhasePlay, PhaseID: "phase-a", Actor: Actor{Kind: "seat", Seat: &actor}, Kind: "draw", Cards: []Card{}, BeforeRevision: 0, AfterRevision: 1, Reason: "player", ServerTimeMS: 1000, Count: &count}
	manifest, pages, err := PaginateHistory(s.Contract.MatchID, s.SnapshotID, s.Cursor.StreamEpoch, []PublicAction{e}, testLimits)
	if err != nil {
		t.Fatal(err)
	}
	s.History = []PublicAction{}
	s.HistoryPages = &manifest
	s.Cursor.EvidenceSeq = 1
	s.Board.Revision = 1
	if _, err := ResolveHistory(s, pages, testLimits); errorCode(err) != ErrHistoryIntegrity {
		t.Fatalf("six-seat actor in four-seat snapshot: %v", err)
	}
}

func TestGoldenPagesAssembleIntoApplicableSnapshot(t *testing.T) {
	s := snapshotFixture(t, "snapshot-paged-history")
	var page HistoryPage
	for _, f := range fixtures(t) {
		if f.Name == "public-history-page" {
			if err := Decode(f.Wire, &page, testLimits); err != nil {
				t.Fatal(err)
			}
		}
	}
	complete, err := ResolveHistory(s, []HistoryPage{page}, testLimits)
	if err != nil {
		t.Fatal(err)
	}
	if complete.HistoryPages != nil || len(complete.History) != 1 || complete.Cursor != s.Cursor || complete.DeadlineMS != s.DeadlineMS {
		t.Fatal("assembly changed snapshot cursor/deadline or lost history")
	}
}

func TestSnapshotScoreVisibilityAndBounds(t *testing.T) {
	s := snapshotFixture(t, "snapshot-nower")
	s.Private.Points = 20
	if err := s.Validate(testLimits); err != nil {
		t.Fatal(err)
	}
	s.Scores = []SeatScore{{Seat: 0, Points: 20}}
	if errorCode(s.Validate(testLimits)) != ErrUnauthorized {
		t.Fatal("premature public score")
	}
	s.Scores = nil
	s.Private.Points = -1
	if err := s.Validate(testLimits); err == nil {
		t.Fatal("negative private points")
	}
	s = snapshotFixture(t, "snapshot-verdict-begun-only")
	if len(s.Scores) != s.Contract.OriginalSize {
		t.Fatal("missing verdict scoreboard")
	}
	s.Scores[1].Seat = s.Scores[0].Seat
	if err := s.Validate(testLimits); err == nil {
		t.Fatal("duplicate score seat")
	}
}
