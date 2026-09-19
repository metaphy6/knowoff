package lobby

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
	"gopkg.in/yaml.v3"
)

type fakeTextValues struct {
	failPreparedCancel           bool
	failCancel                   bool
	mu                           sync.Mutex
	reservations                 map[string]store.TextReservation
	starts, finishes, interrupts int
	awards                       int
	failStart                    bool
}

func (f *fakeTextValues) Reserve(_ context.Context, r store.TextReservation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, old := range f.reservations {
		if old.AccountID == r.AccountID && id != r.ID {
			return errors.New("duplicate admission")
		}
	}
	f.reservations[r.ID] = r
	return nil
}
func (f *fakeTextValues) CancelReservation(_ context.Context, id, account string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCancel {
		return errors.New("database temporarily unavailable")
	}
	delete(f.reservations, id)
	return nil
}
func (f *fakeTextValues) Prepare(context.Context, store.TextMatchRecord, time.Time) error { return nil }
func (f *fakeTextValues) Start(context.Context, string, string, int64, time.Time) error {
	if f.failStart {
		return errors.New("access withdrawn")
	}
	f.starts++
	return nil
}
func (f *fakeTextValues) CancelPrepared(context.Context, string, string, int64, time.Time) error {
	if f.failPreparedCancel {
		return errors.New("cancel prepared temporarily unavailable")
	}
	f.reservations = map[string]store.TextReservation{}
	return nil
}
func (f *fakeTextValues) Award(context.Context, store.TextAward) (int, error) {
	f.awards++
	return 0, nil
}
func (f *fakeTextValues) Abandon(context.Context, store.TextAbandon) error { return nil }
func (f *fakeTextValues) Finish(context.Context, store.TextOutcome) error {
	f.finishes++
	f.reservations = map[string]store.TextReservation{}
	return nil
}
func (f *fakeTextValues) SettlePending(context.Context, string) error { return nil }
func (f *fakeTextValues) Interrupt(context.Context, string, string, int64, time.Time) error {
	f.interrupts++
	return nil
}

func textManagerFixture(t *testing.T) (*TextManager, *fakeTextValues, *time.Time, v2.LobbySettings) {
	t.Helper()
	raw, e := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if e != nil {
		t.Fatal(e)
	}
	cfg := &config.Config{}
	if e = yaml.Unmarshal(raw, &cfg.Tuning); e != nil {
		t.Fatal(e)
	}
	cfg.App.Env = "test"
	cfg.WebSocket.MaxMessageBytes = 65536
	cfg.Text = &config.TextConfig{Version: 1, RulesVersion: "text-v1", Compatibility: config.TextCompatibility{ProtocolVersion: 2, MinClientGeneration: 2}, Modes: map[gamecontract.ModeID]config.TextModeConfig{}}
	for _, mode := range gamecontract.AllModes() {
		cfg.Text.Modes[mode] = config.TextModeConfig{Enabled: true, ContentLanguages: []string{"en"}}
	}
	snap, e := media.LoadTextPack("../../pkg/media/testdata/text-en", media.TextLimits{MaxTextBytes: cfg.Tuning.Contract.MaxTextBytes, MaxRecords: cfg.Tuning.TextCatalog.MaxRecords, MaxFileBytes: cfg.Tuning.TextCatalog.MaxFileBytes, MaxBundleBytes: cfg.Tuning.TextCatalog.MaxBundleBytes})
	if e != nil {
		t.Fatal(e)
	}
	values := &fakeTextValues{reservations: map[string]store.TextReservation{}}
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	m, e := NewTextManager(TextDeps{Config: cfg, Values: values, Prototype: snap, Now: func() time.Time { return now }})
	if e != nil {
		t.Fatal(e)
	}
	return m, values, &now, v2.LobbySettings{ModeID: gamecontract.ModeMissedTheBriefing, Size: 4, ContentLanguage: "en", PackReleaseID: snap.Manifest().ReleaseID, RulesVersion: "text-v1"}
}
func textPeer(t *testing.T, m *TextManager) *TextPeer {
	t.Helper()
	p, e := m.Open(context.Background(), uuid.NewString())
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestTextMaintenanceAndDependenciesPauseIndependently(t *testing.T) {
	m, values, now, settings := textManagerFixture(t)
	ctx := context.Background()
	peers, room := textReadyRoom(t, m, settings)
	if err := m.Start(ctx, peers[0]); err != nil {
		t.Fatal(err)
	}
	waiting, waitingRoom := textReadyRoom(t, m, settings)
	outsider := textPeer(t, m)
	reserved := len(values.reservations)
	paused := func() {
		t.Helper()
		if m.RuntimeReady(ctx) == nil {
			t.Fatal("paused runtime advertised ready")
		}
		for _, cell := range m.Availability(ctx).Modes {
			if cell.Available || len(cell.Languages) != 0 {
				t.Fatal("paused admission advertised", cell)
			}
		}
		if _, err := m.Create(ctx, outsider, settings); !errors.Is(err, ErrTextUnavailable) {
			t.Fatal("paused create", err)
		}
		if err := m.QueueJoin(ctx, outsider, settings); !errors.Is(err, ErrTextUnavailable) {
			t.Fatal("paused queue", err)
		}
		if err := m.Start(ctx, waiting[0]); !errors.Is(err, ErrTextUnavailable) {
			t.Fatal("paused start", err)
		}
		if err := m.Ready(ctx, waiting[0], v2.ReadyAcknowledgement{SettingsRevision: waitingRoom.settingsRevision, MembershipRevision: waitingRoom.membershipRevision}); !errors.Is(err, ErrTextUnavailable) {
			t.Fatal("paused ready", err)
		}
		if len(values.reservations) != reserved {
			t.Fatal("pause mutated reservations")
		}
		for _, p := range peers {
			textDrainFrames(p)
		}
		if err := m.Resync(ctx, peers[0]); err != nil {
			t.Fatal("pause stopped begun snapshot", err)
		}
	}
	m.SetReady(false)
	paused()
	m.SetDependencyReady(true)
	paused()
	m.SetDependencyReady(false)
	m.SetReady(true)
	paused()
	m.SetDependencyReady(true)
	if err := m.RuntimeReady(ctx); err != nil {
		t.Fatal("healthy resumed runtime", err)
	}
	if err := m.Start(ctx, waiting[0]); err != nil {
		t.Fatal("resume failed", err)
	}
	m.SetReady(false)
	textFinishRoom(t, m, room, now)
	if err := m.Rematch(ctx, peers[0]); !errors.Is(err, ErrTextUnavailable) {
		t.Fatal("paused rematch", err)
	}
	m.SetReady(true)
	m.Drain()
	m.SetReady(true)
	m.SetDependencyReady(true)
	if m.RuntimeReady(ctx) == nil {
		t.Fatal("permanent drain reopened")
	}
}

func TestTextWalletVisibilityWaitsForVerdictAcrossDisconnectAndLeave(t *testing.T) {
	m, _, now, settings := textManagerFixture(t)
	ctx := t.Context()
	peers, room := textReadyRoom(t, m, settings)
	reads := 0
	read := func() error { reads++; return nil }
	if err := m.WithWalletAccess(ctx, peers[0].AccountID, read); err != nil || reads != 1 {
		t.Fatal("waiting lobby wallet", err, reads)
	}
	if err := m.Start(ctx, peers[0]); err != nil {
		t.Fatal(err)
	}
	denied := func() {
		t.Helper()
		for _, p := range peers {
			if err := m.WithWalletAccess(ctx, p.AccountID, read); !errors.Is(err, ErrTextWalletHidden) {
				t.Fatal("live match exposed wallet", err)
			}
		}
		if reads != 1 {
			t.Fatal("hidden callback executed", reads)
		}
	}
	denied()
	m.Disconnect(ctx, peers[0])
	denied()
	if err := m.Leave(ctx, peers[1]); err != nil {
		t.Fatal(err)
	}
	denied()
	reconnected, err := m.Open(ctx, peers[1].AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Join(ctx, reconnected, room.code); err != nil {
		t.Fatal(err)
	}
	denied()
	textFinishRoom(t, m, room, now)
	if err = m.WithWalletAccess(ctx, peers[0].AccountID, read); err != nil || reads != 2 {
		t.Fatal("terminal wallet stayed hidden", err, reads)
	}
	m.mu.Lock()
	m.loseAuthority()
	m.mu.Unlock()
	if err = m.WithWalletAccess(ctx, peers[0].AccountID, read); !errors.Is(err, ErrTextUnavailable) || reads != 2 {
		t.Fatal("lost owner exposed wallet", err, reads)
	}
}

func TestTextSystemNoticeCarriesOnlyRefreshSignal(t *testing.T) {
	m, _, _, _ := textManagerFixture(t)
	peers := []*TextPeer{textPeer(t, m), textPeer(t, m)}
	if err := m.NotifyNotices(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, p := range peers {
		frame := <-p.Frames
		if frame.Version != 2 || frame.Type != "system_notice" || string(frame.Payload) != `{"refresh":true}` {
			t.Fatalf("unexpected notice payload: %+v", frame)
		}
	}
	a := &textAuthorityStub{done: make(chan struct{}), lost: true}
	m.deps.Authority = a
	if err := m.NotifyNotices(context.Background()); err == nil {
		t.Fatal("lost owner broadcast")
	}
	for _, p := range peers {
		if len(p.frames) != 0 {
			t.Fatal("lost owner emitted notice")
		}
	}
}

func TestTextRoomIdentityLookupRequiresCurrentMembership(t *testing.T) {
	m, _, _, settings := textManagerFixture(t)
	ctx := context.Background()
	peers, room := textReadyRoom(t, m, settings)
	if got, err := m.RoomAccount(ctx, peers[0].AccountID, room.id, 1); err != nil || got != peers[1].AccountID {
		t.Fatal(got, err)
	}
	outsider := textPeer(t, m)
	for _, c := range []struct {
		account, room string
		seat          int
	}{{outsider.AccountID, room.id, 1}, {peers[0].AccountID, uuid.NewString(), 1}, {peers[0].AccountID, room.id, 6}} {
		if _, err := m.RoomAccount(ctx, c.account, c.room, c.seat); err == nil {
			t.Fatal("unauthorized room identity", c)
		}
	}
	if err := m.Leave(ctx, peers[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RoomAccount(ctx, peers[0].AccountID, room.id, 1); err == nil {
		t.Fatal("departed seat identity retained")
	}
	if _, err := m.RoomAccount(ctx, peers[1].AccountID, room.id, 0); err == nil {
		t.Fatal("departed participant retained lookup access")
	}
	if err := m.Join(ctx, outsider, room.code); err != nil {
		t.Fatal(err)
	}
	if got, err := m.RoomAccount(ctx, peers[0].AccountID, room.id, 1); err != nil || got != outsider.AccountID {
		t.Fatal("seat replacement stale identity", got, err)
	}
}

func TestTextFinalEnforcementClosesCurrentPeerBeforeFailedCleanup(t *testing.T) {
	m, values, _, settings := textManagerFixture(t)
	ctx := context.Background()
	p := textPeer(t, m)
	if err := m.QueueJoin(ctx, p, settings); err != nil {
		t.Fatal(err)
	}
	check := func(_ context.Context, id string) (bool, error) {
		if id != p.AccountID {
			t.Fatal("wrong account")
		}
		return false, nil
	}
	if err := m.EnforceAccount(ctx, p.AccountID, check); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.Done:
		t.Fatal("expired/reversed sanction closed new session")
	default:
	}
	values.failCancel = true
	check = func(context.Context, string) (bool, error) { return true, nil }
	if err := m.EnforceAccount(ctx, p.AccountID, check); err == nil {
		t.Fatal("cleanup failure hidden")
	}
	select {
	case <-p.Done:
	default:
		t.Fatal("failed cleanup retained live socket")
	}
	if err := m.Send(p, "test", "", map[string]string{}); err == nil {
		t.Fatal("enforced peer still receives")
	}
	values.failCancel = false
	if err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(values.reservations) != 0 {
		t.Fatal("enforced reservation lost cleanup retry")
	}
	if err := m.EnforceAccount(ctx, p.AccountID, check); err != nil {
		t.Fatal("repeated enforcement", err)
	}
}
func TestTextManagerClosedAvailabilityAndExactFIFO(t *testing.T) {
	m, values, now, s := textManagerFixture(t)
	ctx := context.Background()
	a := textPeer(t, m)
	if e := m.QueueJoin(ctx, a, s); e != nil {
		t.Fatal(e)
	}
	first := m.queues[a.AccountID].joined
	*now = now.Add(time.Minute)
	if e := m.KeepWaiting(ctx, a); e != nil {
		t.Fatal(e)
	}
	if !m.queues[a.AccountID].joined.Equal(first) {
		t.Fatal("keep waiting lost FIFO")
	}
	other := s
	other.ModeID = gamecontract.ModeTopThat
	b := textPeer(t, m)
	if e := m.QueueJoin(ctx, b, other); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		p := textPeer(t, m)
		if e := m.QueueJoin(ctx, p, s); e != nil {
			t.Fatal(e)
		}
	}
	if m.members[a.AccountID] == nil || m.members[b.AccountID] != nil || len(m.queues) != 1 {
		t.Fatal("cross-tuple or missing assignment")
	}
	if values.starts != 0 {
		t.Fatal("assigned table auto-started without Ready")
	}
	m.deps.Prototype = nil
	if got := m.Availability(ctx); got.Modes[0].Available {
		t.Fatal("closed mode advertised")
	}
	c := textPeer(t, m)
	if e := m.QueueJoin(ctx, c, s); e == nil {
		t.Fatal("unpublished admission")
	}
}
func TestTextManagerReadyRevisionsHostTransferAndStart(t *testing.T) {
	m, values, _, s := textManagerFixture(t)
	ctx := context.Background()
	peers := []*TextPeer{textPeer(t, m), textPeer(t, m), textPeer(t, m), textPeer(t, m)}
	code, e := m.Create(ctx, peers[0], s)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range peers[1:] {
		if e = m.Join(ctx, p, code); e != nil {
			t.Fatal(e)
		}
	}
	r := m.members[peers[0].AccountID]
	for _, p := range peers {
		if e = m.Ready(ctx, p, v2.ReadyAcknowledgement{SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision}); e != nil {
			t.Fatal(e)
		}
	}
	if e = m.Settings(ctx, peers[1], r.settingsRevision, s); e == nil {
		t.Fatal("nonhost settings")
	}
	changed := s
	changed.ModeID = gamecontract.ModeTopThat
	if e = m.Settings(ctx, peers[0], r.settingsRevision, changed); e != nil {
		t.Fatal(e)
	}
	if e = m.Start(ctx, peers[0]); e == nil {
		t.Fatal("stale Ready accepted")
	}
	for _, p := range peers {
		if e = m.Ready(ctx, p, v2.ReadyAcknowledgement{SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision}); e != nil {
			t.Fatal(e)
		}
	}
	if e = m.Start(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	if values.starts != 1 || r.match == nil {
		t.Fatal("missing durable start")
	}
	contract := r.match.Contract()
	if contract.Eligibility.Rewards || contract.Eligibility.Leaderboard || contract.ModeID != changed.ModeID {
		t.Fatal("prototype contract wrong")
	}
	old := peers[1]
	replacement, e := m.Open(ctx, old.AccountID)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Join(ctx, replacement, code); e != nil {
		t.Fatal(e)
	}
	m.Disconnect(ctx, old)
	if r.seats[1].peer != replacement {
		t.Fatal("stale socket evicted reconnect")
	}
}
func TestTextManagerFailureReleasesAndDoesNotPublishMatch(t *testing.T) {
	m, values, _, s := textManagerFixture(t)
	ctx := context.Background()
	peers := []*TextPeer{textPeer(t, m), textPeer(t, m), textPeer(t, m), textPeer(t, m)}
	code, e := m.Create(ctx, peers[0], s)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range peers[1:] {
		if e = m.Join(ctx, p, code); e != nil {
			t.Fatal(e)
		}
	}
	r := m.members[peers[0].AccountID]
	for _, p := range peers {
		if e = m.Ready(ctx, p, v2.ReadyAcknowledgement{SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision}); e != nil {
			t.Fatal(e)
		}
	}
	values.failStart = true
	if e = m.Start(ctx, peers[0]); e == nil {
		t.Fatal("failed durable start accepted")
	}
	if r.match != nil || len(values.reservations) != 0 {
		t.Fatal("failed start leaked match/admissions")
	}
}

func TestTextManagerDownsizePreservesMembersAndHost(t *testing.T) {
	m, _, _, s := textManagerFixture(t)
	ctx := context.Background()
	s.Size = 6
	peers := make([]*TextPeer, 6)
	for i := range peers {
		peers[i] = textPeer(t, m)
	}
	code, e := m.Create(ctx, peers[0], s)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range peers[1:] {
		if e = m.Join(ctx, p, code); e != nil {
			t.Fatal(e)
		}
	}
	if e = m.Leave(ctx, peers[1]); e != nil {
		t.Fatal(e)
	}
	if e = m.Leave(ctx, peers[2]); e != nil {
		t.Fatal(e)
	}
	r := m.members[peers[0].AccountID]
	s.Size = 4
	if e = m.Settings(ctx, peers[0], r.settingsRevision, s); e != nil {
		t.Fatal(e)
	}
	if len(r.seats) != 4 {
		t.Fatal("downsize evicted members")
	}
	for seat := 0; seat < 4; seat++ {
		if r.seats[seat] == nil {
			t.Fatal("downsize left seat outside new bounds")
		}
	}
	if r.seats[r.host].account != peers[0].AccountID {
		t.Fatal("downsize changed host")
	}
}
func TestTextManagerDisconnectFailureKeepsRetryableClosedGeneration(t *testing.T) {
	m, values, _, s := textManagerFixture(t)
	ctx := context.Background()
	p := textPeer(t, m)
	if e := m.QueueJoin(ctx, p, s); e != nil {
		t.Fatal(e)
	}
	values.failCancel = true
	if e := m.Disconnect(ctx, p); e == nil {
		t.Fatal("failed release swallowed")
	}
	select {
	case <-p.Done:
	default:
		t.Fatal("dead connection left active after database failure")
	}
	values.failCancel = false
	if e := m.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if len(m.queues) != 0 || len(values.reservations) != 0 {
		t.Fatal("cleanup retry leaked reservation")
	}
}
func textReadyRoom(t *testing.T, m *TextManager, s v2.LobbySettings) ([]*TextPeer, *textRoom) {
	t.Helper()
	ctx := context.Background()
	peers := make([]*TextPeer, s.Size)
	for i := range peers {
		peers[i] = textPeer(t, m)
	}
	code, e := m.Create(ctx, peers[0], s)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range peers[1:] {
		if e = m.Join(ctx, p, code); e != nil {
			t.Fatal(e)
		}
	}
	r := m.members[peers[0].AccountID]
	for _, p := range peers {
		if e = m.Ready(ctx, p, v2.ReadyAcknowledgement{SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision}); e != nil {
			t.Fatal(e)
		}
	}
	return peers, r
}
func TestTextManagerPreparedCleanupRetainsFenceUntilRetry(t *testing.T) {
	m, values, _, s := textManagerFixture(t)
	ctx := context.Background()
	peers, r := textReadyRoom(t, m, s)
	values.failStart = true
	values.failPreparedCancel = true
	if e := m.Start(ctx, peers[0]); e == nil {
		t.Fatal("failed start/cancel accepted")
	}
	if r.match != nil || len(values.reservations) != 4 {
		t.Fatal("bad failed-start fixture")
	}
	values.failPreparedCancel = false
	if e := m.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if len(values.reservations) != 0 {
		t.Fatal("prepared fence lost; reservations cannot recover")
	}
	values.failStart = false
	for _, p := range peers {
		if e := m.Ready(ctx, p, v2.ReadyAcknowledgement{SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision}); e != nil {
			t.Fatal(e)
		}
	}
	if e := m.Start(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
}
func TestTextManagerDrainRejectsEveryNewReservation(t *testing.T) {
	m, values, _, s := textManagerFixture(t)
	ctx := context.Background()
	host := textPeer(t, m)
	code, e := m.Create(ctx, host, s)
	if e != nil {
		t.Fatal(e)
	}
	r := m.members[host.AccountID]
	changed := s
	changed.ModeID = gamecontract.ModeTopThat
	if e = m.Settings(ctx, host, r.settingsRevision, changed); e != nil {
		t.Fatal(e)
	}
	m.Drain()
	before := len(values.reservations)
	guest := textPeer(t, m)
	if e = m.Join(ctx, guest, code); e == nil {
		t.Fatal("draining room admitted new guest")
	}
	if e = m.Ready(ctx, host, v2.ReadyAcknowledgement{SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision}); e == nil {
		t.Fatal("draining Ready reserved")
	}
	if len(values.reservations) != before {
		t.Fatal("drain reserved admission")
	}
}
func textDrainFrames(p *TextPeer) {
	for {
		select {
		case <-p.Frames:
		default:
			return
		}
	}
}
func textFinishRoom(t *testing.T, m *TextManager, r *textRoom, now *time.Time) {
	t.Helper()
	for step := 0; step < 80; step++ {
		phase, deadline := r.match.Clock()
		if phase == v2.PhaseVerdict {
			return
		}
		*now = deadline
		for _, p := range m.peers {
			textDrainFrames(p)
		}
		if e := m.Tick(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	t.Fatal("clock schedule did not finish")
}
func TestTextManagerQuickRematchKeepsPathHostAndTuple(t *testing.T) {
	m, values, now, s := textManagerFixture(t)
	ctx := context.Background()
	peers := make([]*TextPeer, 4)
	for i := range peers {
		peers[i] = textPeer(t, m)
		if e := m.QueueJoin(ctx, peers[i], s); e != nil {
			t.Fatal(e)
		}
	}
	r := m.members[peers[0].AccountID]
	for _, p := range peers {
		if e := m.Ready(ctx, p, v2.ReadyAcknowledgement{SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision}); e != nil {
			t.Fatal(e)
		}
	}
	if e := m.Start(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	textFinishRoom(t, m, r, now)
	if e := m.Leave(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	if e := m.Rematch(ctx, peers[2]); e != nil {
		t.Fatal(e)
	}
	if r.path != "quick_play" || r.host != 1 || r.match != nil {
		t.Fatal("rematch entry path/lowest original host changed")
	}
	wrong := textPeer(t, m)
	other := s
	other.ModeID = gamecontract.ModeTopThat
	if e := m.QueueJoin(ctx, wrong, other); e != nil {
		t.Fatal(e)
	}
	replacement := textPeer(t, m)
	if e := m.QueueJoin(ctx, replacement, s); e != nil {
		t.Fatal(e)
	}
	if m.members[replacement.AccountID] != r || m.members[wrong.AccountID] != nil || r.host != 1 {
		t.Fatal("replacement changed host or tuple")
	}
	for _, member := range r.seats {
		if member.ready != nil {
			t.Fatal("replacement inherited Ready")
		}
	}
	oldAdmission := r.seats[0].admission
	if e := m.Settings(ctx, peers[1], r.settingsRevision, other); e != nil {
		t.Fatal(e)
	}
	if _, ok := values.reservations[oldAdmission]; ok {
		t.Fatal("old replacement reservation survived settings")
	}
	if r.path != "quick_play" {
		t.Fatal("quick rematch became local")
	}
}
func TestTextManagerSlowConsumerAndSimultaneousAccountGeneration(t *testing.T) {
	m, values, _, s := textManagerFixture(t)
	ctx := context.Background()
	p := textPeer(t, m)
	if e := m.QueueJoin(ctx, p, s); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < cap(p.frames)+1; i++ {
		_ = m.Send(p, "probe", "", struct{}{})
	}
	select {
	case <-p.Done:
	default:
		t.Fatal("unbounded slow consumer")
	}
	if e := m.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if len(values.reservations) != 0 {
		t.Fatal("slow consumer leaked admission")
	}
	account := uuid.NewString()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			peer, e := m.Open(ctx, account)
			if e == nil {
				_ = m.QueueJoin(ctx, peer, s)
			}
		}()
	}
	wg.Wait()
	if len(values.reservations) > 1 || len(m.queues) > 1 {
		t.Fatal("one account obtained multiple reservations")
	}
	m.mu.Lock()
	current := m.peers[account]
	m.mu.Unlock()
	if current == nil || current.closed.Load() {
		t.Fatal("latest generation lost")
	}
}
func TestTextManagerAvailabilityDoesNotBlockLiveMembership(t *testing.T) {
	m, _, _, _ := textManagerFixture(t)
	m.deps.Prototype = nil
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	m.deps.Resolve = func(ctx context.Context, language, rules string) (*media.TextSnapshot, error) {
		once.Do(func() { close(entered) })
		<-release
		return nil, ErrTextUnavailable
	}
	done := make(chan struct{})
	go func() { m.Availability(context.Background()); close(done) }()
	<-entered
	opened := make(chan struct{})
	go func() { _, _ = m.Open(context.Background(), uuid.NewString()); close(opened) }()
	blocked := false
	select {
	case <-opened:
	case <-time.After(100 * time.Millisecond):
		blocked = true
	}
	close(release)
	<-done
	<-opened
	if blocked {
		t.Fatal("catalog I/O held global membership lock")
	}
}

type textAuthorityStub struct {
	done chan struct{}
	lost bool
}

func (a *textAuthorityStub) Check(context.Context) error {
	if a.lost {
		return errors.New("lease lost")
	}
	return nil
}
func (a *textAuthorityStub) Done() <-chan struct{} { return a.done }
func TestTextManagerLostOwnerStopsAdmissionAndPrivateOutput(t *testing.T) {
	m, values, _, s := textManagerFixture(t)
	ctx := context.Background()
	peers, r := textReadyRoom(t, m, s)
	authority := &textAuthorityStub{done: make(chan struct{})}
	m.deps.Authority = authority
	if e := m.Start(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	authority.lost = true
	close(authority.done)
	for _, p := range peers {
		textDrainFrames(p)
	}
	if e := m.Resync(ctx, peers[0]); e == nil {
		t.Fatal("lost owner emitted private snapshot")
	}
	if e := m.Tick(ctx); e == nil {
		t.Fatal("lost owner continued clock")
	}
	select {
	case <-peers[0].Done:
	default:
		t.Fatal("lost owner left peer active")
	}
	if values.interrupts != 0 {
		t.Fatal("lost owner tried to compensate itself")
	}
	if r.match == nil {
		t.Fatal("lost owner fabricated replacement state")
	}
}

func TestTextManagerAuthoredHistorySurvivesVerdictDeparture(t *testing.T) {
	m, _, now, settings := textManagerFixture(t)
	m.deps.ModerateChat = func(_ context.Context, _ string, a v2.Action) (v2.Action, error) { return a, nil }
	author := ""
	blocked := false
	m.deps.HideChat = func(_ context.Context, _ string, id string) (bool, error) { return blocked && id == author, nil }
	ctx := context.Background()
	peers := make([]*TextPeer, 4)
	for i := range peers {
		peers[i] = textPeer(t, m)
		if e := m.QueueJoin(ctx, peers[i], settings); e != nil {
			t.Fatal(e)
		}
	}
	r := m.members[peers[0].AccountID]
	for _, p := range peers {
		if e := m.Ready(ctx, p, v2.ReadyAcknowledgement{SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision}); e != nil {
			t.Fatal(e)
		}
	}
	if e := m.Start(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	_, deadline := r.match.Clock()
	*now = deadline
	if e := m.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	s, e := r.match.Snapshot(0)
	if e != nil {
		t.Fatal(e)
	}
	author = peers[0].AccountID
	req := v2.ActionRequest{Version: 2, RequestID: "historical-chat", ExpectedBoardRevision: s.Board.Revision, MatchID: s.Contract.MatchID, ModeID: s.Contract.ModeID, Round: s.Round, Turn: s.Turn, Phase: s.Phase, PhaseID: s.PhaseID, Action: v2.Action{Kind: v2.ActionChat, Text: "historical words", UILocale: "en"}}
	if e := m.Action(ctx, peers[0], req); e != nil {
		t.Fatal(e)
	}
	projectChat := func() v2.PublicAction {
		textDrainFrames(peers[1])
		if e := m.Resync(ctx, peers[1]); e != nil {
			t.Fatal(e)
		}
		for len(peers[1].frames) > 0 {
			f := <-peers[1].frames
			if f.Type != "snapshot" {
				continue
			}
			var snapshot v2.Snapshot
			if e := json.Unmarshal(f.Payload, &snapshot); e != nil {
				t.Fatal(e)
			}
			for _, event := range snapshot.History {
				if event.Kind == "chat" {
					return event
				}
			}
		}
		t.Fatal("authored history missing")
		return v2.PublicAction{}
	}
	visible := projectChat()
	if visible.Text != "historical words" || visible.PhraseID != "" {
		t.Fatal("authored text missing before block")
	}
	blocked = true
	hidden := projectChat()
	if hidden.Text != "" || hidden.PhraseID != "chat.hidden" {
		t.Fatal("block did not redact authored text")
	}
	hidden.Text, hidden.PhraseID = visible.Text, visible.PhraseID
	if string(mustTextJSON(hidden)) != string(mustTextJSON(visible)) {
		t.Fatal("redaction changed immutable event fields")
	}
	blocked = false
	restored := projectChat()
	if string(mustTextJSON(restored)) != string(mustTextJSON(visible)) {
		t.Fatal("unblock changed original authored bytes or evidence")
	}
	blocked = true
	textFinishRoom(t, m, r, now)
	if e := m.Leave(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	textDrainFrames(peers[1])
	if e := m.Resync(ctx, peers[1]); e != nil {
		t.Fatal("departed author broke required history", e)
	}
	found := false
	for len(peers[1].frames) > 0 {
		f := <-peers[1].frames
		if f.Type != "snapshot" {
			continue
		}
		var snap v2.Snapshot
		if e := json.Unmarshal(f.Payload, &snap); e != nil {
			t.Fatal(e)
		}
		for _, event := range snap.History {
			if event.Kind == "chat" {
				found = true
				if event.Text != "" || event.PhraseID != "chat.hidden" {
					t.Fatal("blocked authored text reappeared")
				}
			}
		}
	}
	if !found {
		t.Fatal("historical chat sequence lost")
	}
}

func TestTextManagerRateBudgetSurvivesReconnect(t *testing.T) {
	m, _, now, _ := textManagerFixture(t)
	m.deps.Config.RateLimit.Enabled = true
	m.deps.Config.RateLimit.MaxIntentsPerSecond = 2
	m.deps.Config.RateLimit.MaxIntentsBurst = 2
	p := textPeer(t, m)
	if !m.AllowRequest(t.Context(), p) || !m.AllowRequest(t.Context(), p) || m.AllowRequest(t.Context(), p) {
		t.Fatal("wrong initial account budget")
	}
	replacement, e := m.Open(context.Background(), p.AccountID)
	if e != nil {
		t.Fatal(e)
	}
	if m.AllowRequest(t.Context(), p) || m.AllowRequest(t.Context(), replacement) {
		t.Fatal("reconnect refreshed budget or old socket accepted")
	}
	*now = now.Add(500 * time.Millisecond)
	if !m.AllowRequest(t.Context(), replacement) || m.AllowRequest(t.Context(), replacement) {
		t.Fatal("budget refill wrong")
	}
	if e := m.Disconnect(context.Background(), replacement); e != nil {
		t.Fatal(e)
	}
	*now = now.Add(2 * time.Second)
	if e := m.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(m.requestRates) != 0 {
		t.Fatal("idle account budget leaked")
	}
}

type fakeTextDeliveries struct {
	rows     []store.TextDelivery
	accounts []string
	claims   int
	guarded  int
	guardErr error
}

func (f *fakeTextDeliveries) WithTextDeliveryEnqueue(_ context.Context, _ string, _ store.TextDelivery, enqueue func() error) error {
	f.guarded++
	if f.guardErr != nil {
		return f.guardErr
	}
	return enqueue()
}

func TestTextManagerPrivateDeliveryRequiresFinalEnqueueGuard(t *testing.T) {
	m, _, _, _ := textManagerFixture(t)
	p := textPeer(t, m)
	textDrainFrames(p)
	source := &fakeTextDeliveries{rows: []store.TextDelivery{{ID: 42, AccountID: p.AccountID, MatchID: "closed-match"}}, guardErr: store.ErrValueFence}
	if err := m.PumpDeliveries(t.Context(), source); !errors.Is(err, store.ErrValueFence) {
		t.Fatal("missing final delivery guard", err)
	}
	if source.guarded != 1 || len(p.frames) != 0 {
		t.Fatal("fenced private delivery enqueued", source.guarded, len(p.frames))
	}
}

func (f *fakeTextDeliveries) RecoverPending(context.Context, int) (int, error) { return 0, nil }
func (f *fakeTextDeliveries) ClaimAccountDeliveries(_ context.Context, _ string, accounts []string, _ time.Time, _ time.Duration, _ int) ([]store.TextDelivery, error) {
	f.accounts = append([]string(nil), accounts...)
	f.claims++
	return f.rows, nil
}
func TestTextManagerPrivateDeliveryWaitsForAuthoritativeTerminal(t *testing.T) {
	m, _, _, settings := textManagerFixture(t)
	ctx := context.Background()
	peers, r := textReadyRoom(t, m, settings)
	if e := m.Start(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	for _, p := range peers {
		textDrainFrames(p)
	}
	source := &fakeTextDeliveries{rows: []store.TextDelivery{{ID: 42, AccountID: peers[0].AccountID, MatchID: r.match.Contract().MatchID}}}
	if e := m.PumpDeliveries(ctx, source); e != nil {
		t.Fatal(e)
	}
	for _, p := range peers {
		if len(p.frames) > 0 {
			t.Fatal("durable terminal receipt exposed before authoritative verdict")
		}
	}
	if _, e := r.match.Close(ctx); e != nil {
		t.Fatal(e)
	}
	if e := m.PumpDeliveries(ctx, source); e != nil {
		t.Fatal(e)
	}
	if len(peers[0].frames) != 1 {
		t.Fatal("terminal private receipt missing")
	}
	f := <-peers[0].frames
	if f.Type != "settlement" {
		t.Fatal("wrong private delivery")
	}
	for _, p := range peers[1:] {
		if len(p.frames) != 0 {
			t.Fatal("settlement broadcast to other seat")
		}
	}
	if len(source.accounts) != 4 {
		t.Fatal("connected account claim filter missing")
	}
}

func TestTextManagerInstantVoteCreditIsInvisibleUntilMatchEnd(t *testing.T) {
	m, values, now, settings := textManagerFixture(t)
	ctx := context.Background()
	peers, r := textReadyRoom(t, m, settings)
	if e := m.Start(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	for step := 0; step < 20; step++ {
		phase, deadline := r.match.Clock()
		if phase == v2.PhaseKnowoff {
			break
		}
		*now = deadline
		if e := m.Tick(ctx); e != nil {
			t.Fatal(e)
		}
	}
	donower := -1
	for i := range peers {
		s, e := r.match.Snapshot(i)
		if e != nil {
			t.Fatal(e)
		}
		if s.Private.Role == "donower" {
			donower = i
		}
		textDrainFrames(peers[i])
	}
	if donower < 0 {
		t.Fatal("fixture has no donower")
	}
	apply := func(seat int, id string, action v2.Action) {
		t.Helper()
		s, e := r.match.Snapshot(seat)
		if e != nil {
			t.Fatal(e)
		}
		if e = m.Action(ctx, peers[seat], v2.ActionRequest{Version: 2, RequestID: id, MatchID: s.Contract.MatchID, ModeID: s.Contract.ModeID, Round: s.Round, Turn: s.Turn, Phase: s.Phase, PhaseID: s.PhaseID, ExpectedBoardRevision: s.Board.Revision, Action: action}); e != nil {
			t.Fatal(e)
		}
	}
	for i := range peers {
		target := donower
		if i == donower {
			target = (i + 1) % 4
		}
		apply(i, "vote", v2.Action{Kind: v2.ActionVote, TargetSeat: &target})
	}
	for i := range peers {
		apply(i, "ready", v2.Action{Kind: v2.ActionReady})
	}
	phase, _ := r.match.Clock()
	if phase != v2.PhaseResult || values.awards != 3 {
		t.Fatal("vote did not commit immediate awards before terminal", phase, values.awards)
	}
	if e := m.Resync(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	for _, p := range peers {
		for len(p.frames) > 0 {
			frame := <-p.frames
			if frame.Type == "award" || frame.Type == "settlement" {
				t.Fatal("role-linked receipt exposed before match end")
			}
		}
	}
}

func TestTextManagerExplicitLeaveReleasesMembershipAtTerminal(t *testing.T) {
	m, _, now, s := textManagerFixture(t)
	ctx := context.Background()
	peers, r := textReadyRoom(t, m, s)
	if e := m.Start(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	if e := m.Leave(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	if e := m.QueueJoin(ctx, peers[0], s); e == nil {
		t.Fatal("active departed seat entered a second match")
	}
	textFinishRoom(t, m, r, now)
	if m.members[peers[0].AccountID] != nil {
		t.Fatal("completed departed match still owns account membership")
	}
	if e := m.QueueJoin(ctx, peers[0], s); e != nil {
		t.Fatal("explicit leave required invisible second room_leave after terminal", e)
	}
}

func TestTextManagerUnavailableDiscoveryHasNoAdmissibleTuples(t *testing.T) {
	for _, lost := range []bool{false, true} {
		m, _, _, _ := textManagerFixture(t)
		if lost {
			authority := &textAuthorityStub{done: make(chan struct{})}
			m.deps.Authority = authority
			authority.lost = true
			close(authority.done)
		} else {
			m.Drain()
		}
		for _, mode := range m.Availability(context.Background()).Modes {
			if mode.Available || len(mode.Languages) != 0 {
				t.Fatal("closed discovery still advertises admissible tuples", lost, mode.ModeID)
			}
		}
	}
}

func TestTextReportAuthorizationUsesOnlyRecipientVisibleContent(t *testing.T) {
	for _, mode := range []gamecontract.ModeID{gamecontract.ModeMissedTheBriefing, gamecontract.ModeSecretScale, gamecontract.ModeMakeRoom, gamecontract.ModeBadBargains, gamecontract.ModeTopThat} {
		for _, size := range []int{4, 6} {
			t.Run(string(mode)+fmt.Sprint(size), func(t *testing.T) {
				m, _, now, settings := textManagerFixture(t)
				settings.ModeID = mode
				settings.Size = size
				ctx := context.Background()
				peers, room := textReadyRoom(t, m, settings)
				if err := m.Start(ctx, peers[0]); err != nil {
					t.Fatal(err)
				}
				snapshots := make([]v2.Snapshot, size)
				nower, donower := -1, -1
				for seat := range peers {
					s, e := room.match.Snapshot(seat)
					if e != nil {
						t.Fatal(e)
					}
					snapshots[seat] = s
					if s.Private.Nown != nil {
						nower = seat
					} else {
						donower = seat
					}
				}
				if nower < 0 || donower < 0 {
					t.Fatal("role fixture")
				}
				own := snapshots[donower].Private.Hand[0].Content
				match := snapshots[0].Contract.MatchID
				if contract, got, err := m.VisibleText(ctx, peers[donower].AccountID, match, own.ContentRef); err != nil || got != own || contract != snapshots[0].Contract {
					t.Fatal("own content unavailable", err)
				}
				nown := *snapshots[nower].Private.Nown
				if _, got, err := m.VisibleText(ctx, peers[nower].AccountID, match, nown.ContentRef); err != nil || got != nown {
					t.Fatal("authorized Nown unavailable", err)
				}
				for _, ref := range []v2.ContentRef{nown.ContentRef, {ContentID: own.ContentID, Revision: own.Revision + 1}, {ContentID: "unknown", Revision: 1}} {
					if _, _, err := m.VisibleText(ctx, peers[donower].AccountID, match, ref); err == nil {
						t.Fatal("hidden/wrong revision content accepted", ref)
					}
				}
				outsider := textPeer(t, m)
				if _, _, err := m.VisibleText(ctx, outsider.AccountID, match, own.ContentRef); err == nil {
					t.Fatal("outsider report access")
				}
				if _, _, err := m.VisibleText(ctx, peers[donower].AccountID, uuid.NewString(), own.ContentRef); err == nil {
					t.Fatal("wrong match report access")
				}
				after, err := room.match.Snapshot(donower)
				if err != nil || after.Cursor.RecipientSeq != snapshots[donower].Cursor.RecipientSeq+1 {
					t.Fatal("report lookup advanced socket stream", err)
				}
				for _, card := range snapshots[donower].Board.Cards {
					if _, got, err := m.VisibleText(ctx, peers[donower].AccountID, match, card.Card.Content.ContentRef); err != nil || got != card.Card.Content {
						t.Fatal("public card unavailable", err)
					}
				}
				textFinishRoom(t, m, room, now)
				// Only begun prompts become public at verdict. The initial prompt is one.
				if _, got, err := m.VisibleText(ctx, peers[donower].AccountID, match, nown.ContentRef); err != nil || got != nown {
					t.Fatal("public verdict Nown unavailable", err)
				}
				if err := m.Leave(ctx, peers[donower]); err != nil {
					t.Fatal(err)
				}
				if _, _, err := m.VisibleText(ctx, peers[donower].AccountID, match, nown.ContentRef); err == nil {
					t.Fatal("former member retained private lookup")
				}
			})
		}
	}
}

func TestTextActiveRejoinBindsRoomBeforePrivateSnapshot(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				m, _, _, settings := textManagerFixture(t)
				settings.ModeID, settings.Size = mode, size
				ctx := t.Context()
				peers, room := textReadyRoom(t, m, settings)
				if err := m.Start(ctx, peers[0]); err != nil {
					t.Fatal(err)
				}
				seat := size - 1
				if err := m.Disconnect(ctx, peers[0]); err != nil {
					t.Fatal(err)
				}
				fresh, err := m.Open(ctx, peers[seat].AccountID)
				if err != nil {
					t.Fatal(err)
				}
				if err = m.Join(ctx, fresh, room.code); err != nil {
					t.Fatal(err)
				}
				frames := []TextEnvelope{}
				for len(fresh.frames) > 0 {
					frames = append(frames, <-fresh.frames)
				}
				if len(frames) < 3 || frames[0].Type != "lobby" || frames[1].Type != "dev_role" || frames[2].Type != "snapshot" {
					t.Fatalf("fresh rejoin must bind room before snapshot: %v", func() []string {
						kinds := []string{}
						for _, f := range frames {
							kinds = append(kinds, f.Type)
						}
						return kinds
					}())
				}
				var binding TextLobbyView
				if err = json.Unmarshal(frames[0].Payload, &binding); err != nil {
					t.Fatal(err)
				}
				var snapshot v2.Snapshot
				if err = json.Unmarshal(frames[2].Payload, &snapshot); err != nil {
					t.Fatal(err)
				}
				if binding.Seat != seat || binding.Code != room.code || binding.Lobby.RoomID != room.id || snapshot.Contract.RoomID != room.id || snapshot.Private.Seat != seat {
					t.Fatal("rejoin changed authorized room or seat")
				}
				if room.host != 0 || binding.Lobby.HostSeat != 1 {
					t.Fatal("binding transferred authoritative host or advertised disconnected host")
				}
				for _, member := range binding.Lobby.Seats {
					if member.Ready != nil || member.Connected != (member.Seat != 0) {
						t.Fatal("active binding retained prematch Ready or fabricated connectivity")
					}
				}
				outsider := textPeer(t, m)
				if err = m.Join(ctx, outsider, room.code); !errors.Is(err, ErrTextMembership) || len(outsider.frames) != 0 {
					t.Fatal("nonmember received active room binding", err)
				}
			})
		}
	}
}
