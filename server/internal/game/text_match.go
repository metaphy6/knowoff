package game

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"maps"
	"math/rand"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/media"
)

// TextAwardEvent is private durable work, never an outbound player event.
type TextAwardEvent struct {
	MatchID    string    `json:"match_id"`
	Seat       int       `json:"seat"`
	Kind       string    `json:"kind"`
	Ordinal    int       `json:"ordinal"`
	OccurredAt time.Time `json:"occurred_at"`
	Amount     int       `json:"amount"`
	Discreet   bool      `json:"discreet"`
}

type TextPlayerResult struct {
	Seat         int    `json:"seat"`
	Role         string `json:"role"`
	Points       int    `json:"points"`
	CorrectVotes int    `json:"correct_votes"`
	VotesCast    int    `json:"votes_cast"`
	Survivals    int    `json:"survivals"`
	Pokes        int    `json:"pokes"`
	Won          bool   `json:"won"`
	Connected    bool   `json:"connected"`
	Absent       bool   `json:"absent"`
	Eliminated   bool   `json:"eliminated"`
}

type TextResult struct {
	Contract           v2.MatchContract   `json:"contract"`
	Outcome            string             `json:"outcome"`
	Winner             string             `json:"winner"`
	OccurredAt         time.Time          `json:"occurred_at"`
	OriginalHumanCount int                `json:"original_human_count"`
	Players            []TextPlayerResult `json:"players"`
}

type TextHooks struct {
	ModerateChat func(context.Context, int, v2.Action) (v2.Action, error)
	Abandon      func(context.Context, TextAbandonEvent) error
	Award        func(context.Context, TextAwardEvent) error
	Finish       func(context.Context, TextResult) error
}

// TextAbandonEvent is server-private durable work for a first expired grace.
// It carries no cards, role, or client-supplied clock.
type TextAbandonEvent struct {
	MatchID    string
	Seat       int
	OccurredAt time.Time
}

// Callers supply a validated, pinned catalog deal; no roles enter its dealing
// API. Advance is driven by the owning server's clock, never by client time.
type TextOptions struct {
	// DevRoles pins only next-match roles in a private prototype.
	DevRoles  map[int]string
	Contract  v2.MatchContract
	Deal      media.TextDeal
	Config    *config.Config
	Seed      int64
	Now       func() time.Time
	Hooks     TextHooks
	Prototype bool
}

type TextActionResult struct {
	Duplicate bool
	Changed   bool
	Public    []v2.PublicAction
}

// Mutations serialize on serial; mu protects only committed state/projection.
// Hooks may call Snapshot, but must not recursively mutate this same match.
// Failed persistence retains the exact candidate for retry without changing its
// occurrence time or logical identity, even if an earlier hook already committed.
type TextMatch struct {
	devEnabled      bool
	serial          sync.Mutex
	mu              sync.RWMutex
	state           *textState
	contract        v2.MatchContract
	limits          v2.Limits
	timers          config.TimersTuning
	points          config.PointsTuning
	noin            config.NoinTuning
	minRewardHumans int
	grace           time.Duration
	penalizeAbandon bool
	seed            int64
	now             func() time.Time
	hooks           TextHooks
	nowns           []v2.TextContent
	seeds           [][]v2.Card
	allCopies       map[v2.CopyID]v2.Card
	initialCopies   int
	seq             []uint64
	epoch           []string
	pending         *textPending
}

type textCopy struct {
	Card     v2.Card
	Zone     string
	Owner    int
	Reserved bool
}
type textPlayer struct {
	Specialty                                         string
	FreeDraws                                         int
	RevealUntil                                       int64
	RevealUsed                                        bool
	Role                                              string
	Connected, Eliminated, Absent                     bool
	GraceDeadline                                     int64
	AbandonRecorded                                   bool
	PointsBeforeResult                                int
	Hand, Reserve                                     []v2.CopyID
	Points, CorrectVotes, VotesCast, Survivals, Pokes int
}
type textState struct {
	RevealCards             *v2.RevealView
	RevealTarget            *int
	ShuffleUsed, RevoteUsed bool
	Round, Turn             int
	Phase                   v2.Phase
	PhaseID                 string
	PhaseSerial             uint64
	Deadline                int64
	ResultRevealAt          int64
	ResultRevealed          bool
	Board                   v2.Board
	Players                 []textPlayer
	Copies                  map[v2.CopyID]textCopy
	Order                   []int
	RemainingVotes          int
	Ready                   map[int]bool
	Pokes                   map[string]bool
	Votes                   map[int]int
	BallotKind              v2.Phase
	Candidates              []int
	BallotResult            *v2.BallotResult
	Offer                   *v2.PendingOffer
	History                 []v2.PublicAction
	Requests                map[string]v2.RequestRecord
	Result                  *TextResult
}
type textPending struct {
	State    *textState
	Key      string
	Hash     string
	Awards   []TextAwardEvent
	Abandons []TextAbandonEvent
	Result   *TextResult
	Events   []v2.PublicAction
}

func NewTextMatch(o TextOptions) (*TextMatch, error) {
	if o.Config == nil {
		return nil, fmt.Errorf("text match requires pinned tuning")
	}
	c := o.Config
	tuningHash, err := c.Tuning.SHA256()
	if err != nil {
		return nil, err
	}
	if o.Contract.Tuning.Version != config.TuningSnapshotVersion || o.Contract.Tuning.SHA256 != tuningHash {
		return nil, fmt.Errorf("tuning contract mismatch")
	}
	frame := c.WebSocket.MaxMessageBytes
	if c.RateLimit.MaxBytesPerFrame > 0 && c.RateLimit.MaxBytesPerFrame < frame {
		frame = c.RateLimit.MaxBytesPerFrame
	}
	limits := v2.Limits{MaxFrameBytes: frame, MaxHistoryEvents: c.Tuning.Contract.MaxHistoryEvents, MaxHistoryPageEvents: c.Tuning.Contract.MaxHistoryPageEvents, MaxTextBytes: c.Tuning.Contract.MaxTextBytes, MaxRequestsPerSeat: c.Tuning.Contract.MaxRequestsPerSeat}
	if e := o.Contract.Validate(limits); e != nil {
		return nil, e
	}
	if _, e := uuid.Parse(o.Contract.MatchID); e != nil {
		return nil, fmt.Errorf("match identity must be UUID")
	}
	size := o.Contract.OriginalSize
	if len(o.Deal.Hands) != size || len(o.Deal.Nowns) != size/2 || len(o.Deal.SystemSeeds) != size/2 {
		return nil, fmt.Errorf("incomplete full-match deal")
	}
	if o.Deal.ReleaseID != o.Contract.PackReleaseID || o.Deal.SnapshotSHA256 != o.Contract.PackSHA256 || o.Deal.Language != o.Contract.ContentLanguage || o.Deal.RulesVersion != o.Contract.RulesVersion {
		return nil, fmt.Errorf("deal contract mismatch")
	}
	if c.Tuning.Game.VotesBySize[size] != size/2 || c.Tuning.Game.DonowersBySize[size] != size/2-1 || c.Tuning.Hand.Size <= 0 || c.Tuning.Hand.DrawPile < 0 {
		return nil, fmt.Errorf("invalid text game tuning")
	}
	if c.Tuning.Timers.PlayTurn <= 0 || c.Tuning.Timers.TradeResponseS <= 0 || c.Tuning.Timers.KnowoffBallot <= 0 || c.Tuning.Timers.KnowoffRunoff <= 0 || c.Tuning.Timers.VoteResultWindow <= 0 || c.Tuning.Timers.DiscussionPerPlayer <= 0 || c.Tuning.Game.ReconnectGraceS <= 0 {
		return nil, fmt.Errorf("invalid text clocks")
	}
	if c.Tuning.Timers.VoteResultFalling <= 0 || c.Tuning.Timers.VoteResultFalling >= c.Tuning.Timers.VoteResultWindow {
		return nil, fmt.Errorf("invalid text result reveal clock")
	}
	if o.Prototype && (o.Contract.Eligibility.Rewards || o.Contract.Eligibility.Leaderboard) {
		return nil, fmt.Errorf("prototype cannot earn live value")
	}
	if !o.Prototype && o.Contract.Eligibility.EntryPath == "quick_play" && o.Hooks.Abandon == nil {
		return nil, fmt.Errorf("live Quick Play requires durable abandonment hook")
	}
	if o.Contract.Eligibility.Rewards && (o.Hooks.Award == nil || o.Hooks.Finish == nil) {
		return nil, fmt.Errorf("reward-eligible match requires durable hooks")
	}
	if o.Seed == 0 {
		var seed [8]byte
		if _, err := cryptorand.Read(seed[:]); err != nil {
			return nil, err
		}
		o.Seed = int64(binary.BigEndian.Uint64(seed[:]))
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	m := &TextMatch{contract: o.Contract, limits: limits, timers: c.Tuning.Timers, points: c.Tuning.Points, noin: c.Tuning.Noin, minRewardHumans: c.Tuning.Liquidity.NoinMinHumans, grace: time.Duration(c.Tuning.Game.ReconnectGraceS) * time.Second, seed: o.Seed, now: o.Now, hooks: o.Hooks, seq: make([]uint64, size), epoch: make([]string, size)}
	m.devEnabled = o.Prototype && c.App.Env != "prod" && c.App.Env != "production"
	m.penalizeAbandon = !o.Prototype && o.Contract.Eligibility.EntryPath == "quick_play"
	s := &textState{Round: 1, Board: v2.Board{ModeID: o.Contract.ModeID, Cards: []v2.BoardCard{}}, Players: make([]textPlayer, size), Copies: map[v2.CopyID]textCopy{}, RemainingVotes: size / 2, Ready: map[int]bool{}, Pokes: map[string]bool{}, Votes: map[int]int{}, History: []v2.PublicAction{}, Requests: map[string]v2.RequestRecord{}}
	m.allCopies = map[v2.CopyID]v2.Card{}
	m.initialCopies = size * (c.Tuning.Hand.Size + c.Tuning.Hand.DrawPile)
	copyIndex := 0
	card := func(c media.TextCard) v2.Card {
		copyIndex++
		v := v2.Card{CopyID: v2.CopyID(uuid.NewHash(sha256.New(), uuid.Nil, []byte(fmt.Sprintf("%d:%s:copy:%d", o.Seed, o.Contract.MatchID, copyIndex)), 5).String()), Content: v2.TextContent{ContentRef: v2.ContentRef{ContentID: v2.ContentID(c.ID), Revision: c.Revision}, Text: c.Text}}
		m.allCopies[v.CopyID] = v
		return v
	}
	for i, h := range o.Deal.Hands {
		if len(h.Cards) != c.Tuning.Hand.Size || len(h.Reserve) != c.Tuning.Hand.DrawPile {
			return nil, fmt.Errorf("incorrect retained hand/reserve")
		}
		s.Players[i] = textPlayer{Role: "nower", Connected: true, Hand: []v2.CopyID{}, Reserve: []v2.CopyID{}}
		for _, row := range h.Cards {
			v := card(row)
			s.Copies[v.CopyID] = textCopy{Card: v, Zone: "hand", Owner: i}
			s.Players[i].Hand = append(s.Players[i].Hand, v.CopyID)
		}
		for _, row := range h.Reserve {
			v := card(row)
			s.Copies[v.CopyID] = textCopy{Card: v, Zone: "reserve", Owner: i}
			s.Players[i].Reserve = append(s.Players[i].Reserve, v.CopyID)
		}
		s.Players[i].Specialty = dealTextSpecialty(o.Seed, i, c.Tuning.Hand.SpecialtyWeights)
		m.epoch[i] = uuid.NewString()
	}
	for _, n := range o.Deal.Nowns {
		m.nowns = append(m.nowns, v2.TextContent{ContentRef: v2.ContentRef{ContentID: v2.ContentID(n.ID), Revision: n.Revision}, Text: n.Text})
	}
	for _, round := range o.Deal.SystemSeeds {
		out := []v2.Card{}
		for _, row := range round {
			out = append(out, card(row))
		}
		m.seeds = append(m.seeds, out)
	}
	if len(o.DevRoles) > 0 && (!o.Prototype || c.App.Env == "prod" || c.App.Env == "production") {
		return nil, fmt.Errorf("dev.role_unavailable")
	}
	remaining := size/2 - 1
	available := size
	for seat, role := range o.DevRoles {
		if seat < 0 || seat >= size || (role != "random" && role != "nower" && role != "donower") {
			return nil, fmt.Errorf("dev.role_invalid")
		}
		if role != "random" {
			available--
		}
		if role == "donower" {
			s.Players[seat].Role = role
			remaining--
		}
	}
	if remaining < 0 || remaining > available {
		return nil, fmt.Errorf("dev.role_conflict")
	}
	perm := rand.New(rand.NewSource(o.Seed)).Perm(size)
	for _, seat := range perm {
		if remaining == 0 {
			break
		}
		if role := o.DevRoles[seat]; role == "nower" || role == "donower" {
			continue
		}
		s.Players[seat].Role = "donower"
		remaining--
	}
	m.state = s
	if e := m.beginRound(s, o.Now()); e != nil {
		return nil, e
	}
	for seat, player := range s.Players {
		if player.Role != "nower" {
			continue
		}
		check := m.project(s, seat, o.Now())
		check.Private.Hand = []v2.Card{}
		board := map[v2.CopyID]bool{}
		for _, c := range s.Board.Cards {
			board[c.Card.CopyID] = true
		}
		for id, c := range m.allCopies {
			if !board[id] {
				check.Private.Hand = append(check.Private.Hand, c)
			}
		}
		seen := map[v2.ContentID]bool{}
		for _, nown := range m.nowns {
			if seen[nown.ContentID] {
				return nil, fmt.Errorf("duplicate scheduled prompt")
			}
			seen[nown.ContentID] = true
			check.Private.Nown = &nown
			if e := check.Validate(m.limits); e != nil {
				return nil, e
			}
		}
		break
	}
	for i := range s.Players {
		if e := m.project(s, i, o.Now()).Validate(m.limits); e != nil {
			return nil, e
		}
	}
	return m, nil
}

func (m *TextMatch) Limits() v2.Limits { return m.limits }
func (m *TextMatch) cloneState() (*textState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := *m.state
	if s.RevealTarget != nil {
		target := *s.RevealTarget
		s.RevealTarget = &target
	}
	if s.RevealCards != nil {
		view := *s.RevealCards
		view.Hand = slices.Clone(view.Hand)
		view.Reserve = slices.Clone(view.Reserve)
		s.RevealCards = &view
	}
	s.Board = s.Board.Clone()
	s.Players = slices.Clone(s.Players)
	for i := range s.Players {
		s.Players[i].Hand = slices.Clone(s.Players[i].Hand)
		s.Players[i].Reserve = slices.Clone(s.Players[i].Reserve)
	}
	s.Copies = maps.Clone(s.Copies)
	s.Order = slices.Clone(s.Order)
	s.Ready = maps.Clone(s.Ready)
	s.Pokes = maps.Clone(s.Pokes)
	s.Votes = maps.Clone(s.Votes)
	s.Candidates = slices.Clone(s.Candidates)
	if s.BallotResult != nil {
		result := s.BallotResult.Clone()
		s.BallotResult = &result
	}
	if s.Offer != nil {
		offer := *s.Offer
		s.Offer = &offer
	}
	s.History = v2.ClonePublicActions(s.History)
	s.Requests = maps.Clone(s.Requests)
	if s.Result != nil {
		result := *s.Result
		result.Players = slices.Clone(result.Players)
		s.Result = &result
	}
	return &s, nil
}
func textError(code v2.ErrorCode, field string) error {
	return &v2.ContractError{Code: code, Field: field}
}

func (m *TextMatch) persist(ctx context.Context, p *textPending) (TextActionResult, error) {
	m.pending = p
	for _, incident := range p.Abandons {
		if m.hooks.Abandon != nil {
			if err := m.hooks.Abandon(ctx, incident); err != nil {
				return TextActionResult{}, err
			}
		}
	}
	for _, award := range p.Awards {
		if m.hooks.Award != nil {
			if e := m.hooks.Award(ctx, award); e != nil {
				return TextActionResult{}, e
			}
		}
	}
	if p.Result != nil && m.hooks.Finish != nil {
		result := *p.Result
		result.Players = append([]TextPlayerResult{}, p.Result.Players...)
		if e := m.hooks.Finish(ctx, result); e != nil {
			return TextActionResult{}, e
		}
	}
	m.mu.Lock()
	m.state = p.State
	m.mu.Unlock()
	m.pending = nil
	return TextActionResult{Changed: true, Public: clonePublic(p.Events)}, nil
}
func clonePublic(in []v2.PublicAction) []v2.PublicAction {
	return v2.ClonePublicActions(in)
}

func (m *TextMatch) Apply(ctx context.Context, seat int, r v2.ActionRequest) (TextActionResult, error) {
	m.serial.Lock()
	defer m.serial.Unlock()
	if e := r.Validate(m.limits); e != nil {
		return TextActionResult{}, e
	}
	if seat < 0 || seat >= m.contract.OriginalSize {
		return TextActionResult{}, textError(v2.ErrUnauthorized, "seat")
	}
	key := fmt.Sprintf("%d:%s", seat, r.RequestID)
	hash, e := v2.RequestHash(r)
	if e != nil {
		return TextActionResult{}, e
	}
	if m.pending != nil {
		if m.pending.Key != key || m.pending.Hash != hash {
			return TextActionResult{}, fmt.Errorf("match persistence pending")
		}
		return m.persist(ctx, m.pending)
	}
	s, e := m.cloneState()
	if e != nil {
		return TextActionResult{}, e
	}
	if old, ok := s.Requests[key]; ok {
		same, e := v2.CheckRequestReuse(old, r, seat)
		if e != nil {
			return TextActionResult{}, e
		}
		if same {
			return TextActionResult{Duplicate: true, Public: []v2.PublicAction{}}, nil
		}
	}
	count := 0
	for _, old := range s.Requests {
		if old.Seat == seat {
			count++
		}
	}
	if e := v2.CheckRequestBudget(count, false, m.limits); e != nil {
		return TextActionResult{}, e
	}
	now := m.now()
	if e := v2.CheckActionContext(r, v2.ActionContext{MatchID: m.contract.MatchID, ModeID: m.contract.ModeID, Round: s.Round, Turn: s.Turn, Phase: s.Phase, PhaseID: s.PhaseID, BoardRevision: s.Board.Revision, DeadlineMS: s.Deadline, NowMS: now.UnixMilli()}); e != nil {
		return TextActionResult{}, e
	}
	if !s.Players[seat].Connected || s.Players[seat].Eliminated {
		return TextActionResult{}, textError(v2.ErrUnauthorized, "actor")
	}
	p := &textPending{State: s, Key: key, Hash: hash}
	start := len(s.History)
	action := r.Action
	if action.Kind == v2.ActionChat {
		if m.hooks.ModerateChat == nil {
			return TextActionResult{}, textError(v2.ErrUnauthorized, "chat_policy")
		}
		action, e = m.hooks.ModerateChat(ctx, seat, action)
		if e != nil {
			return TextActionResult{}, e
		}
		checked := r
		checked.Action = action
		if action.Kind != v2.ActionChat {
			return TextActionResult{}, textError(v2.ErrInvalidAction, "chat_policy")
		}
		if e := checked.Validate(m.limits); e != nil {
			return TextActionResult{}, e
		}
	}
	if e := m.act(s, seat, action, now, p); e != nil {
		return TextActionResult{}, e
	}
	s.Requests[key] = v2.RequestRecord{MatchID: r.MatchID, Seat: seat, RequestID: r.RequestID, BodyHash: hash}
	if e := m.validateState(s); e != nil {
		return TextActionResult{}, e
	}
	p.Events = s.History[start:]
	return m.persist(ctx, p)
}

func (m *TextMatch) Advance(ctx context.Context, now time.Time) (TextActionResult, error) {
	return m.system(ctx, func(s *textState, p *textPending) error { return m.advance(s, now, p) })
}
func (m *TextMatch) SetConnected(ctx context.Context, seat int, connected bool) (TextActionResult, error) {
	return m.system(ctx, func(s *textState, p *textPending) error {
		if seat < 0 || seat >= len(s.Players) {
			return textError(v2.ErrUnauthorized, "seat")
		}
		now := m.now()
		player := &s.Players[seat]
		// An expired grace is an occurrence even if the reconnect arrives before
		// the periodic timer. Process due work before replacing the binding state.
		if connected && !player.Connected && player.GraceDeadline > 0 && now.UnixMilli() >= player.GraceDeadline {
			if err := m.advance(s, now, p); err != nil {
				return err
			}
		}
		if player.Connected == connected {
			return nil
		}
		player.Connected = connected
		if connected {
			player.Absent = false
			player.GraceDeadline = 0
		} else {
			player.GraceDeadline = now.Add(m.grace).UnixMilli()
		}
		if s.Offer != nil && (seat == s.Offer.ProposerSeat || seat == s.Offer.RecipientSeat) {
			m.resolveOffer(s, -1, "cancel", "disconnect", now)
		}
		if !connected && s.Phase == v2.PhasePlay && m.current(s) == seat {
			m.autoPass(s, "disconnect", now)
		}
		if !connected && !player.Eliminated {
			if s.Phase == v2.PhaseKnowoff || s.Phase == v2.PhaseRunoff {
				delete(s.Votes, seat)
			}
			if s.Phase == v2.PhaseDiscussion || s.Phase == v2.PhaseKnowoff || s.Phase == v2.PhaseRunoff || s.Phase == v2.PhaseResult {
				s.Ready[seat] = true
			}
		}
		return m.advance(s, now, p)
	})
}
func (m *TextMatch) Close(ctx context.Context) (TextActionResult, error) {
	return m.system(ctx, func(s *textState, p *textPending) error { m.finish(s, "interrupted", "none", m.now(), p); return nil })
}
func (m *TextMatch) system(ctx context.Context, fn func(*textState, *textPending) error) (TextActionResult, error) {
	m.serial.Lock()
	defer m.serial.Unlock()
	prefix := []v2.PublicAction{}
	if m.pending != nil {
		result, err := m.persist(ctx, m.pending)
		if err != nil {
			return TextActionResult{}, err
		}
		prefix = result.Public
	}
	s, e := m.cloneState()
	if e != nil {
		return TextActionResult{}, e
	}
	start := len(s.History)
	p := &textPending{State: s}
	if e := fn(s, p); e != nil {
		return TextActionResult{}, e
	}
	if e := m.validateState(s); e != nil {
		return TextActionResult{}, e
	}
	p.Events = append(prefix, s.History[start:]...)
	return m.persist(ctx, p)
}
