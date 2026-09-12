package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// This executable design is test-only. A writer freeze is required from manifest
// capture through verification and cutover. Two cursor passes detect the tested
// intervening writes, but cannot exclude a write racing after final verification.
// Phase 5 must implement and operate that freeze; this is not an online migrator.
var (
	errTransitionConflict    = errors.New("transition fixture mapping conflict")
	errTransitionSourceDrift = errors.New("transition fixture source changed")
	errTransitionInjected    = errors.New("transition fixture injected interruption")
)

const transitionZeroID = "00000000-0000-0000-0000-000000000000"
const transitionZeroHash = "0000000000000000000000000000000000000000000000000000000000000000"
const transitionFormatting = `SET LOCAL TIME ZONE 'UTC'; SET LOCAL DateStyle='ISO, YMD'; SET LOCAL extra_float_digits=3; SET LOCAL search_path=pg_catalog,public; SET LOCAL bytea_output='hex'; SET LOCAL intervalstyle='postgres'`
const transitionSources = `SELECT 'challenge_entry'::text AS kind,id,entry_type AS media_type,to_jsonb(c)::text AS row_json FROM public.challenge_entries c
UNION ALL SELECT 'portal_submission',id,media_type,to_jsonb(p)::text FROM public.portal_submissions p`

type transitionSource struct{ Kind, ID, Hash, MediaType string }
type transitionBatchResult struct {
	Pass      string
	Processed int
	Done      bool
}
type transitionProgress struct {
	Phase, UpperKind, UpperID string
	ExpectedCount             int64
	ExpectedHash              string
	CopyKind, CopyID          string
	CopyCount                 int64
	CopyHash                  string
	VerifyKind, VerifyID      string
	VerifyCount               int64
	VerifyHash                string
}
type transitionLegacyState struct {
	Version        *int64
	Dirty          bool
	Tables         []TableFingerprint
	MigrationFiles map[string]string
}

func transitionDesignDB(t *testing.T, head int, seed bool) *sql.DB {
	t.Helper()
	db := disposableMigrationDB(t) // Validates runner token AND connected DB first.
	reset := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
		return err
	}
	if err := reset(); err != nil {
		t.Fatal(err)
	}
	// Restore the current migration head for unrelated sequential package tests.
	t.Cleanup(func() {
		if err := reset(); err != nil {
			t.Error(err)
			return
		}
		if err := MigrateUp(db, "../../migrations"); err != nil {
			t.Error(err)
		}
	})
	path := transitionMigrationPath(t, head)
	if err := MigrateUp(db, path); err != nil {
		t.Fatal(err)
	}
	if seed {
		body, err := os.ReadFile("testdata/transition-design/legacy.sql")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func transitionReconnect(t *testing.T) *sql.DB { t.Helper(); return disposableMigrationDB(t) }

func transitionLegacySnapshot(t *testing.T, ctx context.Context, db *sql.DB) transitionLegacyState {
	t.Helper()
	report, err := ReadTransitionPreflight(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	result := transitionLegacyState{Version: report.MigrationVersion, Dirty: report.MigrationDirty, MigrationFiles: map[string]string{}}
	for _, table := range report.Tables {
		if !strings.HasPrefix(table.Name, "transition_fixture_") {
			result.Tables = append(result.Tables, table)
		}
	}
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacyFiles := files[:0]
	for _, file := range files {
		if filepath.Base(file)[:6] <= "000008" {
			legacyFiles = append(legacyFiles, file)
		}
	}
	if len(legacyFiles) != 16 {
		t.Fatal("fixture must be reviewed when the eight applied migration pairs change")
	}
	for _, file := range legacyFiles {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(body)
		result.MigrationFiles[filepath.Base(file)] = hex.EncodeToString(hash[:])
	}
	return result
}

func assertTransitionLegacyUnchanged(t *testing.T, before, after transitionLegacyState) {
	t.Helper()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("legacy row fingerprints, migration state or applied SQL changed")
	}
}

func transitionFixtureSnapshot(t *testing.T, ctx context.Context, db *sql.DB) []TableFingerprint {
	t.Helper()
	report, err := ReadTransitionPreflight(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	var result []TableFingerprint
	for _, table := range report.Tables {
		if strings.HasPrefix(table.Name, "transition_fixture_") {
			result = append(result, table)
		}
	}
	return result
}

func transitionFixtureExpand(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count, version int64
	var dirty bool
	if err := tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(min(version),0),COALESCE(bool_or(dirty),false) FROM public.schema_migrations`).Scan(&count, &version, &dirty); err != nil {
		return err
	}
	if count != 1 || version != 8 || dirty {
		return errors.New("fixture requires clean legacy migration 000008 before DDL")
	}
	body, err := os.ReadFile("testdata/transition-design/expand.sql")
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return err
	}
	return tx.Commit()
}

func transitionFixtureDown(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	body, err := os.ReadFile("testdata/transition-design/down.sql")
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return err
	}
	return tx.Commit()
}

// The small design fixture materializes its manifest for assertions. Source reads
// are still bounded by LIMIT. Production capture must stream this same ordered
// accumulator under the writer freeze, rather than retain a corpus in memory.
func transitionSourceManifest(t *testing.T, ctx context.Context, db *sql.DB) []transitionSource {
	t.Helper()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, transitionFormatting); err != nil {
		t.Fatal(err)
	}
	var result []transitionSource
	kind, id := "", transitionZeroID
	for {
		batch, err := transitionSourcePage(ctx, tx, kind, id, "portal_submission", "ffffffff-ffff-ffff-ffff-ffffffffffff", 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(batch) == 0 {
			break
		}
		result = append(result, batch...)
		kind, id = batch[len(batch)-1].Kind, batch[len(batch)-1].ID
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return result
}

func transitionSourcePage(ctx context.Context, tx *sql.Tx, afterKind, afterID, upperKind, upperID string, limit int) ([]transitionSource, error) {
	rows, err := tx.QueryContext(ctx, `SELECT kind,id::text,media_type,row_json FROM (`+transitionSources+`) source
WHERE (kind COLLATE "C",id)>($1::text COLLATE "C",$2::uuid) AND (kind COLLATE "C",id)<=($3::text COLLATE "C",$4::uuid)
ORDER BY kind COLLATE "C",id LIMIT $5`, afterKind, afterID, upperKind, upperID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []transitionSource
	for rows.Next() {
		var row transitionSource
		var body string
		if err := rows.Scan(&row.Kind, &row.ID, &row.MediaType, &body); err != nil {
			return nil, err
		}
		hash := sha256.Sum256([]byte(body))
		row.Hash = hex.EncodeToString(hash[:])
		result = append(result, row)
	}
	return result, rows.Err()
}

// Chain encoding: SHA256(previous 32 digest bytes || uint64-BE key byte length
// || UTF-8 kind+":"+canonical UUID || 32 source digest bytes), starting at zero.
func transitionAccumulate(previous string, row transitionSource) string {
	prior, _ := hex.DecodeString(previous)
	source, _ := hex.DecodeString(row.Hash)
	key := row.Kind + ":" + row.ID
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(key)))
	hash := sha256.New()
	hash.Write(prior)
	hash.Write(size[:])
	hash.Write([]byte(key))
	hash.Write(source)
	return hex.EncodeToString(hash.Sum(nil))
}

func transitionFixtureStart(ctx context.Context, db *sql.DB, manifest []transitionSource) error {
	manifest = append([]transitionSource(nil), manifest...)
	sort.Slice(manifest, func(i, j int) bool {
		if manifest[i].Kind != manifest[j].Kind {
			return manifest[i].Kind < manifest[j].Kind
		}
		return manifest[i].ID < manifest[j].ID
	})
	hash, upperKind, upperID := transitionZeroHash, "", transitionZeroID
	seen := map[string]bool{}
	for _, row := range manifest {
		key := row.Kind + ":" + row.ID
		parsed, idErr := uuid.Parse(row.ID)
		digest, hashErr := hex.DecodeString(row.Hash)
		if seen[key] || (row.Kind != "portal_submission" && row.Kind != "challenge_entry") || idErr != nil || parsed.String() != row.ID || hashErr != nil || len(digest) != 32 || strings.ToLower(row.Hash) != row.Hash || (row.MediaType != "text" && row.MediaType != "image") {
			return errTransitionConflict
		}
		seen[key] = true
		hash = transitionAccumulate(hash, row)
		upperKind, upperID = row.Kind, row.ID
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO transition_fixture_progress(job_id,phase,upper_kind,upper_id,expected_count,expected_sha256,copy_sha256,verify_sha256)
VALUES(1,'copy',$1,$2,$3,$4,$5,$5) ON CONFLICT(job_id) DO NOTHING`, upperKind, upperID, len(manifest), hash, transitionZeroHash); err != nil {
		return err
	}
	p, err := transitionReadProgress(ctx, tx)
	if err != nil {
		return err
	}
	if p.ExpectedCount != int64(len(manifest)) || p.ExpectedHash != hash || p.UpperKind != upperKind || p.UpperID != upperID {
		return errTransitionConflict
	}
	return tx.Commit()
}

func transitionReadProgress(ctx context.Context, tx *sql.Tx) (transitionProgress, error) {
	var p transitionProgress
	err := tx.QueryRowContext(ctx, `SELECT phase,upper_kind,upper_id::text,expected_count,expected_sha256,copy_kind,copy_id::text,copy_count,copy_sha256,verify_kind,verify_id::text,verify_count,verify_sha256 FROM transition_fixture_progress WHERE job_id=1 FOR UPDATE`).Scan(&p.Phase, &p.UpperKind, &p.UpperID, &p.ExpectedCount, &p.ExpectedHash, &p.CopyKind, &p.CopyID, &p.CopyCount, &p.CopyHash, &p.VerifyKind, &p.VerifyID, &p.VerifyCount, &p.VerifyHash)
	return p, err
}

func transitionFixturePut(ctx context.Context, db *sql.DB, row transitionSource) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := transitionInsertMapping(ctx, tx, row); err != nil {
		return err
	}
	return tx.Commit()
}

func transitionInsertMapping(ctx context.Context, tx *sql.Tx, row transitionSource) error {
	var submission, entry any
	if row.Kind == "portal_submission" {
		submission = row.ID
	} else if row.Kind == "challenge_entry" {
		entry = row.ID
	} else {
		return errTransitionConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO transition_fixture_archive(source_kind,source_id,source_sha256,content_id,media_type,submission_id,entry_id)
VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, row.Kind, row.ID, row.Hash, "legacy:"+row.Kind+":"+row.ID, row.MediaType, submission, entry); err != nil {
		return err
	}
	return transitionCheckMapping(ctx, tx, row, errTransitionConflict)
}

func transitionCheckMapping(ctx context.Context, tx *sql.Tx, row transitionSource, mismatch error) error {
	var hash, contentID, mediaType, state string
	err := tx.QueryRowContext(ctx, `SELECT source_sha256,content_id,media_type,archival_state FROM transition_fixture_archive WHERE source_kind=$1 AND source_id=$2`, row.Kind, row.ID).Scan(&hash, &contentID, &mediaType, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return mismatch
	}
	if err != nil {
		return err
	}
	if hash != row.Hash || contentID != "legacy:"+row.Kind+":"+row.ID || mediaType != row.MediaType || state != "legacy_unreviewed" {
		return mismatch
	}
	return nil
}

func transitionFixtureBatch(ctx context.Context, db *sql.DB, limit, failAfter int) (transitionBatchResult, error) {
	var result transitionBatchResult
	if limit < 1 || limit > 1000 {
		return result, errors.New("fixture batch bound must be 1..1000")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, transitionFormatting); err != nil {
		return result, err
	}
	p, err := transitionReadProgress(ctx, tx)
	if err != nil {
		return result, err
	}
	result.Pass = p.Phase
	if p.Phase == "complete" {
		result.Done = true
		return result, tx.Commit()
	}
	var beyond bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM (`+transitionSources+`) source WHERE (kind COLLATE "C",id)>($1::text COLLATE "C",$2::uuid) LIMIT 1)`, p.UpperKind, p.UpperID).Scan(&beyond); err != nil {
		return result, err
	}
	if beyond {
		return result, errTransitionSourceDrift
	}
	kind, id, count, hash := p.CopyKind, p.CopyID, p.CopyCount, p.CopyHash
	if p.Phase == "verify" {
		kind, id, count, hash = p.VerifyKind, p.VerifyID, p.VerifyCount, p.VerifyHash
	}
	batch, err := transitionSourcePage(ctx, tx, kind, id, p.UpperKind, p.UpperID, limit)
	if err != nil {
		return result, err
	}
	for i, row := range batch {
		if p.Phase == "copy" {
			err = transitionInsertMapping(ctx, tx, row)
		} else {
			err = transitionCheckMapping(ctx, tx, row, errTransitionSourceDrift)
		}
		if err != nil {
			return result, err
		}
		count++
		hash = transitionAccumulate(hash, row)
		kind, id = row.Kind, row.ID
		if failAfter == i+1 {
			return result, errTransitionInjected
		}
	}
	if len(batch) == 0 {
		if count != p.ExpectedCount || hash != p.ExpectedHash {
			return result, errTransitionSourceDrift
		}
		if p.Phase == "copy" {
			p.Phase = "verify"
		} else {
			p.Phase = "complete"
			result.Done = true
		}
	}
	if result.Pass == "copy" {
		p.CopyKind, p.CopyID, p.CopyCount, p.CopyHash = kind, id, count, hash
	} else {
		p.VerifyKind, p.VerifyID, p.VerifyCount, p.VerifyHash = kind, id, count, hash
	}
	if _, err := tx.ExecContext(ctx, `UPDATE transition_fixture_progress SET phase=$1,copy_kind=$2,copy_id=$3,copy_count=$4,copy_sha256=$5,verify_kind=$6,verify_id=$7,verify_count=$8,verify_sha256=$9 WHERE job_id=1`, p.Phase, p.CopyKind, p.CopyID, p.CopyCount, p.CopyHash, p.VerifyKind, p.VerifyID, p.VerifyCount, p.VerifyHash); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	result.Processed = len(batch)
	return result, nil
}

func transitionMutateSource(t *testing.T, ctx context.Context, db *sql.DB, kind string, manifest []transitionSource) {
	t.Helper()
	var err error
	switch kind {
	case "changed_behind_cursor":
		_, err = db.ExecContext(ctx, `UPDATE challenge_entries SET content='changed after copy' WHERE id=$1`, manifest[0].ID)
	case "changed_ahead_cursor":
		_, err = db.ExecContext(ctx, `UPDATE portal_submissions SET content='changed before copy' WHERE id=$1`, manifest[len(manifest)-1].ID)
	case "deleted_ahead_cursor":
		_, err = db.ExecContext(ctx, `DELETE FROM portal_submissions WHERE id=$1`, manifest[len(manifest)-1].ID)
	case "late_high_key":
		_, err = db.ExecContext(ctx, `INSERT INTO portal_submissions(id,account_id,media_type,content,terms_version,terms_accepted_at) VALUES('ffffffff-ffff-ffff-ffff-ffffffffffff','00000000-0000-0000-0000-000000000001','text','late source','fixture-legacy-terms','2025-01-01T00:00:00Z')`)
	case "late_low_key":
		_, err = db.ExecContext(ctx, `INSERT INTO accounts(id,nickname) VALUES('00000000-0000-0000-0000-000000000003','fixture-late'); INSERT INTO challenge_entries(id,account_id,topic_id,entry_type,content,terms_version,terms_accepted_at,slot_number) VALUES('00000000-0000-0000-0000-000000000010','00000000-0000-0000-0000-000000000003','20000000-0000-0000-0000-000000000001','text','late low key','fixture-legacy-terms','2025-01-01T00:00:00Z',3)`)
	default:
		t.Fatal("unknown source drift case")
	}
	if err != nil {
		t.Fatal(err)
	}
}

// Pins historical upgrade/controlled-down proofs to exact real SQL bytes even
// when the current application head grows. It never substitutes design fixtures.
func transitionMigrationPath(t *testing.T, head int) string {
	t.Helper()
	path := t.TempDir()
	{
		files, err := filepath.Glob("../../migrations/*.sql")
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			var version int
			if _, err := fmt.Sscanf(filepath.Base(file), "%06d_", &version); err != nil {
				t.Fatal(err)
			}
			if version > head {
				continue
			}
			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, filepath.Base(file)), body, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}

	return path
}
