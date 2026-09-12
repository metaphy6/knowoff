package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOAuthConcurrentSubjectLinkCannotReportTwoOwners(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := newTestManager(db)
	accounts := make([]string, 2)
	for i := range accounts {
		p, err := m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
		if err != nil {
			t.Fatal(err)
		}
		accounts[i] = p.AccountID
		if _, err := db.ExecContext(ctx, `INSERT INTO noin_wallets(account_id,balance) VALUES($1,80)`, p.AccountID); err != nil {
			t.Fatal(err)
		}
	}
	barrier, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback()
	if _, err := barrier.ExecContext(ctx, `LOCK TABLE oauth_links IN SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	type linked struct {
		index int
		err   error
	}
	results := make(chan linked, 2)
	subject := "provider-subject-" + uuid.NewString()
	for i, account := range accounts {
		go func() { results <- linked{i, m.LinkOAuth(ctx, account, "google", subject, account+"@example.com")} }()
	}
	for {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 2 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("missing real concurrent-link barrier", ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	if err := barrier.Commit(); err != nil {
		t.Fatal(err)
	}
	winners := []int{}
	for range accounts {
		r := <-results
		if r.err == nil {
			winners = append(winners, r.index)
		}
	}
	if len(winners) != 1 {
		t.Fatalf("subject link reported %d owners", len(winners))
	}
	var owner, email string
	if err := db.QueryRowContext(ctx, `SELECT account_id,provider_email FROM oauth_links WHERE provider='google' AND provider_subject=$1`, subject).Scan(&owner, &email); err != nil {
		t.Fatal(err)
	}
	if owner != accounts[winners[0]] || email != owner+"@example.com" {
		t.Fatal("loser overwrote provider identity")
	}
	for _, account := range accounts {
		var balance int
		if err := db.QueryRowContext(ctx, `SELECT balance FROM noin_wallets WHERE account_id=$1`, account).Scan(&balance); err != nil || balance != 80 {
			t.Fatal("collision merged or lost value", balance, err)
		}
	}
}

func TestOAuthDirectLinkRejectsInvalidProviderSubject(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	ctx := context.Background()
	p, err := m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][2]string{{"unknown", "subject"}, {"google", ""}, {"google", "subject\n"}} {
		if err := m.LinkOAuth(ctx, p.AccountID, input[0], input[1], ""); err == nil {
			t.Fatal("invalid provider identity accepted")
		}
	}
}
