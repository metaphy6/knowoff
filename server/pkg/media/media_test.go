package media

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func TestReviewedBandsRequireActualHighAndDistantCoverage(t *testing.T) {
	b := textFixtureBundle(t, "en")
	for i := range b.Suitability {
		b.Suitability[i].Band = "high"
	}
	b, err := SealTextBundle(b, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewTextSnapshot(b, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	report := CertifyText(snapshot, textFixtureTuning(), 1, 71)
	if report.Passed || len(report.Cells) != 10 {
		t.Fatal("missing distant coverage certified", report)
	}
	for _, cell := range report.Cells {
		if cell.Failures == 0 {
			t.Fatal("infeasible mode/size deal passed", cell)
		}
	}
}

func TestTextManifestAndAcceptedProvenanceCannotBeMissing(t *testing.T) {
	for _, field := range []string{"language", "release", "rules", "provenance"} {
		t.Run(field, func(t *testing.T) {
			b := textFixtureBundle(t, "en")
			switch field {
			case "language":
				b.Manifest.Language = ""
			case "release":
				b.Manifest.ReleaseID = ""
			case "rules":
				b.Manifest.RulesVersion = ""
			case "provenance":
				b.Cards[0].Provenance.AcceptedTextSHA256 = ""
			}
			output := filepath.Join(t.TempDir(), "output")
			if err := WriteTextBundle(output, b, textFixtureLimits()); err == nil {
				t.Fatal("incomplete manifest/provenance wrote bundle")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("invalid bundle left output", err)
			}
		})
	}
}

func TestTextReferencedDataAndEvidenceCannotDisappearOrChange(t *testing.T) {
	for _, name := range []string{"media.jsonl", "cards.jsonl", "suitability.jsonl", "technical.json"} {
		for _, operation := range []string{"missing", "tampered"} {
			t.Run(name+"/"+operation, func(t *testing.T) {
				b := textTestReviewedBundle(t, "retirement-integrity")
				root := retirementTextDirectory(t, b)
				if _, err := LoadTextPack(root, textFixtureLimits()); err != nil {
					t.Fatal(err)
				}
				var err error
				if operation == "missing" {
					err = os.Remove(filepath.Join(root, name))
				} else {
					err = os.WriteFile(filepath.Join(root, name), []byte("{}\n"), 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := LoadTextPack(root, textFixtureLimits()); err == nil {
					t.Fatal("referenced member corruption accepted")
				}
			})
		}
	}
}

func TestRetainedTextDealReplacesLegacySinglePromptDeal(t *testing.T) {
	snapshot, err := LoadTextPack("testdata/text-en", textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			deal, err := snapshot.Deal(mode, size, textFixtureTuning(), TextRandomness{Schedule: 17, Hands: 31, System: 59})
			if err != nil {
				t.Fatal(err)
			}
			again, err := snapshot.Deal(mode, size, textFixtureTuning(), TextRandomness{Schedule: 17, Hands: 31, System: 59})
			if err != nil || !reflect.DeepEqual(deal, again) {
				t.Fatal("retained deal is not replayable", err)
			}
			if len(deal.Hands) != size || len(deal.Nowns) != size/2 {
				t.Fatal("partial table/schedule")
			}
			for _, hand := range deal.Hands {
				if len(hand.Cards) != 5 || len(hand.Reserve) != 3 {
					t.Fatal("retained hand/reserve budget changed")
				}
				retained := append(append([]TextCard(nil), hand.Cards...), hand.Reserve...)
				if err := snapshot.CheckTextCoverage(mode, deal.Nowns, retained, textFixtureTuning()); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
