package auth

import (
	"context"
	"github.com/google/uuid"
	"sync"
	"testing"
	"time"
)

func TestDevelopmentIdentityCannotEnterProductionOrBecomePlayer(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	dev, prod := newTestManager(db), newTestManager(db)
	if _, err := dev.CreateDevelopmentAccount(ctx); err == nil {
		t.Fatal("default dev admission open")
	}
	if err := dev.ConfigureDevelopment("prod", true); err == nil {
		t.Fatal("production enabled prototype identity")
	}
	if err := dev.ConfigureDevelopment("local", true); err != nil {
		t.Fatal(err)
	}
	pair, err := dev.CreateDevelopmentAccount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := dev.ValidateAccessToken(ctx, pair.AccessToken); err != nil || got != pair.AccountID {
		t.Fatal(got, err)
	}
	if _, err := prod.ValidateAccessToken(ctx, pair.AccessToken); err == nil {
		t.Fatal("production accepted dev token")
	}
	if _, err := prod.Refresh(ctx, pair.RefreshToken); err == nil {
		t.Fatal("production refreshed dev token")
	}
	claims, err := dev.parseToken(pair.AccessToken, TokenAccess)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Purpose != "development" {
		t.Fatal("unsigned/inferred development purpose")
	}
	var nickname string
	if err := db.QueryRow(`SELECT nickname FROM accounts WHERE id=$1`, pair.AccountID).Scan(&nickname); err != nil || len(nickname) > 20 {
		t.Fatal("development identity exceeds public nickname limit", err)
	}
	if _, err := dev.AuthenticateDevice(ctx, claims.DeviceHash); err == nil {
		t.Fatal("dev namespace accepted by ordinary device auth")
	}
	if err := dev.LinkOAuth(ctx, pair.AccountID, "google", "fixture-subject", ""); err == nil {
		t.Fatal("development identity linked to player OAuth")
	}
	if _, err := db.Exec(`UPDATE accounts SET auth_purpose='player' WHERE id=$1`, pair.AccountID); err == nil {
		t.Fatal("development identity converted")
	}
	renewed, err := dev.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prod.ValidateAccessToken(ctx, renewed.AccessToken); err == nil {
		t.Fatal("refresh lost purpose")
	}
	if _, err := dev.Refresh(ctx, pair.RefreshToken); err == nil {
		t.Fatal("refresh replay accepted")
	}
	player, err := dev.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prod.ValidateAccessToken(ctx, player.AccessToken); err != nil {
		t.Fatal("ordinary player rejected", err)
	}
}

func TestTimedSuspensionAndGuardAccessBoundaries(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	ctx := context.Background()
	p, err := m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	guard, err := m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO guard_freezes(id,account_id,guard_account_id,reason,frozen_at,expires_at) VALUES($1,$2,$3,'test case',now(),now()+interval '1 hour')`, uuid.NewString(), p.AccountID, guard.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ValidateAccessToken(ctx, p.AccessToken); err != nil {
		t.Fatal("Guard-only freeze blocked safety/report access", err)
	}
	if _, err := db.Exec(`UPDATE accounts SET suspended_until=$2 WHERE id=$1`, p.AccountID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ValidateAccessToken(ctx, p.AccessToken); err == nil {
		t.Fatal("timed admin suspension accepted access")
	}
	if _, err := m.Refresh(ctx, p.RefreshToken); err == nil {
		t.Fatal("timed admin suspension refreshed")
	}
	if _, err := db.Exec(`UPDATE accounts SET suspended_until=now()-interval '1 second' WHERE id=$1`, p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ValidateAccessToken(ctx, p.AccessToken); err != nil {
		t.Fatal("expired suspension still blocks", err)
	}
	if _, err := m.Refresh(ctx, p.RefreshToken); err != nil {
		t.Fatal("refusal consumed refresh credential", err)
	}
	if _, err := db.Exec(`UPDATE accounts SET banned_at=now() WHERE id=$1`, p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ValidateAccessToken(ctx, p.AccessToken); err == nil {
		t.Fatal("expired suspension cleared independent ban")
	}
}

func TestAuthRejectsDeletedMissingAccountsAndConcurrentRefresh(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	ctx := context.Background()
	for _, missing := range []bool{false, true} {
		p, err := m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
		if err != nil {
			t.Fatal(err)
		}
		if missing {
			p, err = m.signTokens(uuid.NewString(), HashDevice(uuid.NewString()), "player", 0)
		} else {
			_, err = db.Exec("UPDATE accounts SET deleted_at=now() WHERE id=$1", p.AccountID)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err = m.ValidateAccessToken(ctx, p.AccessToken); err == nil {
			t.Fatal("missing/deleted access accepted")
		}
		if _, err = m.Refresh(ctx, p.RefreshToken); err == nil {
			t.Fatal("missing/deleted refresh accepted")
		}
	}
	p, err := m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 8)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, e := m.Refresh(ctx, p.RefreshToken); results <- e }()
	}
	close(start)
	wg.Wait()
	close(results)
	passed := 0
	for e := range results {
		if e == nil {
			passed++
		}
	}
	if passed != 1 {
		t.Fatalf("refresh granted %d token pairs", passed)
	}
}
