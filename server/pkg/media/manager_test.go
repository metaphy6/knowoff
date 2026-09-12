package media

import (
	"bytes"
	"testing"
)

func TestManager_ActiveAndAssetBytes(t *testing.T) {
	pack, err := LoadPack("testdata/golden-pack", DefaultDealingTuning())
	if err != nil {
		t.Fatalf("load golden pack: %v", err)
	}
	// The golden bundle is text-only; attach a real synthetic static WebP
	// to exercise the retained asset lookup without a conditional skip.
	data := imageBytes(t, staticWebP)
	ref := ContentHash(data)
	pack.Assets = map[string][]byte{ref: data}

	m := NewManager(pack)
	if got := m.Active(); got == nil {
		t.Fatal("expected active pack")
	}
	if got := m.Active().Manifest.PackTag; got != "test-golden" {
		t.Fatalf("expected pack tag test-golden, got %q", got)
	}

	if got := m.AssetBytes(ref); !bytes.Equal(got, data) {
		t.Fatal("expected exact asset bytes")
	}
	if got := m.AssetBytes("missing"); got != nil {
		t.Fatal("expected nil for missing asset")
	}
}

func TestManager_HotSwapKeepsPreviousPack(t *testing.T) {
	p1, err := LoadPack("testdata/golden-pack", DefaultDealingTuning())
	if err != nil {
		t.Fatalf("load golden pack: %v", err)
	}
	p2, err := LoadPack("testdata/band-starved-pack", DefaultDealingTuning())
	if err != nil {
		t.Fatalf("load band-starved pack: %v", err)
	}

	m := NewManager(p1)
	captured := m.Active()

	m.Load(p2)
	if m.Active().Manifest.PackTag != p2.Manifest.PackTag {
		t.Fatal("expected active pack to be swapped")
	}
	if captured.Manifest.PackTag != p1.Manifest.PackTag {
		t.Fatal("captured pack reference must remain stable")
	}
}
