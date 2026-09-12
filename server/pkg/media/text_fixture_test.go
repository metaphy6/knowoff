package media

import (
	"fmt"
	"testing"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func textFixtureLimits() TextLimits {
	return TextLimits{MaxTextBytes: 512, MaxRecords: 20000, MaxFileBytes: 16777216, MaxBundleBytes: 67108864}
}
func textFixtureTuning() TextDealTuning {
	return TextDealTuning{HandSize: 5, ReserveSize: 3, MinHigh: 2, MinDistant: 2, MaxSearchNodes: 100000}
}
func textFixtureBundle(t *testing.T, language string) TextBundle {
	t.Helper()
	b := TextBundle{Manifest: TextManifest{SchemaVersion: TextSchemaVersion, ReleaseID: "synthetic-text-" + language, Language: language, RulesVersion: "text-v2", Modes: gamecontract.AllModes(), SuitabilityVersion: TextSuitabilityVersion, AgeRating: "everyone", Synthetic: true}, Artifacts: map[string][]byte{}}
	provenance := func(id, text string) TextProvenance {
		return TextProvenance{SourceKind: "synthetic", SourceID: id, SourceRevision: 1, AcceptedTextSHA256: ContentHash([]byte(text)), License: "CC0-1.0", Attribution: "Synthetic engineering fixture; no human approval"}
	}
	for _, kind := range []string{"situation", "criterion", "plan"} {
		var modes []gamecontract.ModeID
		for _, mode := range gamecontract.AllModes() {
			expected, _ := textModeKind(mode)
			if expected == kind {
				modes = append(modes, mode)
			}
		}
		for i := 0; i < 3; i++ {
			id := fmt.Sprintf("%s-%d", kind, i)
			text := fmt.Sprintf("Synthetic %s number %d", kind, i)
			if language == "tr" {
				text = fmt.Sprintf("Deneme %s çayı %d", kind, i)
			}
			if language == "ar" {
				text = fmt.Sprintf("تجربة %s %d", kind, i)
			}
			b.Nowns = append(b.Nowns, TextNown{ID: id, Revision: 1, Type: "text", Text: text, Kind: kind, Modes: modes, ToneBucket: "social-awkwardness", Provenance: provenance(id, text)})
		}
	}
	for _, pool := range []string{"response", "item"} {
		var modes []gamecontract.ModeID
		for _, mode := range gamecontract.AllModes() {
			_, expected := textModeKind(mode)
			if expected == pool {
				modes = append(modes, mode)
			}
		}
		for i := 0; i < 12; i++ {
			id := fmt.Sprintf("%s-%02d", pool, i)
			text := fmt.Sprintf("Synthetic %s %02d", pool, i)
			if language == "tr" {
				text = fmt.Sprintf("Deneme %s çay %02d", pool, i)
			}
			if language == "ar" {
				text = fmt.Sprintf("بطاقة %s %02d", pool, i)
			}
			b.Cards = append(b.Cards, TextCard{ID: id, Revision: 1, Type: "text", Text: text, Pool: pool, Modes: modes, ToneBucket: "chaos", Provenance: provenance(id, text)})
		}
	}
	for _, mode := range b.Manifest.Modes {
		for _, n := range b.Nowns {
			if !textHasMode(n.Modes, mode) {
				continue
			}
			for i, c := range b.Cards {
				if !textHasMode(c.Modes, mode) {
					continue
				}
				b.Suitability = append(b.Suitability, TextSuitability{Mode: mode, NownID: n.ID, NownRevision: 1, CardID: c.ID, CardRevision: 1, Band: []string{"high", "distant", "chaos"}[i%3], ReviewReference: "synthetic-fixture-evidence"})
			}
		}
	}
	sealed, err := SealTextBundle(b, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
