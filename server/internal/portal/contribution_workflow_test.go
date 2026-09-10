package portal

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestDraftRequiresExplicitConsent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	account := newAccount(t, db)
	if _, err := m.CreateDraft(context.Background(), account, MediaText, "The meeting has become sentient."); err == nil {
		t.Fatal("draft accepted without explicit consent")
	}
}

func TestContributionConcurrentCapAndDecision(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	m.cfg.Tuning.Portal.SubmissionsPerContributorPerDay = 1
	account := newAccount(t, db)
	reviewer := newAdmin(t, db)
	ctx := context.Background()
	if err := m.GrantRole(ctx, reviewer, account, RoleContributor); err != nil {
		t.Fatal(err)
	}
	// Consent is supplied after its public contract is introduced by this change.
	a, err := m.CreateDraft(ctx, account, MediaText, "The printer requests annual leave.", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.CreateDraft(ctx, account, MediaText, "The calendar has booked a meeting about meetings.", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for _, id := range []string{a.ID, b.ID} {
		wg.Add(1)
		go func(id string) { defer wg.Done(); outcomes <- m.SubmitDraft(ctx, account, id) }(id)
	}
	wg.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("daily cap admitted %d parallel submissions; want 1", successes)
	}
	subs, err := m.ListSubmissions(ctx, account, "submitted")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 {
		t.Fatalf("submitted=%d", len(subs))
	}
	decisions := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); decisions <- m.DecideSubmission(ctx, reviewer, subs[0].ID, true, "") }()
	}
	wg.Wait()
	close(decisions)
	successes = 0
	for err := range decisions {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("decisions succeeded=%d", successes)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1 AND event_type='contributor_reward'`, account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reward count=%d", count)
	}
	if err := db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE target_id=$1 AND action='submission_decide'`, subs[0].ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("durable decision audit count=%d", count)
	}
}

func TestRejectApplicationRequiresReason(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	err := m.RejectApplication(context.Background(), newAdmin(t, db), "00000000-0000-0000-0000-000000000000", "  ")
	if err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("expected reason validation, got %v", err)
	}
}

func TestDraftOwnershipImmutabilityStaleTermsAndQueueReplay(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	owner := newAccount(t, db)
	other := newAccount(t, db)
	admin := newAdmin(t, db)
	for _, id := range []string{owner, other} {
		if err := m.GrantRole(ctx, admin, id, RoleContributor); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.CreateDraft(ctx, owner, MediaText, "draft", ContributionConsent{Version: "v0", Accepted: true}); err == nil {
		t.Fatal("stale consent accepted")
	}
	d, err := m.CreateDraft(ctx, owner, MediaText, "draft", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.EditDraft(ctx, other, d.ID, "stolen"); err == nil {
		t.Fatal("other owner edited draft")
	}
	if err = m.EditDraft(ctx, owner, d.ID, "edited"); err != nil {
		t.Fatal(err)
	}
	if err = m.SubmitDraft(ctx, owner, d.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.SubmitDraft(ctx, owner, d.ID); err == nil {
		t.Fatal("replayed submit accepted")
	}
	if err = m.EditDraft(ctx, owner, d.ID, "mutated after submit"); err == nil {
		t.Fatal("submitted content mutated")
	}
	if err = m.WithdrawSubmission(ctx, other, d.ID); err == nil {
		t.Fatal("other owner withdrew submission")
	}
	if err = m.WithdrawSubmission(ctx, owner, d.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.EditDraft(ctx, owner, d.ID, "corrected"); err != nil {
		t.Fatal(err)
	}
	if err = m.SubmitDraft(ctx, owner, d.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT count FROM portal_submission_counts WHERE account_id=$1`, owner).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("withdraw/resubmit used %d queue slots", count)
	}
}

func TestRoleReapplicationRetainsDecisionHistory(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	account := newAccount(t, db)
	admin := newAdmin(t, db)
	for i := 0; i < 2; i++ {
		if err := m.ApplyForRole(ctx, account, RoleContributor); err != nil {
			t.Fatal(err)
		}
		if err := m.GrantRole(ctx, admin, account, RoleContributor); err != nil {
			t.Fatal(err)
		}
		if err := m.RevokeRole(ctx, admin, account, RoleContributor); err != nil {
			t.Fatal(err)
		}
	}
	apps, err := m.ListOwnApplications(ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 {
		t.Fatalf("history length=%d", len(apps))
	}
	for _, a := range apps {
		if a.Status != ApplicationApproved {
			t.Fatalf("history changed to %s", a.Status)
		}
	}
}

func TestContributorRewardExcludesPlayCap(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	m.cfg.Tuning.Noin.DailyEarnCap = 1
	account := newAccount(t, db)
	admin := newAdmin(t, db)
	if err := m.GrantRole(ctx, admin, account, RoleContributor); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES($1,CURRENT_DATE,1)`, account); err != nil {
		t.Fatal(err)
	}
	d, err := m.CreateDraft(ctx, account, MediaText, "caption", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SubmitDraft(ctx, account, d.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.DecideSubmission(ctx, admin, d.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	var amount int
	if err = db.QueryRow(`SELECT amount FROM noin_ledger WHERE account_id=$1 AND event_type='contributor_reward'`, account).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != m.cfg.Tuning.Noin.ContributorAcceptedAsset {
		t.Fatalf("community award capped to %d", amount)
	}
}

func TestGuardCannotCreateOrReviewContentByRoleInheritance(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	account := newAccount(t, db)
	admin := newAdmin(t, db)
	if err := m.GrantRole(ctx, admin, account, RoleGuard); err != nil {
		t.Fatal(err)
	}
	for _, role := range []Role{RoleContributor, RoleCurator} {
		has, err := m.HasRole(ctx, account, role)
		if err != nil {
			t.Fatal(err)
		}
		if has {
			t.Fatalf("Guard inherited %s", role)
		}
	}
	if _, err := m.CreateDraft(ctx, account, MediaText, "guard draft", ContributionConsent{Version: "v1", Accepted: true}); err == nil {
		t.Fatal("Guard created contributor draft")
	}
	has, err := m.HasRole(ctx, account, RoleGuard)
	if err != nil || !has {
		t.Fatal("Guard lost its own role")
	}
}

func TestHumanReviewRevisionRejectsChangedResubmission(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := context.Background()
	account := newAccount(t, db)
	admin := newAdmin(t, db)
	if err := m.GrantRole(ctx, admin, account, RoleContributor); err != nil {
		t.Fatal(err)
	}
	d, err := m.CreateDraft(ctx, account, MediaText, "reviewed joke", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SubmitDraft(ctx, account, d.ID); err != nil {
		t.Fatal(err)
	}
	revision := ContentRevision(d.Content)
	if err = m.WithdrawSubmission(ctx, account, d.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.EditDraft(ctx, account, d.ID, "unseen replacement"); err != nil {
		t.Fatal(err)
	}
	if err = m.SubmitDraft(ctx, account, d.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.DecideSubmission(ctx, admin, d.ID, true, "", revision); err == nil {
		t.Fatal("stale human review approved unseen replacement")
	}
	got, err := m.GetSubmission(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusSubmitted {
		t.Fatalf("stale decision changed status=%s", got.Status)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1`, account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("stale review granted reward")
	}
}
