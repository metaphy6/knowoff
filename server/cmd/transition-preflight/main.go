// transition-preflight prints private aggregate database evidence; it never
// changes schema/data or connects unless KNOWOFF_PREFLIGHT_DSN is explicitly set.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/knowoff/knowoff/server/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

type migrationFile struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type preflightArtifact struct {
	FormatVersion   int                        `json:"format_version"`
	ObservedAt      time.Time                  `json:"observed_at"`
	Database        *store.TransitionPreflight `json:"database"`
	LocalMigrations []migrationFile            `json:"local_migration_files"`
	Coverage        map[string]string          `json:"coverage"`
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("transition-preflight", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // Flag values may contain private operator paths.
	timeout := flags.Duration("timeout", 30*time.Second, "maximum read-only database observation duration")
	migrations := flags.String("migrations-dir", "migrations", "local migration files to fingerprint")
	err := flags.Parse(args)
	if err != nil || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: transition-preflight [--timeout=30s] [--migrations-dir=migrations]")
		fmt.Fprintln(stderr, "--timeout bounds the database observation; local SQL hashing runs first")
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *timeout <= 0 {
		fmt.Fprintln(stderr, "timeout must be positive")
		return 2
	}
	dsn := getenv("KNOWOFF_PREFLIGHT_DSN")
	if dsn == "" {
		fmt.Fprintln(stderr, "set KNOWOFF_PREFLIGHT_DSN to an explicitly selected database with SELECT privileges")
		return 2
	}
	files, err := migrationManifest(*migrations)
	if err != nil {
		fmt.Fprintln(stderr, "preflight: invalid or unreadable local migration manifest")
		return 2
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintln(stderr, "preflight: unable to configure database")
		return 1
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report, err := store.ReadTransitionPreflight(ctx, db)
	if err != nil {
		// Database diagnostics may include a DSN, role name or row values. Never
		// put them in an exported operator artifact or ordinary command log.
		fmt.Fprintln(stderr, "preflight failed; verify database access, SELECT privileges and timeout in private diagnostics")
		return 1
	}
	artifact := preflightArtifact{FormatVersion: 2, ObservedAt: time.Now().UTC(), Database: report, LocalMigrations: files, Coverage: map[string]string{
		"database": "observed", "local_migration_files": "observed",
		"deployed_clients": "not_observed", "deployed_images": "not_observed",
		"active_matches": "not_observed", "content_releases": "not_observed",
		"object_consumers": "not_observed", "paid_benefit_equivalence": "not_observed",
	}}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(artifact); err != nil {
		fmt.Fprintln(stderr, "preflight: unable to write report")
		return 1
	}
	return 0
}

// This manifest describes local files only. A database migration version does
// not establish that these exact bytes were deployed or previously applied.
func migrationManifest(directory string) ([]migrationFile, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	pattern := regexp.MustCompile(`^([0-9]{6})_([a-z0-9_]+)\.(up|down)\.sql$`)
	pairs := make(map[string]map[string]string)
	var files []migrationFile
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := pattern.FindStringSubmatch(entry.Name())
		if parts == nil || !entry.Type().IsRegular() {
			return nil, fmt.Errorf("invalid migration entry")
		}
		pair := pairs[parts[1]]
		if pair == nil {
			pair = make(map[string]string)
			pairs[parts[1]] = pair
		}
		if _, duplicate := pair[parts[3]]; duplicate {
			return nil, fmt.Errorf("duplicate migration version/direction")
		}
		pair[parts[3]] = parts[2]
		file, err := os.Open(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		hash := sha256.New()
		size, readErr := io.Copy(hash, file)
		closeErr := file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		files = append(files, migrationFile{Name: entry.Name(), Bytes: size, SHA256: hex.EncodeToString(hash.Sum(nil))})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("empty migration manifest")
	}
	for _, pair := range pairs {
		if len(pair) != 2 || pair["up"] != pair["down"] {
			return nil, fmt.Errorf("unpaired migration")
		}
	}
	return files, nil
}
