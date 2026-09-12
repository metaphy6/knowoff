package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
)

var ErrTextArchiveDrift = errors.New("text_archive.source_drift")
var ErrTextArchiveConflict = errors.New("text_archive.identity_conflict")

const archiveZeroID = "00000000-0000-0000-0000-000000000000"
const archiveZeroHash = "0000000000000000000000000000000000000000000000000000000000000000"
const archiveFormatting = `SET LOCAL TIME ZONE 'UTC'; SET LOCAL DateStyle='ISO, YMD'; SET LOCAL extra_float_digits=3; SET LOCAL search_path=pg_catalog,public; SET LOCAL bytea_output='hex'; SET LOCAL intervalstyle='postgres'; SET LOCAL row_security=off`
const archiveSources = `SELECT 'challenge_entry'::text AS kind,id,entry_type AS media_type,to_jsonb(c)::text AS row_json FROM public.challenge_entries c UNION ALL SELECT 'portal_submission',id,media_type,to_jsonb(p)::text FROM public.portal_submissions p`

type archiveSource struct{ Kind, ID, Hash, MediaType, RowJSON string }
type TextArchiveBatch struct {
	Pass      string
	Processed int
	Done      bool
}
type archiveProgress struct {
	Phase, UpperKind, UpperID      string
	ExpectedCount                  int64
	ExpectedHash, CopyKind, CopyID string
	CopyCount                      int64
	CopyHash, VerifyKind, VerifyID string
	VerifyCount                    int64
	VerifyHash                     string
}
type TextArchiveStore struct {
	db           *sql.DB
	beforeCommit func() error
}

func NewTextArchiveStore(db *sql.DB) *TextArchiveStore { return &TextArchiveStore{db: db} }

// Start streams the manifest under database-enforced writer exclusion, retaining
// at most limit source records. Committed progress keeps source writes frozen
// through crashes until both bounded passes have verified the original rows.
func (s *TextArchiveStore) Start(ctx context.Context, limit int) error {
	return s.start(ctx, "", limit)
}

// StartAs binds the operator audit to the captured manifest transaction.
func (s *TextArchiveStore) StartAs(ctx context.Context, admin string, limit int) error {
	if !valueUUID(admin) {
		return ErrTextArchiveConflict
	}
	return s.start(ctx, admin, limit)
}
func (s *TextArchiveStore) start(ctx context.Context, admin string, limit int) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("archive capture requires a context deadline")
	}
	if limit < 1 || limit > 1000 {
		return ErrTextArchiveConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if admin != "" {
		if err = textAdmin(ctx, tx, admin); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, archiveFormatting); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `LOCK TABLE portal_submissions,challenge_entries IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM text_archive_progress)`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Commit()
	}
	kind, id, hash := "", archiveZeroID, archiveZeroHash
	var count int64
	for {
		rows, err := archiveSourcePage(ctx, tx, kind, id, "portal_submission", "ffffffff-ffff-ffff-ffff-ffffffffffff", limit)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			hash = archiveAccumulate(hash, row)
			count++
			kind, id = row.Kind, row.ID
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO text_archive_progress(job_id,phase,upper_kind,upper_id,expected_count,expected_sha256,copy_sha256,verify_sha256) VALUES(1,'copy',$1,$2,$3,$4,$5,$5)`, kind, id, count, hash, archiveZeroHash); err != nil {
		return err
	}
	if admin != "" {
		if err = textReleaseAudit(ctx, tx, admin, "text_archive_start", "1", map[string]any{"count": count, "sha256": hash}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func archiveSourcePage(ctx context.Context, tx *sql.Tx, afterKind, afterID, upperKind, upperID string, limit int) ([]archiveSource, error) {
	rows, err := tx.QueryContext(ctx, `SELECT kind,id::text,media_type,row_json FROM (`+archiveSources+`) source
WHERE (kind COLLATE "C",id)>($1::text COLLATE "C",$2::uuid) AND (kind COLLATE "C",id)<=($3::text COLLATE "C",$4::uuid)
ORDER BY kind COLLATE "C",id LIMIT $5`, afterKind, afterID, upperKind, upperID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []archiveSource
	for rows.Next() {
		var row archiveSource
		var body string
		if err := rows.Scan(&row.Kind, &row.ID, &row.MediaType, &body); err != nil {
			return nil, err
		}
		hash := sha256.Sum256([]byte(body))
		row.Hash = hex.EncodeToString(hash[:])
		row.RowJSON = body
		result = append(result, row)
	}
	return result, rows.Err()
}

// Chain encoding: SHA256(previous 32 digest bytes || uint64-BE key byte length
// || UTF-8 kind+":"+canonical UUID || 32 source digest bytes), starting at zero.

func archiveAccumulate(previous string, row archiveSource) string {
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

func archiveReadProgress(ctx context.Context, tx *sql.Tx) (archiveProgress, error) {
	var p archiveProgress
	err := tx.QueryRowContext(ctx, `SELECT phase,upper_kind,upper_id::text,expected_count,expected_sha256,copy_kind,copy_id::text,copy_count,copy_sha256,verify_kind,verify_id::text,verify_count,verify_sha256 FROM text_archive_progress WHERE job_id=1 FOR UPDATE`).Scan(&p.Phase, &p.UpperKind, &p.UpperID, &p.ExpectedCount, &p.ExpectedHash, &p.CopyKind, &p.CopyID, &p.CopyCount, &p.CopyHash, &p.VerifyKind, &p.VerifyID, &p.VerifyCount, &p.VerifyHash)
	return p, err
}

func archiveInsertMapping(ctx context.Context, tx *sql.Tx, row archiveSource) error {
	var submission, entry any
	if row.Kind == "portal_submission" {
		submission = row.ID
	} else if row.Kind == "challenge_entry" {
		entry = row.ID
	} else {
		return ErrTextArchiveConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO text_legacy_archive(source_kind,source_id,source_sha256,content_id,media_type,submission_id,entry_id,source_row)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, row.Kind, row.ID, row.Hash, "legacy:"+row.Kind+":"+row.ID, row.MediaType, submission, entry, row.RowJSON); err != nil {
		return err
	}
	return archiveCheckMapping(ctx, tx, row, ErrTextArchiveConflict)
}

func archiveCheckMapping(ctx context.Context, tx *sql.Tx, row archiveSource, mismatch error) error {
	var hash, contentID, mediaType, state, body string
	err := tx.QueryRowContext(ctx, `SELECT source_sha256,content_id,media_type,archival_state,source_row::text FROM text_legacy_archive WHERE source_kind=$1 AND source_id=$2`, row.Kind, row.ID).Scan(&hash, &contentID, &mediaType, &state, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return mismatch
	}
	if err != nil {
		return err
	}
	if body != row.RowJSON || hash != row.Hash || contentID != "legacy:"+row.Kind+":"+row.ID || mediaType != row.MediaType || state != "legacy_unreviewed" {
		return mismatch
	}
	return nil
}

func (s *TextArchiveStore) Batch(ctx context.Context, limit int) (TextArchiveBatch, error) {
	return s.batch(ctx, "", limit)
}
func (s *TextArchiveStore) BatchAs(ctx context.Context, admin string, limit int) (TextArchiveBatch, error) {
	if !valueUUID(admin) {
		return TextArchiveBatch{}, ErrTextArchiveConflict
	}
	return s.batch(ctx, admin, limit)
}
func (s *TextArchiveStore) batch(ctx context.Context, admin string, limit int) (TextArchiveBatch, error) {
	var result TextArchiveBatch
	if limit < 1 || limit > 1000 {
		return result, errors.New("archive batch bound must be 1..1000")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, archiveFormatting); err != nil {
		return result, err
	}
	p, err := archiveReadProgress(ctx, tx)
	if admin != "" {
		if actorErr := textAdmin(ctx, tx, admin); actorErr != nil {
			return result, actorErr
		}
	}
	if err != nil {
		return result, err
	}
	result.Pass = p.Phase
	if p.Phase == "complete" {
		result.Done = true
		return result, tx.Commit()
	}
	var beyond bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM (`+archiveSources+`) source WHERE (kind COLLATE "C",id)>($1::text COLLATE "C",$2::uuid) LIMIT 1)`, p.UpperKind, p.UpperID).Scan(&beyond); err != nil {
		return result, err
	}
	if beyond {
		return result, ErrTextArchiveDrift
	}
	kind, id, count, hash := p.CopyKind, p.CopyID, p.CopyCount, p.CopyHash
	if p.Phase == "verify" {
		kind, id, count, hash = p.VerifyKind, p.VerifyID, p.VerifyCount, p.VerifyHash
	}
	batch, err := archiveSourcePage(ctx, tx, kind, id, p.UpperKind, p.UpperID, limit)
	if err != nil {
		return result, err
	}
	for _, row := range batch {
		if p.Phase == "copy" {
			err = archiveInsertMapping(ctx, tx, row)
		} else {
			err = archiveCheckMapping(ctx, tx, row, ErrTextArchiveDrift)
		}
		if err != nil {
			return result, err
		}
		count++
		hash = archiveAccumulate(hash, row)
		kind, id = row.Kind, row.ID

	}
	if len(batch) == 0 {
		if count != p.ExpectedCount || hash != p.ExpectedHash {
			return result, ErrTextArchiveDrift
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
	if _, err := tx.ExecContext(ctx, `UPDATE text_archive_progress SET phase=$1,copy_kind=$2,copy_id=$3,copy_count=$4,copy_sha256=$5,verify_kind=$6,verify_id=$7,verify_count=$8,verify_sha256=$9 WHERE job_id=1`, p.Phase, p.CopyKind, p.CopyID, p.CopyCount, p.CopyHash, p.VerifyKind, p.VerifyID, p.VerifyCount, p.VerifyHash); err != nil {
		return result, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return result, err
		}
	}
	if admin != "" {
		if err = textReleaseAudit(ctx, tx, admin, "text_archive_batch", "1", map[string]any{"pass": result.Pass, "processed": len(batch), "phase": p.Phase}); err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	result.Processed = len(batch)
	return result, nil
}
