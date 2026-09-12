package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	"github.com/lib/pq"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTextArchiveActualUpgradeBoundedResumeAndParity(t *testing.T) {
	db := transitionDesignDB(t, 8, true)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	before := transitionLegacySnapshot(t, ctx, db)
	manifest := transitionSourceManifest(t, ctx, db)
	if err := MigrateUp(db, transitionMigrationPath(t, 11)); err != nil {
		t.Fatal(err)
	}
	a := NewTextArchiveStore(db)
	if err := a.Start(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(ctx, 2); err != nil {
		t.Fatal("start retry", err)
	}
	// The durable capture freezes both source writers, including lower UUID keys.
	if _, err := db.Exec(`UPDATE portal_submissions SET content=content||'changed'`); err == nil {
		t.Fatal("source not frozen during copy")
	}
	first, err := a.Batch(ctx, 2)
	if err != nil || first.Processed != 2 {
		t.Fatalf("first batch %+v %v", first, err)
	}
	injected := errors.New("commit injection")
	a.beforeCommit = func() error { return injected }
	if _, err = a.Batch(ctx, 2); !errors.Is(err, injected) {
		t.Fatal("missing injected rollback", err)
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_legacy_archive`); n != 2 {
		t.Fatal("partial batch committed", n)
	}
	a = NewTextArchiveStore(transitionReconnect(t))
	counts := map[string]int{"copy": 2}
	for i := 0; i < 30; i++ {
		step, err := a.Batch(ctx, 2)
		if err != nil {
			t.Fatal(err)
		}
		if step.Processed > 2 {
			t.Fatal("unbounded page")
		}
		counts[step.Pass] += step.Processed
		if step.Done {
			break
		}
		if i == 29 {
			t.Fatal("resume did not complete")
		}
	}
	if counts["copy"] != len(manifest) || counts["verify"] != len(manifest) {
		t.Fatal(counts)
	}
	if step, err := a.Batch(ctx, 2); err != nil || !step.Done || step.Processed != 0 {
		t.Fatalf("completed replay %+v %v", step, err)
	}
	after := transitionLegacySnapshot(t, ctx, db)
	byName := map[string]TableFingerprint{}
	for _, table := range after.Tables {
		byName[table.Name] = table
	}
	if after.Version == nil || *after.Version != 11 || after.Dirty {
		t.Fatal("actual upgrade did not reach clean11")
	}
	for _, table := range before.Tables {
		if table.Name == "schema_migrations" {
			continue
		}
		if !reflect.DeepEqual(table, byName[table.Name]) {
			t.Fatalf("legacy table changed %s", table.Name)
		}
	}
	if !reflect.DeepEqual(before.MigrationFiles, after.MigrationFiles) {
		t.Fatal("applied migration bytes changed")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_legacy_archive WHERE mode_id IS NULL AND content_language IS NULL AND content_revision IS NULL AND archival_state='legacy_unreviewed'`); n != int64(len(manifest)) {
		t.Fatal("fabricated legacy labels", n)
	}
	if _, err = db.Exec(`UPDATE text_legacy_archive SET source_sha256=repeat('a',64)`); err == nil {
		t.Fatal("archive identity mutable")
	}
}

func TestTextArchiveDriftRefusalAndBound(t *testing.T) {
	for _, kind := range []string{"changed_behind_cursor", "changed_ahead_cursor", "deleted_ahead_cursor", "late_low_key", "late_high_key"} {
		t.Run(kind, func(t *testing.T) {
			db := transitionDesignDB(t, 8, true)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			manifest := transitionSourceManifest(t, ctx, db)
			if err := MigrateUp(db, "../../migrations"); err != nil {
				t.Fatal(err)
			}
			a := NewTextArchiveStore(db)
			if err := a.Start(ctx, 2); err != nil {
				t.Fatal(err)
			}
			if _, err := a.Batch(ctx, 0); err == nil {
				t.Fatal("zero batch bound accepted")
			}
			if kind == "changed_behind_cursor" {
				if _, err := a.Batch(ctx, 2); err != nil {
					t.Fatal(err)
				}
			}
			// Explicit disposable-fixture corruption bypasses both source guards to
			// prove the archive still independently detects a mismatched source.
			if _, err := db.Exec(`ALTER TABLE portal_submissions DISABLE TRIGGER text_archive_freeze; ALTER TABLE challenge_entries DISABLE TRIGGER text_archive_freeze; ALTER TABLE portal_submissions DISABLE TRIGGER text_review_identity; ALTER TABLE challenge_entries DISABLE TRIGGER text_review_identity`); err != nil {
				t.Fatal(err)
			}
			transitionMutateSource(t, ctx, db, kind, manifest)
			refused := false
			for i := 0; i < 30; i++ {
				step, err := a.Batch(ctx, 2)
				if err != nil {
					if !errors.Is(err, ErrTextArchiveDrift) {
						t.Fatal(err)
					}
					refused = true
					break
				}
				if step.Done {
					break
				}
			}
			if !refused {
				t.Fatal("source drift accepted", kind)
			}
		})
	}
}

func TestTextArchiveDestinationAndPostCopyPreservation(t *testing.T) {
	db := transitionDesignDB(t, 8, true)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := MigrateUp(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	a := NewTextArchiveStore(db)
	if err := a.Start(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Batch(ctx, 2); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, archiveFormatting); err != nil {
		t.Fatal(err)
	}
	rows, err := archiveSourcePage(ctx, tx, "", archiveZeroID, "portal_submission", "ffffffff-ffff-ffff-ffff-ffffffffffff", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = archiveInsertMapping(ctx, tx, rows[0]); err != nil {
		t.Fatal("identical destination retry", err)
	}
	changed := rows[0]
	changed.Hash = archiveZeroHash
	if err = archiveInsertMapping(ctx, tx, changed); !errors.Is(err, ErrTextArchiveConflict) {
		t.Fatal("destination collision", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		b, err := a.Batch(ctx, 2)
		if err != nil {
			t.Fatal(err)
		}
		if b.Done {
			break
		}
		if i == 29 {
			t.Fatal("not complete")
		}
	}
	var body, hash string
	if err = db.QueryRow(`SELECT source_row::text,source_sha256 FROM text_legacy_archive WHERE source_kind=$1 AND source_id=$2`, rows[0].Kind, rows[0].ID).Scan(&body, &hash); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE challenge_entries SET content='changed after archived',asset_blob=decode('abcd','hex') WHERE id=$1`, rows[0].ID); err == nil {
		t.Fatal("submitted source accepted rewrite")
	}
	// Even privileged corruption cannot change the captured original payload.
	if _, err = db.Exec(`ALTER TABLE challenge_entries DISABLE TRIGGER text_review_identity`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE challenge_entries SET content='changed after archived',asset_blob=decode('abcd','hex') WHERE id=$1`, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	var preserved string
	if err = db.QueryRow(`SELECT source_row::text FROM text_legacy_archive WHERE source_kind=$1 AND source_id=$2`, rows[0].Kind, rows[0].ID).Scan(&preserved); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(preserved))
	if body != preserved || hex.EncodeToString(digest[:]) != hash {
		t.Fatal("original archival bytes lost")
	}
	if _, err = db.Exec(`INSERT INTO text_accepted_inputs(id,source_kind,source_id,text_content,provenance) VALUES(gen_random_uuid(),'portal_submission',gen_random_uuid(),'orphan','{}')`); err == nil {
		t.Fatal("NULL FK bypass accepted")
	}
}
func TestTextArchiveExactDownRefusesRetainedRows(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(fmt.Sprintf("populated_%v", populated), func(t *testing.T) {
			db := transitionDesignDB(t, 11, false)
			if populated {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := NewTextArchiveStore(db).Start(ctx, 2); err != nil {
					t.Fatal(err)
				}
			}
			err := runMigration(db, "../../migrations", "one_step_down", func(m *migrate.Migrate) error { return m.Steps(-1) })
			if populated {
				if err == nil {
					t.Fatal("retained archival progress deleted")
				}
				if n := valueCount(t, db, `SELECT count(*) FROM text_archive_progress`); n != 1 {
					t.Fatal(n)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if n := valueCount(t, db, `SELECT version FROM schema_migrations`); n != 10 {
					t.Fatal(n)
				}
			}
		})
	}
}

func TestTextArchiveOldSnapshotExclusion(t *testing.T) {
	db := transitionDesignDB(t, 11, true)
	db.SetMaxOpenConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var id string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM challenge_entries ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	a := NewTextArchiveStore(db)
	if err = a.Start(ctx, 2); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		b, err := a.Batch(ctx, 2)
		if err != nil {
			t.Fatal(err)
		}
		if b.Pass == "verify" && b.Processed > 0 {
			break
		}
		if i == 39 {
			t.Fatal("verify not reached")
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE challenge_entries SET content=content||' changed after verified' WHERE id=$1`, id); err != nil {
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "P0001" || !strings.Contains(pg.Message, "source writes require read committed") {
			t.Fatalf("unexpected source rejection: %v", err)
		}
		return
	}
	if err = tx.Commit(); err != nil {
		t.Fatalf("unexpected commit rejection: %v", err)
	}
	for i := 0; i < 40; i++ {
		b, err := a.Batch(ctx, 2)
		if err != nil {
			if !errors.Is(err, ErrTextArchiveDrift) {
				t.Fatalf("unexpected archive error: %v", err)
			}
			return
		}
		if b.Done {
			t.Fatal("old-snapshot writer bypassed freeze behind verification cursor; job falsely complete")
		}
	}
	t.Fatal("archive did not finish")
}
