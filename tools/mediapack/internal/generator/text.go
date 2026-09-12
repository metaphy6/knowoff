package generator

import (
	"fmt"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
)

// SyntheticTextBundle creates a small, isolated engineering fixture, never human
// approval or production content. Counts are fixture structure, not live tuning.
func SyntheticTextBundle(language, rules, release string, limits media.TextLimits) (media.TextBundle, error) {
	if language != "en" && language != "tr" && language != "ar" {
		return media.TextBundle{}, fmt.Errorf("fixture language must be en, tr or ar")
	}
	b := media.TextBundle{Manifest: media.TextManifest{SchemaVersion: media.TextSchemaVersion, ReleaseID: release, Language: language, RulesVersion: rules, Modes: gamecontract.AllModes(), SuitabilityVersion: media.TextSuitabilityVersion, AgeRating: "everyone", Synthetic: true}, Artifacts: map[string][]byte{}}
	provenance := func(id, text string) media.TextProvenance {
		return media.TextProvenance{SourceKind: "synthetic", SourceID: id, SourceRevision: 1, AcceptedTextSHA256: media.ContentHash([]byte(text)), License: "CC0-1.0", Attribution: "Synthetic engineering fixture; no human approval"}
	}
	kindFor := func(mode gamecontract.ModeID) string {
		switch mode {
		case gamecontract.ModeMissedTheBriefing:
			return "situation"
		case gamecontract.ModeSecretScale, gamecontract.ModeTopThat:
			return "criterion"
		default:
			return "plan"
		}
	}
	for _, kind := range []string{"situation", "criterion", "plan"} {
		var modes []gamecontract.ModeID
		for _, mode := range gamecontract.AllModes() {
			if kindFor(mode) == kind {
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
			b.Nowns = append(b.Nowns, media.TextNown{ID: id, Revision: 1, Type: "text", Text: text, Kind: kind, Modes: modes, ToneBucket: "social-awkwardness", Provenance: provenance(id, text)})
		}
	}
	for _, pool := range []string{"response", "item"} {
		var modes []gamecontract.ModeID
		for _, mode := range gamecontract.AllModes() {
			if (pool == "response") == (mode == gamecontract.ModeMissedTheBriefing) {
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
			b.Cards = append(b.Cards, media.TextCard{ID: id, Revision: 1, Type: "text", Text: text, Pool: pool, Modes: modes, ToneBucket: "chaos", Provenance: provenance(id, text)})
		}
	}
	has := func(modes []gamecontract.ModeID, want gamecontract.ModeID) bool {
		for _, mode := range modes {
			if mode == want {
				return true
			}
		}
		return false
	}
	for _, mode := range b.Manifest.Modes {
		for _, n := range b.Nowns {
			if !has(n.Modes, mode) {
				continue
			}
			for i, c := range b.Cards {
				if !has(c.Modes, mode) {
					continue
				}
				b.Suitability = append(b.Suitability, media.TextSuitability{Mode: mode, NownID: n.ID, NownRevision: 1, CardID: c.ID, CardRevision: 1, Band: []string{"high", "distant", "chaos"}[i%3], ReviewReference: "synthetic-fixture-evidence"})
			}
		}
	}
	return media.SealTextBundle(b, limits)
}
