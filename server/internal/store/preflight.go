package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/lib/pq"
)

// TransitionPreflight is operational evidence, not migration or release approval.
// It contains aggregate fingerprints only: no account identifiers, receipts,
// content wording, secrets or connection strings. Treat the report as private
// operational metadata nevertheless. Live process/client/object inventories must
// be collected separately; PostgreSQL cannot prove that no matches are running.
type TransitionPreflight struct {
	FormatVersion    int                `json:"format_version"`
	MigrationVersion *int64             `json:"migration_version"`
	MigrationDirty   bool               `json:"migration_dirty"`
	SchemaSHA256     string             `json:"schema_sha256"`
	Tables           []TableFingerprint `json:"tables"`
	Inventory        LegacyInventory    `json:"legacy_inventory"`
}

type TableFingerprint struct {
	Name   string `json:"name"`
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

// ReadTransitionPreflight observes one repeatable-read, read-only database
// snapshot. It never migrates, repairs dirty state, locks rows for mutation or
// exports row values. The caller must supply a bounded context and appropriate
// SELECT privileges. Missing privileges are errors, never an incomplete report.
func ReadTransitionPreflight(ctx context.Context, db *sql.DB) (*TransitionPreflight, error) {
	if _, ok := ctx.Deadline(); !ok && ctx.Err() == nil {
		return nil, fmt.Errorf("preflight requires a context deadline")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin read-only preflight: %w", err)
	}
	defer tx.Rollback()
	// Timestamp text and row ordering must not vary with operator/session locale.
	// row_security=off refuses queries that would silently filter rows; it does
	// not bypass row policies or grant the operator any additional privileges.
	if _, err := tx.ExecContext(ctx, `SET LOCAL TIME ZONE 'UTC'; SET LOCAL DateStyle = 'ISO, YMD'; SET LOCAL extra_float_digits = 3; SET LOCAL search_path=pg_catalog,public; SET LOCAL bytea_output='hex'; SET LOCAL intervalstyle='postgres'; SET LOCAL row_security=off`); err != nil {
		return nil, fmt.Errorf("set preflight formatting: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname='public' ORDER BY tablename COLLATE "C"`)
	if err != nil {
		return nil, fmt.Errorf("inventory public tables: %w", err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read table inventory: %w", err)
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("finish table inventory: %w", err)
	}
	result := &TransitionPreflight{FormatVersion: 1, Tables: make([]TableFingerprint, 0, len(names))}
	for _, name := range names {
		if name == "schema_migrations" {
			var version sql.NullInt64
			var count int64
			err := tx.QueryRowContext(ctx, `SELECT count(*),min(version),COALESCE(bool_or(dirty),false) FROM public.schema_migrations`).Scan(&count, &version, &result.MigrationDirty)
			if err != nil {
				return nil, fmt.Errorf("read migration state: %w", err)
			}
			if count > 1 {
				return nil, fmt.Errorf("ambiguous migration state")
			}
			if version.Valid {
				result.MigrationVersion = &version.Int64
			}
		}
		// Quote catalog-provided identifiers; never concatenate them unescaped.
		query := `SELECT to_jsonb(row_data)::text FROM public.` + pq.QuoteIdentifier(name) + ` AS row_data ORDER BY to_jsonb(row_data)::text COLLATE "C"`
		fingerprint, err := fingerprintQuery(ctx, tx, query)
		if err != nil {
			return nil, fmt.Errorf("fingerprint table %q: %w", name, err)
		}
		fingerprint.Name = name
		result.Tables = append(result.Tables, fingerprint)
	}
	result.Inventory, err = readLegacyInventory(ctx, tx, names)
	if err != nil {
		return nil, fmt.Errorf("read legacy inventory: %w", err)
	}
	// Definitions cover column types/defaults, constraints and indexes without
	// exposing data. Applied SQL file checksums belong to the artifact manifest.
	schema, err := fingerprintQuery(ctx, tx, `
SELECT definition FROM (
 SELECT jsonb_build_array('column',table_name,column_name,ordinal_position,data_type,udt_name,is_nullable,column_default)::text AS definition
 FROM information_schema.columns WHERE table_schema='public'
 UNION ALL
 SELECT jsonb_build_array('constraint',c.relname,k.conname,pg_get_constraintdef(k.oid))::text
 FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'
 UNION ALL
 SELECT jsonb_build_array('index',tablename,indexname,indexdef)::text FROM pg_indexes WHERE schemaname='public'
) definitions ORDER BY definition COLLATE "C"`)
	if err != nil {
		return nil, fmt.Errorf("fingerprint schema: %w", err)
	}
	result.SchemaSHA256 = schema.SHA256
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("complete read-only preflight: %w", err)
	}
	return result, nil
}

func fingerprintQuery(ctx context.Context, tx *sql.Tx, query string) (TableFingerprint, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return TableFingerprint{}, err
	}
	defer rows.Close()
	result := TableFingerprint{}
	hash := sha256.New()
	var size [8]byte
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			return TableFingerprint{}, err
		}
		binary.BigEndian.PutUint64(size[:], uint64(len(row)))
		hash.Write(size[:])
		hash.Write([]byte(row))
		result.Rows++
	}
	if err := rows.Err(); err != nil {
		return TableFingerprint{}, err
	}
	result.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return result, nil
}
