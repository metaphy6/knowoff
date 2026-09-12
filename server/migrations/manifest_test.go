package migrations

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"testing/fstest"
)

func TestCompiledManifestHashesExactPairedSQL(t *testing.T) {
	got, err := Compiled()
	if err != nil || got.SchemaVersion < 21 || len(got.Files) != int(got.SchemaVersion)*2 {
		t.Fatal(got, err)
	}
	for _, file := range got.Files {
		raw, err := sqlFiles.ReadFile(file.Name)
		if err != nil || int64(len(raw)) != file.Bytes || fmt.Sprintf("%x", sha256.Sum256(raw)) != file.SHA256 {
			t.Fatal("manifest does not bind SQL bytes", file.Name, err)
		}
	}
}

func TestManifestRejectsIncompleteOrAmbiguousBuildInputs(t *testing.T) {
	for _, names := range [][]string{
		{}, {"000001_init.up.sql"}, {"000001_init.up.sql", "000001_other.down.sql"},
		{"000002_init.up.sql", "000002_init.down.sql"},
		{"000001_init.up.sql", "000001_init.down.sql", "000001_other.up.sql"},
		{"bad.sql"}, {"000000_init.up.sql", "000000_init.down.sql"},
	} {
		fixture := fstest.MapFS{}
		for _, name := range names {
			fixture[name] = &fstest.MapFile{Data: []byte("SELECT 1;\n")}
		}
		if _, err := readManifest(fixture); err == nil {
			t.Fatal("invalid migration set accepted", names)
		}
	}
	fixture := fstest.MapFS{"000001_init.up.sql": {Data: []byte("SELECT 1;\n")}, "000001_init.down.sql": {Data: []byte("SELECT 2;\n")}}
	got, err := readManifest(fixture)
	if err != nil || got.SchemaVersion != 1 || got.Files[0].Name != "000001_init.down.sql" {
		t.Fatal(got, err)
	}
	before := got.SHA256
	fixture["000001_init.up.sql"].Data = []byte("SELECT 3;\n")
	changed, err := readManifest(fixture)
	if err != nil || changed.SHA256 == before {
		t.Fatal("changed SQL retained artifact identity", err)
	}
}
