package lobby

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/store"
)

// Decisions are always browser-authorized. Only their already committed delivery
// is retried by the existing runtime clock without the initiating session.
type TextRoomOperations interface {
	Decide(context.Context, string, store.AdminOperationCommand, []string) (store.AdminOperationReceipt, error)
	Get(context.Context, string) (store.AdminOperationReceipt, error)
	ResolveRoomDecision(context.Context, string, store.AdminOperationCommand, []string) (store.AdminOperationReceipt, error)
}
type textRoomOperationValues interface {
	ApplyRoomOperation(context.Context, store.AdminRoomApplication) (store.AdminOperationReceipt, error)
}
type textRoomOperation struct {
	receipt     store.AdminOperationReceipt
	application store.AdminRoomApplication
	unconfirmed bool
}
type textOperatorCloseContext struct{}

// This context retains cancellation and a fixed delivery deadline, but carries
// no browser session authority. The immutable decision authorizes this retry.
func textOperatorDeliveryContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	stop := context.AfterFunc(parent, cancel)
	if parent.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}
func (m *TextManager) operatorToken() (store.TextOwnerToken, error) {
	authority, ok := m.deps.Authority.(interface{ Token() store.TextOwnerToken })
	if !ok || m.deps.Operations == nil {
		return store.TextOwnerToken{}, ErrTextUnavailable
	}
	if _, ok = m.deps.Values.(textRoomOperationValues); !ok {
		return store.TextOwnerToken{}, ErrTextUnavailable
	}
	token := authority.Token()
	if token.IncarnationID != m.owner || token.Generation < 1 {
		return token, ErrTextUnavailable
	}
	return token, nil
}
func (m *TextManager) roomByID(id string) *textRoom {
	for _, room := range m.rooms {
		if room.id == id {
			return room
		}
	}
	return nil
}

// DecideRoomOperation snapshots affected accounts/admissions on the server. A
// durable decision is returned as pending if its delivery must be retried.
func (m *TextManager) DecideRoomOperation(ctx context.Context, actor string, command store.AdminOperationCommand) (store.AdminOperationReceipt, error) {
	var receipt store.AdminOperationReceipt
	if command.Kind != "room_close" && command.Kind != "room_kick" {
		return receipt, store.ErrAdminOperation
	}
	token, err := m.operatorToken()
	if err != nil {
		return receipt, err
	}
	if command.OwnerID != token.IncarnationID || command.OwnerGeneration != token.Generation {
		return receipt, store.ErrValueFence
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Completed room removal must not break exact command/session replay.
	prior, err := m.deps.Operations.Get(ctx, command.ID)
	if err == nil {
		receipt, err = m.deps.Operations.Decide(ctx, actor, command, prior.AffectedAccounts)
		if err != nil || receipt.Status != "pending" {
			return receipt, err
		}
		delivered, deliveryErr := m.retryRoomOperation(ctx, command.RoomID, command.ID)
		if deliveryErr == nil {
			return delivered, nil
		}
		return receipt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return receipt, err
	}
	if err = waitTextLock(ctx, m.mu.TryRLock); err != nil {
		return receipt, err
	}
	room := m.roomByID(command.RoomID)
	if room == nil {
		m.mu.RUnlock()
		return receipt, ErrTextMembership
	}
	if err = waitTextLock(ctx, room.runtimeMu.TryLock); err != nil {
		m.mu.RUnlock()
		return receipt, err
	}
	unlock := func() { room.runtimeMu.Unlock(); m.mu.RUnlock() }
	if err = m.checkAuthority(ctx); err != nil {
		unlock()
		return receipt, err
	}
	if room.pendingOperator != nil || command.Kind == "room_kick" && room.pendingAbort != "" {
		unlock()
		return receipt, ErrTextUnavailable
	}
	accounts := m.accounts(room)
	if room.match != nil {
		accounts = slices.Clone(room.matchAccounts)
	}
	slices.Sort(accounts)
	if command.Kind == "room_kick" {
		_, member := m.member(room, command.TargetAccountID)
		if member == nil || room.excludedAccounts[command.TargetAccountID] {
			unlock()
			return receipt, ErrTextMembership
		}
	}
	application := store.AdminRoomApplication{OperationID: command.ID, RoomID: room.id, At: m.deps.Now()}
	if room.match != nil {
		application.MatchID = room.match.Contract().MatchID
		application.Effect = "close"
		if command.Kind == "room_kick" {
			application.Effect = "kick"
		}
	} else if room.pendingAbort != "" {
		application.MatchID = room.pendingAbort
		application.Effect = "close"
	} else {
		application.Effect = "release"
		application.Reservations = map[string]string{}
		for _, member := range room.seats {
			application.Reservations[member.account] = member.admission
		}
	}
	receipt, err = m.deps.Operations.Decide(ctx, actor, command, accounts)
	if err != nil {
		// A failed response may follow a committed decision. Freeze the exact
		// captured room until a serialized probe proves commit or rollback.
		originalErr := err
		probe, finishProbe := context.WithTimeout(context.Background(), 3*time.Second)
		receipt, err = m.deps.Operations.ResolveRoomDecision(probe, actor, command, accounts)
		finishProbe()
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, store.ErrAdminOperation) {
				room.pendingOperator = &textRoomOperation{
					receipt:     store.AdminOperationReceipt{ActorID: actor, Command: command, AffectedAccounts: slices.Clone(accounts)},
					application: application, unconfirmed: true,
				}
			}
			unlock()
			return store.AdminOperationReceipt{}, originalErr
		}
		room.pendingOperator = &textRoomOperation{receipt: receipt, application: application}
		unlock()
		// The trusted probe authorizes delivery of an old decision, never a
		// successful foreground response after failed session authorization.
		return store.AdminOperationReceipt{}, originalErr
	}
	room.pendingOperator = &textRoomOperation{receipt: receipt, application: application}
	unlock()
	delivered, deliveryErr := m.retryRoomOperation(ctx, room.id, command.ID)
	if deliveryErr == nil {
		return delivered, nil
	}
	return receipt, nil
}

func (m *TextManager) retryRoomOperation(parent context.Context, roomID, operationID string) (store.AdminOperationReceipt, error) {
	var receipt store.AdminOperationReceipt
	ctx, cancel := textOperatorDeliveryContext(parent)
	defer cancel()
	if err := waitTextLock(ctx, m.mu.TryRLock); err != nil {
		return receipt, err
	}
	room := m.roomByID(roomID)
	if room == nil {
		m.mu.RUnlock()
		return receipt, ErrTextMembership
	}
	if err := waitTextLock(ctx, room.runtimeMu.TryLock); err != nil {
		m.mu.RUnlock()
		return receipt, err
	}
	unlock := func() { room.runtimeMu.Unlock(); m.mu.RUnlock() }
	pending := room.pendingOperator
	if pending == nil || pending.receipt.Command.ID != operationID {
		unlock()
		return receipt, ErrTextMembership
	}
	receipt = pending.receipt
	if err := m.checkAuthority(ctx); err != nil {
		unlock()
		return receipt, err
	}
	if pending.unconfirmed {
		confirmed, probeErr := m.deps.Operations.ResolveRoomDecision(ctx, receipt.ActorID, receipt.Command, receipt.AffectedAccounts)
		if probeErr != nil {
			unlock()
			if !errors.Is(probeErr, sql.ErrNoRows) && !errors.Is(probeErr, store.ErrAdminOperation) {
				return store.AdminOperationReceipt{}, probeErr
			}
			// No domain effect occurred. Clear only this still-tentative fence;
			// another caller may have confirmed it while we reacquired W.
			if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
				return store.AdminOperationReceipt{}, err
			}
			defer m.mu.Unlock()
			if m.rooms[room.code] == room && room.pendingOperator == pending && pending.unconfirmed {
				room.pendingOperator = nil
			}
			return store.AdminOperationReceipt{}, nil
		}
		pending.receipt = confirmed
		pending.unconfirmed = false
		receipt = confirmed
	}
	values, ok := m.deps.Values.(textRoomOperationValues)
	if !ok {
		unlock()
		return receipt, ErrTextUnavailable
	}
	var err error
	if pending.receipt.Status == "pending" {
		if room.match != nil {
			if pending.receipt.Command.Kind == "room_close" {
				_, err = room.match.Close(context.WithValue(ctx, textOperatorCloseContext{}, operationID))
			} else {
				seat, member := m.member(room, pending.receipt.Command.TargetAccountID)
				if member == nil {
					err = ErrTextMembership
				} else {
					_, err = room.match.SetConnected(ctx, seat, false)
					if err == nil {
						m.closePeer(member.peer)
						if room.excludedAccounts == nil {
							room.excludedAccounts = map[string]bool{}
						}
						room.excludedAccounts[member.account] = true
					}
				}
			}
		}
		if err == nil && pending.receipt.Status == "pending" {
			var delivered store.AdminOperationReceipt
			delivered, err = values.ApplyRoomOperation(ctx, pending.application)
			if err == nil {
				pending.receipt = delivered
			}
		}
		receipt = pending.receipt
		if err != nil {
			unlock()
			return receipt, err
		}
	}
	// Commit-only frames contain each recipient's normal projection. Membership
	// pruning happens separately under W, never by upgrading this read lock.
	if room.match != nil && pending.receipt.Command.Kind == "room_kick" {
		_ = m.broadcastMatchFrames(ctx, room)
	}
	receipt = pending.receipt
	unlock()
	if err = waitTextLock(ctx, m.mu.TryLock); err != nil {
		return receipt, err
	}
	defer m.mu.Unlock()
	if m.rooms[room.code] != room || room.pendingOperator != pending {
		return receipt, nil
	}
	if err = m.checkAuthority(ctx); err != nil {
		return receipt, err
	}
	if pending.receipt.Status == "pending" {
		return receipt, ErrTextUnavailable
	}
	m.completeRoomOperation(room, pending)
	return receipt, nil
}

// Called only with manager W after durable completion. It never invokes value
// hooks and cannot make a committed close/kick spend or settle a second time.
func (m *TextManager) completeRoomOperation(room *textRoom, pending *textRoomOperation) {
	command := pending.receipt.Command
	if command.Kind == "room_close" {
		for _, member := range room.seats {
			if m.members[member.account] == room {
				delete(m.members, member.account)
			}
			m.closePeer(member.peer)
			if m.peers[member.account] == member.peer {
				delete(m.peers, member.account)
			}
		}
		delete(m.rooms, room.code)
		room.pendingOperator = nil
		return
	}
	seat, member := m.member(room, command.TargetAccountID)
	if member != nil {
		m.closePeer(member.peer)
		if m.peers[member.account] == member.peer {
			delete(m.peers, member.account)
		}
		member.peer = nil
		if room.excludedAccounts == nil {
			room.excludedAccounts = map[string]bool{}
		}
		room.excludedAccounts[member.account] = true
		if room.match == nil {
			member.admission = ""
			delete(m.members, member.account)
			delete(room.seats, seat)
			room.membershipRevision++
			m.invalidate(room)
			if room.host == seat {
				m.elect(room)
			}
		}
	}
	room.pendingOperator = nil
	if len(room.seats) == 0 {
		delete(m.rooms, room.code)
	} else if room.match == nil {
		m.broadcastLobby(room)
	} else {
		m.pruneFinishedRoom(room)
	}
}

// Existing Tick is the only live retry worker. It copies a bounded set of room
// identities, then each delivery uses only that room's runtime mutex.
func (m *TextManager) retryRoomOperations(ctx context.Context) error {
	if m.deps.Operations == nil {
		return nil
	}
	if err := waitTextLock(ctx, m.mu.TryRLock); err != nil {
		return err
	}
	type target struct{ room, id string }
	targets := []target{}
	for _, room := range m.rooms {
		if err := waitTextLock(ctx, room.runtimeMu.TryLock); err != nil {
			m.mu.RUnlock()
			return err
		}
		if room.pendingOperator != nil {
			targets = append(targets, target{room.id, room.pendingOperator.receipt.Command.ID})
		}
		room.runtimeMu.Unlock()
		if len(targets) == 100 {
			break
		}
	}
	m.mu.RUnlock()
	var result error
	for _, target := range targets {
		_, err := m.retryRoomOperation(ctx, target.room, target.id)
		result = errors.Join(result, err)
	}
	return result
}

func (m *TextManager) operatorFinish(room *textRoom, ordinary func(context.Context, game.TextResult) error) func(context.Context, game.TextResult) error {
	return func(ctx context.Context, result game.TextResult) error {
		id, _ := ctx.Value(textOperatorCloseContext{}).(string)
		pending := room.pendingOperator
		if id == "" || pending == nil || pending.receipt.Command.ID != id || pending.receipt.Command.Kind != "room_close" || result.Outcome != "interrupted" {
			return ordinary(ctx, result)
		}
		values, ok := m.deps.Values.(textRoomOperationValues)
		if !ok {
			return ErrTextUnavailable
		}
		application := pending.application
		application.At = result.OccurredAt
		receipt, err := values.ApplyRoomOperation(ctx, application)
		if err == nil {
			pending.receipt = receipt
		}
		return err
	}
}
