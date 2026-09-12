package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/pkg/media"
	"github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

// All text, screening and human release records in this test are disposable
// fixtures. Exercising the production workflow does not approve a real pack.
func TestAcceptedContributionReleaseJourneyPreservesValueAndConsent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	ctx := t.Context()
	raw, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = yaml.Unmarshal(raw, &m.cfg.Tuning); err != nil {
		t.Fatal(err)
	}
	c := m.cfg.Tuning
	limits := media.TextLimits{MaxTextBytes: c.Contract.MaxTextBytes, MaxRecords: c.TextCatalog.MaxRecords, MaxFileBytes: c.TextCatalog.MaxFileBytes, MaxBundleBytes: c.TextCatalog.MaxBundleBytes}
	tuning := media.TextDealTuning{HandSize: c.Hand.Size, ReserveSize: c.Hand.DrawPile, MinHigh: c.Dealing.MinHighPerNown, MinDistant: c.Dealing.MinDistantPerNown, MaxSearchNodes: c.TextCatalog.MaxSearchNodes}
	r := store.NewTextReleaseStore(db, c, m.screenText)
	base, err := media.LoadTextPack("../../pkg/media/testdata/text-en", limits)
	if err != nil {
		t.Fatal(err)
	}
	b := base.Bundle()
	b.Manifest.Synthetic = false
	b.Manifest.ReleaseID = "test-contributor-journey"
	admin := newAdmin(t, db)
	var first *Submission
	accepted := 0
	var acceptedAccounts []string
	accept := func(text string) media.TextProvenance {
		t.Helper()
		// A separate fixture contributor keeps the real daily intake cap intact.
		account := newAccount(t, db)
		if err := m.GrantRole(ctx, admin, account, RoleContributor); err != nil {
			t.Fatal(err)
		}
		draft, err := m.CreateDraft(ctx, account, MediaText, text, ContributionConsent{Version: "v1", Accepted: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = r.CaptureAccepted(ctx, admin, "portal_submission", draft.ID); err == nil {
			t.Fatal("private draft captured for publication")
		}
		if err = m.SubmitDraft(ctx, account, draft.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = r.CaptureAccepted(ctx, admin, "portal_submission", draft.ID); err == nil {
			t.Fatal("unapproved submission captured")
		}
		if first == nil {
			screener := m.screener
			m.screener = nil
			if err = m.DecideSubmission(ctx, admin, draft.ID, true, "TEST ONLY", ContentRevision(text)); err == nil {
				t.Fatal("unavailable screening approved contribution")
			}
			m.screener = screener
			if balance, err := m.economy.Wallet.Balance(ctx, account); err != nil || balance != 0 {
				t.Fatalf("failed approval granted value: %d %v", balance, err)
			}
		}
		if err = m.DecideSubmission(ctx, admin, draft.ID, true, "TEST ONLY", ContentRevision(text)); err != nil {
			t.Fatal(err)
		}
		if err = m.DecideSubmission(ctx, admin, draft.ID, true, "TEST ONLY", ContentRevision(text)); err == nil {
			t.Fatal("repeated approval reran decision")
		}
		approved, err := m.GetSubmission(ctx, draft.ID)
		if err != nil {
			t.Fatal(err)
		}
		if approved.Content != text || approved.TermsVersion != draft.TermsVersion || !approved.TermsAcceptedAt.Equal(draft.TermsAcceptedAt) || approved.DecidedBy == nil || *approved.DecidedBy != admin || approved.DecidedAt == nil || approved.Status != StatusApproved {
			t.Fatalf("approval changed original consent/wording or lost reviewer: %+v", approved)
		}
		p, err := r.CaptureAccepted(ctx, admin, "portal_submission", draft.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.ApprovalReference != draft.ID || p.TermsVersion != draft.TermsVersion || p.ConsentAtMS != draft.TermsAcceptedAt.UnixMilli() || p.EditorReference != admin || p.AcceptedTextSHA256 != media.ContentHash([]byte(text)) {
			t.Fatal("capture broke accepted source identity", p)
		}
		if again, err := r.CaptureAccepted(ctx, admin, "portal_submission", draft.ID); err != nil || again != p {
			t.Fatal("capture retry changed provenance", again, err)
		}
		profile, err := m.profile.Get(ctx, account, true)
		if err != nil || !reflect.DeepEqual(profile.ContributorCredits, []string{draft.ID}) {
			t.Fatal("expected exactly one original contribution credit", profile, err)
		}
		if balance, err := m.economy.Wallet.Balance(ctx, account); err != nil || balance != int64(c.Noin.ContributorAcceptedAsset) {
			t.Fatalf("approval/capture reward %d %v", balance, err)
		}
		if first == nil {
			first = approved
		}
		accepted++
		acceptedAccounts = append(acceptedAccounts, account)
		return p
	}
	for i := range b.Nowns {
		b.Nowns[i].Provenance = accept(b.Nowns[i].Text)
	}
	for i := range b.Cards {
		b.Cards[i].Provenance = accept(b.Cards[i].Text)
	}
	// Fingerprint every column, including credit arrays, consent and timestamps.
	// Publication may add its own audit records, never change these source rows.
	fingerprint := func() map[string]string {
		t.Helper()
		out := map[string]string{}
		for _, table := range []string{"profiles", "noin_ledger", "portal_submissions", "portal_terms", "user_terms_acceptances"} {
			var hash string
			if err := db.QueryRowContext(ctx, "SELECT md5(COALESCE(string_agg(to_jsonb(t)::text, E'\\n' ORDER BY to_jsonb(t)::text),'')) FROM "+table+" t").Scan(&hash); err != nil {
				t.Fatal(err)
			}
			out[table] = hash
		}
		return out
	}
	before := fingerprint()
	b.Artifacts = map[string][]byte{}
	b.Manifest.CertificationArtifacts = map[string]string{}
	b, err = media.SealTextBundle(b, limits)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := media.NewTextSnapshot(b, limits)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Publish(ctx, admin, snapshot, store.TextPackAccess{Class: "core"}); err == nil {
		t.Fatal("accepted contributions published without release evidence")
	}
	addArtifact := func(name string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		b.Artifacts[name] = data
		b.Manifest.CertificationArtifacts[name] = media.ContentHash(data)
	}
	certificate := media.CertifyText(snapshot, tuning, 20, 42)
	if !certificate.Passed {
		t.Fatal("fixture retained-card certification failed", certificate.Errors)
	}
	addArtifact("technical.json", certificate)
	addArtifact("replay.json", media.NewTextReplay(snapshot, tuning, 20, 42))
	for _, gate := range []string{"editorial", "actions", "screening", "release"} {
		evidence := media.TextGateEvidence{SchemaVersion: 2, SnapshotSHA256: snapshot.SHA256(), RulesVersion: b.Manifest.RulesVersion, Language: b.Manifest.Language, Gate: gate, ActorReference: "test-only-human-double", TuningSHA256: media.TextTuningSHA256(tuning)}
		for _, cell := range certificate.Cells {
			evidence.Cells = append(evidence.Cells, media.TextGateCell{Mode: cell.Mode, TableSize: cell.TableSize, Passed: true, RecordReference: "test-only-record", EvidenceSHA256: media.ContentHash([]byte(fmt.Sprintf("TEST FIXTURE %s/%s/%d", gate, cell.Mode, cell.TableSize)))})
		}
		addArtifact(gate+".json", evidence)
	}
	snapshot, err = media.NewTextSnapshot(b, limits)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = r.Publish(ctx, admin, snapshot, store.TextPackAccess{Class: "core"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = r.Resolve(ctx, "en", b.Manifest.RulesVersion); err == nil {
		t.Fatal("publication implicitly activated content")
	}
	if err = r.Activate(ctx, admin, b.Manifest.ReleaseID); err != nil {
		t.Fatal(err)
	}
	pinned, err := r.Resolve(ctx, "en", b.Manifest.RulesVersion)
	if err != nil || pinned.SHA256() != snapshot.SHA256() {
		t.Fatal("activated bytes differ", err)
	}
	// A reconstructed service uses durable lineage, not a process-local map.
	r = store.NewTextReleaseStore(db, c, func(context.Context, string) error { return nil })
	if err = r.Publish(ctx, admin, snapshot, store.TextPackAccess{Class: "core"}); err != nil {
		t.Fatal(err)
	}
	if err = r.Activate(ctx, admin, b.Manifest.ReleaseID); err != nil {
		t.Fatal(err)
	}
	if restored, err := r.Resolve(ctx, "en", b.Manifest.RulesVersion); err != nil || restored.SHA256() != pinned.SHA256() {
		t.Fatal("restart lost release identity", err)
	}
	if err = r.Takedown(ctx, admin, b.Manifest.ReleaseID, "TEST ONLY"); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Resolve(ctx, "en", b.Manifest.RulesVersion); err == nil {
		t.Fatal("withdrawn release resolved for a new match")
	}
	if !reflect.DeepEqual(pinned.Bundle(), snapshot.Bundle()) {
		t.Fatal("takedown changed already-pinned wording/provenance")
	}
	if after := fingerprint(); !reflect.DeepEqual(before, after) {
		t.Fatal("release lifecycle rewrote approval/credit/ledger/consent", before, after)
	}
	var count, total int
	if err = db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(amount),0) FROM noin_ledger WHERE event_type='contributor_reward' AND account_id=ANY($1::uuid[])`, pq.Array(acceptedAccounts)).Scan(&count, &total); err != nil || count != accepted || total != accepted*c.Noin.ContributorAcceptedAsset {
		t.Fatalf("publication reward replay: count=%d total=%d accepted=%d err=%v", count, total, accepted, err)
	}
}
