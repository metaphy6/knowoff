// Package textcert executes release witnesses through the authoritative engine.
// Its dependency direction is textcert -> game -> media, never media -> game.
package textcert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
	"gopkg.in/yaml.v3"
)

// Tuning exposes the existing complete server policy to offline tools without a
// second schema or a forbidden import of internal packages from another module.
type Tuning = config.TuningConfig

func DecodeTuning(raw []byte) (Tuning, error) {
	var t Tuning
	d := yaml.NewDecoder(bytes.NewReader(raw))
	d.KnownFields(true)
	if err := d.Decode(&t); err != nil {
		return t, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return t, fmt.Errorf("trailing tuning document")
	}
	return t, nil
}
func Limits(t Tuning) media.TextLimits {
	return media.TextLimits{MaxTextBytes: t.Contract.MaxTextBytes, MaxRecords: t.TextCatalog.MaxRecords, MaxFileBytes: t.TextCatalog.MaxFileBytes, MaxBundleBytes: t.TextCatalog.MaxBundleBytes}
}
func Dealing(t Tuning) media.TextDealTuning {
	return media.TextDealTuning{HandSize: t.Hand.Size, ReserveSize: t.Hand.DrawPile, MinHigh: t.Dealing.MinHighPerNown, MinDistant: t.Dealing.MinDistantPerNown, MaxSearchNodes: t.TextCatalog.MaxSearchNodes}
}

// Certify records two full-match branch witnesses per sampled schedule and cell.
// Scenario policy is versioned; it is a test driver, never a gameplay engine.
func Certify(s *media.TextSnapshot, t Tuning, samples int, seed int64) (media.TextActionEvidence, error) {
	return CertifyContext(context.Background(), s, t, samples, seed)
}

// CertifyContext checks cancellation before any work and each witness/engine step.
func CertifyContext(ctx context.Context, s *media.TextSnapshot, t Tuning, samples int, seed int64) (media.TextActionEvidence, error) {
	if err := ctx.Err(); err != nil {
		return media.TextActionEvidence{}, err
	}
	e := media.TextActionEvidence{}
	if s == nil || len(s.Manifest().Modes) == 0 || samples < 1 || t.TextCatalog.MaxSearchNodes < 1 || t.Contract.MaxHistoryEvents < 1 || t.TextCatalog.MaxFileBytes < 1 || samples > t.TextCatalog.MaxSearchNodes/(4*len(s.Manifest().Modes)) || seed == 0 || seed > math.MaxInt64-int64(samples)-2 || seed < 0 {
		return e, fmt.Errorf("invalid action certification inputs/budget")
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return e, err
	}
	m := s.Manifest()
	e = media.TextActionEvidence{SchemaVersion: 2, Algorithm: media.TextActionAlgorithm, Scope: media.TextActionScope, SnapshotSHA256: s.SHA256(), RulesVersion: m.RulesVersion, Language: m.Language, TuningSHA256: media.ContentHash(raw), Tuning: raw, Samples: samples, Seed: seed}
	modes := append([]gamecontract.ModeID(nil), m.Modes...)
	sort.Slice(modes, func(i, j int) bool { return modes[i] < modes[j] })
	remaining := t.TextCatalog.MaxSearchNodes
	for _, mode := range modes {
		for _, size := range []int{4, 6} {
			c := media.TextActionCell{Mode: mode, TableSize: size}
			for sample := 0; sample < samples; sample++ {
				for _, scenario := range []string{"actions", "timeouts"} {
					if err := ctx.Err(); err != nil {
						return media.TextActionEvidence{}, err
					}
					w, err := witness(ctx, s, t, mode, size, seed+int64(sample), scenario, remaining)
					if err != nil {
						return media.TextActionEvidence{}, fmt.Errorf("action witness %s/%d/%s: %w", mode, size, scenario, err)
					}
					remaining -= len(w.Steps)
					c.Witnesses = append(c.Witnesses, w)
				}
			}
			e.Cells = append(e.Cells, c)
			// Enforce private artifact byte budget during construction as well as decode.
			encoded, err := json.Marshal(e)
			if err != nil || int64(len(encoded)) > t.TextCatalog.MaxFileBytes {
				return media.TextActionEvidence{}, fmt.Errorf("action artifact byte budget exceeded")
			}
		}
	}
	return e, nil
}

// Verify recomputes the entire ordered script and history under the exact full
// policy. Changed counters, seeds, clocks, requests or coverage cannot pass.
func Verify(s *media.TextSnapshot, t Tuning, raw []byte) error {
	return VerifyContext(context.Background(), s, t, raw)
}

// VerifyContext aborts new replay work when its request is canceled.
func VerifyContext(ctx context.Context, s *media.TextSnapshot, t Tuning, raw []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	e, err := media.DecodeTextActionEvidence(raw, s, t.TextCatalog.MaxFileBytes, t.TextCatalog.MaxSearchNodes)
	if err != nil {
		return err
	}
	policy, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if e.TuningSHA256 != media.ContentHash(policy) || !bytes.Equal(e.Tuning, policy) {
		return fmt.Errorf("action tuning mismatch")
	}
	expected, err := CertifyContext(ctx, s, t, e.Samples, e.Seed)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(e, expected) {
		return fmt.Errorf("action replay cannot be reproduced")
	}
	return nil
}
func ValidateActivation(s *media.TextSnapshot, rules string, t Tuning) error {
	return ValidateActivationContext(context.Background(), s, rules, t)
}

// ValidateActivationContext composes the legacy retained-card/human gates with
// cancellable action replay. The legacy media gate itself remains synchronous.
func ValidateActivationContext(ctx context.Context, s *media.TextSnapshot, rules string, t Tuning) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ValidateActivation(rules, Dealing(t)); err != nil {
		return err
	}
	return VerifyContext(ctx, s, t, s.Bundle().Artifacts["action-replay.json"])
}

func witness(ctx context.Context, pack *media.TextSnapshot, t Tuning, mode gamecontract.ModeID, size int, seed int64, scenario string, budget int) (media.TextActionWitness, error) {
	w := media.TextActionWitness{Seed: seed, Scenario: scenario, Steps: []media.TextActionStep{}}
	deal, err := pack.Deal(mode, size, Dealing(t), media.TextRandomness{Schedule: seed, Hands: seed + 1, System: seed + 2})
	if err != nil {
		return w, err
	}
	hash, err := t.SHA256()
	if err != nil {
		return w, err
	}
	contract := v2.MatchContract{ProtocolVersion: 2, MatchID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("textcert:%s:%d:%d:%s", mode, size, seed, scenario))).String(), RoomID: "certification", OriginalSize: size, ModeID: mode, RulesVersion: deal.RulesVersion, ContentLanguage: deal.Language, PackReleaseID: deal.ReleaseID, PackSHA256: deal.SnapshotSHA256, Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: hash}, Eligibility: v2.Eligibility{AdmissionID: "certification", EntryPath: "local"}}
	now := time.Unix(1700000000, 0)
	match, err := game.NewTextMatch(game.TextOptions{Contract: contract, Deal: deal, Config: &config.Config{WebSocket: config.WebSocketConfig{MaxMessageBytes: int(t.TextCatalog.MaxFileBytes)}, Tuning: t}, Seed: seed, Prototype: true, Now: func() time.Time { return now }})
	if err != nil {
		return w, err
	}
	roles := make([]string, size)
	for seat := 0; seat < size; seat++ {
		s, err := match.SnapshotProjection(seat)
		if err != nil {
			return w, err
		}
		roles[seat] = s.Private.Role
	}
	drawn := map[string]bool{}
	trades := 0
	covered := map[string]bool{}
	for n := 0; n < min(budget, t.Contract.MaxHistoryEvents); n++ {
		if err := ctx.Err(); err != nil {
			return w, err
		}
		views := make([]v2.Snapshot, size)
		for seat := range views {
			views[seat], err = match.SnapshotProjection(seat)
			if err != nil {
				return w, err
			}
			if err = privacy(views[seat], deal); err != nil {
				return w, err
			}
		}
		if err := inventoryPrivacy(views); err != nil {
			return w, err
		}
		s := views[0]
		if s.Phase == v2.PhaseVerdict {
			w.Rounds = s.Round
			if w.Rounds != size/2 {
				return w, fmt.Errorf("witness ended before full schedule")
			}
			for _, event := range s.History {
				covered[event.Kind] = true
				if event.Kind == "resolve_offer" {
					covered[event.Resolution] = true
				}
			}
			if !covered["draw"] || scenario == "timeouts" && !covered["auto_pass"] {
				return w, fmt.Errorf("missing draw/timeout branch")
			}
			if scenario == "actions" {
				kind := map[gamecontract.ModeID]string{gamecontract.ModeMissedTheBriefing: "respond", gamecontract.ModeSecretScale: "place", gamecontract.ModeMakeRoom: "replace", gamecontract.ModeTopThat: "top", gamecontract.ModeBadBargains: "offer"}[mode]
				if !covered[kind] {
					return w, fmt.Errorf("missing mode action")
				}
				if mode == gamecontract.ModeBadBargains && (!covered["accept"] || !covered["refuse"] || !covered["timeout"]) {
					return w, fmt.Errorf("missing trade branches")
				}
			}
			raw, _ := json.Marshal(s.History)
			w.HistorySHA256 = media.ContentHash(raw)
			return w, nil
		}
		seat := -1
		var action *v2.Action
		if s.Phase == v2.PhaseTradeResponse {
			if trades%3 != 2 {
				seat = s.PendingOffer.RecipientSeat
				resolution := "accept"
				if trades%3 == 1 {
					resolution = "refuse"
				}
				action = &v2.Action{Kind: v2.ActionResolveOffer, OfferID: s.PendingOffer.OfferID, Resolution: resolution}
			}
			trades++
		} else if s.Phase == v2.PhasePlay && s.CurrentSeat != nil {
			seat = *s.CurrentSeat
			view := views[seat]
			key := fmt.Sprintf("%d/%d", s.Round, s.Turn)
			if !drawn[key] && view.Private.ReserveCount > 0 {
				drawn[key] = true
				one := 1
				action = &v2.Action{Kind: v2.ActionDraw, Count: &one}
			} else if scenario == "actions" {
				action = modeAction(view)
			}
		} else if s.Phase == v2.PhaseKnowoff || s.Phase == v2.PhaseRunoff {
			// Privileged ballot steering preserves the complete schedule. Card decisions
			// above see only an authorized view; this is not a model of human inference.
			target := -1
			want := "nower"
			if s.Round < size/2-1 {
				want = "donower"
			}
			for _, p := range s.Seats {
				if !p.Eliminated && roles[p.Seat] == want {
					target = p.Seat
					break
				}
			}
			for i, view := range views {
				if i == target || view.Seats[i].Eliminated {
					continue
				}
				voted := false
				for _, v := range view.Ballot.Votes {
					if v.Seat == i {
						voted = true
					}
				}
				if !voted {
					seat = i
					action = &v2.Action{Kind: v2.ActionVote, TargetSeat: &target}
					break
				}
			}
		}
		step := media.TextActionStep{AtMS: now.UnixMilli(), Seat: -1}
		if action != nil {
			view := views[seat]
			req := v2.ActionRequest{Version: 2, RequestID: fmt.Sprintf("cert-%d", n), MatchID: contract.MatchID, ModeID: mode, Round: view.Round, Turn: view.Turn, Phase: view.Phase, PhaseID: view.PhaseID, ExpectedBoardRevision: view.Board.Revision, Action: *action}
			if _, err = match.Apply(ctx, seat, req); err != nil {
				return w, err
			}
			step.Seat = seat
			step.Request, _ = json.Marshal(req)
		} else {
			now = time.UnixMilli(s.DeadlineMS)
			step.AtMS = now.UnixMilli()
			if _, err = match.Advance(ctx, now); err != nil {
				return w, err
			}
		}
		w.Steps = append(w.Steps, step)
		if err = match.CheckConservation(); err != nil {
			return w, err
		}
	}
	return w, fmt.Errorf("action replay step budget exhausted")
}

func modeAction(s v2.Snapshot) *v2.Action {
	if len(s.Private.Hand) == 0 {
		return nil
	}
	card := s.Private.Hand[0]
	switch s.Contract.ModeID {
	case gamecontract.ModeMissedTheBriefing:
		return &v2.Action{Kind: v2.ActionRespond, CopyID: card.CopyID}
	case gamecontract.ModeSecretScale:
		rating := 1 + (s.Turn-1)%5
		return &v2.Action{Kind: v2.ActionPlace, CopyID: card.CopyID, Rating: &rating}
	case gamecontract.ModeMakeRoom:
		return &v2.Action{Kind: v2.ActionReplace, CopyID: card.CopyID, Slot: s.Board.Cards[(s.Turn-1)%len(s.Board.Cards)].Slot}
	case gamecontract.ModeTopThat:
		return &v2.Action{Kind: v2.ActionTop, CopyID: card.CopyID, TargetCopyID: s.Board.Cards[len(s.Board.Cards)-1].Card.CopyID}
	case gamecontract.ModeBadBargains:
		for _, b := range s.Board.Cards {
			if b.Seat != nil && *b.Seat != s.Private.Seat && !s.Seats[*b.Seat].Eliminated {
				return &v2.Action{Kind: v2.ActionOffer, CopyID: card.CopyID, TargetCopyID: b.Card.CopyID, TargetSeat: b.Seat}
			}
		}
	}
	return nil
}

func privacy(s v2.Snapshot, deal media.TextDeal) error {
	eliminated := s.Seats[s.Private.Seat].Eliminated
	if (s.Private.Role != "nower" || eliminated || s.Phase == v2.PhaseVerdict) && s.Private.Nown != nil {
		return fmt.Errorf("unauthorized Nown projection")
	}
	if eliminated && (len(s.Private.Hand) > 0 || s.Private.ReserveCount != 0) {
		return fmt.Errorf("eliminated inventory projection")
	}
	if s.Private.Nown != nil && (string(s.Private.Nown.ContentID) != deal.Nowns[s.Round-1].ID || s.Private.Nown.Revision != deal.Nowns[s.Round-1].Revision || s.Private.Nown.Text != deal.Nowns[s.Round-1].Text) {
		return fmt.Errorf("wrong Nown projection")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	for _, nown := range deal.Nowns[s.Round:] {
		id, _ := json.Marshal(nown.ID)
		if bytes.Contains(raw, id) {
			return fmt.Errorf("future Nown projection")
		}
	}
	for _, e := range s.History {
		if e.Kind == "draw" && (len(e.Cards) != 0 || e.Count == nil) {
			return fmt.Errorf("draw identity leak")
		}
	}
	return nil
}

// A previously exposed traded card can legitimately remain in public history.
// Unexposed hand-copy identities must appear only in their owner's projection.
func inventoryPrivacy(views []v2.Snapshot) error {
	public := map[v2.CopyID]bool{}
	for _, event := range views[0].History {
		for _, card := range event.Cards {
			public[card.CopyID] = true
		}
	}
	for _, card := range views[0].Board.Cards {
		public[card.Card.CopyID] = true
	}
	for seat, view := range views {
		raw, err := json.Marshal(view)
		if err != nil {
			return err
		}
		for owner, other := range views {
			if owner == seat {
				continue
			}
			for _, card := range other.Private.Hand {
				if public[card.CopyID] {
					continue
				}
				id, _ := json.Marshal(card.CopyID)
				if bytes.Contains(raw, id) {
					return fmt.Errorf("foreign hidden hand projection")
				}
			}
		}
	}
	return nil
}
