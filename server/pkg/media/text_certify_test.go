package media

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTextCertificationReproducibleAndHonest(t *testing.T) {
	snapshot, err := NewTextSnapshot(textFixtureBundle(t, "en"), textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	first := CertifyText(snapshot, textFixtureTuning(), 20, 71)
	second := CertifyText(snapshot, textFixtureTuning(), 20, 71)
	if !reflect.DeepEqual(first, second) || !first.Passed {
		t.Fatalf("technical replay failed: %+v", first)
	}
	if first.ProductionReady || first.Scope != "sampled-complete-schedule-retained-cards" || first.SnapshotSHA256 != snapshot.SHA256() || len(first.Cells) != 10 {
		t.Fatal("certificate misstated proof scope")
	}
	for _, cell := range first.Cells {
		if cell.Deals != 20 || cell.Failures != 0 || cell.UnobservedCards != 0 {
			t.Fatal("incorrect simulation denominators/reachability")
		}
	}
	if CertifyText(snapshot, textFixtureTuning(), 0, 71).Passed {
		t.Fatal("zero-sample certification accepted")
	}
}

func TestTextActivationGateTampering(t *testing.T) {
	for _, name := range []string{"missing-replay", "seed", "samples", "tuning", "action_tuning", "missing_cell", "duplicate_cell", "forged_counts"} {
		t.Run(name, func(t *testing.T) {
			b := textTestReviewedBundle(t, "tamper-test")
			mutate := func(file string, change func(map[string]any)) {
				var value map[string]any
				if err := json.Unmarshal(b.Artifacts[file], &value); err != nil {
					t.Fatal(err)
				}
				change(value)
				raw, _ := json.Marshal(value)
				b.Artifacts[file] = raw
				b.Manifest.CertificationArtifacts[file] = ContentHash(raw)
			}
			switch name {
			case "missing-replay":
				delete(b.Artifacts, "replay.json")
				delete(b.Manifest.CertificationArtifacts, "replay.json")
			case "seed":
				mutate("replay.json", func(v map[string]any) { v["seed"] = 999 })
			case "samples":
				mutate("replay.json", func(v map[string]any) { v["samples"] = 0 })
			case "tuning":
				mutate("replay.json", func(v map[string]any) { v["tuning"].(map[string]any)["HandSize"] = 4 })
			case "action_tuning":
				mutate("actions.json", func(v map[string]any) { v["tuning_sha256"] = ContentHash([]byte("other hand settings")) })
			case "missing_cell":
				mutate("editorial.json", func(v map[string]any) { v["cells"] = v["cells"].([]any)[1:] })
			case "duplicate_cell":
				mutate("screening.json", func(v map[string]any) { cells := v["cells"].([]any); cells[1] = cells[0] })
			case "forged_counts":
				mutate("technical.json", func(v map[string]any) { v["cells"].([]any)[0].(map[string]any)["distinct_schedules"] = 999 })
			}
			snapshot, err := NewTextSnapshot(b, textFixtureLimits())
			if err != nil {
				t.Fatal(err)
			}
			if err := snapshot.ValidateActivation("text-v2", textFixtureTuning()); err == nil {
				t.Fatal("tampered/missing release evidence accepted")
			}
		})
	}
}

func TestTextActivationRequiresEvidenceAndPreservesPins(t *testing.T) {
	manager := new(TextManager)
	synthetic, _ := NewTextSnapshot(textFixtureBundle(t, "en"), textFixtureLimits())
	if err := manager.Activate(synthetic, "text-v2", textFixtureTuning()); err == nil {
		t.Fatal("synthetic fixture activated for production")
	}
	bundle := textTestReviewedBundle(t, "release-one")
	first, err := NewTextSnapshot(bundle, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(first, "text-v2", textFixtureTuning()); err != nil {
		t.Fatal(err)
	}
	pinned := manager.Active()
	before, _ := pinned.Nown(bundle.Nowns[0].ID)
	changedEvidence := textClone(bundle)
	var changedGate TextGateEvidence
	if err := json.Unmarshal(changedEvidence.Artifacts["release.json"], &changedGate); err != nil {
		t.Fatal(err)
	}
	changedGate.ActorReference = "different-test-reviewer"
	changedEvidence.Artifacts["release.json"], _ = json.Marshal(changedGate)
	changedEvidence.Manifest.CertificationArtifacts["release.json"] = ContentHash(changedEvidence.Artifacts["release.json"])
	changedSnapshot, err := NewTextSnapshot(changedEvidence, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(changedSnapshot, "text-v2", textFixtureTuning()); err == nil || manager.Active() != pinned {
		t.Fatal("immutable release artifacts were replaced under the same release ID")
	}
	bad := textTestReviewedBundle(t, "release-bad")
	delete(bad.Artifacts, "editorial.json")
	delete(bad.Manifest.CertificationArtifacts, "editorial.json")
	bad, _ = SealTextBundle(bad, textFixtureLimits())
	invalid, _ := NewTextSnapshot(bad, textFixtureLimits())
	if err := manager.Activate(invalid, "text-v2", textFixtureTuning()); err == nil || manager.Active() != pinned {
		t.Fatal("failed activation replaced prior release")
	}
	secondBundle := textTestReviewedBundle(t, "release-two")
	secondBundle.Nowns[0].Text = "Changed accepted wording"
	secondBundle.Nowns[0].Provenance.AcceptedTextSHA256 = ContentHash([]byte(secondBundle.Nowns[0].Text))
	secondBundle = textTestEvidence(t, secondBundle)
	redefined, _ := NewTextSnapshot(secondBundle, textFixtureLimits())
	if err := manager.Activate(redefined, "text-v2", textFixtureTuning()); err == nil {
		t.Fatal("same content revision was redefined")
	}
	secondBundle.Nowns[0].Revision++
	secondBundle.Nowns[0].Provenance.SourceRevision++
	for i := range secondBundle.Suitability {
		if secondBundle.Suitability[i].NownID == secondBundle.Nowns[0].ID {
			secondBundle.Suitability[i].NownRevision++
		}
	}
	secondBundle = textTestEvidence(t, secondBundle)
	second, err := NewTextSnapshot(secondBundle, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(second, "text-v2", textFixtureTuning()); err != nil {
		t.Fatal(err)
	}
	if got, _ := pinned.Nown(before.ID); !reflect.DeepEqual(got, before) {
		t.Fatal("mid-match activation reworded pinned history")
	}
	if err := manager.Activate(first, "text-v2", textFixtureTuning()); err != nil {
		t.Fatal("compatible certified rollback:", err)
	}
	if err := manager.Activate(first, "other-rules", textFixtureTuning()); err == nil {
		t.Fatal("incompatible rollback accepted")
	}
}

// Simulated evidence is confined to this unit test. It is not a human record,
// production approval, published artifact or evidence for enabling a mode.
func textTestReviewedBundle(t *testing.T, release string) TextBundle {
	t.Helper()
	b := textFixtureBundle(t, "en")
	b.Manifest.ReleaseID = release
	b.Manifest.Synthetic = false
	fill := func(p *TextProvenance) {
		p.SourceKind = "contribution"
		p.TermsVersion = "test-terms"
		p.ConsentReference = "test-consent"
		p.ConsentAtMS = 1
		p.ApprovalReference = "test-approval"
		p.EditorReference = "test-editor"
		p.ReviewedAtMS = 2
	}
	for i := range b.Nowns {
		fill(&b.Nowns[i].Provenance)
	}
	for i := range b.Cards {
		fill(&b.Cards[i].Provenance)
	}
	return textTestEvidence(t, b)
}

func textTestEvidence(t *testing.T, b TextBundle) TextBundle {
	t.Helper()
	b.Manifest.CertificationArtifacts = nil
	b.Artifacts = map[string][]byte{}
	sealed, err := SealTextBundle(b, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewTextSnapshot(sealed, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	sealed.Manifest.CertificationArtifacts = map[string]string{}
	technical := CertifyText(snapshot, textFixtureTuning(), 20, 71)
	raw, _ := json.Marshal(technical)
	sealed.Artifacts["technical.json"] = raw
	replayRaw, _ := json.Marshal(NewTextReplay(snapshot, textFixtureTuning(), 20, 71))
	sealed.Artifacts["replay.json"] = replayRaw
	for _, gate := range []string{"editorial", "actions", "screening", "release"} {
		evidence := TextGateEvidence{SchemaVersion: 2, SnapshotSHA256: snapshot.SHA256(), RulesVersion: "text-v2", Language: "en", Gate: gate, ActorReference: "simulated-test-reviewer", TuningSHA256: TextTuningSHA256(textFixtureTuning())}
		for _, mode := range b.Manifest.Modes {
			for _, size := range []int{4, 6} {
				evidence.Cells = append(evidence.Cells, TextGateCell{Mode: mode, TableSize: size, Passed: true, RecordReference: "simulated-test-record", EvidenceSHA256: ContentHash([]byte("simulated unit-test evidence"))})
			}
		}
		raw, _ := json.Marshal(evidence)
		sealed.Artifacts[gate+".json"] = raw
	}
	for name, raw := range sealed.Artifacts {
		sealed.Manifest.CertificationArtifacts[name] = ContentHash(raw)
	}
	sealed, err = SealTextBundle(sealed, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
