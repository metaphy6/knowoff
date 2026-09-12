package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/knowoff/knowoff/server/migrations"
)

var ErrRuntimeSchema = errors.New("runtime requires its exact clean compiled schema version")

// CheckRuntimeSchema is read-only and precedes ownership or worker startup.
// It certifies version compatibility, not the provenance of applied DDL bytes.
func CheckRuntimeSchema(ctx context.Context, db *sql.DB) error {
	manifest, err := migrations.Compiled()
	if err != nil || db == nil {
		return ErrRuntimeSchema
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ErrRuntimeSchema
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SET LOCAL row_security=off`); err != nil {
		return ErrRuntimeSchema
	}
	var count int
	var version sql.NullInt64
	var dirty bool
	err = tx.QueryRowContext(ctx, `SELECT count(*),min(version),COALESCE(bool_or(dirty),false) FROM public.schema_migrations`).Scan(&count, &version, &dirty)
	if err != nil || count != 1 || !version.Valid || version.Int64 < 1 || uint64(version.Int64) != manifest.SchemaVersion || dirty {
		return ErrRuntimeSchema
	}
	if err = tx.Commit(); err != nil {
		return ErrRuntimeSchema
	}
	return nil
}
