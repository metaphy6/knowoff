package store

import (
	"context"
	"errors"
	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestTextBlocksSerializeStartAndStayPrivate(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	trust := NewTextTrustStore(db)
	m, ids := valuePreparedMatch(t, s, db, now, false)
	if err := trust.Block(ctx, ids[0], ids[1], now); err != nil {
		t.Fatal(err)
	}
	if err := trust.Block(ctx, ids[0], ids[1], now); err != nil {
		t.Fatal("retry", err)
	}
	if err := trust.Block(ctx, ids[0], ids[0], now); err == nil {
		t.Fatal("self block")
	}
	if err := trust.CanMatch(ctx, []string{ids[1], ids[0]}, now); !errors.Is(err, ErrTextTrust) {
		t.Fatal("asymmetric", err)
	}
	if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, now); !errors.Is(err, ErrTextTrust) {
		t.Fatal("blocked prepared start", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM daily_quickplay_counts`); n != 0 {
		t.Fatal("failed start charged", n)
	}
	if got, err := trust.Blocks(ctx, ids[1]); err != nil || len(got) != 0 {
		t.Fatal("incoming block disclosed", got, err)
	}
	if err := trust.Unblock(ctx, ids[0], ids[1]); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, now); err != nil {
		t.Fatal(err)
	}
	if err := trust.Block(ctx, ids[1], ids[0], now); err != nil {
		t.Fatal(err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_matches WHERE state='started'`); n != 1 {
		t.Fatal("block changed current game")
	}
	if _, err := s.Award(ctx, TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Kind: "correct_vote", Ordinal: 1, Amount: s.tuning.Noin.CorrectVote, At: now}); err != nil {
		t.Fatal("block changed current value", err)
	}
}
func TestTextTermsVersionedAndNoImplicitAcceptance(t *testing.T) {
	db, _ := textValueDB(t)
	ctx := context.Background()
	trust := NewTextTrustStore(db)
	id := valueAccount(t, db)
	now := time.Now().UTC()
	if err := trust.RequireTerms(ctx, id, "test-user-v1"); !errors.Is(err, ErrTextTerms) {
		t.Fatal(err)
	}
	if err := trust.AcceptTerms(ctx, id, "unknown", now); err == nil {
		t.Fatal("unknown accepted")
	}
	if _, err := db.Exec(`INSERT INTO user_terms_versions(version,body,active_from) VALUES('test-user-v1','Test only terms',$1),('test-user-v2','Test only revised terms',$2)`, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := trust.AcceptTerms(ctx, id, "test-user-v2", now); err == nil {
		t.Fatal("future terms accepted")
	}
	for i := 0; i < 2; i++ {
		if err := trust.AcceptTerms(ctx, id, "test-user-v1", now); err != nil {
			t.Fatal(err)
		}
	}
	if err := trust.RequireTerms(ctx, id, "test-user-v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE user_terms_acceptances SET accepted_at=now()`); err == nil {
		t.Fatal("consent mutable")
	}
	if _, err := db.Exec(`UPDATE user_terms_versions SET body='Changed'`); err == nil {
		t.Fatal("terms mutable")
	}
	if err := trust.RequireTerms(ctx, id, "test-user-v2"); !errors.Is(err, ErrTextTerms) {
		t.Fatal("acceptance transferred", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger`); n != 0 {
		t.Fatal("terms paid value")
	}
	if err := trust.Block(ctx, id, uuid.NewString(), now); err == nil {
		t.Fatal("unknown target")
	}
}

func TestTimedSuspensionFencesAdmissionAndAdminButRetainsEarnedValue(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	now := valueTime(time.Now().UTC())
	trust := NewTextTrustStore(db)
	m, ids := valuePreparedMatch(t, s, db, now, false)
	until := now.Add(time.Hour)
	if _, err := db.Exec(`UPDATE accounts SET suspended_until=$2 WHERE id=$1`, ids[0], until); err != nil {
		t.Fatal(err)
	}
	if err := trust.CanMatch(ctx, ids, now); !errors.Is(err, ErrTextTrust) {
		t.Fatal("suspension allowed reservation", err)
	}
	if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, now); !errors.Is(err, ErrTextTrust) {
		t.Fatal("suspension allowed start", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM daily_quickplay_counts`); n != 0 {
		t.Fatal("refused start consumed quota", n)
	}
	if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, until); err != nil {
		t.Fatal("expiry did not restore future admission", err)
	}
	if _, err := db.Exec(`UPDATE accounts SET banned_at=now(),suspended_until=$2 WHERE id=$1`, ids[0], until.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Award(ctx, TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Kind: "correct_vote", Ordinal: 1, Amount: s.tuning.Noin.CorrectVote, At: until}); err != nil {
		t.Fatal("account enforcement erased an already-earned award", err)
	}
	if err := trust.CanMatch(ctx, ids, until.Add(2*time.Hour)); !errors.Is(err, ErrTextTrust) {
		t.Fatal("suspension expiry cleared independent ban", err)
	}
	admin, actor := releaseTestAdmin(t, db)
	if _, err := db.Exec(`UPDATE accounts SET suspended_until=now()+interval '1 hour' WHERE id=$1`, actor); err != nil {
		t.Fatal(err)
	}
	if err := trust.PublishTerms(ctx, admin, "suspended-admin-terms", "Test only", now); err == nil {
		t.Fatal("suspended admin mutated terms")
	}
}

func TestTextTermsPublicationAndPrivateBlockPages(t *testing.T) {
	db, _ := textValueDB(t)
	ctx := context.Background()
	s := NewTextTrustStore(db)
	admin, actor := releaseTestAdmin(t, db)
	at := valueTime(time.Now().UTC())
	const version = "test-user-terms"
	const body = "Test only user terms. <script>inert</script>\nİstanbul — العربية"
	if got, err := s.Terms(ctx, actor, version, at); err != nil || got.Available || got.Accepted || got.Body != "" {
		t.Fatal("unpublished terms visible/accepted", got, err)
	}
	if err := s.PublishTerms(ctx, uuid.NewString(), version, body, at); err == nil {
		t.Fatal("unauthorized terms publication")
	}
	for i := 0; i < 2; i++ {
		if err := s.PublishTerms(ctx, admin, version, body, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.PublishTerms(ctx, admin, version, body+" changed", at); err == nil {
		t.Fatal("published version rewritten")
	}
	if got, err := s.Terms(ctx, actor, version, at.Add(-time.Second)); err != nil || got.Available || got.Body != "" {
		t.Fatal("future publication disclosed", got, err)
	}
	if got, err := s.Terms(ctx, actor, version, at); err != nil || !got.Available || got.Accepted || got.Body != body {
		t.Fatal(got, err)
	}
	if err := s.AcceptTerms(ctx, actor, version, at); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Terms(ctx, actor, version, at); err != nil || !got.Accepted {
		t.Fatal(got, err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM admin_audit_log WHERE action='user_terms_publish'`); n != 1 {
		t.Fatal("publication audit replay", n)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger`); n != 0 {
		t.Fatal("terms granted value", n)
	}
	var targets []string
	for i := 0; i < 3; i++ {
		id := valueAccount(t, db)
		targets = append(targets, id)
		if err := s.Block(ctx, actor, id, at); err != nil {
			t.Fatal(err)
		}
	}
	page, next, err := s.BlockPage(ctx, actor, "", 2)
	if err != nil || len(page) != 2 || next != page[1] {
		t.Fatal(page, next, err)
	}
	last, end, err := s.BlockPage(ctx, actor, next, 2)
	if err != nil || len(last) != 1 || end != "" || last[0] <= next {
		t.Fatal(last, end, err)
	}
	if got, _, err := s.BlockPage(ctx, targets[0], "", 2); err != nil || len(got) != 0 {
		t.Fatal("incoming relation disclosed", got, err)
	}
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, targets[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.Unblock(ctx, actor, targets[0]); err != nil {
		t.Fatal("cannot remove deleted target block", err)
	}
}

func TestTextBlockRacingStartKeepsOneConsistentAdmission(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	trust := NewTextTrustStore(db)
	for i := 0; i < 10; i++ {
		now := time.Now().UTC()
		m, ids := valuePreparedMatch(t, s, db, now, false)
		ready := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var startErr, blockErr error
		go func() { defer wg.Done(); <-ready; startErr = s.Start(ctx, m.Contract.MatchID, m.Owner, 1, now) }()
		go func() { defer wg.Done(); <-ready; blockErr = trust.Block(ctx, ids[0], ids[1], now) }()
		close(ready)
		wg.Wait()
		if blockErr != nil {
			t.Fatal(blockErr)
		}
		if startErr != nil && !errors.Is(startErr, ErrTextTrust) {
			t.Fatal(startErr)
		}
		want := int64(0)
		if startErr == nil {
			want = 1
		}
		if n := valueCount(t, db, `SELECT COALESCE((SELECT count FROM daily_quickplay_counts WHERE account_id=$1),0)`, ids[0]); n != want {
			t.Fatal("partial start", n, want)
		}
		if err := trust.CanMatch(ctx, ids, now); !errors.Is(err, ErrTextTrust) {
			t.Fatal("future matching ignored committed block", err)
		}
	}
}
func TestTextTrustActualMigrationParityAndExactDown(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "consented"}[populated], func(t *testing.T) {
			db := transitionDesignDB(t, 11, true)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			before := transitionLegacySnapshot(t, ctx, db)
			path := transitionMigrationPath(t, 12)
			if err := MigrateUp(db, path); err != nil {
				t.Fatal(err)
			}
			after := transitionLegacySnapshot(t, ctx, db)
			for _, old := range before.Tables {
				if old.Name == "schema_migrations" {
					continue
				}
				found := false
				for _, new := range after.Tables {
					if old.Name == new.Name {
						found = true
						if !reflect.DeepEqual(old, new) {
							t.Fatal("legacy changed", old.Name)
						}
					}
				}
				if !found {
					t.Fatal("missing old table", old.Name)
				}
			}
			if populated {
				id := valueAccount(t, db)
				now := valueTime(time.Now().UTC())
				if _, err := db.Exec(`INSERT INTO user_terms_versions(version,body,active_from) VALUES('test-retained','Test only',$1)`, now); err != nil {
					t.Fatal(err)
				}
				if err := NewTextTrustStore(db).AcceptTerms(ctx, id, "test-retained", now); err != nil {
					t.Fatal(err)
				}
			}
			err := runMigration(db, path, "exact trust down", func(m *migrate.Migrate) error { return m.Steps(-1) })
			if populated {
				if err == nil {
					t.Fatal("retained consent lost")
				}
				if n := valueCount(t, db, `SELECT count(*) FROM user_terms_acceptances`); n != 1 {
					t.Fatal(n)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
			}
		})
	}
}
