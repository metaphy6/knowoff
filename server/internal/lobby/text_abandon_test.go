package lobby

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
)

type lostAbandonResponse struct {
	*store.TextValueStore
	fail   bool
	events []store.TextAbandon
}

func (v *lostAbandonResponse) Abandon(ctx context.Context, a store.TextAbandon) error {
	v.events = append(v.events, a)
	if err := v.TextValueStore.Abandon(ctx, a); err != nil {
		return err
	}
	if v.fail {
		v.fail = false
		return errors.New("receipt response lost")
	}
	return nil
}

// This is an engine/durable-adapter fixture, not a published content release or
// production admission proof. Synthetic text never enters the discovery API.
func TestTextValueAdapterRealAbandonRetryAndTerminal(t *testing.T) {
	db := textAdapterDB(t)
	ctx := context.Background()
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				manager, _, clock, _ := textManagerFixture(t)
				cfg := manager.deps.Config
				policy := cfg.Tuning
				values := &lostAbandonResponse{TextValueStore: store.NewTextValueStore(db, policy), fail: true}
				catalog := manager.deps.Prototype
				deal, err := catalog.Deal(mode, size, media.TextDealTuning{HandSize: policy.Hand.Size, ReserveSize: policy.Hand.DrawPile, MinHigh: policy.Dealing.MinHighPerNown, MinDistant: policy.Dealing.MinDistantPerNown, MaxSearchNodes: policy.TextCatalog.MaxSearchNodes}, media.TextRandomness{Schedule: 1, Hands: 2, System: 3})
				if err != nil {
					t.Fatal(err)
				}
				accounts, admissions := make([]string, size), make([]string, size)
				for seat := range accounts {
					accounts[seat] = uuid.NewString()
					admissions[seat] = uuid.NewString()
					if _, err = db.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,$1::uuid::text)`, accounts[seat]); err != nil {
						t.Fatal(err)
					}
					if _, err = db.Exec(`INSERT INTO profiles(account_id) VALUES($1)`, accounts[seat]); err != nil {
						t.Fatal(err)
					}
					if err = values.Reserve(ctx, store.TextReservation{ID: admissions[seat], AccountID: accounts[seat], EntryPath: "quick_play", At: *clock}); err != nil {
						t.Fatal(err)
					}
				}
				hash, err := policy.SHA256()
				if err != nil {
					t.Fatal(err)
				}
				c := v2.MatchContract{ProtocolVersion: 2, MatchID: uuid.NewString(), RoomID: uuid.NewString(), ModeID: mode, OriginalSize: size, RulesVersion: deal.RulesVersion, ContentLanguage: deal.Language, PackReleaseID: deal.ReleaseID, PackSHA256: deal.SnapshotSHA256, Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: hash}, Eligibility: v2.Eligibility{AdmissionID: uuid.NewString(), EntryPath: "quick_play", Rewards: true, Leaderboard: true}}
				owner := uuid.NewString()
				record := store.TextMatchRecord{Contract: c, Owner: owner, Epoch: 1, AdmissionIDs: admissions, Policy: policy}
				if err = values.Prepare(ctx, record, *clock); err != nil {
					t.Fatal(err)
				}
				if err = values.Start(ctx, c.MatchID, owner, 1, *clock); err != nil {
					t.Fatal(err)
				}
				engine, err := game.NewTextMatch(game.TextOptions{Config: cfg, Contract: c, Deal: deal, Seed: 42, Now: func() time.Time { return *clock }, Hooks: TextValueHooks(values, owner, 1, accounts)})
				if err != nil {
					t.Fatal(err)
				}
				target := -1
				for seat := range accounts {
					s, e := engine.Snapshot(seat)
					if e != nil {
						t.Fatal(e)
					}
					if s.Private.Role == "nower" {
						target = seat
						break
					}
				}
				if target < 0 {
					t.Fatal("no Nower")
				}
				if _, err = engine.SetConnected(ctx, target, false); err != nil {
					t.Fatal(err)
				}
				expiry := clock.Add(time.Duration(policy.Game.ReconnectGraceS) * time.Second)
				*clock = expiry
				if _, err = engine.Advance(ctx, *clock); err == nil || err.Error() != "receipt response lost" {
					t.Fatal("missing injected committed receipt failure", err)
				}
				var count int
				var until time.Time
				if err = db.QueryRow(`SELECT abandon_count,cooldown_until FROM queue_cooldowns WHERE account_id=$1`, accounts[target]).Scan(&count, &until); err != nil {
					t.Fatal(err)
				}
				if count != 1 || !until.Equal(expiry.Add(time.Duration(policy.Game.AbandonCooldownsS[0])*time.Second)) {
					t.Fatal("wrong committed cooldown", count, until)
				}
				*clock = clock.Add(time.Hour)
				if _, err = engine.Close(ctx); err != nil {
					t.Fatal(err)
				}
				if len(values.events) != 2 || values.events[0] != values.events[1] {
					t.Fatal("adapter retry identity changed", values.events)
				}
				if err = db.QueryRow(`SELECT count(*) FROM text_abandons WHERE match_id=$1`, c.MatchID).Scan(&count); err != nil || count != 1 {
					t.Fatal("interruption fabricated/repeated abandonment", count, err)
				}
				if err = db.QueryRow(`SELECT count(*) FROM text_settlements WHERE match_id=$1 AND state='applied' AND effects->>'interrupted'='true'`, c.MatchID).Scan(&count); err != nil || count != size {
					t.Fatal("interrupted durable end not delivered", count, err)
				}
				if err = values.Reserve(ctx, store.TextReservation{ID: uuid.NewString(), AccountID: accounts[target], EntryPath: "quick_play", At: expiry.Add(time.Second)}); !errors.Is(err, store.ErrTextCooldown) {
					t.Fatal("actual receipt not enforced", err)
				}
			})
		}
	}
}
