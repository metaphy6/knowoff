package lobby

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func operatorRuntimeFixture(t *testing.T) (*TextManager, *store.TextValueStore, *sql.DB, context.Context, string, *time.Time, v2.LobbySettings) {
	t.Helper()
	db := textAdapterDB(t)
	owner, err := store.AcquireTextOwner(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Release(context.Background()) })
	m, _, now, settings := textManagerFixture(t)
	values, err := store.NewTextValueStore(db, m.deps.Config.Tuning).WithOwner(owner)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := owner.RecoverLostOwners(t.Context(), values, 100)
	if err != nil || !recovery.Done {
		t.Fatal(recovery, err)
	}
	m.deps.Values = values
	m.deps.Authority = owner
	m.deps.Operations = store.NewAdminOperationStore(db)
	m.owner = owner.Token().IncarnationID
	actor, account := uuid.NewString(), uuid.NewString()
	if _, err = db.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,'operator')`, account); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$1::uuid::text,'fixture','fixture')`, actor, account); err != nil {
		t.Fatal(err)
	}
	ctx := store.WithAdminAuthorization(t.Context(), actor, func(context.Context, *sql.Tx) (string, error) { return actor, nil })
	return m, values, db, ctx, actor, now, settings
}
func operatorRuntimeRoom(t *testing.T, m *TextManager, db *sql.DB, settings v2.LobbySettings, start bool) ([]*TextPeer, *textRoom) {
	t.Helper()
	peers := make([]*TextPeer, settings.Size)
	for i := range peers {
		id := uuid.NewString()
		if _, err := db.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,$2)`, id, id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO profiles(account_id) VALUES($1)`, id); err != nil {
			t.Fatal(err)
		}
		var err error
		peers[i], err = m.Open(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
	}
	code, err := m.Create(t.Context(), peers[0], settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range peers[1:] {
		if err = m.Join(t.Context(), p, code); err != nil {
			t.Fatal(err)
		}
	}
	r := m.members[peers[0].AccountID]
	for _, p := range peers {
		if err = m.Ready(t.Context(), p, v2.ReadyAcknowledgement{SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision}); err != nil {
			t.Fatal(err)
		}
	}
	if start {
		if err = m.Start(t.Context(), peers[0]); err != nil {
			t.Fatal(err)
		}
	}
	return peers, r
}
func operatorRoomCommand(m *TextManager, r *textRoom, kind, target string) store.AdminOperationCommand {
	token := m.deps.Authority.(interface{ Token() store.TextOwnerToken }).Token()
	return store.AdminOperationCommand{ID: uuid.NewString(), Kind: kind, RoomID: r.id, OwnerID: token.IncarnationID, OwnerGeneration: token.Generation, TargetAccountID: target, Reason: "reviewed fixture intervention"}
}
func TestOperatorWaitingRoomDecisionPrecedesEffectsAndReplays(t *testing.T) {
	for _, kind := range []string{"room_close", "room_kick"} {
		t.Run(kind, func(t *testing.T) {
			m, _, db, ctx, actor, _, settings := operatorRuntimeFixture(t)
			peers, r := operatorRuntimeRoom(t, m, db, settings, false)
			target := ""
			if kind == "room_kick" {
				target = peers[0].AccountID
			}
			command := operatorRoomCommand(m, r, kind, target)
			denied := store.WithAdminAuthorization(t.Context(), actor, func(context.Context, *sql.Tx) (string, error) { return "", errors.New("revoked session") })
			if _, err := m.DecideRoomOperation(denied, actor, command); err == nil {
				t.Fatal("decision ignored revoked session")
			}
			if r.pendingOperator != nil || len(r.seats) != settings.Size {
				t.Fatal("effect before decision")
			}
			receipt, err := m.DecideRoomOperation(ctx, actor, command)
			if err != nil || receipt.Status != "applied" {
				t.Fatal(receipt, err)
			}
			expected := settings.Size
			if kind == "room_kick" {
				expected = 1
			}
			var released, results, ledger int
			if err = db.QueryRow(`SELECT count(*) FROM text_admissions WHERE state='released'`).Scan(&released); err != nil || released != expected {
				t.Fatal(released, err)
			}
			for i := 0; i < 2; i++ {
				again, e := m.DecideRoomOperation(ctx, actor, command)
				if e != nil || again.Status != "applied" {
					t.Fatal(again, e)
				}
			}
			if _, err = m.DecideRoomOperation(denied, actor, command); err == nil {
				t.Fatal("completed replay ignored revoked initiating session")
			}
			_ = db.QueryRow(`SELECT count(*) FROM admin_operation_results`).Scan(&results)
			_ = db.QueryRow(`SELECT count(*) FROM noin_ledger`).Scan(&ledger)
			if results != 1 || ledger != 0 {
				t.Fatal("replay fabricated effects", results, ledger)
			}
			if kind == "room_kick" {
				if len(r.seats) != settings.Size-1 || r.host == 0 {
					t.Fatal("waiting host not removed/elected")
				}
				replacement, e := m.Open(t.Context(), target)
				if e != nil {
					t.Fatal(e)
				}
				if e = m.Join(t.Context(), replacement, r.code); e == nil {
					t.Fatal("kicked account rejoined room")
				}
			}
		})
	}
}
func TestOperatorLiveCloseAllModesAndSizesKeepsPrivateSettlement(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(string(mode)+string(rune('0'+size)), func(t *testing.T) {
				m, _, db, ctx, actor, _, settings := operatorRuntimeFixture(t)
				settings.ModeID = mode
				settings.Size = size
				peers, r := operatorRuntimeRoom(t, m, db, settings, true)
				contract := r.match.Contract()
				receipt, err := m.DecideRoomOperation(ctx, actor, operatorRoomCommand(m, r, "room_close", ""))
				if err != nil || receipt.Status != "applied" {
					t.Fatal(receipt, err)
				}
				var state string
				var settlements, grants int
				if err = db.QueryRow(`SELECT state FROM text_matches WHERE id=$1`, contract.MatchID).Scan(&state); err != nil || state != "interrupted" {
					t.Fatal(state, err)
				}
				_ = db.QueryRow(`SELECT count(*) FROM text_settlements WHERE match_id=$1 AND state='applied'`, contract.MatchID).Scan(&settlements)
				_ = db.QueryRow(`SELECT count(*) FROM noin_ledger`).Scan(&grants)
				if settlements != size || grants != 0 {
					t.Fatal("prototype interruption fabricated value or lost private receipt", settlements, grants)
				}
				if len(m.members) != 0 || m.rooms[r.code] != nil {
					t.Fatal("closed room remains admitted")
				}
				for _, p := range peers {
					if m.current(p) {
						if err = m.Resync(t.Context(), p); err == nil {
							t.Fatal("closed room still resyncs")
						}
					}
				}
			})
		}
	}
}

type blockedOperatorValues struct {
	*store.TextValueStore
	entered, release chan struct{}
	once             sync.Once
	operation        string
}

func (b *blockedOperatorValues) ApplyRoomOperation(ctx context.Context, a store.AdminRoomApplication) (store.AdminOperationReceipt, error) {
	if a.OperationID == b.operation {
		b.once.Do(func() { close(b.entered) })
		select {
		case <-b.release:
		case <-ctx.Done():
			return store.AdminOperationReceipt{}, ctx.Err()
		}
	}
	return b.TextValueStore.ApplyRoomOperation(ctx, a)
}
func TestOperatorPendingRoomDoesNotStallOtherRoomOrQueuedTick(t *testing.T) {
	m, values, db, ctx, actor, now, settings := operatorRuntimeFixture(t)
	first, room := operatorRuntimeRoom(t, m, db, settings, true)
	second, other := operatorRuntimeRoom(t, m, db, settings, true)
	_, deadline := room.match.Clock()
	*now = deadline
	if err := m.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	c := operatorRoomCommand(m, room, "room_close", "")
	blocked := &blockedOperatorValues{TextValueStore: values, operation: c.ID, entered: make(chan struct{}), release: make(chan struct{})}
	m.deps.Values = blocked
	// The close hook resolves the current value adapter, retaining the captured
	// owner/contract rather than an earlier mutable room or caller identity.
	done := make(chan error, 1)
	go func() { _, err := m.DecideRoomOperation(ctx, actor, c); done <- err }()
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("completion not reached")
	}
	tickCtx, cancelTick := context.WithCancel(t.Context())
	defer cancelTick()
	tickDone := make(chan error, 1)
	go func() { tickDone <- m.Tick(tickCtx) }()
	readCtx, cancelRead := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancelRead()
	s, err := other.match.SnapshotProjection(0)
	if err != nil || s.CurrentSeat == nil {
		t.Fatal(err)
	}
	seat := *s.CurrentSeat
	s, err = other.match.SnapshotProjection(seat)
	if err != nil {
		t.Fatal(err)
	}
	one := 1
	req := v2.ActionRequest{Version: 2, RequestID: "other-draw", MatchID: s.Contract.MatchID, ModeID: s.Contract.ModeID, Round: s.Round, Turn: s.Turn, Phase: s.Phase, PhaseID: s.PhaseID, ExpectedBoardRevision: s.Board.Revision, Action: v2.Action{Kind: v2.ActionDraw, Count: &one}}
	if err = m.Action(readCtx, second[seat], req); err != nil {
		t.Error("unrelated action", err)
	}
	if err = m.Resync(readCtx, second[seat]); err != nil {
		t.Error("unrelated resync", err)
	}
	cancelTick()
	select {
	case <-tickDone:
	case <-time.After(200 * time.Millisecond):
		t.Error("ticker ignored cancellation")
	}
	close(blocked.release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if slices.Contains(m.accounts(other), first[0].AccountID) {
		t.Fatal("operator touched unrelated membership")
	}
}

func TestOperatorLiveKickPendingAuditRetainsFirstGraceAllCells(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(string(mode)+string(rune('0'+size)), func(t *testing.T) {
				m, _, db, ctx, actor, now, settings := operatorRuntimeFixture(t)
				m.deps.Config.Tuning.Game.ReconnectGraceS = 2
				values, rebindErr := store.NewTextValueStore(db, m.deps.Config.Tuning).WithOwner(m.deps.Authority.(*store.TextOwner))
				if rebindErr != nil {
					t.Fatal(rebindErr)
				}
				m.deps.Values = values
				settings.ModeID = mode
				settings.Size = size
				peers, room := operatorRuntimeRoom(t, m, db, settings, true)
				_, deadline := room.match.Clock()
				*now = deadline
				if err := m.Tick(t.Context()); err != nil {
					t.Fatal(err)
				}
				initial, err := room.match.SnapshotProjection(0)
				if err != nil || initial.CurrentSeat == nil {
					t.Fatal(err)
				}
				target := (*initial.CurrentSeat + 1) % size
				command := operatorRoomCommand(m, room, "room_kick", peers[target].AccountID)
				if _, err = db.Exec(`CREATE FUNCTION refuse_live_kick_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='operator_applied' THEN RAISE EXCEPTION 'fixture audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER refuse_live_kick_audit BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION refuse_live_kick_audit()`); err != nil {
					t.Fatal(err)
				}
				receipt, err := m.DecideRoomOperation(ctx, actor, command)
				if err != nil || receipt.Status != "pending" {
					t.Fatal("accepted decision lost pending status", receipt, err)
				}
				if room.pendingOperator == nil || room.pendingOperator.receipt.Command.ID != command.ID {
					t.Fatal("failed completion lost immutable retry identity")
				}
				select {
				case <-peers[target].Done:
				default:
					t.Fatal("persisted kick did not close transport")
				}
				after, err := room.match.SnapshotProjection(0)
				if err != nil || after.Seats[target].Connected {
					t.Fatal("kick did not commit normal disconnected seat", err)
				}
				_, firstGrace := room.match.Clock()
				if !firstGrace.Equal(now.Add(2 * time.Second)) {
					t.Fatal("unexpected first grace", firstGrace, *now)
				}
				for _, p := range peers {
					if p == peers[target] {
						continue
					}
					if err = m.Resync(t.Context(), p); err == nil {
						t.Fatal("pending room resynced")
					}
				}
				if err = m.Join(t.Context(), peers[0], room.code); err == nil {
					t.Fatal("pending room joined")
				}
				if err = m.Leave(t.Context(), peers[0]); err == nil {
					t.Fatal("pending room changed captured membership")
				}
				*now = now.Add(time.Second)
				if _, err = m.retryRoomOperation(t.Context(), room.id, command.ID); err == nil {
					t.Fatal("audit failure unexpectedly cleared")
				}
				_, retryGrace := room.match.Clock()
				if !retryGrace.Equal(firstGrace) {
					t.Fatal("retry extended grace", firstGrace, retryGrace)
				}
				if _, err = db.Exec(`DROP TRIGGER refuse_live_kick_audit ON admin_audit_log; DROP FUNCTION refuse_live_kick_audit()`); err != nil {
					t.Fatal(err)
				}
				if err = m.Tick(t.Context()); err != nil {
					t.Fatal(err)
				}
				if room.pendingOperator != nil || m.members[peers[target].AccountID] != room {
					t.Fatal("kick lost original active seat or pending fence")
				}
				replacement, err := m.Open(t.Context(), peers[target].AccountID)
				if err != nil {
					t.Fatal(err)
				}
				if err = m.Join(t.Context(), replacement, room.code); err == nil {
					t.Fatal("kicked account reclaimed original seat")
				}
				var grants, results int
				if err = db.QueryRow(`SELECT count(*) FROM noin_ledger`).Scan(&grants); err != nil {
					t.Fatal(err)
				}
				if err = db.QueryRow(`SELECT count(*) FROM admin_operation_results`).Scan(&results); err != nil {
					t.Fatal(err)
				}
				if grants != 0 || results != 1 {
					t.Fatal("kick fabricated value or duplicated receipt", grants, results)
				}
			})
		}
	}
}

type uncertainRoomDecision struct {
	*store.AdminOperationStore
	commit    bool
	probeDown bool
}

func (s *uncertainRoomDecision) Decide(ctx context.Context, actor string, c store.AdminOperationCommand, accounts []string) (store.AdminOperationReceipt, error) {
	if s.commit {
		if _, err := s.AdminOperationStore.Decide(ctx, actor, c, accounts); err != nil {
			return store.AdminOperationReceipt{}, err
		}
	}
	return store.AdminOperationReceipt{}, errors.New("fixture lost database response")
}
func (s *uncertainRoomDecision) ResolveRoomDecision(ctx context.Context, actor string, c store.AdminOperationCommand, accounts []string) (store.AdminOperationReceipt, error) {
	if s.probeDown {
		return store.AdminOperationReceipt{}, errors.New("fixture unavailable reconciliation")
	}
	resolver, ok := any(s.AdminOperationStore).(interface {
		ResolveRoomDecision(context.Context, string, store.AdminOperationCommand, []string) (store.AdminOperationReceipt, error)
	})
	if !ok {
		return store.AdminOperationReceipt{}, errors.New("missing serialized resolution")
	}
	return resolver.ResolveRoomDecision(ctx, actor, c, accounts)
}
func TestOperatorUncertainDecisionNeverLosesFenceOrFabricatesAuthority(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(fmt.Sprintf("commit_%t", commit), func(t *testing.T) {
			m, _, db, ctx, actor, _, settings := operatorRuntimeFixture(t)
			_, room := operatorRuntimeRoom(t, m, db, settings, false)
			source := &uncertainRoomDecision{AdminOperationStore: store.NewAdminOperationStore(db), commit: commit, probeDown: true}
			m.deps.Operations = source
			command := operatorRoomCommand(m, room, "room_close", "")
			if _, err := m.DecideRoomOperation(ctx, actor, command); err == nil {
				t.Fatal("uncertain decision reported confirmed success")
			}
			if room.pendingOperator == nil {
				t.Fatal("uncertain response lost room fence")
			}
			var released int
			if err := db.QueryRow(`SELECT count(*) FROM text_admissions WHERE state='released'`).Scan(&released); err != nil || released != 0 {
				t.Fatal("effect before authoritative decision", released, err)
			}
			source.probeDown = false
			if err := m.Tick(t.Context()); err != nil {
				t.Fatal(err)
			}
			if room.pendingOperator != nil {
				t.Fatal("confirmed resolution did not drain fence")
			}
			if commit {
				if m.rooms[room.code] != nil {
					t.Fatal("confirmed accepted decision not delivered")
				}
			} else {
				if m.rooms[room.code] != room || len(room.seats) != settings.Size {
					t.Fatal("proven absent decision changed room")
				}
			}
			if err := db.QueryRow(`SELECT count(*) FROM text_admissions WHERE state='released'`).Scan(&released); err != nil {
				t.Fatal(err)
			}
			want := 0
			if commit {
				want = settings.Size
			}
			if released != want {
				t.Fatal("wrong resolved effects", released, want)
			}
			m.deps.Operations = source.AdminOperationStore
			denied := store.WithAdminAuthorization(t.Context(), actor, func(context.Context, *sql.Tx) (string, error) { return "", errors.New("revoked session") })
			if _, err := m.DecideRoomOperation(denied, actor, command); err == nil {
				t.Fatal("revoked initiating session replayed or created a decision")
			}
			var decisions int
			if err := db.QueryRow(`SELECT count(*) FROM admin_operation_decisions`).Scan(&decisions); err != nil {
				t.Fatal(err)
			}
			expected := 0
			if commit {
				expected = 1
			}
			if decisions != expected {
				t.Fatal("revoked replay created decision", decisions, expected)
			}
		})
	}
}

func TestOperatorLostDecisionResponseDoesNotReturnAuthorizedSuccess(t *testing.T) {
	m, _, db, ctx, actor, _, settings := operatorRuntimeFixture(t)
	_, room := operatorRuntimeRoom(t, m, db, settings, false)
	m.deps.Operations = &uncertainRoomDecision{AdminOperationStore: store.NewAdminOperationStore(db), commit: true}
	command := operatorRoomCommand(m, room, "room_close", "")
	if _, err := m.DecideRoomOperation(ctx, actor, command); err == nil {
		t.Fatal("failed foreground authorization/commit response became success through trusted probe")
	}
	if room.pendingOperator == nil || room.pendingOperator.unconfirmed {
		t.Fatal("committed decision lost confirmed retry fence")
	}
	if err := m.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.rooms[room.code] != nil {
		t.Fatal("trusted retry failed to deliver committed decision")
	}
}
