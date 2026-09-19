package lobby

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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
	ErrTextWalletHidden = errors.New("wallet.match_in_progress")
)

// TextValues is the durable admission/value boundary. No hidden engine state is stored here.
type TextValues interface {
	Reserve(context.Context, store.TextReservation) error
	CancelReservation(context.Context, string, string) error
	Prepare(context.Context, store.TextMatchRecord, time.Time) error
	Start(context.Context, string, string, int64, time.Time) error
	CancelPrepared(context.Context, string, string, int64, time.Time) error
	Award(context.Context, store.TextAward) (int, error)
	Abandon(context.Context, store.TextAbandon) error
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
	Operations     TextRoomOperations
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
	binding   TextPeerBinding
	AccountID string
	ID        string
	Frames    <-chan TextEnvelope
	Done      <-chan struct{}
	frames    chan TextEnvelope
	done      chan struct{}
	enqueueMu sync.Mutex // final enqueue and close; never covers socket I/O
	closed    atomic.Bool
}
type textMember struct {
	account, admission string
	devRole            string
	peer               *TextPeer
	joined             uint64
	originalSeat       int
	ready              *v2.ReadyAcknowledgement
}
type textRoom struct {
	runtimeMu                            sync.Mutex // under manager read lock
	pendingOperator                      *textRoomOperation
	excludedAccounts                     map[string]bool
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
	// Writers own membership/admission changes. Active room operations retain a
	// read lock plus runtimeMu, so unrelated tables progress independently.
	// Callbacks have bounded contexts and must not reenter this manager.
	mu                    sync.RWMutex
	rateMu                sync.Mutex
	deps                  TextDeps
	peers                 map[string]*TextPeer
	members               map[string]*textRoom
	rooms                 map[string]*textRoom
	queues                map[string]*textQueue
	order                 uint64
	owner                 string
	draining              atomic.Bool
	lost                  atomic.Bool
	maintenancePaused     bool
	dependencyUnavailable bool
}

// SetReady implements notices.MatchmakingPauser. Maintenance and dependency
// state are independent, so recovery of either cannot reopen the other's fence.
func (m *TextManager) SetReady(ready bool) {
	_ = waitTextLock(context.Background(), m.mu.TryLock)
	defer m.mu.Unlock()
	m.maintenancePaused = !ready
}

func (m *TextManager) SetDependencyReady(ready bool) {
	_ = waitTextLock(context.Background(), m.mu.TryLock)
	defer m.mu.Unlock()
	m.dependencyUnavailable = !ready
}

// admissionPaused is called with mu held. Begun matches keep their own clocks
// and reconnect projection while reversible admission pauses are in force.
func (m *TextManager) admissionPaused() bool {
	return m.draining.Load() || m.maintenancePaused || m.dependencyUnavailable
}

// WithWalletAccess serializes a bounded, database-only wallet operation with
// admission and game actions. Even disconnected seats retain the privacy fence
// until verdict; checking first and reading later would race a new match start.
func (m *TextManager) WithWalletAccess(ctx context.Context, account string, operation func() error) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	if err := m.checkAuthority(ctx); err != nil {
		return err
	}
	if room := m.members[account]; room != nil && room.match != nil {
		if phase, _ := room.match.Clock(); phase != v2.PhaseVerdict {
			return ErrTextWalletHidden
		}
	}
	return operation()
}

func (m *TextManager) RuntimeReady(ctx context.Context) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	if err := m.checkAuthority(ctx); err != nil {
		return err
	}
	if m.admissionPaused() {
		return ErrTextUnavailable
	}
	return nil
}

// NotifyNotices exposes no game or account data. Connected clients fetch the
// public, localized inbox on this bounded signal and on connection recovery.
func (m *TextManager) NotifyNotices(ctx context.Context) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	if err := m.checkAuthority(ctx); err != nil {
		return err
	}
	var result error
	for _, p := range m.peers {
		result = errors.Join(result, m.emit(p, "system_notice", "", map[string]bool{"refresh": true}))
	}
	return result
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
	if p == nil || p.closed.Load() {
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
	// Serialization may overlap another reader observing owner loss. Recheck at
	// the enqueue boundary, mutually exclusive with closing this peer.
	p.enqueueMu.Lock()
	if m.lost.Load() || p.closed.Load() {
		p.enqueueMu.Unlock()
		return ErrTextUnavailable
	}
	if m.deps.Authority != nil {
		select {
		case <-m.deps.Authority.Done():
			p.enqueueMu.Unlock()
			m.loseAuthority()
			return ErrTextUnavailable
		default:
		}
	}
	select {
	case p.frames <- env:
		p.enqueueMu.Unlock()
		return nil
	default:
		p.enqueueMu.Unlock()
		m.closePeer(p)
		return ErrTextSlowConsumer
	}
}
func (m *TextManager) closePeer(p *TextPeer) {
	if p == nil {
		return
	}
	p.enqueueMu.Lock()
	defer p.enqueueMu.Unlock()
	if p.closed.CompareAndSwap(false, true) {
		close(p.done)
	}
}
func (m *TextManager) Send(p *TextPeer, kind, request string, payload any) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.current(p) {
		return ErrTextMembership
	}
	return m.emit(p, kind, request, payload)
}
func (m *TextManager) current(p *TextPeer) bool {
	return p != nil && !p.closed.Load() && m.peers[p.AccountID] == p
}
func (m *TextManager) Open(ctx context.Context, account string) (*TextPeer, error) {
	return m.OpenAuthenticated(ctx, func(context.Context) (TextPeerBinding, error) { return TextPeerBinding{AccountID: account}, nil })
}

type TextPeerBinding = store.TextAdmissionBinding

// Authentication runs under the same mutex as live sanction delivery; a peer
// cannot appear after a delivery has observed its absence using a stale token.
func (m *TextManager) OpenAuthenticated(ctx context.Context, authenticate func(context.Context) (TextPeerBinding, error)) (*TextPeer, error) {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return nil, err
	}
	defer m.mu.Unlock()
	if authenticate == nil {
		return nil, ErrTextMembership
	}
	binding, err := authenticate(ctx)
	if err != nil {
		return nil, err
	}
	account := binding.AccountID
	if e := m.checkAuthority(ctx); e != nil {
		return nil, e
	}
	if _, e := uuid.Parse(account); e != nil {
		return nil, ErrTextMembership
	}
	if room := m.members[account]; room != nil && room.pendingOperator != nil {
		return nil, ErrTextUnavailable
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
	p.binding = binding
	m.peers[account] = p
	return p, nil
}
func (m *TextManager) Availability(ctx context.Context) TextAvailability {
	// Snapshot immutable configuration; catalog/database I/O never stalls a live
	// connection or match mutation. Admission revalidates publication separately.
	if err := waitTextLock(ctx, m.mu.TryRLock); err != nil {
		return TextAvailability{Prototype: m.deps.Prototype != nil, ProtocolVersion: 2, ClientGeneration: 2, Limits: m.Limits(), Modes: []TextModeAvailability{}}
	}
	d, draining := m.deps, m.admissionPaused()
	m.mu.RUnlock()
	if d.Authority != nil {
		if err := d.Authority.Check(ctx); err != nil {
			// A cancelled discovery request closes only its own admission result.
			// Physical authority loss still fences every peer below.
			if ctx.Err() == nil || !errors.Is(err, ctx.Err()) {
				m.mu.RLock()
				m.loseAuthority()
				m.mu.RUnlock()
			}
			draining = true
		}
		select {
		case <-d.Authority.Done():
			m.mu.RLock()
			m.loseAuthority()
			m.mu.RUnlock()
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
	if m.admissionPaused() {
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
	bindings := []store.TextAdmissionBinding{}
	if peer := m.peers[account]; peer != nil {
		bindings = append(bindings, peer.binding)
	}
	ctx = store.WithTextAdmissionBindings(ctx, bindings)
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
		s.Seats = append(s.Seats, v2.LobbySeat{Seat: seat, Connected: member.peer != nil && !member.peer.closed.Load(), Ready: member.ready})
		if member.account == p.AccountID {
			my = seat
		}
	}
	if r.match != nil {
		// This active-match envelope only binds a fresh client's room and seat.
		// Prematch Ready is obsolete; a connected projected host satisfies the
		// lobby contract without transferring the room's actual host authority.
		hostConnected := false
		for i := range s.Seats {
			s.Seats[i].Ready = nil
			if s.Seats[i].Seat == s.HostSeat && s.Seats[i].Connected {
				hostConnected = true
			}
		}
		if !hostConnected {
			for _, seat := range s.Seats {
				if seat.Connected {
					s.HostSeat = seat.Seat
					break
				}
			}
		}
	}
	if e := s.Validate(m.wireLimits()); e != nil {
		return e
	}
	if err := m.emit(p, "lobby", "", TextLobbyView{my, r.code, s}); err != nil {
		return err
	}
	return m.devRoleFrame(r, p)
}
func (m *TextManager) broadcastLobby(r *textRoom) {
	for _, member := range r.seats {
		if member.peer != nil {
			_ = m.lobby(r, member.peer)
		}
	}
}

func (m *TextManager) loseAuthority() {
	m.lost.Store(true)
	m.draining.Store(true)
	for _, p := range m.peers {
		m.closePeer(p)
	}
}
func (m *TextManager) checkAuthority(ctx context.Context) error {
	if m.lost.Load() {
		return ErrTextUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.deps.Authority == nil {
		return nil
	}
	if e := m.deps.Authority.Check(ctx); e != nil {
		if ctx.Err() != nil && errors.Is(e, ctx.Err()) {
			select {
			case <-m.deps.Authority.Done():
			default:
				return e
			}
		}
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
func (m *TextManager) AllowRequest(ctx context.Context, p *TextPeer) bool {
	if waitTextLock(ctx, m.mu.TryRLock) != nil {
		return false
	}
	defer m.mu.RUnlock()
	if !m.current(p) || m.lost.Load() {
		return false
	}
	cfg := m.deps.Config.RateLimit
	if !cfg.Enabled {
		return true
	}
	if cfg.MaxIntentsPerSecond <= 0 || cfg.MaxIntentsBurst <= 0 {
		return false
	}
	if waitTextLock(ctx, m.rateMu.TryLock) != nil {
		return false
	}
	defer m.rateMu.Unlock()
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
