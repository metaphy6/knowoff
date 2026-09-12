package lobby

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
)

var (
	ErrTextUnavailable  = errors.New("mode.unavailable")
	ErrTextMembership   = errors.New("lobby.membership")
	ErrTextReady        = errors.New("lobby.ready_required")
	ErrTextHost         = errors.New("lobby.host_required")
	ErrTextRevision     = errors.New("lobby.stale_revision")
	ErrTextSlowConsumer = errors.New("connection.slow_consumer")
)

// TextValues is the durable admission/value boundary. No hidden engine state is stored here.
type TextValues interface {
	Reserve(context.Context, store.TextReservation) error
	CancelReservation(context.Context, string, string) error
	Prepare(context.Context, store.TextMatchRecord, time.Time) error
	Start(context.Context, string, string, int64, time.Time) error
	CancelPrepared(context.Context, string, string, int64, time.Time) error
	Award(context.Context, store.TextAward) (int, error)
	Finish(context.Context, store.TextOutcome) error
	SettlePending(context.Context, string) error
	Interrupt(context.Context, string, string, int64, time.Time) error
}
type TextAuthority interface {
	Check(context.Context) error
	Done() <-chan struct{}
}
type TextDeps struct {
	Owner          string
	Authority      TextAuthority
	Config         *config.Config
	Values         TextValues
	Resolve        func(context.Context, string, string) (*media.TextSnapshot, error)
	ResolveRelease func(context.Context, string, string, string) (*media.TextSnapshot, error)
	Prototype      *media.TextSnapshot // Explicit server configuration, refused in production.
	Now            func() time.Time
	CanMatch       func(context.Context, []string) error
	CheckAccess    func(context.Context, string, v2.LobbySettings, string) error
	ModerateChat   func(context.Context, string, v2.Action) (v2.Action, error)
	HideChat       func(context.Context, string, string) (bool, error)
}

type TextEnvelope struct {
	Version   int             `json:"v"`
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}
type TextLimits struct {
	MaxFrameBytes        int `json:"max_frame_bytes"`
	MaxHistoryEvents     int `json:"max_history_events"`
	MaxHistoryPageEvents int `json:"max_history_page_events"`
	MaxTextBytes         int `json:"max_text_bytes"`
	MaxRequestsPerSeat   int `json:"max_requests_per_seat"`
}
type TextLanguage struct {
	ContentLanguage string `json:"content_language"`
	PackReleaseID   string `json:"pack_release_id"`
	RulesVersion    string `json:"rules_version"`
}
type TextModeAvailability struct {
	ModeID    gamecontract.ModeID `json:"mode_id"`
	Available bool                `json:"available"`
	Languages []TextLanguage      `json:"languages"`
}
type TextAvailability struct {
	Prototype        bool                   `json:"prototype"`
	ProtocolVersion  int                    `json:"protocol_version"`
	ClientGeneration int                    `json:"client_generation"`
	Limits           TextLimits             `json:"limits"`
	Modes            []TextModeAvailability `json:"modes"`
}
type TextQueueState struct {
	QueueID      string           `json:"queue_id"`
	Status       string           `json:"status"`
	JoinedAtMS   int64            `json:"joined_at_ms"`
	DecisionAtMS int64            `json:"decision_at_ms"`
	Settings     v2.LobbySettings `json:"settings"`
}
type TextLobbyView struct {
	Seat  int           `json:"seat"`
	Code  string        `json:"code"`
	Lobby v2.LobbyState `json:"lobby"`
}

// One network writer consumes Frames. Payloads are marshaled before enqueueing,
// never shared mutable engine structs. The manager performs no socket writes.
type TextPeer struct {
	AccountID string
	ID        string
	Frames    <-chan TextEnvelope
	Done      <-chan struct{}
	frames    chan TextEnvelope
	done      chan struct{}
	closed    bool // manager.mu only
}
type textMember struct {
	account, admission string
	peer               *TextPeer
	joined             uint64
	originalSeat       int
	ready              *v2.ReadyAcknowledgement
}
type textRoom struct {
	pendingAbort                         string
	id, code, path                       string
	settings                             v2.LobbySettings
	settingsRevision, membershipRevision uint64
	host                                 int
	seats                                map[int]*textMember
	match                                *game.TextMatch
	matchAccounts                        []string
	rematching                           bool
}
type textQueue struct {
	id               string
	peer             *TextPeer
	settings         v2.LobbySettings
	joined, decision time.Time
	order            uint64
	choice           bool
}
type textRateState struct {
	tokens float64
	last   time.Time
}

type TextManager struct {
	requestRates map[string]textRateState
	// Serialize all membership/admission/output transitions. Engine and durable
	// callbacks have bounded caller contexts; no callback may reenter this manager.
	mu       sync.Mutex
	deps     TextDeps
	peers    map[string]*TextPeer
	members  map[string]*textRoom
	rooms    map[string]*textRoom
	queues   map[string]*textQueue
	order    uint64
	owner    string
	draining bool
	lost     bool
}

func NewTextManager(d TextDeps) (*TextManager, error) {
	if d.Config == nil || d.Config.Text == nil || d.Values == nil || d.Config.Tuning.Contract.MaxHistoryPageEvents < 1 || d.Config.WebSocket.MaxMessageBytes < 1 {
		return nil, ErrTextUnavailable
	}
	if d.Prototype != nil && (d.Config.App.Env == "prod" || d.Config.App.Env == "production" || !d.Prototype.Manifest().Synthetic) {
		return nil, ErrTextUnavailable
	}
	c := *d.Config
	c.Tuning = d.Config.Tuning.Clone()
	tc := *d.Config.Text
	tc.Modes = map[gamecontract.ModeID]config.TextModeConfig{}
	for k, v := range d.Config.Text.Modes {
		v.ContentLanguages = append([]string(nil), v.ContentLanguages...)
		tc.Modes[k] = v
	}
	c.Text = &tc
	d.Config = &c
	if d.Now == nil {
		d.Now = time.Now
	}
	owner := d.Owner
	if owner == "" {
		owner = uuid.NewString()
	}
	if _, err := uuid.Parse(owner); err != nil {
		return nil, ErrTextUnavailable
	}
	return &TextManager{requestRates: map[string]textRateState{}, deps: d, peers: map[string]*TextPeer{}, members: map[string]*textRoom{}, rooms: map[string]*textRoom{}, queues: map[string]*textQueue{}, owner: owner}, nil
}
func (m *TextManager) Limits() TextLimits {
	c := m.deps.Config
	frame := c.WebSocket.MaxMessageBytes
	if c.RateLimit.MaxBytesPerFrame > 0 && c.RateLimit.MaxBytesPerFrame < frame {
		frame = c.RateLimit.MaxBytesPerFrame
	}
	return TextLimits{frame, c.Tuning.Contract.MaxHistoryEvents, c.Tuning.Contract.MaxHistoryPageEvents, c.Tuning.Contract.MaxTextBytes, c.Tuning.Contract.MaxRequestsPerSeat}
}
func (m *TextManager) wireLimits() v2.Limits {
	l := m.Limits()
	return v2.Limits{MaxFrameBytes: l.MaxFrameBytes, MaxHistoryEvents: l.MaxHistoryEvents, MaxHistoryPageEvents: l.MaxHistoryPageEvents, MaxTextBytes: l.MaxTextBytes, MaxRequestsPerSeat: l.MaxRequestsPerSeat}
}
func (m *TextManager) emit(p *TextPeer, kind, request string, payload any) error {
	if m.deps.Authority != nil {
		select {
		case <-m.deps.Authority.Done():
			m.loseAuthority()
			return ErrTextUnavailable
		default:
		}
	}
	if p == nil || p.closed {
		return ErrTextMembership
	}
	raw, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	env := TextEnvelope{Version: 2, Type: kind, RequestID: request, Payload: raw}
	encoded, e := json.Marshal(env)
	if e != nil {
		return e
	}
	if len(encoded) > m.Limits().MaxFrameBytes {
		m.closePeer(p)
		return ErrTextSlowConsumer
	}
	select {
	case p.frames <- env:
		return nil
	default:
		m.closePeer(p)
		return ErrTextSlowConsumer
	}
}
func (m *TextManager) closePeer(p *TextPeer) {
	if p != nil && !p.closed {
		p.closed = true
		close(p.done)
	}
}
func (m *TextManager) Send(p *TextPeer, kind, request string, payload any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.current(p) {
		return ErrTextMembership
	}
	return m.emit(p, kind, request, payload)
}
func (m *TextManager) current(p *TextPeer) bool {
	return p != nil && !p.closed && m.peers[p.AccountID] == p
}
func (m *TextManager) Open(ctx context.Context, account string) (*TextPeer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.checkAuthority(ctx); e != nil {
		return nil, e
	}
	if _, e := uuid.Parse(account); e != nil {
		return nil, ErrTextMembership
	}
	if old := m.peers[account]; old != nil {
		if e := m.disconnect(ctx, old); e != nil {
			return nil, e
		}
	}
	n := 2 * (m.Limits().MaxHistoryEvents/m.Limits().MaxHistoryPageEvents + 4)
	ch := make(chan TextEnvelope, n)
	done := make(chan struct{})
	p := &TextPeer{AccountID: account, ID: uuid.NewString(), Frames: ch, Done: done, frames: ch, done: done}
	m.peers[account] = p
	return p, nil
}
func (m *TextManager) Availability(ctx context.Context) TextAvailability {
	// Snapshot immutable configuration; catalog/database I/O never stalls a live
	// connection or match mutation. Admission revalidates publication separately.
	m.mu.Lock()
	d, draining := m.deps, m.draining
	m.mu.Unlock()
	if d.Authority != nil {
		if err := d.Authority.Check(ctx); err != nil {
			m.mu.Lock()
			m.loseAuthority()
			m.mu.Unlock()
			draining = true
		}
		select {
		case <-d.Authority.Done():
			draining = true
		default:
		}
	}
	a := TextAvailability{Prototype: d.Prototype != nil, ProtocolVersion: 2, ClientGeneration: 2, Limits: m.Limits(), Modes: []TextModeAvailability{}}
	cache := map[string]*media.TextSnapshot{}
	loaded := map[string]bool{}
	for _, mode := range gamecontract.AllModes() {
		v := TextModeAvailability{ModeID: mode, Languages: []TextLanguage{}}
		cfg := d.Config.Text.Modes[mode]
		languages := append([]string(nil), cfg.ContentLanguages...)
		if d.Prototype != nil {
			languages = []string{d.Prototype.Manifest().Language}
		} else if !cfg.Enabled {
			languages = nil
		}
		sort.Strings(languages)
		for _, lang := range languages {
			key := lang + "\x00" + d.Config.Text.RulesVersion
			if !loaded[key] {
				loaded[key] = true
				if d.Prototype != nil {
					cache[key] = d.Prototype
				} else if d.Resolve != nil {
					snapshot, err := d.Resolve(ctx, lang, d.Config.Text.RulesVersion)
					if err == nil {
						cache[key] = snapshot
					}
				}
			}
			snapshot := cache[key]
			if snapshot == nil {
				continue
			}
			manifest := snapshot.Manifest()
			if manifest.Language != lang || manifest.RulesVersion != d.Config.Text.RulesVersion || (d.Prototype == nil && manifest.Synthetic) {
				continue
			}
			for _, supported := range manifest.Modes {
				if supported == mode {
					v.Languages = append(v.Languages, TextLanguage{lang, manifest.ReleaseID, manifest.RulesVersion})
					break
				}
			}
		}
		v.Available = !draining && len(v.Languages) > 0
		if !v.Available {
			v.Languages = []TextLanguage{}
		}
		a.Modes = append(a.Modes, v)
	}
	return a
}
func (m *TextManager) resolve(ctx context.Context, s v2.LobbySettings) (*media.TextSnapshot, error) {
	if !s.ModeID.Valid() || (s.Size != 4 && s.Size != 6) || !gamecontract.ValidContentLanguage(s.ContentLanguage) || s.RulesVersion != m.deps.Config.Text.RulesVersion {
		return nil, ErrTextUnavailable
	}
	var snapshot *media.TextSnapshot
	if m.deps.Prototype != nil {
		snapshot = m.deps.Prototype
	} else {
		mode := m.deps.Config.Text.Modes[s.ModeID]
		if !mode.Enabled || m.deps.Resolve == nil && m.deps.ResolveRelease == nil {
			return nil, ErrTextUnavailable
		}
		supported := false
		for _, lang := range mode.ContentLanguages {
			supported = supported || lang == s.ContentLanguage
		}
		if !supported {
			return nil, ErrTextUnavailable
		}
		var e error
		if s.PackReleaseID != "" && m.deps.ResolveRelease != nil {
			snapshot, e = m.deps.ResolveRelease(ctx, s.ContentLanguage, s.RulesVersion, s.PackReleaseID)
		} else if m.deps.Resolve != nil {
			snapshot, e = m.deps.Resolve(ctx, s.ContentLanguage, s.RulesVersion)
		} else {
			return nil, ErrTextUnavailable
		}
		if e != nil {
			return nil, e
		}
	}
	if snapshot == nil {
		return nil, ErrTextUnavailable
	}
	mft := snapshot.Manifest()
	if mft.Language != s.ContentLanguage || mft.RulesVersion != s.RulesVersion || (s.PackReleaseID != "" && mft.ReleaseID != s.PackReleaseID) || (m.deps.Prototype == nil && mft.Synthetic) {
		return nil, ErrTextUnavailable
	}
	for _, mode := range mft.Modes {
		if mode == s.ModeID {
			return snapshot, nil
		}
	}
	return nil, ErrTextUnavailable
}
func (m *TextManager) access(ctx context.Context, account string, s v2.LobbySettings, path string) error {
	if e := m.checkAuthority(ctx); e != nil {
		return e
	}
	if m.draining {
		return ErrTextUnavailable
	}
	if s.PackReleaseID == "" {
		return ErrTextUnavailable
	}
	if _, e := m.resolve(ctx, s); e != nil {
		return e
	}
	if m.deps.Prototype != nil {
		return nil
	}
	if m.deps.CheckAccess == nil {
		return ErrTextUnavailable
	}
	return m.deps.CheckAccess(ctx, account, s, path)
}
func (m *TextManager) canMatch(ctx context.Context, accounts []string) error {
	if m.deps.CanMatch != nil {
		return m.deps.CanMatch(ctx, append([]string(nil), accounts...))
	}
	if m.deps.Prototype == nil {
		return ErrTextUnavailable
	}
	return nil
}
func (m *TextManager) reserve(ctx context.Context, account, path string) (string, error) {
	if e := m.checkAuthority(ctx); e != nil {
		return "", e
	}
	id := uuid.NewString()
	e := m.deps.Values.Reserve(ctx, store.TextReservation{ID: id, AccountID: account, EntryPath: path, Prototype: m.deps.Prototype != nil, At: m.deps.Now()})
	return id, e
}
func (m *TextManager) release(ctx context.Context, p *textMember) error {
	if p.admission == "" {
		return nil
	}
	if e := m.deps.Values.CancelReservation(ctx, p.admission, p.account); e != nil {
		return e
	}
	p.admission = ""
	return nil
}
func (m *TextManager) newRoom(s v2.LobbySettings, path string) *textRoom {
	var code string
	for {
		code = strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:6]
		if m.rooms[code] == nil {
			break
		}
	}
	r := &textRoom{id: uuid.NewString(), code: code, path: path, settings: s, settingsRevision: 1, membershipRevision: 1, host: 0, seats: map[int]*textMember{}}
	m.rooms[code] = r
	return r
}
func (m *TextManager) invalidate(r *textRoom) {
	for _, s := range r.seats {
		s.ready = nil
	}
}
func (m *TextManager) lobby(r *textRoom, p *TextPeer) error {
	s := v2.LobbyState{ProtocolVersion: 2, RoomID: r.id, Settings: r.settings, SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision, HostSeat: r.host, Seats: []v2.LobbySeat{}}
	my := -1
	for seat := 0; seat < r.settings.Size; seat++ {
		member := r.seats[seat]
		if member == nil {
			continue
		}
		s.Seats = append(s.Seats, v2.LobbySeat{Seat: seat, Connected: member.peer != nil && !member.peer.closed, Ready: member.ready})
		if member.account == p.AccountID {
			my = seat
		}
	}
	if e := s.Validate(m.wireLimits()); e != nil {
		return e
	}
	return m.emit(p, "lobby", "", TextLobbyView{my, r.code, s})
}
func (m *TextManager) broadcastLobby(r *textRoom) {
	for _, member := range r.seats {
		if member.peer != nil {
			_ = m.lobby(r, member.peer)
		}
	}
}

func (m *TextManager) loseAuthority() {
	m.lost = true
	m.draining = true
	for _, p := range m.peers {
		m.closePeer(p)
	}
}
func (m *TextManager) checkAuthority(ctx context.Context) error {
	if m.lost {
		return ErrTextUnavailable
	}
	if m.deps.Authority == nil {
		return nil
	}
	if e := m.deps.Authority.Check(ctx); e != nil {
		m.loseAuthority()
		return ErrTextUnavailable
	}
	select {
	case <-m.deps.Authority.Done():
		m.loseAuthority()
		return ErrTextUnavailable
	default:
		return nil
	}
}

// AllowRequest keeps the rate budget on an account across socket generations.
func (m *TextManager) AllowRequest(p *TextPeer) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.current(p) || m.lost {
		return false
	}
	cfg := m.deps.Config.RateLimit
	if !cfg.Enabled {
		return true
	}
	if cfg.MaxIntentsPerSecond <= 0 || cfg.MaxIntentsBurst <= 0 {
		return false
	}
	now := m.deps.Now()
	rate, capacity := float64(cfg.MaxIntentsPerSecond), float64(cfg.MaxIntentsBurst)
	state, ok := m.requestRates[p.AccountID]
	if !ok {
		state = textRateState{tokens: capacity, last: now}
	}
	state.tokens = min(capacity, state.tokens+max(0, now.Sub(state.last).Seconds())*rate)
	state.last = now
	allowed := state.tokens >= 1
	if allowed {
		state.tokens--
	}
	m.requestRates[p.AccountID] = state
	return allowed
}
func (m *TextManager) pruneRequestRates(now time.Time) {
	cfg := m.deps.Config.RateLimit
	if cfg.MaxIntentsPerSecond <= 0 {
		return
	}
	refill := float64(cfg.MaxIntentsBurst) / float64(cfg.MaxIntentsPerSecond)
	for account, state := range m.requestRates {
		if now.Sub(state.last).Seconds() >= refill {
			delete(m.requestRates, account)
		}
	}
}

func (m *TextManager) Prototype() bool { return m.deps.Prototype != nil }
