package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
	"gopkg.in/yaml.v3"
)

// textPolicy sees exactly one authorized observation. It cannot access a catalog,
// other hands/roles, the future schedule or private relevance annotations.
func textPolicy(s v2.Snapshot, rng *rand.Rand) *v2.Action {
	has := func(kind v2.ActionKind) bool {
		for _, v := range s.Private.Capabilities {
			if v == kind {
				return true
			}
		}
		return false
	}
	pointer := func(n int) *int { return &n }
	if has(v2.ActionResolveOffer) {
		resolution := "refuse"
		if rng.Intn(2) == 0 {
			resolution = "accept"
		}
		return &v2.Action{Kind: v2.ActionResolveOffer, OfferID: s.PendingOffer.OfferID, Resolution: resolution}
	}
	if s.CurrentSeat != nil && *s.CurrentSeat == s.Private.Seat && s.Phase == v2.PhasePlay {
		if len(s.Private.Hand) == 0 {
			if has(v2.ActionDraw) {
				return &v2.Action{Kind: v2.ActionDraw, Count: pointer(1)}
			}
			return nil
		}
		card := s.Private.Hand[rng.Intn(len(s.Private.Hand))]
		switch s.Contract.ModeID {
		case gamecontract.ModeMissedTheBriefing:
			return &v2.Action{Kind: v2.ActionRespond, CopyID: card.CopyID}
		case gamecontract.ModeSecretScale:
			return &v2.Action{Kind: v2.ActionPlace, CopyID: card.CopyID, Rating: pointer(1 + rng.Intn(5))}
		case gamecontract.ModeMakeRoom:
			return &v2.Action{Kind: v2.ActionReplace, CopyID: card.CopyID, Slot: s.Board.Cards[rng.Intn(len(s.Board.Cards))].Slot}
		case gamecontract.ModeTopThat:
			return &v2.Action{Kind: v2.ActionTop, CopyID: card.CopyID, TargetCopyID: s.Board.Cards[len(s.Board.Cards)-1].Card.CopyID}
		case gamecontract.ModeBadBargains:
			choices := []v2.BoardCard{}
			for _, b := range s.Board.Cards {
				if b.Seat != nil && *b.Seat != s.Private.Seat {
					for _, p := range s.Seats {
						if p.Seat == *b.Seat && p.Connected && !p.Eliminated {
							choices = append(choices, b)
						}
					}
				}
			}
			if len(choices) == 0 {
				return nil
			}
			b := choices[rng.Intn(len(choices))]
			return &v2.Action{Kind: v2.ActionOffer, CopyID: card.CopyID, TargetSeat: b.Seat, TargetCopyID: b.Card.CopyID}
		}
	}
	for _, seat := range s.ReadySeats {
		if seat == s.Private.Seat {
			return nil
		}
	}
	if has(v2.ActionVote) && s.Ballot != nil {
		voted := false
		for _, v := range s.Ballot.Votes {
			voted = voted || v.Seat == s.Private.Seat
		}
		if !voted {
			choices := []int{}
			for _, seat := range s.Ballot.Candidates {
				if seat != s.Private.Seat {
					choices = append(choices, seat)
				}
			}
			if len(choices) > 0 {
				return &v2.Action{Kind: v2.ActionVote, TargetSeat: pointer(choices[rng.Intn(len(choices))])}
			}
		}
	}
	if has(v2.ActionReady) {
		return &v2.Action{Kind: v2.ActionReady}
	}
	return nil
}

type textReplayStep struct {
	AtMS    int64             `json:"at_ms"`
	Seat    int               `json:"seat"`
	Request *v2.ActionRequest `json:"request,omitempty"`
	Advance bool              `json:"advance,omitempty"`
}
type textSimulation struct {
	Seed           int64            `json:"seed"`
	Contract       v2.MatchContract `json:"contract"`
	Steps          []textReplayStep `json:"steps"`
	Outcome        string           `json:"outcome"`
	Rounds         int              `json:"rounds"`
	EvidenceSHA256 string           `json:"evidence_sha256"`
}

func simulateText(packPath, tuningPath string, mode gamecontract.ModeID, size int, seed int64) (textSimulation, error) {
	return runText(packPath, tuningPath, mode, size, seed, nil)
}
func replayText(packPath, tuningPath string, script textSimulation) (textSimulation, error) {
	return runText(packPath, tuningPath, script.Contract.ModeID, script.Contract.OriginalSize, script.Seed, &script)
}
func runText(packPath, tuningPath string, mode gamecontract.ModeID, size int, seed int64, script *textSimulation) (textSimulation, error) {
	var report textSimulation
	raw, err := os.ReadFile(tuningPath)
	if err != nil {
		return report, err
	}
	var tuning config.TuningConfig
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err = decoder.Decode(&tuning); err != nil {
		return report, err
	}
	bounds := tuning.TextCatalog
	pack, err := media.LoadTextPack(packPath, media.TextLimits{MaxRecords: bounds.MaxRecords, MaxFileBytes: bounds.MaxFileBytes, MaxBundleBytes: bounds.MaxBundleBytes, MaxTextBytes: tuning.Contract.MaxTextBytes})
	if err != nil {
		return report, err
	}
	deal, err := pack.Deal(mode, size, media.TextDealTuning{HandSize: tuning.Hand.Size, ReserveSize: tuning.Hand.DrawPile, MinHigh: tuning.Dealing.MinHighPerNown, MinDistant: tuning.Dealing.MinDistantPerNown, MaxSearchNodes: bounds.MaxSearchNodes}, media.TextRandomness{Schedule: seed, Hands: seed + 1, System: seed + 2})
	if err != nil {
		return report, err
	}
	hash, err := tuning.SHA256()
	if err != nil {
		return report, err
	}
	contract := v2.MatchContract{ProtocolVersion: 2, MatchID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("text-simulation:%d:%s:%d", seed, mode, size))).String(), RoomID: "simulation", OriginalSize: size, ModeID: mode, RulesVersion: deal.RulesVersion, ContentLanguage: deal.Language, PackReleaseID: deal.ReleaseID, PackSHA256: deal.SnapshotSHA256, Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: hash}, Eligibility: v2.Eligibility{AdmissionID: "simulation", EntryPath: "local"}}
	now := time.Unix(1700000000, 0)
	opts := game.TextOptions{Contract: contract, Deal: deal, Config: &config.Config{WebSocket: config.WebSocketConfig{MaxMessageBytes: 1 << 20}, Tuning: tuning}, Seed: seed, Prototype: true, Now: func() time.Time { return now }}
	if script != nil && script.Contract != contract {
		return report, fmt.Errorf("replay contract mismatch")
	}
	report = textSimulation{Seed: seed, Contract: contract, Steps: []textReplayStep{}}
	opts.Hooks.Finish = func(_ context.Context, result game.TextResult) error { report.Outcome = result.Outcome; return nil }
	m, err := game.NewTextMatch(opts)
	if err != nil {
		return report, err
	}
	rng := rand.New(rand.NewSource(seed))
	ctx := context.Background()
	for step := 0; step < tuning.Contract.MaxHistoryEvents; step++ {
		initial, err := m.Snapshot(0)
		if err != nil {
			return report, err
		}
		if initial.Phase == v2.PhaseVerdict {
			report.Rounds = initial.Round
			raw, _ := json.Marshal(initial.History)
			sum := sha256.Sum256(raw)
			report.EvidenceSHA256 = hex.EncodeToString(sum[:])
			if script != nil && (report.EvidenceSHA256 != script.EvidenceSHA256 || len(report.Steps) != len(script.Steps) || report.Outcome != script.Outcome || report.Rounds != script.Rounds) {
				return report, fmt.Errorf("replay evidence mismatch")
			}
			return report, nil
		}

		if script != nil {
			if len(report.Steps) >= len(script.Steps) {
				return report, fmt.Errorf("incomplete replay")
			}
			next := script.Steps[len(report.Steps)]
			if next.AtMS < now.UnixMilli() || next.Advance == (next.Request != nil) {
				return report, fmt.Errorf("invalid replay step")
			}
			now = time.UnixMilli(next.AtMS)
			if next.Advance {
				if next.Seat != -1 {
					return report, fmt.Errorf("invalid replay clock actor")
				}
				_, err = m.Advance(ctx, now)
			} else {
				_, err = m.Apply(ctx, next.Seat, *next.Request)
			}
			if err != nil {
				return report, err
			}
			report.Steps = append(report.Steps, next)
			continue
		}
		acted := false
		for seat := 0; seat < size; seat++ {
			snapshot, err := m.Snapshot(seat)
			if err != nil {
				return report, err
			}
			action := textPolicy(snapshot, rng)
			if action == nil {
				continue
			}
			request := v2.ActionRequest{Version: 2, RequestID: fmt.Sprintf("simulation-%d", len(report.Steps)+1), MatchID: contract.MatchID, ModeID: mode, Round: snapshot.Round, Turn: snapshot.Turn, Phase: snapshot.Phase, PhaseID: snapshot.PhaseID, ExpectedBoardRevision: snapshot.Board.Revision, Action: *action}
			if _, err = m.Apply(ctx, seat, request); err != nil {
				return report, err
			}
			report.Steps = append(report.Steps, textReplayStep{AtMS: now.UnixMilli(), Seat: seat, Request: &request})
			acted = true
			break
		}
		if !acted {
			now = time.UnixMilli(initial.DeadlineMS)
			if _, err = m.Advance(ctx, now); err != nil {
				return report, err
			}
			report.Steps = append(report.Steps, textReplayStep{AtMS: now.UnixMilli(), Seat: -1, Advance: true})
		}
		if err = m.CheckConservation(); err != nil {
			return report, err
		}
	}
	return report, fmt.Errorf("simulation exceeded configured history budget")
}

func readTextScript(path string) (textSimulation, error) {
	var script textSimulation
	file, err := os.Open(path)
	if err != nil {
		return script, err
	}
	defer file.Close()
	const maximum = 16 << 20
	info, err := file.Stat()
	if err != nil {
		return script, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return script, fmt.Errorf("replay must be a bounded regular file")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maximum+1))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&script); err != nil {
		return script, err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return script, fmt.Errorf("trailing replay data")
	}
	return script, nil
}

func writeTextScript(path string, script textSimulation) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	if err = json.NewEncoder(file).Encode(script); err != nil {
		return err
	}
	err = file.Close()
	return err
}
