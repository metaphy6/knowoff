package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureMigrations(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"000001_fixture.up.sql", "000001_fixture.down.sql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPreflightCommandHelpWithoutDatabase(t *testing.T) {
	var out, diagnostics bytes.Buffer
	if code := run([]string{"--help"}, func(string) string { return "" }, &out, &diagnostics); code != 0 || !strings.Contains(diagnostics.String(), "usage:") {
		t.Fatal("help must succeed without selecting a database")
	}
}

func TestPreflightCommandRefusalsAndErrorPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		dsn  string
	}{
		{"missing DSN", nil, ""},
		{"zero timeout", []string{"--timeout=0s"}, "private-dsn-value"},
		{"unknown flag", []string{"--not-a-flag"}, "private-dsn-value"},
		{"positional", []string{"unexpected"}, "private-dsn-value"},
		{"missing migrations", []string{"--migrations-dir=" + filepath.Join(t.TempDir(), "absent")}, "private-dsn-value"},
		{"expired context", []string{"--timeout=1ns", "--migrations-dir=" + fixtureMigrations(t)}, "postgres://private-user:private-password@127.0.0.1:1/private-db?sslmode=disable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errors bytes.Buffer
			code := run(tc.args, func(string) string { return tc.dsn }, &out, &errors)
			if code == 0 || out.Len() != 0 || errors.Len() == 0 {
				t.Fatal("invalid invocation must fail without a partial artifact")
			}
			for _, secret := range []string{"private-dsn-value", "private-user", "private-password", "private-db"} {
				if strings.Contains(errors.String(), secret) {
					t.Fatal("private diagnostic escaped")
				}
			}
		})
	}
}

func TestMigrationManifestHashesAndRejectsAmbiguity(t *testing.T) {
	dir := fixtureMigrations(t)
	files, err := migrationManifest(dir)
	if err != nil || len(files) != 2 {
		t.Fatalf("manifest: %v", err)
	}
	hash := sha256.Sum256([]byte("SELECT 1;\n"))
	for _, file := range files {
		if file.SHA256 != hex.EncodeToString(hash[:]) || file.Bytes != 10 {
			t.Fatal("manifest bytes differ")
		}
	}
	if files[0].Name >= files[1].Name {
		t.Fatal("manifest not sorted")
	}
	for _, name := range []string{"000001_duplicate.up.sql", "000002_unpaired.up.sql", "unexpected.sql"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("SELECT 2;"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := migrationManifest(dir); err == nil {
			t.Fatalf("accepted invalid manifest entry %s", name)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	up := filepath.Join(dir, "000001_fixture.up.sql")
	if err := os.Remove(up); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "000001_fixture.down.sql"), up); err != nil {
		t.Fatal(err)
	}
	if _, err := migrationManifest(dir); err == nil {
		t.Fatal("symlinked migration accepted")
	}
}

func TestPreflightCommandReadOnlyArtifact(t *testing.T) {
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	if dsn == "" {
		t.Fatal("run xops/test/tests-lints.py for disposable PostgreSQL")
	}
	var out, errors bytes.Buffer
	if code := run([]string{"--migrations-dir=../../migrations"}, func(string) string { return dsn }, &out, &errors); code != 0 {
		t.Fatalf("preflight exit %d: %s", code, errors.String())
	}
	var report preflightArtifact
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	expectedFiles, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(expectedFiles) < 16 {
		t.Fatalf("migration source inventory: %v", err)
	}
	if report.FormatVersion != 2 || report.ObservedAt.IsZero() || report.Database == nil || report.Database.SchemaSHA256 == "" || len(report.LocalMigrations) != len(expectedFiles) {
		t.Fatal("missing versioned observation or local migration manifest")
	}
	if report.Database.MigrationVersion == nil && report.Database.Inventory.Status != "unavailable_schema" {
		t.Fatal("missing legacy schema must not be reported as measured empty categories")
	}
	for _, kind := range []string{"deployed_clients", "deployed_images", "active_matches", "content_releases", "object_consumers", "paid_benefit_equivalence"} {
		if report.Coverage[kind] != "not_observed" {
			t.Fatalf("database cannot prove %s", kind)
		}
	}
	if strings.Contains(out.String(), dsn) {
		t.Fatal("DSN escaped into artifact")
	}
}
