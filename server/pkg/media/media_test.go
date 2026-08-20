package media

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestCosineIdentical(t *testing.T) {
	v := []float32{1, 0, 0, 0}
	if got := Cosine(v, v); math.Abs(got-1) > 1e-6 {
		t.Fatalf("expected 1, got %f", got)
	}
}

func TestCosineOrthogonal(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{0, 1}
	if got := Cosine(a, b); math.Abs(got) > 1e-6 {
		t.Fatalf("expected 0, got %f", got)
	}
}

func TestBandFor(t *testing.T) {
	d := DealingTuning{BandHigh: 0.55, BandLow: 0.30}
	if BandFor(0.7, d) != "high" {
		t.Errorf("expected high")
	}
	if BandFor(0.4, d) != "distant" {
		t.Errorf("expected distant")
	}
	if BandFor(0.1, d) != "chaos" {
		t.Errorf("expected chaos")
	}
}

func TestCertify_GoldenPackPasses(t *testing.T) {
	pack := buildGoldenPack(t)
	cert := Certify(pack, defaultDealing(), defaultHand())
	if !cert.Passed {
		t.Fatalf("golden pack should certify: %v", cert.Errors)
	}
}

func TestCertify_BandStarvedPackFails(t *testing.T) {
	pack := buildBandStarvedPack(t)
	cert := Certify(pack, defaultDealing(), defaultHand())
	if cert.Passed {
		t.Fatal("band-starved pack should fail certification")
	}
	found := false
	for _, e := range cert.Errors {
		if contains(e, "band coverage") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected band coverage error, got: %v", cert.Errors)
	}
}

func TestCertify_ManifestIncompleteFails(t *testing.T) {
	pack := buildGoldenPack(t)
	pack.Manifest.Language = ""
	cert := Certify(pack, defaultDealing(), defaultHand())
	if cert.Passed {
		t.Fatal("manifest-incomplete pack should fail certification")
	}
}

func TestLoadPack_TamperedAssetRefused(t *testing.T) {
	dir := t.TempDir()
	writeGoldenBundle(t, dir)

	// Tamper with a dummy asset file.
	assetsDir := filepath.Join(dir, "assets")
	entries, err := os.ReadDir(assetsDir)
	if err != nil {
		t.Fatalf("read assets: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no assets to tamper")
	}
	target := filepath.Join(assetsDir, entries[0].Name())
	if err := os.WriteFile(target, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper asset: %v", err)
	}

	_, err = LoadPack(dir, defaultDealing())
	if err == nil {
		t.Fatal("expected error for tampered asset")
	}
	if !contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected hash mismatch error, got: %v", err)
	}
}

func TestLoadPack_MissingMediaAssetRefused(t *testing.T) {
	dir := t.TempDir()
	writeBundleWithAssetRef(t, dir)

	// Load to discover the referenced asset hash, then remove it.
	pack, err := LoadPack(dir, defaultDealing())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var ref string
	for _, m := range pack.Media {
		if m.AssetRef != "" {
			ref = m.AssetRef
			break
		}
	}
	if ref == "" {
		t.Fatal("need media with asset_ref")
	}
	if err := os.Remove(filepath.Join(dir, "assets", ref)); err != nil {
		t.Fatalf("remove asset: %v", err)
	}

	_, err = LoadPack(dir, defaultDealing())
	if err == nil {
		t.Fatal("expected error for missing asset")
	}
}

func TestDealer_DealSatisfiesConstraints(t *testing.T) {
	pack := buildGoldenPack(t)
	dealer := &Dealer{Pack: pack, Dealing: defaultDealing(), Hand: defaultHand()}
	nowns := []string{pack.Media[0].ID, pack.Media[1].ID}
	res, err := dealer.Deal(4, nowns, newDeterministicRng(t, 1))
	if err != nil {
		t.Fatalf("deal failed: %v", err)
	}
	if len(res.Hands) != 4 {
		t.Fatalf("expected 4 hands, got %d", len(res.Hands))
	}
	for i, h := range res.Hands {
		if len(h.Cards) != defaultHand().Size {
			t.Errorf("hand %d has %d cards, want %d", i, len(h.Cards), defaultHand().Size)
		}
		if len(h.DrawPile) != defaultHand().DrawPile {
			t.Errorf("hand %d draw pile has %d cards, want %d", i, len(h.DrawPile), defaultHand().DrawPile)
		}
	}
}

func defaultDealing() DealingTuning {
	return DealingTuning{BandHigh: 0.55, BandLow: 0.30, MinHighPerNown: 2, MinDistantPerNown: 2}
}

func defaultHand() HandTuning {
	return HandTuning{Size: 5, DrawPile: 3}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
