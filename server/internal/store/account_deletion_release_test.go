package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func TestPrivacyContentCertifiedReleaseAdmissionAndPinnedMatch(t *testing.T) {
	db, value := textValueDB(t)
	ctx := context.Background()
	r, snapshot, admin, author := privacyCertifiedReleaseFixture(t, db, value)
	pinned, err := r.Resolve(ctx, "en", "text-v1")
	if err != nil {
		t.Fatal(err)
	}
	guarded := value.WithStartGuard(r.ValidateStart)
	started, _ := valueUnpreparedMatch(t, guarded, db, time.Now().UTC(), false)
	started.Contract.PackReleaseID, started.Contract.PackSHA256 = snapshot.Manifest().ReleaseID, snapshot.SHA256()
	if err := guarded.Prepare(ctx, started, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := guarded.Start(ctx, started.Contract.MatchID, started.Owner, 1, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	prepared, players := valueUnpreparedMatch(t, guarded, db, time.Now().UTC(), false)
	prepared.Contract.PackReleaseID, prepared.Contract.PackSHA256 = snapshot.Manifest().ReleaseID, snapshot.SHA256()
	if err := guarded.Prepare(ctx, prepared, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := db.QueryRow(`SELECT to_jsonb(m)::text FROM text_matches m WHERE id=$1`, started.Contract.MatchID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(privacyContentConfirm, privacyContentProof(t, db, author)...); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CaptureAccepted(ctx, admin, "portal_submission", snapshot.Bundle().Nowns[0].Provenance.ApprovalReference); err == nil {
		t.Fatal("deleted source recaptured")
	}
	if err := r.Publish(ctx, admin, snapshot, TextPackAccess{Class: "core"}); err == nil {
		t.Fatal("deleted source republished on exact receipt replay")
	}
	if err := r.Activate(ctx, admin, snapshot.Manifest().ReleaseID); err == nil {
		t.Fatal("deleted source reactivated")
	}
	for _, reader := range []*TextReleaseStore{r, NewTextReleaseStore(db, value.tuning, nil)} {
		if _, err := reader.Resolve(ctx, "en", "text-v1"); err == nil {
			t.Fatal("cached or restarted reader admitted deleted source")
		}
	}
	if err := guarded.Start(ctx, prepared.Contract.MatchID, prepared.Owner, 1, time.Now().UTC()); err == nil {
		t.Fatal("prepared match started after author deletion")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM daily_quickplay_counts WHERE account_id=$1`, players[0]); n != 0 {
		t.Fatal("refused start consumed quota")
	}
	var after string
	if err := db.QueryRow(`SELECT to_jsonb(m)::text FROM text_matches m WHERE id=$1`, started.Contract.MatchID).Scan(&after); err != nil || after != before {
		t.Fatal("author deletion changed an already started match", err)
	}
	if pinned.SHA256() != snapshot.SHA256() {
		t.Fatal("in-flight content pin changed")
	}
}

func privacyCertifiedReleaseFixture(t *testing.T, db *sql.DB, value *TextValueStore) (*TextReleaseStore, *media.TextSnapshot, string, string) {
	t.Helper()
	ctx := context.Background()
	admin, _ := releaseTestAdmin(t, db)
	author := valueAccount(t, db)
	r := NewTextReleaseStore(db, value.tuning, func(context.Context, string) error { return nil })
	fixture, err := media.LoadTextPack("../../pkg/media/testdata/text-en", r.limits())
	if err != nil {
		t.Fatal(err)
	}
	bundle := fixture.Bundle()
	bundle.Manifest.Synthetic = false
	bundle.Manifest.ReleaseID = "privacy-reviewed-release"
	accept := func(text string) media.TextProvenance {
		id := uuid.NewString()
		if _, err := db.Exec(`INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at,decided_at,decided_by) VALUES($1,$2,'text',$3,'approved','test-terms',now()-interval '1 day',now(),$4)`, id, author, text, admin); err != nil {
			t.Fatal(err)
		}
		p, err := r.CaptureAccepted(ctx, admin, "portal_submission", id)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	for i := range bundle.Nowns {
		bundle.Nowns[i].Provenance = accept(bundle.Nowns[i].Text)
	}
	for i := range bundle.Cards {
		bundle.Cards[i].Provenance = accept(bundle.Cards[i].Text)
	}
	// Engine-backed technical certificates and synthetic editorial attestations
	// exercise the real publication path; they are not human release approval.
	snapshot := releaseTestEvidence(t, r, bundle)
	if err := r.Publish(ctx, admin, snapshot, TextPackAccess{Class: "core"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Activate(ctx, admin, snapshot.Manifest().ReleaseID); err != nil {
		t.Fatal(err)
	}
	return r, snapshot, admin, author
}

func privacyReleaseWaitLock(t *testing.T, db *sql.DB, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := db.QueryRow(`SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("transaction never reached expected PostgreSQL lock wait")
}
func TestPrivacyContentActualStartConfirmationBothOrders(t *testing.T) {
	for _, first := range []string{"start", "confirmation"} {
		t.Run(first, func(t *testing.T) {
			db, value := textValueDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			release, snapshot, _, author := privacyCertifiedReleaseFixture(t, db, value)
			prepared, players := valueUnpreparedMatch(t, value, db, time.Now().UTC(), false)
			prepared.Contract.PackReleaseID, prepared.Contract.PackSHA256 = snapshot.Manifest().ReleaseID, snapshot.SHA256()
			if err := value.Prepare(ctx, prepared, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			proof := privacyContentProof(t, db, author)
			startDone := make(chan error, 1)
			if first == "start" {
				validated := make(chan struct{})
				resume := make(chan struct{})
				defer func() {
					select {
					case <-resume:
					default:
						close(resume)
					}
				}()
				guarded := value.WithStartGuard(func(ctx context.Context, tx *sql.Tx, record TextMatchRecord, accounts []string, at time.Time) error {
					if err := release.ValidateStart(ctx, tx, record, accounts, at); err != nil {
						return err
					}
					close(validated)
					select {
					case <-resume:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
				go func() {
					startDone <- guarded.Start(ctx, prepared.Contract.MatchID, prepared.Owner, 1, time.Now().UTC())
				}()
				select {
				case <-validated:
				case <-ctx.Done():
					t.Fatal("Start never validated release")
				}
				conn, err := db.Conn(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				var pid int
				if err = conn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
					t.Fatal(err)
				}
				confirmationDone := make(chan error, 1)
				go func() { _, e := conn.ExecContext(ctx, privacyContentConfirm, proof...); confirmationDone <- e }()
				privacyReleaseWaitLock(t, db, pid)
				close(resume)
				if err = <-startDone; err != nil {
					t.Fatal("validated Start failed", err)
				}
				if err = <-confirmationDone; err != nil {
					t.Fatal("waiting confirmation failed", err)
				}
				if valueCount(t, db, `SELECT count(*) FROM text_matches WHERE id=$1 AND state='started' AND contract#>>'{Contract,pack_release_id}'=$2`, prepared.Contract.MatchID, snapshot.Manifest().ReleaseID) != 1 {
					t.Fatal("started match/pin changed")
				}
			} else {
				holder, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer holder.Rollback()
				if _, err = holder.ExecContext(ctx, privacyContentConfirm, proof...); err != nil {
					t.Fatal(err)
				}
				entered := make(chan int, 1)
				guarded := value.WithStartGuard(func(ctx context.Context, tx *sql.Tx, record TextMatchRecord, accounts []string, at time.Time) error {
					var pid int
					if err := tx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
						return err
					}
					entered <- pid
					return release.ValidateStart(ctx, tx, record, accounts, at)
				})
				go func() {
					startDone <- guarded.Start(ctx, prepared.Contract.MatchID, prepared.Owner, 1, time.Now().UTC())
				}()
				var pid int
				select {
				case pid = <-entered:
				case <-ctx.Done():
					t.Fatal("Start never reached release guard")
				}
				privacyReleaseWaitLock(t, db, pid)
				if err = holder.Commit(); err != nil {
					t.Fatal(err)
				}
				if err = <-startDone; err == nil {
					t.Fatal("waiting Start admitted withdrawn release")
				}
				if valueCount(t, db, `SELECT count(*) FROM text_matches WHERE id=$1 AND state='prepared'`, prepared.Contract.MatchID) != 1 {
					t.Fatal("refused Start mutated match")
				}
				for _, account := range players {
					if valueCount(t, db, `SELECT count(*) FROM daily_quickplay_counts WHERE account_id=$1`, account) != 0 {
						t.Fatal("refused Start consumed quota")
					}
				}
			}
			if valueCount(t, db, `SELECT count(*) FROM text_active_releases WHERE release_id=$1`, snapshot.Manifest().ReleaseID) != 0 {
				t.Fatal("confirmation left active publication")
			}
		})
	}
}
