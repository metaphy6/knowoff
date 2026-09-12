package bundlewriter

import (
	"github.com/knowoff/knowoff/server/pkg/media"
	"github.com/knowoff/knowoff/tools/mediapack/internal/generator"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writerTextFixture(t *testing.T) (media.TextBundle, media.TextLimits) {
	t.Helper()
	limits := media.TextLimits{MaxTextBytes: 512, MaxRecords: 20000, MaxFileBytes: 16 << 20, MaxBundleBytes: 64 << 20}
	bundle, err := generator.SyntheticTextBundle("en", "text-v1", "synthetic-retirement", limits)
	if err != nil {
		t.Fatal(err)
	}
	return bundle, limits
}

func TestWriteRejectsUnsupportedContentBeforeCreatingBundle(t *testing.T) {
	for _, kind := range []string{"image", "gif", "video", "webp"} {
		t.Run(kind, func(t *testing.T) {
			bundle, limits := writerTextFixture(t)
			bundle.Cards[0].Type = kind
			dir := filepath.Join(t.TempDir(), "invalid-pack")
			if err := media.WriteTextBundle(dir, bundle, limits); err == nil {
				t.Fatal("non-text content must be rejected")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("rejected bundle left output behind: %v", err)
			}
		})
	}
}

func TestWriteCanonicalTextPackRemainsLoadableWithoutOverwrite(t *testing.T) {
	bundle, limits := writerTextFixture(t)
	dir := filepath.Join(t.TempDir(), "pack")
	if err := media.WriteTextBundle(dir, bundle, limits); err != nil {
		t.Fatal(err)
	}
	loaded, err := media.LoadTextPack(dir, limits)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Bundle(), bundle) {
		t.Fatal("canonical text/provenance/revisions changed during bundle writing")
	}
	manifest := filepath.Join(dir, "manifest.json")
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := media.WriteTextBundle(dir, bundle, limits); err == nil {
		t.Fatal("existing immutable output must not be overwritten")
	}
	after, err := os.ReadFile(manifest)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed overwrite changed the existing manifest", err)
	}
}

func TestWriteRejectsArtifactPathsAndChecksumConflictsBeforeCreatingBundle(t *testing.T) {
	for _, name := range []string{"../../escape", "/absolute", "technical.json"} {
		t.Run(name, func(t *testing.T) {
			bundle, limits := writerTextFixture(t)
			bundle.Artifacts = map[string][]byte{name: []byte("unreviewed evidence")}
			bundle.Manifest.CertificationArtifacts = map[string]string{name: media.ContentHash([]byte("different evidence"))}
			dir := filepath.Join(t.TempDir(), "invalid-pack")
			if err := media.WriteTextBundle(dir, bundle, limits); err == nil {
				t.Fatal("unsafe artifact name or conflicting hash must be rejected")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("rejected bundle left output behind: %v", err)
			}
		})
	}
}
