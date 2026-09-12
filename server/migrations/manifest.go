// Package migrations binds the runtime compatibility contract to its build's
// exact SQL files. Reading this manifest never executes a migration.
package migrations

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
)

//go:embed *.sql
var sqlFiles embed.FS

type File struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	SchemaVersion uint64 `json:"schema_version"`
	Files         []File `json:"migration_files"`
	SHA256        string `json:"manifest_sha256"`
}

func Compiled() (Manifest, error) { return readManifest(sqlFiles) }

func readManifest(source fs.FS) (Manifest, error) {
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return Manifest{}, err
	}
	pattern := regexp.MustCompile(`^([0-9]{6})_([a-z0-9_]+)\.(up|down)\.sql$`)
	pairs := map[uint64]map[string]string{}
	result := Manifest{Files: []File{}}
	for _, entry := range entries {
		parts := pattern.FindStringSubmatch(entry.Name())
		if parts == nil || !entry.Type().IsRegular() {
			return Manifest{}, fmt.Errorf("invalid compiled migration entry")
		}
		version, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil || version == 0 {
			return Manifest{}, fmt.Errorf("invalid compiled migration version")
		}
		if pairs[version] == nil {
			pairs[version] = map[string]string{}
		}
		if _, exists := pairs[version][parts[3]]; exists {
			return Manifest{}, fmt.Errorf("duplicate compiled migration direction")
		}
		pairs[version][parts[3]] = parts[2]
		raw, err := fs.ReadFile(source, entry.Name())
		if err != nil {
			return Manifest{}, err
		}
		sum := sha256.Sum256(raw)
		result.Files = append(result.Files, File{Name: entry.Name(), Bytes: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])})
		if version > result.SchemaVersion {
			result.SchemaVersion = version
		}
	}
	if result.SchemaVersion == 0 || result.SchemaVersion != uint64(len(pairs)) {
		return Manifest{}, fmt.Errorf("empty or gapped compiled migrations")
	}
	for _, pair := range pairs {
		if len(pair) != 2 || pair["up"] != pair["down"] {
			return Manifest{}, fmt.Errorf("unpaired compiled migration")
		}
	}
	raw, err := json.Marshal(result.Files)
	if err != nil {
		return Manifest{}, err
	}
	sum := sha256.Sum256(raw)
	result.SHA256 = hex.EncodeToString(sum[:])
	return result, nil
}
