package media

import (
	"path/filepath"
	"testing"
)

func TestFixtureGolden_LoadsAndCertifies(t *testing.T) {
	pack, err := LoadPack(filepath.Join("testdata", "golden-pack"), defaultDealing())
	if err != nil {
		t.Fatalf("load golden fixture: %v", err)
	}
	cert := Certify(pack, defaultDealing(), defaultHand())
	if !cert.Passed {
		t.Fatalf("golden fixture should certify: %v", cert.Errors)
	}
}

func TestFixtureBandStarved_LoadsAndFailsCertification(t *testing.T) {
	pack, err := LoadPack(filepath.Join("testdata", "band-starved-pack"), defaultDealing())
	if err != nil {
		t.Fatalf("load band-starved fixture: %v", err)
	}
	cert := Certify(pack, defaultDealing(), defaultHand())
	if cert.Passed {
		t.Fatal("band-starved fixture should fail certification")
	}
}
