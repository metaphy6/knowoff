package lobby

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	_ "github.com/lib/pq"
)

func textAdapterDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn, token := os.Getenv("KNOWOFF_TEST_DSN"), os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	u, e := url.Parse(dsn)
	valid := e == nil && len(token) == 12 && strings.Trim(token, "0123456789abcdef") == "" && (u.Scheme == "postgres" || u.Scheme == "postgresql") && u.Path == "/knowoff_test_"+token && u.Fragment == ""
	if valid {
		host := u.Hostname()
		valid = host == "postgres" || host == "localhost" || host == "127.0.0.1"
		q, err := url.ParseQuery(u.RawQuery)
		valid = valid && err == nil
		for key := range q {
			valid = valid && key == "sslmode"
		}
	}
	if !valid {
		t.Fatal("adapter proofs require the runner's unique disposable PostgreSQL and token")
	}
	db, e := sql.Open("postgres", dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var name string
	if e = db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); e != nil || name != "knowoff_test_"+token {
		t.Fatal("disposable database identity mismatch")
	}
	if _, e = db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); e != nil {
		t.Fatal(e)
	}
	if e = store.MigrateUp(db, "../../migrations"); e != nil {
		t.Fatal(e)
	}
	return db
}
func textAdapterRoom(t *testing.T, db *sql.DB) (*TextManager, []*TextPeer, *textRoom, *time.Time) {
	t.Helper()
	m, _, now, s := textManagerFixture(t)
	m.deps.Values = store.NewTextValueStore(db, m.deps.Config.Tuning)
	ctx := context.Background()
	peers := make([]*TextPeer, 4)
	for i := range peers {
		id := uuid.NewString()
		if _, e := db.ExecContext(ctx, `INSERT INTO accounts(id,nickname) VALUES($1,$2)`, id, id); e != nil {
			t.Fatal(e)
		}
		if _, e := db.ExecContext(ctx, `INSERT INTO profiles(account_id) VALUES($1)`, id); e != nil {
			t.Fatal(e)
		}
		p, e := m.Open(ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		peers[i] = p
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
	if e = m.Start(ctx, peers[0]); e != nil {
		t.Fatal(e)
	}
	return m, peers, r, now
}
func TestTextValueAdapterRealLowPopulationAndInterruption(t *testing.T) {
	db := textAdapterDB(t)
	ctx := context.Background()
	m, peers, r, now := textAdapterRoom(t, db)
	retained := map[string]bool{}
	for seat, p := range peers {
		s, e := r.match.Snapshot(seat)
		if e != nil {
			t.Fatal(e)
		}
		if !retained[s.Private.Role] {
			retained[s.Private.Role] = true
			continue
		}
		if e = m.Disconnect(ctx, p); e != nil {
			t.Fatal(e)
		}
	}
	*now = now.Add(time.Duration(m.deps.Config.Tuning.Game.ReconnectGraceS+1) * time.Second)
	if e := m.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	id := r.match.Contract().MatchID
	var state, winner string
	if e := db.QueryRowContext(ctx, `SELECT state,COALESCE(outcome->>'Winner','') FROM text_matches WHERE id=$1`, id).Scan(&state, &winner); e != nil {
		t.Fatal(e)
	}
	if state != "scored_low_population" || winner != "" {
		t.Fatalf("low population adapter: %s %q", state, winner)
	}
	var settlements int
	if e := db.QueryRowContext(ctx, `SELECT count(*) FROM text_settlements WHERE match_id=$1 AND state='applied'`, id).Scan(&settlements); e != nil || settlements != 4 {
		t.Fatalf("low population settlement %d %v", settlements, e)
	}
	next, _, room, _ := textAdapterRoom(t, db)
	lostID := room.match.Contract().MatchID
	for i := 0; i < 2; i++ {
		if e := next.Close(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if e := db.QueryRowContext(ctx, `SELECT state FROM text_matches WHERE id=$1`, lostID).Scan(&state); e != nil || state != "interrupted" {
		t.Fatalf("close adapter: %s %v", state, e)
	}
	if e := db.QueryRowContext(ctx, `SELECT count(*) FROM text_settlements WHERE match_id=$1 AND state='applied' AND effects->>'interrupted'='true' AND (effects->>'points')::bigint=0 AND (effects->>'xp')::bigint=0 AND effects->>'leaderboard_counted'='false' AND effects->'awards'='[]'::jsonb`, lostID).Scan(&settlements); e != nil || settlements != 4 {
		t.Fatalf("interruption must deliver exactly four private zero-effect receipts: %d %v", settlements, e)
	}
	var grants, deliveries int
	if e := db.QueryRowContext(ctx, `SELECT count(*) FROM text_award_receipts WHERE match_id=$1`, lostID).Scan(&grants); e != nil || grants != 0 {
		t.Fatal("interruption fabricated grants", e)
	}
	if e := db.QueryRowContext(ctx, `SELECT count(*) FROM text_outbox WHERE match_id=$1`, lostID).Scan(&deliveries); e != nil || deliveries != 4 {
		t.Fatal("interruption delivery missing or duplicated", e)
	}
}

func TestTextWalletCancelledRequestPreservesLiveOwner(t *testing.T) {
	db := textAdapterDB(t)
	ctx := t.Context()
	owner, err := store.AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = owner.Release(context.Background())
	})
	m, _, _, settings := textManagerFixture(t)
	m.deps.Authority = owner
	peers, _ := textReadyRoom(t, m, settings)
	if err = m.Start(ctx, peers[0]); err != nil {
		t.Fatal(err)
	}
	request, cancel := context.WithCancel(ctx)
	cancel()
	if err = m.WithWalletAccess(request, peers[0].AccountID, func() error { t.Fatal("cancelled wallet callback ran"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled wallet", err)
	}
	if err = owner.Check(ctx); err != nil {
		t.Fatal("wallet cancellation lost PG owner", err)
	}
	if m.lost.Load() {
		t.Fatal("wallet cancellation lost runtime")
	}
	for _, peer := range peers {
		select {
		case <-peer.Done:
			t.Fatal("wallet cancellation closed active peer")
		default:
		}
	}
	if err = m.WithWalletAccess(ctx, peers[0].AccountID, func() error { t.Fatal("live wallet exposed"); return nil }); !errors.Is(err, ErrTextWalletHidden) {
		t.Fatal(err)
	}
}

func TestTextAvailabilityCancelledRequestPreservesLiveOwner(t *testing.T) {
	db := textAdapterDB(t)
	ctx := t.Context()
	owner, err := store.AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = owner.Release(context.Background())
	})
	m, _, _, settings := textManagerFixture(t)
	m.deps.Authority = owner
	peers, _ := textReadyRoom(t, m, settings)
	if err = m.Start(ctx, peers[0]); err != nil {
		t.Fatal(err)
	}
	request, cancel := context.WithCancel(ctx)
	cancel()
	unavailable := m.Availability(request)
	for _, mode := range unavailable.Modes {
		if mode.Available {
			t.Fatal("cancelled availability advertised admission")
		}
	}
	if err = owner.Check(ctx); err != nil {
		t.Fatal("availability cancellation lost PG owner", err)
	}
	if m.lost.Load() {
		t.Fatal("availability cancellation lost runtime")
	}
	for _, peer := range peers {
		select {
		case <-peer.Done:
			t.Fatal("availability cancellation closed active peer")
		default:
		}
	}
	for _, mode := range m.Availability(ctx).Modes {
		if !mode.Available {
			t.Fatal("healthy availability did not recover", mode.ModeID)
		}
	}
	if err = owner.Release(ctx); err != nil {
		t.Fatal(err)
	}
	for _, mode := range m.Availability(ctx).Modes {
		if mode.Available {
			t.Fatal("lost authority advertised admission")
		}
	}
	if !m.lost.Load() {
		t.Fatal("actual authority loss did not fence runtime")
	}
	for _, peer := range peers {
		select {
		case <-peer.Done:
		default:
			t.Fatal("actual authority loss left active peer open")
		}
	}
}
