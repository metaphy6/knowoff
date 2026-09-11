package bundlewriter

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/knowoff/knowoff/server/pkg/media"
	"github.com/knowoff/knowoff/tools/mediapack/internal/generator"
)

func TestWriteRejectsUnsupportedContentBeforeCreatingBundle(t *testing.T) {
	pack, err := generator.SyntheticPack("format-test", 3, 60, 1)
	if err != nil {
		t.Fatal(err)
	}
	pack.Cards[0].Type = media.MediaType("gif")
	dir := filepath.Join(t.TempDir(), "invalid-pack")
	if err := Write(pack, dir); err == nil {
		t.Fatal("unsupported card format must be rejected")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("rejected bundle left output behind: %v", err)
	}
}

func TestWriteImageAndTextPackRemainsLoadable(t *testing.T) {
	pack, err := generator.SyntheticPack("format-test", 3, 60, 1)
	if err != nil {
		t.Fatal(err)
	}
	image, err := base64.StdEncoding.DecodeString("UklGRhwAAABXRUJQVlA4TA8AAAAvAUAAAAcQ/Y/+ByKi/wEA")
	if err != nil {
		t.Fatal(err)
	}
	ref := media.ContentHash(image)
	pack.Assets = map[string][]byte{ref: image}
	pack.Media[0].Type, pack.Media[0].AssetRef = media.MediaTypeImage, ref
	pack.Cards[0].Type, pack.Cards[0].AssetRef = media.MediaTypeImage, ref
	dir := t.TempDir()
	if err := Write(pack, dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := media.LoadPack(dir, media.DefaultDealingTuning())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Media) != len(pack.Media) || len(loaded.Cards) != len(pack.Cards) {
		t.Fatal("supported content changed during bundle writing")
	}
	if !bytes.Equal(loaded.Assets[ref], image) || loaded.Media[0].Type != media.MediaTypeImage || loaded.Cards[0].Type != media.MediaTypeImage {
		t.Fatal("static image bytes and types must survive the bundle round trip")
	}
}

func TestWriteRejectsAssetPathsBeforeCreatingBundle(t *testing.T) {
	pack, err := generator.SyntheticPack("format-test", 3, 60, 1)
	if err != nil {
		t.Fatal(err)
	}
	image, err := base64.StdEncoding.DecodeString("UklGRhwAAABXRUJQVlA4TA8AAAAvAUAAAAcQ/Y/+ByKi/wEA")
	if err != nil {
		t.Fatal(err)
	}
	pack.Assets = map[string][]byte{"../../escape": image}
	dir := filepath.Join(t.TempDir(), "invalid-pack")
	if err := Write(pack, dir); err == nil {
		t.Fatal("asset keys must be content hashes, never paths")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("rejected bundle left output behind: %v", err)
	}
}
