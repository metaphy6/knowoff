package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseManifestNeedsNoConfigurationOrServices(t *testing.T) {
	t.Setenv("KNOWOFF_CONFIG", filepath.Join(t.TempDir(), "must-not-read.yaml"))
	t.Setenv("KNOWOFF_DB_PASSWORD", "")
	t.Setenv("KNOWOFF_JWT_KEY", "")
	savedArgs, savedStdout := os.Args, os.Stdout
	output, err := os.CreateTemp(t.TempDir(), "manifest")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	os.Args = []string{"knowoffd", "release-manifest"}
	os.Stdout = output
	defer func() { os.Args = savedArgs; os.Stdout = savedStdout }()
	if err = run(); err != nil {
		t.Fatal("read-only manifest attempted config/service startup", err)
	}
	raw, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Format   int      `json:"format_version"`
		Protocol int      `json:"protocol_version"`
		Modes    []string `json:"supported_modes"`
		Database struct {
			Version uint64 `json:"schema_version"`
			Hash    string `json:"manifest_sha256"`
		} `json:"database"`
	}
	if err = json.Unmarshal(raw, &got); err != nil || got.Format != 1 || got.Protocol != 2 || len(got.Modes) != 5 || got.Database.Version < 21 || len(got.Database.Hash) != 64 {
		t.Fatalf("invalid compiled artifact manifest: %+v %v", got, err)
	}
}
