package media

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func TestTextNormalizationPreservesLanguageAndRejectsSpoofing(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"  I İ ı i  ", "I İ ı i"}, {"cafe\u0301", "café"}, {"شاي وقهوة", "شاي وقهوة"}, {"**plain** & <b>text</b>", "**plain** & <b>text</b>"}} {
		got, err := NormalizeText(tc.in, 512)
		if err != nil || got != tc.want {
			t.Fatalf("language-safe normalization: %q %v", got, err)
		}
	}
	for _, raw := range []string{"", " \t ", string([]byte{0xff}), "hidden\u202Etext", "zero\u200Bwidth", "null\x00text", "line\nspoof", strings.Repeat("ş", 257), "https://example.invalid/asset"} {
		if _, err := NormalizeText(raw, 512); err == nil {
			t.Fatalf("unsafe text accepted: %q", raw)
		}
	}
	if _, err := NormalizeText("word", 0); err == nil {
		t.Fatal("missing configured bound accepted")
	}
}

func TestTextIngestionRejectsEscapedUnpairedSurrogates(t *testing.T) {
	for _, raw := range []string{`"\ud800"`, `"\udfff"`, `"\ud800\u0041"`, `"\ud800x"`} {
		var decoded string
		if err := textStrictJSON([]byte(raw), &decoded); err == nil {
			t.Fatalf("malformed escaped Unicode became %q", decoded)
		}
	}
	var emoji string
	if err := textStrictJSON([]byte(`"\ud83d\ude00"`), &emoji); err != nil || emoji != "😀" {
		t.Fatalf("valid emoji pair rejected: %q %v", emoji, err)
	}
	b := textFixtureBundle(t, "en")
	b.Cards[0].Text = "�"
	b.Cards[0].Provenance.AcceptedTextSHA256 = ContentHash([]byte("�"))
	raw, _ := json.Marshal(b)
	raw = []byte(strings.Replace(string(raw), "�", `\ud800`, 1))
	if _, err := DecodeTextBundle(raw, textFixtureLimits()); err == nil {
		t.Fatal("candidate ingestion repaired malformed Unicode silently")
	}
}

func TestTextSnapshotIntegrityAndImmutability(t *testing.T) {
	bundle := textFixtureBundle(t, "en")
	limits := textFixtureLimits()
	snapshot, err := NewTextSnapshot(bundle, limits)
	if err != nil {
		t.Fatal(err)
	}
	before, ok := snapshot.Nown(bundle.Nowns[0].ID)
	if !ok {
		t.Fatal("missing pinned nown")
	}
	bundle.Nowns[0].Text = "caller mutation"
	bundle.Nowns[0].Modes[0] = gamecontract.ModeTopThat
	got, _ := snapshot.Nown(before.ID)
	got.Text = "lookup mutation"
	got.Modes[0] = gamecontract.ModeTopThat
	again, _ := snapshot.Nown(before.ID)
	if !reflect.DeepEqual(again, before) {
		t.Fatal("caller changed immutable snapshot")
	}
	manifest := snapshot.Manifest()
	manifest.Modes[0] = gamecontract.ModeTopThat
	manifest.Checksums["media.jsonl"] = "tamper"
	if snapshot.Manifest().Checksums["media.jsonl"] == "tamper" {
		t.Fatal("manifest exposed mutable storage")
	}
	lineage := snapshot.Lineage()
	lineage.Revisions[0].SHA256 = "mutated"
	if snapshot.Lineage().Revisions[0].SHA256 == "mutated" {
		t.Fatal("lineage exposed mutable storage")
	}
}

func TestTextCheckedInFixtures(t *testing.T) {
	for _, language := range []string{"en", "tr", "ar"} {
		snapshot, err := LoadTextPack(filepath.Join("testdata", "text-"+language), textFixtureLimits())
		if err != nil {
			t.Fatal(err)
		}
		if !snapshot.Manifest().Synthetic || snapshot.Manifest().RulesVersion != "text-v1" || snapshot.Manifest().Language != language {
			t.Fatal("checked-in fixture lost its isolation/contract")
		}
		for _, mode := range gamecontract.AllModes() {
			for _, size := range []int{4, 6} {
				if _, err := snapshot.Deal(mode, size, textFixtureTuning(), TextRandomness{1, 2, 3}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestTextLoaderStrictBoundedMembers(t *testing.T) {
	for _, name := range []string{"valid", "tamper", "unknown_field", "duplicate_key", "unknown_member", "symlink", "oversized_file", "missing_member"} {
		t.Run(name, func(t *testing.T) {
			bundle := textFixtureBundle(t, "tr")
			root := filepath.Join(t.TempDir(), "pack")
			limits := textFixtureLimits()
			if err := WriteTextBundle(root, bundle, limits); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "tamper":
				if err := os.WriteFile(filepath.Join(root, "media.jsonl"), []byte("{}\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unknown_field", "duplicate_key":
				raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
				if err != nil {
					t.Fatal(err)
				}
				field := `"unexpected":true,`
				if name == "duplicate_key" {
					field = `"schema_version":2,`
				}
				raw = append([]byte("{"+field), raw[1:]...)
				if err := os.WriteFile(filepath.Join(root, "manifest.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "unknown_member":
				if err := os.WriteFile(filepath.Join(root, "image.png"), []byte("not playable"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(filepath.Join(root, "cards.jsonl")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("manifest.json", filepath.Join(root, "cards.jsonl")); err != nil {
					t.Fatal(err)
				}
			case "oversized_file":
				limits.MaxFileBytes = 10
			case "missing_member":
				if err := os.Remove(filepath.Join(root, "suitability.jsonl")); err != nil {
					t.Fatal(err)
				}
			}
			_, err := LoadTextPack(root, limits)
			if name == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid bundle accepted")
			}
		})
	}
}

func TestTextSnapshotRejectsUnreviewedOrIncompatibleRecords(t *testing.T) {
	for _, name := range []string{"nontext", "duplicate_id", "wrong_revision", "wrong_pool", "wrong_kind", "wrong_language", "missing_relation", "bad_provenance", "noncanonical_text", "traversal", "unknown_version"} {
		t.Run(name, func(t *testing.T) {
			b := textFixtureBundle(t, "en")
			switch name {
			case "nontext":
				b.Cards[0].Type = "image"
			case "duplicate_id":
				b.Cards = append(b.Cards, b.Cards[0])
			case "wrong_revision":
				b.Suitability[0].CardRevision++
			case "wrong_pool":
				b.Cards[0].Pool = "item"
			case "wrong_kind":
				b.Nowns[0].Kind = "plan"
			case "wrong_language":
				b.Manifest.Language = "EN_us"
			case "missing_relation":
				b.Suitability = b.Suitability[1:]
			case "bad_provenance":
				b.Cards[0].Provenance.AcceptedTextSHA256 = strings.Repeat("0", 64)
			case "noncanonical_text":
				b.Cards[0].Text = " padded "
			case "traversal":
				b.Manifest.CertificationArtifacts = map[string]string{"../outside.json": strings.Repeat("0", 64)}
			case "unknown_version":
				b.Manifest.SchemaVersion++
			}
			if _, err := SealTextBundle(b, textFixtureLimits()); err == nil {
				t.Fatal("invalid record contract accepted")
			}
		})
	}
}

func TestTextDuplicateReviewIsExplicit(t *testing.T) {
	b := textFixtureBundle(t, "tr")
	b.Cards[1].Text = b.Cards[0].Text
	b.Cards[1].Provenance.AcceptedTextSHA256 = ContentHash([]byte(b.Cards[1].Text))
	if _, err := SealTextBundle(b, textFixtureLimits()); err == nil {
		t.Fatal("unreviewed canonical duplicate accepted")
	}
	b.Manifest.DuplicateReviews = []TextDuplicateReview{{FirstID: b.Cards[0].ID, SecondID: b.Cards[1].ID, ReviewReference: "synthetic-duplicate-review"}}
	sealed, err := SealTextBundle(b, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Cards[1].Text != b.Cards[0].Text {
		t.Fatal("duplicate review rewrote accepted content")
	}
	encoded, _ := json.Marshal(sealed)
	if !json.Valid(encoded) {
		t.Fatal("text is not safe JSON display data")
	}
}
