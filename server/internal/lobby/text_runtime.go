package lobby

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func privateTextSeed() (int64, error) {
	var b [8]byte
	_, e := rand.Read(b[:])
	return int64(binary.LittleEndian.Uint64(b[:])), e
}
func (m *TextManager) Start(ctx context.Context, p *TextPeer) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	r := m.members[p.AccountID]
	if !m.current(p) || r == nil || r.match != nil {
		return ErrTextMembership
	}
	if m.admissionPaused() || r.pendingAbort != "" || r.pendingOperator != nil {
		return ErrTextUnavailable
	}
	seat, _ := m.member(r, p.AccountID)
	if seat != r.host {
		return ErrTextHost
	}
	if len(r.seats) != r.settings.Size {
		return ErrTextReady
	}
	for i := 0; i < r.settings.Size; i++ {
		s := r.seats[i]
		if s == nil || !m.current(s.peer) || s.ready == nil || s.ready.SettingsRevision != r.settingsRevision || s.ready.MembershipRevision != r.membershipRevision {
			return ErrTextReady
		}
	}
	return m.start(ctx, r)
}
func (m *TextManager) start(ctx context.Context, r *textRoom) (err error) {
	prepared := false
	matchID := uuid.NewString()
	at := m.deps.Now()
	defer func() {
		if err != nil {
			var cleanup error
			if prepared {
				r.pendingAbort = matchID
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				cleanup = m.abortPrepared(cleanupCtx, r)
				cancel()
			} else {
				for _, s := range r.seats {
					cleanup = errors.Join(cleanup, m.release(ctx, s))
				}
			}
			m.invalidate(r)
			m.broadcastLobby(r)
			err = errors.Join(err, cleanup)
		}
	}()
	if err = m.canMatch(ctx, m.accounts(r)); err != nil {
		return err
	}
	host := r.seats[r.host].account
	if err = m.access(ctx, host, r.settings, r.path); err != nil {
		return err
	}
	snapshot, err := m.resolve(ctx, r.settings)
	if err != nil {
		return err
	}
	t := m.deps.Config.Tuning
	random := media.TextRandomness{}
	if random.Schedule, err = privateTextSeed(); err != nil {
		return err
	}
	if random.Hands, err = privateTextSeed(); err != nil {
		return err
	}
	if random.System, err = privateTextSeed(); err != nil {
		return err
	}
	deal, err := snapshot.Deal(r.settings.ModeID, r.settings.Size, media.TextDealTuning{HandSize: t.Hand.Size, ReserveSize: t.Hand.DrawPile, MinHigh: t.Dealing.MinHighPerNown, MinDistant: t.Dealing.MinDistantPerNown, MaxSearchNodes: t.TextCatalog.MaxSearchNodes}, random)
	if err != nil {
		return err
	}
	hash, err := t.SHA256()
	if err != nil {
		return err
	}
	prototype := m.deps.Prototype != nil
	contract := v2.MatchContract{ProtocolVersion: 2, MatchID: matchID, RoomID: r.id, OriginalSize: r.settings.Size, ModeID: r.settings.ModeID, RulesVersion: deal.RulesVersion, ContentLanguage: deal.Language, PackReleaseID: deal.ReleaseID, PackSHA256: deal.SnapshotSHA256, Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: hash}, Eligibility: v2.Eligibility{AdmissionID: uuid.NewString(), EntryPath: r.path, Rewards: !prototype, Leaderboard: !prototype && r.path == "quick_play" && r.settings.Size >= t.Liquidity.LeaderboardMinHumans}}
	accounts := make([]string, r.settings.Size)
	admissions := make([]string, r.settings.Size)
	for i := range accounts {
		s := r.seats[i]
		accounts[i] = s.account
		if s.admission == "" {
			s.admission, err = m.reserve(ctx, s.account, r.path)
			if err != nil {
				return err
			}
		}
		admissions[i] = s.admission
	}
	hooks := TextValueHooks(m.deps.Values, m.owner, 1, accounts)
	hooks.Finish = m.operatorFinish(r, hooks.Finish)
	if m.deps.ModerateChat != nil {
		hooks.ModerateChat = func(ctx context.Context, seat int, a v2.Action) (v2.Action, error) {
			return m.deps.ModerateChat(ctx, accounts[seat], a)
		}
	}
	devRoles := map[int]string{}
	for seat, member := range r.seats {
		if member.devRole != "" && member.devRole != "random" {
			devRoles[seat] = member.devRole
		}
	}
	donowers, nowers := 0, 0
	for _, role := range devRoles {
		if role == "donower" {
			donowers++
		}
		if role == "nower" {
			nowers++
		}
	}
	if donowers > r.settings.Size/2-1 || nowers > r.settings.Size-(r.settings.Size/2-1) {
		return ErrTextDevRoleConflict
	}
	engine, err := game.NewTextMatch(game.TextOptions{DevRoles: devRoles, Contract: contract, Deal: deal, Config: m.deps.Config, Now: m.deps.Now, Hooks: hooks, Prototype: prototype})
	if err != nil {
		return err
	}
	record := store.TextMatchRecord{Contract: contract, Policy: t, Owner: m.owner, Epoch: 1, AdmissionIDs: admissions, Prototype: prototype}
	if r.path == "local" {
		record.SponsorAccountID = host
	}
	// The record may have committed even when the database response was lost.
	// Retain its exact identity before attempting Prepare for safe cancellation.
	prepared = true
	if err = m.deps.Values.Prepare(ctx, record, at); err != nil {
		return err
	}
	prepared = true
	// Start owns the atomic durable trust/entitlement/quota recheck. The public
	// engine is installed only after that commit; a preflight deal cannot spend.
	if err = m.access(ctx, host, r.settings, r.path); err != nil {
		return err
	}
	bindings := make([]store.TextAdmissionBinding, 0, len(r.seats))
	for _, seat := range r.seats {
		if seat.peer != nil {
			bindings = append(bindings, seat.peer.binding)
		}
	}
	admissionCtx := store.WithTextAdmissionBindings(ctx, bindings)
	if err = m.deps.Values.Start(admissionCtx, matchID, m.owner, 1, at); err != nil {
		return err
	}
	r.match = engine
	r.matchAccounts = append([]string(nil), accounts...)
	r.rematching = false
	prepared = false
	err = nil
	// Transport failure cannot cancel a committed Start; slow peers disconnect and
	// normal grace policy applies. Durable interruption is reserved for owner loss.
	_ = m.broadcastMatch(ctx, r)
	return nil
}

// TextValueHooks captures immutable seat-account identities and the durable owner
// fence. Engine "none" becomes the store's no-winner sentinel. Interrupted state
// always uses its compensation path, never completion/points settlement.
func TextValueHooks(values TextValues, owner string, epoch int64, accounts []string) game.TextHooks {
	accounts = append([]string(nil), accounts...)
	return game.TextHooks{
		Abandon: func(ctx context.Context, e game.TextAbandonEvent) error {
			if e.Seat < 0 || e.Seat >= len(accounts) {
				return store.ErrValueConflict
			}
			return values.Abandon(ctx, store.TextAbandon{MatchID: e.MatchID, Owner: owner, Epoch: epoch, AccountID: accounts[e.Seat], Seat: e.Seat, At: e.OccurredAt})
		},
		Award: func(ctx context.Context, e game.TextAwardEvent) error {
			if e.Seat < 0 || e.Seat >= len(accounts) {
				return store.ErrValueConflict
			}
			_, err := values.Award(ctx, store.TextAward{MatchID: e.MatchID, Owner: owner, Epoch: epoch, AccountID: accounts[e.Seat], Kind: e.Kind, Ordinal: e.Ordinal, Amount: e.Amount, At: e.OccurredAt})
			return err
		},
		Finish: func(ctx context.Context, r game.TextResult) error {
			if r.Outcome == "interrupted" {
				return values.Interrupt(ctx, r.Contract.MatchID, owner, epoch, r.OccurredAt)
			}
			winner := r.Winner
			if winner == "none" {
				winner = ""
			}
			out := store.TextOutcome{MatchID: r.Contract.MatchID, Owner: owner, Epoch: epoch, Kind: r.Outcome, Winner: winner, At: r.OccurredAt}
			for _, p := range r.Players {
				if p.Seat < 0 || p.Seat >= len(accounts) {
					return store.ErrValueConflict
				}
				out.Players = append(out.Players, store.TextPlayerResult{AccountID: accounts[p.Seat], Seat: p.Seat, Role: p.Role, Points: int64(p.Points), CorrectVotes: p.CorrectVotes, VotesCast: p.VotesCast, Survivals: p.Survivals, Pokes: p.Pokes, Absent: p.Absent})
			}
			if e := values.Finish(ctx, out); e != nil {
				return e
			}
			return values.SettlePending(ctx, r.Contract.MatchID)
		},
	}
}
func (m *TextManager) snapshot(ctx context.Context, r *textRoom, seat int, p *TextPeer) error {
	s, e := r.match.SnapshotProjection(seat)
	if e != nil {
		return e
	}
	history := s.History
	hidden := map[int]bool{}
	for _, event := range history {
		if event.Kind != "chat" || event.Text == "" || event.Actor.Seat == nil {
			continue
		}
		author := *event.Actor.Seat
		if _, ok := hidden[author]; ok {
			continue
		}
		if m.deps.HideChat != nil {
			if author < 0 || author >= len(r.matchAccounts) {
				return ErrTextMembership
			}
			hide, err := m.deps.HideChat(ctx, p.AccountID, r.matchAccounts[author])
			if err != nil {
				return err
			}
			hidden[author] = hide
		}
	}
	for i := range history {
		event := &history[i]
		if event.Kind == "chat" && event.Text != "" && event.Actor.Seat != nil && hidden[*event.Actor.Seat] {
			event.Text = ""
			event.PhraseID = "chat.hidden"
		}
	}
	s.History = history
	s.HistoryPages = nil
	encoded, e := json.Marshal(TextEnvelope{Version: 2, Type: "snapshot", Payload: mustTextJSON(s)})
	if e != nil {
		return e
	}
	var pages []v2.HistoryPage
	if len(encoded) > m.Limits().MaxFrameBytes {
		limits := m.wireLimits()
		overhead, _ := json.Marshal(TextEnvelope{Version: 2, Type: "history_page", Payload: json.RawMessage("null")})
		limits.MaxFrameBytes -= len(overhead) - 4
		manifest, newPages, err := v2.PaginateHistory(s.Contract.MatchID, s.SnapshotID, s.Cursor.StreamEpoch, history, limits)
		if err != nil {
			return err
		}
		s.History = []v2.PublicAction{}
		s.HistoryPages = &manifest
		pages = newPages
	}
	if e = s.Validate(m.wireLimits()); e != nil {
		return e
	}
	if e = m.emit(p, "snapshot", "", s); e != nil {
		return e
	}
	for _, page := range pages {
		if e = m.emit(p, "history_page", "", page); e != nil {
			return e
		}
	}
	return nil
}
func mustTextJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func (m *TextManager) broadcastMatch(ctx context.Context, r *textRoom) error {
	defer m.pruneFinishedRoom(r)
	return m.broadcastMatchFrames(ctx, r)
}

// Caller holds either the manager write lock, or its read lock plus runtimeMu.
// Closing a slow peer is safe here; membership pruning belongs to a writer.
func (m *TextManager) broadcastMatchFrames(ctx context.Context, r *textRoom) error {
	var result error
	for seat := 0; seat < r.settings.Size; seat++ {
		member := r.seats[seat]
		if member != nil && m.current(member.peer) {
			if e := m.snapshot(ctx, r, seat, member.peer); e != nil {
				m.closePeer(member.peer)
				result = errors.Join(result, e)
			}
		}
	}
	return result
}
func (m *TextManager) Resync(ctx context.Context, p *TextPeer) error {
	r, unlock, err := m.lockRuntime(ctx, p)
	if err != nil {
		return err
	}
	defer unlock()
	if r.match == nil {
		return m.lobby(r, p)
	}
	seat, _ := m.member(r, p.AccountID)
	if e := r.match.ResetStream(seat); e != nil {
		return e
	}
	if e := m.snapshot(ctx, r, seat, p); e != nil {
		return e
	}
	return m.devRoleFrame(r, p)
}
func (m *TextManager) Action(ctx context.Context, p *TextPeer, req v2.ActionRequest) error {
	r, unlock, err := m.lockRuntime(ctx, p)
	if err != nil {
		return err
	}
	defer func() {
		terminal := false
		if r.match != nil {
			phase, _ := r.match.Clock()
			terminal = phase == v2.PhaseVerdict
		}
		unlock()
		// Tick retries terminal pruning if this request has no lock budget left.
		if terminal && m.lockDrain(ctx) == nil {
			if m.rooms[r.code] == r {
				m.pruneFinishedRoom(r)
			}
			m.mu.Unlock()
		}
	}()
	if r.match == nil {
		return ErrTextMembership
	}
	seat, member := m.member(r, p.AccountID)
	if member.peer != p {
		return ErrTextMembership
	}
	result, e := r.match.Apply(ctx, seat, req)
	if e != nil {
		return e
	}
	if result.Changed {
		if e = m.broadcastMatchFrames(ctx, r); e != nil {
			return e
		}
	} else if e = m.snapshot(ctx, r, seat, p); e != nil {
		return e
	}
	return m.emit(p, "action_ack", req.RequestID, map[string]any{"request_id": req.RequestID, "duplicate": result.Duplicate})
}

// Keep membership stable throughout a room operation, while permitting other
// rooms to run. Both waits are cancellable; there is no read-to-write upgrade.
func (m *TextManager) lockRuntime(ctx context.Context, p *TextPeer) (*textRoom, func(), error) {
	if err := waitTextLock(ctx, m.mu.TryRLock); err != nil {
		return nil, nil, err
	}
	if !m.current(p) {
		m.mu.RUnlock()
		return nil, nil, ErrTextMembership
	}
	r := m.members[p.AccountID]
	if r == nil {
		m.mu.RUnlock()
		return nil, nil, ErrTextMembership
	}
	if err := waitTextLock(ctx, r.runtimeMu.TryLock); err != nil {
		m.mu.RUnlock()
		return nil, nil, err
	}
	unlock := func() { r.runtimeMu.Unlock(); m.mu.RUnlock() }
	if err := m.checkAuthority(ctx); err != nil {
		unlock()
		return nil, nil, err
	}
	if !m.current(p) {
		unlock()
		return nil, nil, ErrTextMembership
	}
	if r.pendingOperator != nil || r.excludedAccounts[p.AccountID] {
		unlock()
		return nil, nil, ErrTextUnavailable
	}
	return r, unlock, nil
}

func waitTextLock(ctx context.Context, acquire func() bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if acquire() {
		return nil
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ctx.Err(); err != nil {
				return err
			}
			if acquire() {
				return nil
			}
		}
	}
}
func (m *TextManager) Tick(ctx context.Context) error {
	result := m.tick(ctx)
	if ctx.Err() == nil {
		result = errors.Join(result, m.retryRoomOperations(ctx))
	}
	return result
}
func (m *TextManager) tick(ctx context.Context) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	if e := m.checkAuthority(ctx); e != nil {
		return e
	}
	var result error
	now := m.deps.Now()
	if m.draining.Load() {
		result = errors.Join(result, m.cleanupDrain(ctx))
	}
	m.pruneRequestRates(now)
	for _, p := range m.peers {
		if room := m.members[p.AccountID]; room != nil && room.pendingOperator != nil {
			continue
		}
		if p.closed.Load() {
			result = errors.Join(result, m.disconnect(ctx, p))
		}
	}
	for _, q := range m.queues {
		if !q.choice && !now.Before(q.decision) {
			q.choice = true
			result = errors.Join(result, m.emit(q.peer, "queue", "", m.queueView(q, "choice_required")))
		}
	}
	for _, r := range m.rooms {
		if r.pendingOperator != nil {
			continue
		}
		if r.pendingAbort != "" {
			result = errors.Join(result, m.abortPrepared(ctx, r))
			continue
		}
		if r.match == nil {
			continue
		}
		phase, deadline := r.match.Clock()
		if phase == v2.PhaseVerdict {
			m.pruneFinishedRoom(r)
			continue
		}
		if deadline.IsZero() || now.Before(deadline) {
			continue
		}
		out, e := r.match.Advance(ctx, now)
		if e != nil {
			result = errors.Join(result, e)
			continue
		}
		if out.Changed {
			result = errors.Join(result, m.broadcastMatch(ctx, r))
		}
	}
	return result
}

// An explicit departure retains its live seat for grace/forfeit rules, but must
// release account membership once terminal so a later queue does not require an
// invisible second room_leave. Immutable match accounts retain history identity.
func (m *TextManager) pruneFinishedRoom(r *textRoom) {
	if r.match == nil {
		return
	}
	phase, _ := r.match.Clock()
	if phase != v2.PhaseVerdict {
		return
	}
	for seat, member := range r.seats {
		if member.peer == nil || member.peer.closed.Load() {
			if m.members[member.account] == r {
				delete(m.members, member.account)
			}
			delete(r.seats, seat)
		}
	}
	if len(r.seats) == 0 {
		delete(m.rooms, r.code)
	}
}
func (m *TextManager) Drain() {
	_ = waitTextLock(context.Background(), m.mu.TryLock)
	defer m.mu.Unlock()
	m.draining.Store(true)
}
func (m *TextManager) ActiveMatches() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, r := range m.rooms {
		if r.pendingAbort != "" {
			n++
			continue
		}
		if r.match != nil {
			phase, _ := r.match.Clock()
			if phase != v2.PhaseVerdict {
				n++
			}
		}
	}
	return n
}
func (m *TextManager) Close(ctx context.Context) error {
	pendingErr := m.retryRoomOperations(ctx)
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	if e := m.checkAuthority(ctx); e != nil {
		return e
	}
	m.draining.Store(true)
	result := pendingErr
	for _, r := range m.rooms {
		if r.pendingOperator != nil {
			result = errors.Join(result, ErrTextUnavailable)
			continue
		}
		if r.pendingAbort != "" {
			result = errors.Join(result, m.abortPrepared(ctx, r))
			continue
		}
		if r.match != nil {
			_, e := r.match.Close(ctx)
			result = errors.Join(result, e)
		} else {
			for _, s := range r.seats {
				result = errors.Join(result, m.release(ctx, s))
			}
		}
	}
	for _, q := range m.queues {
		result = errors.Join(result, m.deps.Values.CancelReservation(ctx, q.id, q.peer.AccountID))
	}
	if result == nil {
		for _, p := range m.peers {
			m.closePeer(p)
		}
		m.queues = map[string]*textQueue{}
	}
	return result
}

// Run is owned by process lifetime. Clock inputs and rate are server authored.
func (m *TextManager) Run(ctx context.Context, onError func(error)) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			work, cancel := context.WithTimeout(ctx, 5*time.Second)
			e := m.Tick(work)
			cancel()
			if e != nil && onError != nil {
				onError(e)
			}
		}
	}
}

func (m *TextManager) abortPrepared(ctx context.Context, r *textRoom) error {
	if r.pendingAbort == "" {
		return nil
	}
	e := m.deps.Values.CancelPrepared(ctx, r.pendingAbort, m.owner, 1, m.deps.Now())
	if errors.Is(e, store.ErrValueFence) {
		e = m.deps.Values.Interrupt(ctx, r.pendingAbort, m.owner, 1, m.deps.Now())
	}
	if errors.Is(e, sql.ErrNoRows) {
		e = nil
		for _, s := range r.seats {
			e = errors.Join(e, m.release(ctx, s))
		}
	}
	if e != nil {
		return e
	}
	r.pendingAbort = ""
	for _, s := range r.seats {
		s.admission = ""
	}
	m.invalidate(r)
	return nil
}

// RejectAction keeps admitted action errors in the same serialized recipient
// stream. Authentication and pre-admission controls have no match cursor.
func (m *TextManager) RejectAction(ctx context.Context, p *TextPeer, requestID string, code v2.ErrorCode) error {
	if err := waitTextLock(ctx, m.mu.TryRLock); err != nil {
		return err
	}
	defer m.mu.RUnlock()
	if !m.current(p) {
		return ErrTextMembership
	}
	r := m.members[p.AccountID]
	if r == nil || r.match == nil {
		return m.emit(p, "error", requestID, map[string]any{"code": code, "request_id": requestID})
	}
	if err := waitTextLock(ctx, r.runtimeMu.TryLock); err != nil {
		return err
	}
	defer r.runtimeMu.Unlock()
	if err := m.checkAuthority(ctx); err != nil {
		return err
	}
	if !m.current(p) {
		return ErrTextMembership
	}
	seat, _ := m.member(r, p.AccountID)
	event, err := r.match.ActionError(seat, requestID, code)
	if err != nil {
		return m.emit(p, "error", "", map[string]any{"code": v2.ErrMalformed})
	}
	return m.emit(p, "error", requestID, event)
}
