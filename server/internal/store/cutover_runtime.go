package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrRuntimeCutover = errors.New("runtime.cutover_unavailable")

// CheckRuntimeCutover checks startup eligibility before ownership or recovery
// writes. Empty registries allow initial ordinary startup; retained history
// requires a ready instance matching the actual physical database. This bounded,
// read-only check neither provisions an instance nor fences concurrent writers.
func CheckRuntimeCutover(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return ErrRuntimeCutover
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ErrRuntimeCutover
	}
	defer tx.Rollback()
	// PostgreSQL refuses a query affected by RLS when row_security is off;
	// silently hidden history must never be interpreted as initial startup.
	if _, err = tx.ExecContext(ctx, `SET LOCAL row_security=off`); err != nil {
		return ErrRuntimeCutover
	}
	var systemID, databaseOID, databaseName string
	err = tx.QueryRowContext(ctx, `SELECT system_identifier::text,d.oid::text,d.datname
FROM pg_catalog.pg_control_system(),pg_catalog.pg_database d
WHERE d.datname=pg_catalog.current_database()`).Scan(&systemID, &databaseOID, &databaseName)
	if err != nil {
		return ErrRuntimeCutover
	}
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT count(*)=0 OR count(*) FILTER (
WHERE cluster_system_identifier=$1::numeric AND database_oid=$2::oid
AND database_name=$3 AND phase='ready')=1 FROM public.cutover_instances`, systemID, databaseOID, databaseName).Scan(&valid)
	if err != nil || !valid {
		return ErrRuntimeCutover
	}
	if err = tx.Commit(); err != nil {
		return ErrRuntimeCutover
	}
	return nil
}
