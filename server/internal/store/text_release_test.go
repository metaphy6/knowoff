package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/media"
	"testing"
	"time"
)

// Every approval/certificate below is a disposable test double, never a release artifact.
func TestTextReleaseAcceptedLineageRestartAndTakedown(t *testing.T) {
	db, value := textValueDB(t)
	ctx := context.Background()
	admin, author := releaseTestAdmin(t, db)
	var screens int
	r := NewTextReleaseStore(db, value.tuning, func(context.Context, string) error { screens++; return nil })
	snap, err := media.LoadTextPack("../../pkg/media/testdata/text-en", r.limits())
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Publish(ctx, admin, snap, TextPackAccess{Class: "core"}); err == nil {
		t.Fatal("synthetic fixture published")
	}
	b := snap.Bundle()
	b.Manifest.Synthetic = false
	b.Manifest.ReleaseID = "test-reviewed-one"
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
	for i := range b.Nowns {
		b.Nowns[i].Provenance = accept(b.Nowns[i].Text)
	}
	for i := range b.Cards {
		b.Cards[i].Provenance = accept(b.Cards[i].Text)
	}
	// A later profile rename cannot rewrite frozen credit or make a safe capture
	// retry fail. Neither operation repeats approval or its reward.
	if _, err := db.Exec(`UPDATE accounts SET nickname='New display name' WHERE id=$1`, author); err != nil {
		t.Fatal(err)
	}
	first := b.Nowns[0].Provenance
	if again, err := r.CaptureAccepted(ctx, admin, "portal_submission", first.ApprovalReference); err != nil || again != first {
		t.Fatal("capture retry changed original attribution", again, err)
	}
	snap = releaseTestEvidence(t, r, b)
	if err = r.Publish(ctx, uuid.NewString(), snap, TextPackAccess{Class: "core"}); err == nil {
		t.Fatal("unauthorized publication")
	}
	if err = r.Publish(ctx, admin, snap, TextPackAccess{Class: "core"}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Resolve(ctx, "en", "text-v1"); err == nil {
		t.Fatal("publication implicitly activated")
	}
	sourceID := b.Nowns[0].Provenance.ApprovalReference
	if _, err = db.Exec(`UPDATE portal_submissions SET status='rejected' WHERE id=$1`, sourceID); err != nil {
		t.Fatal(err)
	}
	if err = r.Activate(ctx, admin, snap.Manifest().ReleaseID); err == nil {
		t.Fatal("withdrawn source activated after publication")
	}
	if _, err = db.Exec(`UPDATE portal_submissions SET status='approved' WHERE id=$1`, sourceID); err != nil {
		t.Fatal(err)
	}
	if err = r.Activate(ctx, admin, snap.Manifest().ReleaseID); err != nil {
		t.Fatal(err)
	}
	pinned, err := r.Resolve(ctx, "en", "text-v1")
	if err != nil || pinned.SHA256() != snap.SHA256() {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := r.Resolve(ctx, "en", "text-v1")
		if err != nil || again != pinned {
			t.Fatalf("verified immutable snapshot not cached: %v", err)
		}
	}
	if len(r.verified) != 1 {
		t.Fatal("verification cache", len(r.verified))
	}
	if _, err = db.Exec(`UPDATE portal_submissions SET status='rejected' WHERE id=$1`, sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Resolve(ctx, "en", "text-v1"); err == nil {
		t.Fatal("cached release ignored source withdrawal")
	}
	if _, err = db.Exec(`UPDATE portal_submissions SET status='approved' WHERE id=$1`, sourceID); err != nil {
		t.Fatal(err)
	}
	restarted := NewTextReleaseStore(db, value.tuning, nil)
	if err = restarted.Publish(ctx, admin, snap, TextPackAccess{Class: "core"}); err != nil {
		t.Fatal("publish retry", err)
	}
	if err = restarted.Activate(ctx, admin, snap.Manifest().ReleaseID); err != nil {
		t.Fatal("activate retry", err)
	}
	// Fresh production starts bind the approved active release and policy checks.
	guarded := value.WithStartGuard(r.ValidateStart)
	record, players := valueUnpreparedMatch(t, guarded, db, time.Now().UTC(), false)
	record.Contract.PackReleaseID = snap.Manifest().ReleaseID
	record.Contract.PackSHA256 = snap.SHA256()
	if err = guarded.Prepare(ctx, record, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err = r.Takedown(ctx, admin, snap.Manifest().ReleaseID, "start revalidation fixture"); err != nil {
		t.Fatal(err)
	}
	if err = guarded.Start(ctx, record.Contract.MatchID, record.Owner, 1, time.Now().UTC()); err == nil {
		t.Fatal("withdrawn pack started")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM daily_quickplay_counts WHERE account_id=$1`, players[0]); n != 0 {
		t.Fatal("withdrawn start consumed quota")
	}
	// Restore through a new immutable release, never by undoing withdrawal.
	themeBundle := b
	themeBundle.Manifest.ReleaseID = "test-theme-one"
	theme := releaseTestEvidence(t, r, themeBundle)
	if err = r.Publish(ctx, admin, theme, TextPackAccess{Class: "theme", EntitlementKey: "test_theme"}); err != nil {
		t.Fatal(err)
	}
	if err = r.Activate(ctx, admin, theme.Manifest().ReleaseID); err != nil {
		t.Fatal(err)
	}
	settings := v2.LobbySettings{ModeID: record.Contract.ModeID, Size: 4, RulesVersion: "text-v1", ContentLanguage: "en", PackReleaseID: theme.Manifest().ReleaseID}
	if _, err = db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'theme_pack','test_theme')`, players[1]); err != nil {
		t.Fatal(err)
	}
	if err = r.CheckAccess(ctx, players[0], settings, "local", time.Now().UTC()); err == nil {
		t.Fatal("guest sponsorship accepted")
	}
	if err = r.CheckAccess(ctx, players[1], settings, "quick_play", time.Now().UTC()); err == nil {
		t.Fatal("paid pack entered Quick Play")
	}
	if err = r.CheckAccess(ctx, players[1], settings, "local", time.Now().UTC()); err != nil {
		t.Fatal("host sponsorship", err)
	}
	if _, err = db.Exec(`INSERT INTO named_entitlement_items(account_id,entitlement_type,value,source_id) VALUES($1,'theme_pack','other_theme',$2)`, players[0], uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if err = r.CheckAccess(ctx, players[0], settings, "local", time.Now().UTC()); err == nil {
		t.Fatal("different named theme sponsored pack")
	}
	if _, err = db.Exec(`INSERT INTO named_entitlement_items(account_id,entitlement_type,value,source_id) VALUES($1,'theme_pack','test_theme',$2)`, players[0], uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if err = r.CheckAccess(ctx, players[0], settings, "local", time.Now().UTC()); err != nil {
		t.Fatal("named host sponsorship", err)
	}
	if err = r.CheckAccess(ctx, players[0], settings, "quick_play", time.Now().UTC()); err == nil {
		t.Fatal("named theme entered Quick Play")
	}
	// The earlier core snapshot remains pinned, even after active withdrawal.
	if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger`); n != 0 {
		t.Fatal("publication paid approval again")
	}
	b2 := b
	b2.Manifest.ReleaseID = "test-reviewed-two"
	b2.Nowns = append([]media.TextNown(nil), b.Nowns...)
	b2.Nowns[0].Text = "A changed wording"
	b2.Nowns[0].Provenance = accept(b2.Nowns[0].Text)
	conflicting := releaseTestEvidence(t, r, b2)
	if err = restarted.Publish(ctx, admin, conflicting, TextPackAccess{Class: "core"}); err == nil {
		t.Fatal("same revision redefined after restart")
	}
	if _, err = db.Exec(`UPDATE portal_submissions SET content='Changed after approval' WHERE id=$1`, sourceID); err == nil {
		t.Fatal("approved wording mutable")
	}
	if err = restarted.Takedown(ctx, admin, snap.Manifest().ReleaseID, "test takedown"); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Resolve(ctx, "en", "text-v1"); err == nil {
		t.Fatal("withdrawn content available")
	}
	if err = restarted.Activate(ctx, admin, snap.Manifest().ReleaseID); err == nil {
		t.Fatal("withdrawn release reactivated")
	}
	if pinned.SHA256() != snap.SHA256() {
		t.Fatal("in-flight pin changed")
	}
	if screens == 0 {
		t.Fatal("accepted input bypassed screen")
	}
}
func TestTextReleaseRefusesScreenFailureAndStaleAcceptance(t *testing.T) {
	db, value := textValueDB(t)
	ctx := context.Background()
	admin, author := releaseTestAdmin(t, db)
	id := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at,decided_at,decided_by) VALUES($1,$2,'text','Test accepted words','approved','test-terms',now()-interval '1 day',now(),$3)`, id, author, admin); err != nil {
		t.Fatal(err)
	}
	r := NewTextReleaseStore(db, value.tuning, func(context.Context, string) error { return errors.New("screen unavailable") })
	if _, err := r.CaptureAccepted(ctx, admin, "portal_submission", id); err == nil {
		t.Fatal("failed screen accepted")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_accepted_inputs`); n != 0 {
		t.Fatal(n)
	}
	r.screen = func(context.Context, string) error {
		_, err := db.Exec(`UPDATE portal_submissions SET content='Changed during screen' WHERE id=$1`, id)
		return err
	}
	if _, err := r.CaptureAccepted(ctx, admin, "portal_submission", id); err == nil {
		t.Fatal("stale screened bytes accepted")
	}
}
func releaseTestAdmin(t *testing.T, db *sql.DB) (string, string) {
	t.Helper()
	account := valueAccount(t, db)
	admin := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret) VALUES($1,$2,$3,'test','test')`, admin, account, admin+"@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO portal_terms(version,title,body) VALUES('test-terms','Test only','Simulated consent')`); err != nil {
		t.Fatal(err)
	}
	return admin, account
}
func releaseTestEvidence(t *testing.T, r *TextReleaseStore, b media.TextBundle) *media.TextSnapshot {
	t.Helper()
	b.Manifest.CertificationArtifacts = nil
	b.Artifacts = map[string][]byte{}
	sealed, err := media.SealTextBundle(b, r.limits())
	if err != nil {
		t.Fatal(err)
	}
	s, err := media.NewTextSnapshot(sealed, r.limits())
	if err != nil {
		t.Fatal(err)
	}
	sealed.Manifest.CertificationArtifacts = map[string]string{}
	sealed.Artifacts["technical.json"], _ = json.Marshal(media.CertifyText(s, r.dealing(), 20, 71))
	sealed.Artifacts["replay.json"], _ = json.Marshal(media.NewTextReplay(s, r.dealing(), 20, 71))
	for _, gate := range []string{"editorial", "actions", "screening", "release"} {
		e := media.TextGateEvidence{SchemaVersion: 2, SnapshotSHA256: s.SHA256(), RulesVersion: b.Manifest.RulesVersion, Language: b.Manifest.Language, Gate: gate, ActorReference: "simulated-test-reviewer", TuningSHA256: media.TextTuningSHA256(r.dealing())}
		for _, mode := range b.Manifest.Modes {
			for _, size := range []int{4, 6} {
				e.Cells = append(e.Cells, media.TextGateCell{Mode: mode, TableSize: size, Passed: true, RecordReference: "simulated-test-only", EvidenceSHA256: media.ContentHash([]byte("simulated fixture evidence"))})
			}
		}
		sealed.Artifacts[gate+".json"], _ = json.Marshal(e)
	}
	for name, raw := range sealed.Artifacts {
		sealed.Manifest.CertificationArtifacts[name] = media.ContentHash(raw)
	}
	sealed, err = media.SealTextBundle(sealed, r.limits())
	if err != nil {
		t.Fatal(err)
	}
	s, err = media.NewTextSnapshot(sealed, r.limits())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
