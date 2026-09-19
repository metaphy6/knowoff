package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/knowoff/knowoff/server/migrations"
	"github.com/lib/pq"
)

var (
	ErrCutoverInvalid     = errors.New("cutover.invalid")
	ErrCutoverBusy        = errors.New("cutover.busy")
	ErrCutoverUnavailable = errors.New("cutover.unavailable")
	ErrCutoverConflict    = errors.New("cutover.conflict")
	ErrCutoverLease       = errors.New("cutover.lease")
	ErrCutoverPending     = errors.New("cutover.pending")
	ErrCutoverParity      = errors.New("cutover.parity")
)

type CutoverIdentity struct {
	InstanceID              string `json:"instance_id"`
	ClusterSystemIdentifier string `json:"cluster_system_identifier"`
	DatabaseOID             uint32 `json:"database_oid"`
	DatabaseName            string `json:"database_name"`
}
type CutoverPins struct {
	SchemaSHA256  string `json:"schema_sha256"`
	ImageSHA256   string `json:"image_sha256"`
	ConfigSHA256  string `json:"config_sha256"`
	ContentSHA256 string `json:"content_sha256"`
}
type CutoverBegin struct {
	Identity      CutoverIdentity `json:"identity"`
	RequestID     string          `json:"request_id"`
	PredecessorID string          `json:"predecessor_id"`
	Pins          CutoverPins     `json:"pins"`
	Lease         []byte          `json:"lease"`
	LeaseSeconds  int64           `json:"lease_seconds"`
}
type CutoverProof struct {
	InstanceID string `json:"instance_id"`
	RequestID  string `json:"request_id"`
	Generation int64  `json:"generation"`
	Lease      []byte `json:"lease"`
}
type CutoverSeal struct {
	Proof       CutoverProof `json:"proof"`
	WatermarkID string       `json:"watermark_id"`
}
type CutoverHandoff struct {
	Proof       CutoverProof    `json:"proof"`
	WatermarkID string          `json:"watermark_id"`
	HandoffID   string          `json:"handoff_id"`
	Target      CutoverIdentity `json:"target"`
}
type CutoverBootstrap = CutoverHandoff

type CutoverReceipt struct {
	Identity       CutoverIdentity `json:"identity"`
	RequestID      string          `json:"request_id,omitempty"`
	Generation     int64           `json:"generation"`
	Phase          string          `json:"phase"`
	WatermarkID    string          `json:"watermark_id,omitempty"`
	HandoffID      string          `json:"handoff_id,omitempty"`
	EvidenceSHA256 string          `json:"evidence_sha256,omitempty"`
	ExpiresAt      time.Time       `json:"expires_at,omitempty"`
}

// CutoverController is an offline trusted operator boundary, never an app
// credential. One dedicated physical connection owns the schema26 session lane.
// Closing, cancellation and lease expiry never re-enable source LOGIN. Methods
// have finite deadlines and refuse concurrent use instead of queuing it.
type CutoverController struct {
	mu   sync.Mutex
	conn *sql.Conn
	spec CutoverControlRoleSpec
}

func OpenCutoverController(ctx context.Context, db *sql.DB, spec CutoverControlRoleSpec) (*CutoverController, error) {
	if db == nil || !cutoverName(spec.Control) {
		return nil, ErrCutoverInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, ErrCutoverUnavailable
	}
	c := &CutoverController{conn: conn, spec: spec}
	var locked bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(1263486022,26)`).Scan(&locked); err != nil || !locked {
		c.Close()
		if err != nil {
			return nil, ErrCutoverUnavailable
		}
		return nil, ErrCutoverBusy
	}
	err = c.transaction(ctx, func(tx *sql.Tx) error { return c.verify(ctx, tx, spec.WritersEnabled) })
	if err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}
func (c *CutoverController) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	// Always discard rather than return a possibly locked/uncertain physical
	// session to a pool. ErrBadConn is the database/sql documented discard signal.
	err := c.conn.Raw(func(any) error { return driver.ErrBadConn })
	c.conn.Close()
	c.conn = nil
	if err != nil && !errors.Is(err, driver.ErrBadConn) && !errors.Is(err, sql.ErrConnDone) {
		return ErrCutoverUnavailable
	}
	return nil
}
func (c *CutoverController) transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	if c == nil || !c.mu.TryLock() {
		return ErrCutoverBusy
	}
	defer c.mu.Unlock()
	if c.conn == nil {
		return ErrCutoverUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := c.conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ErrCutoverUnavailable
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SET LOCAL row_security=off;SET LOCAL search_path=pg_catalog,public;SET LOCAL timezone='UTC';SET LOCAL datestyle='ISO, YMD';SET LOCAL extra_float_digits=3;SET LOCAL bytea_output='hex';SET LOCAL lock_timeout='3s';SET LOCAL statement_timeout='25s'`); err != nil {
		return ErrCutoverUnavailable
	}
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return ErrCutoverUnavailable
	}
	return nil
}
func (c *CutoverController) verify(ctx context.Context, tx *sql.Tx, enabled bool) error {
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT session_user=current_user AND current_user=$1 AND EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND pid=pg_backend_pid() AND classid=1263486022 AND objid=26 AND objsubid=2 AND granted)`, c.spec.Control).Scan(&valid); err != nil || !valid {
		return ErrCutoverUnavailable
	}
	if err := verifyCutoverRolesSnapshot(ctx, tx, c.spec.Roles, c.spec.Control, enabled); err != nil {
		return err
	}
	// The reviewed schema has no rewrite rules, inheritance or row policies.
	// These can alter writes without changing rows or the version marker.
	if err := tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_rewrite r JOIN pg_class c ON c.oid=r.ev_class JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public') AND NOT EXISTS(SELECT 1 FROM pg_inherits i JOIN pg_class c ON c.oid=i.inhrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public') AND NOT EXISTS(SELECT 1 FROM pg_policy p JOIN pg_class c ON c.oid=p.polrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public')`).Scan(&valid); err != nil {
		return ErrCutoverUnavailable
	}
	if !valid {
		return ErrCutoverParity
	}
	return nil
}
func cutoverPhysical(ctx context.Context, q cutoverRoleQuerier) (CutoverIdentity, error) {
	var id CutoverIdentity
	err := q.QueryRowContext(ctx, `SELECT system_identifier::text,d.oid,d.datname FROM pg_catalog.pg_control_system(),pg_catalog.pg_database d WHERE d.datname=current_database()`).Scan(&id.ClusterSystemIdentifier, &id.DatabaseOID, &id.DatabaseName)
	if err != nil {
		return id, ErrCutoverUnavailable
	}
	return id, nil
}
func validCutoverIdentity(id CutoverIdentity) bool {
	n, err := strconv.ParseUint(id.ClusterSystemIdentifier, 10, 64)
	return valueUUID(id.InstanceID) && err == nil && n > 0 && strconv.FormatUint(n, 10) == id.ClusterSystemIdentifier && id.DatabaseOID > 0 && cutoverName(id.DatabaseName)
}
func samePhysical(a, b CutoverIdentity) bool {
	return a.ClusterSystemIdentifier == b.ClusterSystemIdentifier && a.DatabaseOID == b.DatabaseOID && a.DatabaseName == b.DatabaseName
}
func (c *CutoverController) Identity(ctx context.Context) (CutoverIdentity, error) {
	var result CutoverIdentity
	err := c.transaction(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = cutoverPhysical(ctx, tx)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `SELECT id FROM cutover_instances WHERE cluster_system_identifier=$1 AND database_oid=$2 AND database_name=$3`, result.ClusterSystemIdentifier, result.DatabaseOID, result.DatabaseName).Scan(&result.InstanceID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ErrCutoverUnavailable
		}
		return nil
	})
	return result, err
}
func validCutoverPins(p CutoverPins) bool {
	for _, s := range []string{p.SchemaSHA256, p.ImageSHA256, p.ConfigSHA256, p.ContentSHA256} {
		b, e := hex.DecodeString(s)
		if e != nil || len(b) != 32 || s != strings.ToLower(s) {
			return false
		}
	}
	manifest, err := migrations.Compiled()
	return err == nil && p.SchemaSHA256 == manifest.SHA256
}
func (c *CutoverController) Begin(ctx context.Context, b CutoverBegin) (CutoverReceipt, error) {
	var r CutoverReceipt
	if !validCutoverIdentity(b.Identity) || !valueUUID(b.RequestID) || b.PredecessorID != "" && !valueUUID(b.PredecessorID) || !validCutoverPins(b.Pins) || len(b.Lease) != 32 || b.LeaseSeconds < 1 || b.LeaseSeconds > 3600 {
		return r, ErrCutoverInvalid
	}
	digest := sha256.Sum256(b.Lease)
	err := c.transaction(ctx, func(tx *sql.Tx) error {
		id, err := cutoverPhysical(ctx, tx)
		if err != nil {
			return err
		}
		if !samePhysical(id, b.Identity) {
			return ErrCutoverConflict
		}
		var phase string
		var generation int64
		var current sql.NullString
		err = tx.QueryRowContext(ctx, `SELECT phase,generation,current_request FROM cutover_instances WHERE id=$1`, b.Identity.InstanceID).Scan(&phase, &generation, &current)
		if errors.Is(err, sql.ErrNoRows) {
			if b.PredecessorID != "" {
				return ErrCutoverConflict
			}
			if err = c.verify(ctx, tx, true); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO cutover_instances(id,cluster_system_identifier,database_oid,database_name) VALUES($1,$2,$3,$4)`, b.Identity.InstanceID, id.ClusterSystemIdentifier, id.DatabaseOID, id.DatabaseName); err != nil {
				return ErrCutoverConflict
			}
			phase = "ready"
		} else if err != nil {
			return ErrCutoverUnavailable
		}
		if phase != "ready" {
			if err = c.verify(ctx, tx, false); err != nil {
				return err
			}
		} else if err = c.verify(ctx, tx, true); err != nil {
			return err
		}
		if current.String == b.RequestID {
			var matches bool
			err = tx.QueryRowContext(ctx, `SELECT predecessor_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid AND schema_sha256=$3 AND image_sha256=$4 AND config_sha256=$5 AND content_sha256=$6 AND extract(epoch FROM expires_at-issued_at)=$7 AND lease_sha256=$8 FROM cutover_requests WHERE id=$1`, b.RequestID, b.PredecessorID, b.Pins.SchemaSHA256, b.Pins.ImageSHA256, b.Pins.ConfigSHA256, b.Pins.ContentSHA256, b.LeaseSeconds, digest[:]).Scan(&matches)
			if err != nil || !matches {
				return ErrCutoverConflict
			}
			r, _, err = c.authenticate(ctx, tx, CutoverProof{b.Identity.InstanceID, b.RequestID, generation, b.Lease})
			return err
		}
		if current.String != b.PredecessorID || generation == math.MaxInt64 {
			return ErrCutoverConflict
		}
		if current.Valid {
			var recoverable bool
			if err = tx.QueryRowContext(ctx, `SELECT expires_at<=clock_timestamp() AND NOT EXISTS(SELECT 1 FROM cutover_handoffs WHERE instance_id=$2) FROM cutover_requests WHERE id=$1`, current.String, b.Identity.InstanceID).Scan(&recoverable); err != nil || !recoverable {
				return ErrCutoverLease
			}
		}
		var writers string
		if err = tx.QueryRowContext(ctx, `SELECT jsonb_agg(jsonb_build_object('name',rolname,'oid',oid::bigint) ORDER BY rolname)::text FROM pg_roles WHERE rolname=ANY($1::text[])`, pq.Array(c.spec.Roles.writers())).Scan(&writers); err != nil {
			return ErrCutoverUnavailable
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO cutover_requests(id,instance_id,generation,predecessor_id,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at) SELECT $1,$2,$3,NULLIF($4,'')::uuid,$5::jsonb,$6,$7,$8,$9,$10,at,at+$11*interval '1 second' FROM (SELECT clock_timestamp() at) clock`, b.RequestID, b.Identity.InstanceID, generation+1, b.PredecessorID, writers, b.Pins.SchemaSHA256, b.Pins.ImageSHA256, b.Pins.ConfigSHA256, b.Pins.ContentSHA256, digest[:], b.LeaseSeconds)
		if err != nil {
			return ErrCutoverConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE cutover_instances SET phase='closing',generation=$2,current_request=$3 WHERE id=$1`, b.Identity.InstanceID, generation+1, b.RequestID); err != nil {
			return ErrCutoverConflict
		}
		for _, role := range c.spec.Roles.writers() {
			if _, err = tx.ExecContext(ctx, `ALTER ROLE `+pq.QuoteIdentifier(role)+` NOLOGIN`); err != nil {
				return ErrCutoverUnavailable
			}
		}
		r, _, err = c.authenticate(ctx, tx, CutoverProof{b.Identity.InstanceID, b.RequestID, generation + 1, b.Lease})
		return err
	})
	if err != nil {
		return CutoverReceipt{}, err
	}
	// LOGIN and the closing request commit together before terminating pooled and
	// in-flight writers. Interrupted cleanup is safe to retry using the same ID.
	err = c.transaction(ctx, func(tx *sql.Tx) error {
		if err := c.verify(ctx, tx, false); err != nil {
			return err
		}
		return c.fenceSessions(ctx, tx, true)
	})
	return r, err
}

func (c *CutoverController) authenticate(ctx context.Context, tx *sql.Tx, p CutoverProof, terminalReplay ...bool) (CutoverReceipt, CutoverPins, error) {
	var r CutoverReceipt
	var pins CutoverPins
	if !valueUUID(p.InstanceID) || !valueUUID(p.RequestID) || p.Generation < 1 || len(p.Lease) != 32 {
		return r, pins, ErrCutoverInvalid
	}
	var digest []byte
	var live bool
	err := tx.QueryRowContext(ctx, `SELECT i.id,i.cluster_system_identifier::text,i.database_oid,i.database_name,i.phase,i.generation,i.current_request,r.expires_at,r.expires_at>clock_timestamp(),r.lease_sha256,r.schema_sha256,r.image_sha256,r.config_sha256,r.content_sha256 FROM cutover_instances i JOIN cutover_requests r ON r.id=i.current_request WHERE i.id=$1 AND i.current_request=$2 AND i.generation=$3`, p.InstanceID, p.RequestID, p.Generation).Scan(&r.Identity.InstanceID, &r.Identity.ClusterSystemIdentifier, &r.Identity.DatabaseOID, &r.Identity.DatabaseName, &r.Phase, &r.Generation, &r.RequestID, &r.ExpiresAt, &live, &digest, &pins.SchemaSHA256, &pins.ImageSHA256, &pins.ConfigSHA256, &pins.ContentSHA256)
	if err != nil {
		return r, pins, ErrCutoverConflict
	}
	sum := sha256.Sum256(p.Lease)
	if (!live && (len(terminalReplay) != 1 || !terminalReplay[0])) || subtle.ConstantTimeCompare(digest, sum[:]) != 1 {
		return r, pins, ErrCutoverLease
	}
	id, err := cutoverPhysical(ctx, tx)
	if err != nil || !samePhysical(id, r.Identity) {
		return r, pins, ErrCutoverConflict
	}
	var writersMatch bool
	if err = tx.QueryRowContext(ctx, `SELECT writer_roles=(SELECT jsonb_agg(jsonb_build_object('name',rolname,'oid',oid::bigint) ORDER BY rolname) FROM pg_roles WHERE rolname=ANY($2::text[])) FROM cutover_requests WHERE id=$1`, p.RequestID, pq.Array(c.spec.Roles.writers())).Scan(&writersMatch); err != nil || !writersMatch {
		return r, pins, ErrCutoverPrivileges
	}
	return r, pins, nil
}

// fenceSessions never signals a PID from a stale list: the statement rechecks
// database OID, session-user OID, PID and backend_start together. SET ROLE does
// not change pg_stat_activity.usesysid. Capture sessions are read-only by ACL.
func (c *CutoverController) fenceSessions(ctx context.Context, tx *sql.Tx, terminate bool) error {
	// Authentication checks LOGIN before pg_stat_activity publishes the session.
	// Its startup transaction already has locks, even before database assignment.
	// Any unclassified lock PID is conservatively busy across the whole cluster;
	// guessing its database/role or signaling it would bypass the exact scope.
	if _, err := tx.ExecContext(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
		return ErrCutoverUnavailable
	}
	var unpublished bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks l WHERE l.pid IS NOT NULL AND l.pid<>pg_backend_pid() AND NOT EXISTS(SELECT 1 FROM pg_stat_activity a WHERE a.pid=l.pid))`).Scan(&unpublished); err != nil {
		return ErrCutoverUnavailable
	}
	if unpublished {
		return ErrCutoverBusy
	}
	var unknown bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND backend_type='client backend' AND pid<>pg_backend_pid() AND usesysid NOT IN(SELECT oid FROM pg_roles WHERE rolname=ANY($1::text[]))) OR EXISTS(SELECT 1 FROM pg_prepared_xacts WHERE database=current_database())`, pq.Array(append(c.spec.Roles.writers(), c.spec.Roles.Capture))).Scan(&unknown); err != nil {
		return ErrCutoverUnavailable
	}
	if unknown {
		return ErrCutoverBusy
	}
	rows, err := tx.QueryContext(ctx, `SELECT pid,backend_start,usesysid,datid FROM pg_stat_activity WHERE datid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND backend_type='client backend' AND usesysid IN(SELECT oid FROM pg_roles WHERE rolname=ANY($1::text[]))`, pq.Array(c.spec.Roles.writers()))
	if err != nil {
		return ErrCutoverUnavailable
	}
	type backend struct {
		pid            int
		at             time.Time
		role, database uint32
	}
	var active []backend
	for rows.Next() {
		var b backend
		if err = rows.Scan(&b.pid, &b.at, &b.role, &b.database); err != nil {
			rows.Close()
			return ErrCutoverUnavailable
		}
		active = append(active, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ErrCutoverUnavailable
	}
	if !terminate && len(active) > 0 {
		return ErrCutoverBusy
	}
	for _, b := range active {
		// Re-read the identity immediately before each signal; a transaction-cached
		// activity row cannot establish that a recycled PID is still this backend.
		if _, err = tx.ExecContext(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			return ErrCutoverUnavailable
		}
		var ok bool
		err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT pg_terminate_backend(pid,1000) FROM pg_stat_activity WHERE pid=$1 AND backend_start=$2 AND usesysid=$3 AND datid=$4 AND backend_type='client backend'),true)`, b.pid, b.at, b.role, b.database).Scan(&ok)
		if err != nil || !ok {
			return ErrCutoverUnavailable
		}
	}
	if terminate {
		return c.fenceSessions(ctx, tx, false)
	}
	return nil
}

type cutoverFingerprint struct {
	Count  int64  `json:"count"`
	SHA256 string `json:"sha256"`
}
type cutoverEvidence struct {
	Schema    cutoverFingerprint            `json:"schema"`
	Version   int                           `json:"version"`
	Pins      CutoverPins                   `json:"pins"`
	Tables    map[string]cutoverFingerprint `json:"tables"`
	Sequences map[string]cutoverFingerprint `json:"sequences"`
	Pending   map[string]int64              `json:"pending"`
}

var cutoverAuthorityTables = []string{"cutover_instances", "cutover_requests", "cutover_watermarks", "cutover_handoffs"}

func cutoverTableFingerprint(ctx context.Context, tx *sql.Tx, name string, sequence bool) (cutoverFingerprint, error) {
	// Names come exclusively from the reviewed schema inventory, not input.
	query := `SELECT hash FROM (SELECT encode(sha256(convert_to(to_jsonb(t)::text,'UTF8')),'hex') AS hash FROM public.` + pq.QuoteIdentifier(name) + ` t) hashed ORDER BY hash COLLATE "C"`
	if sequence {
		query = `SELECT encode(sha256(convert_to(jsonb_build_object('last_value',last_value,'is_called',is_called)::text,'UTF8')),'hex') FROM public.` + pq.QuoteIdentifier(name)
	}
	return cutoverQueryFingerprint(ctx, tx, query)
}
func cutoverQueryFingerprint(ctx context.Context, tx *sql.Tx, query string, args ...any) (cutoverFingerprint, error) {
	var result cutoverFingerprint
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return result, ErrCutoverUnavailable
	}
	defer rows.Close()
	h := sha256.New()
	for rows.Next() {
		var sum string
		if err = rows.Scan(&sum); err != nil {
			return result, ErrCutoverUnavailable
		}
		result.Count++
		h.Write([]byte(sum))
	}
	if err = rows.Err(); err != nil {
		return result, ErrCutoverUnavailable
	}
	result.SHA256 = hex.EncodeToString(h.Sum(nil))
	return result, nil
}
func cutoverPending(ctx context.Context, tx *sql.Tx) (map[string]int64, error) {
	counts := map[string]int64{}
	queries := map[string]string{
		"prepared_matches":    `SELECT count(*) FROM text_matches WHERE state='prepared'`,
		"started_matches":     `SELECT count(*) FROM text_matches WHERE state='started'`,
		"reserved_admissions": `SELECT count(*) FROM text_admissions WHERE state='reserved'`,
		"pending_settlements": `SELECT count(*) FROM text_settlements WHERE state='pending'`,
		"missing_settlements": `SELECT count(*) FROM text_admissions a JOIN text_matches m ON m.id=a.match_id WHERE m.state IN('completed','scored_low_population','interrupted') AND NOT EXISTS(SELECT 1 FROM text_settlements s WHERE s.match_id=a.match_id AND s.account_id=a.account_id)`,
		"provider_tasks":      `SELECT count(*) FROM billing_provider_tasks WHERE state='pending'`,
		"subscription_tasks":  `SELECT count(*) FROM billing_subscription_tasks WHERE state='pending'`,
		"operator_decisions":  `SELECT count(*) FROM admin_operation_decisions d WHERE NOT EXISTS(SELECT 1 FROM admin_operation_results r WHERE r.operation_id=d.id)`,
		"privacy_publication": `SELECT count(*) FROM privacy_requests WHERE suppression_sequence IS NULL`,
		"oauth_exchanges":     `SELECT count(*) FROM oauth_flows WHERE status='exchanging'`,
	}
	for name, query := range queries {
		var n int64
		if err := tx.QueryRowContext(ctx, query).Scan(&n); err != nil {
			return nil, ErrCutoverUnavailable
		}
		counts[name] = n
	}
	return counts, nil
}
func cutoverEvidenceAt(ctx context.Context, tx *sql.Tx, pins CutoverPins) ([]byte, error) {
	pending, err := cutoverPending(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, n := range pending {
		if n != 0 {
			return nil, ErrCutoverPending
		}
	}
	e := cutoverEvidence{Version: 1, Pins: pins, Tables: map[string]cutoverFingerprint{}, Sequences: map[string]cutoverFingerprint{}, Pending: pending}
	e.Schema, err = cutoverQueryFingerprint(ctx, tx, cutoverSchemaFingerprintSQL)
	if err != nil {
		return nil, err
	}
	for _, name := range cutoverTables {
		if strings.HasPrefix(name, "cutover_") {
			continue
		}
		f, err := cutoverTableFingerprint(ctx, tx, name, false)
		if err != nil {
			return nil, err
		}
		e.Tables[name] = f
	}
	for _, name := range cutoverSequences {
		f, err := cutoverTableFingerprint(ctx, tx, name, true)
		if err != nil {
			return nil, err
		}
		e.Sequences[name] = f
	}
	// Undelivered outbox notifications carry already committed value, not pending
	// mutation. Their complete rows are retained and fingerprinted for replay.
	raw, err := json.Marshal(e)
	if err != nil || len(raw) > 60000 {
		return nil, ErrCutoverUnavailable
	}
	return raw, nil
}

// physicalPreflight must commit before opening the retained-data snapshot.
// A census alone cannot repair a snapshot taken while a writer was still alive:
// that writer may commit and disappear before the census, leaving stale data.
// NOLOGIN plus the complete published/startup census prevents a new writer from
// entering between this transaction and the fresh snapshot. The session lane
// remains held throughout. Only the exact already-ready target permits writers.
func (c *CutoverController) physicalPreflight(ctx context.Context, readyTarget *CutoverBootstrap) error {
	return c.transaction(ctx, func(tx *sql.Tx) error {
		enabled := false
		if readyTarget != nil {
			var err error
			enabled, err = cutoverTargetReady(ctx, tx, *readyTarget)
			if err != nil {
				return err
			}
		}
		if err := c.verify(ctx, tx, enabled); err != nil {
			return err
		}
		if enabled {
			return nil
		}
		return c.fenceSessions(ctx, tx, false)
	})
}
func cutoverTargetReady(ctx context.Context, q cutoverRoleQuerier, b CutoverBootstrap) (bool, error) {
	var exists bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cutover_instances WHERE id=$1 AND parent_instance_id=$2 AND parent_watermark_id=$3 AND phase='ready' AND generation=0 AND cluster_system_identifier=$4 AND database_oid=$5 AND database_name=$6)`, b.Target.InstanceID, b.Proof.InstanceID, b.WatermarkID, b.Target.ClusterSystemIdentifier, b.Target.DatabaseOID, b.Target.DatabaseName).Scan(&exists); err != nil {
		return false, ErrCutoverUnavailable
	}
	return exists, nil
}

func (c *CutoverController) Seal(ctx context.Context, s CutoverSeal) (CutoverReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var result CutoverReceipt
	if !valueUUID(s.WatermarkID) {
		return result, ErrCutoverInvalid
	}
	if err := c.physicalPreflight(ctx, nil); err != nil {
		return result, err
	}
	err := c.transaction(ctx, func(tx *sql.Tx) error {
		if err := c.verify(ctx, tx, false); err != nil {
			return err
		}
		r, pins, err := c.authenticate(ctx, tx, s.Proof)
		if err != nil {
			return err
		}
		if err = c.fenceSessions(ctx, tx, false); err != nil {
			return err
		}
		raw, err := cutoverEvidenceAt(ctx, tx, pins)
		if err != nil {
			return err
		}
		if err = c.fenceSessions(ctx, tx, false); err != nil {
			return err
		}
		var priorID, hash string
		var same bool
		err = tx.QueryRowContext(ctx, `SELECT id,evidence_sha256,evidence=$2::jsonb FROM cutover_watermarks WHERE request_id=$1`, s.Proof.RequestID, string(raw)).Scan(&priorID, &hash, &same)
		if err == nil {
			if priorID != s.WatermarkID || !same || r.Phase != "sealed" {
				return ErrCutoverParity
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			if r.Phase != "closing" {
				return ErrCutoverConflict
			}
			err = tx.QueryRowContext(ctx, `INSERT INTO cutover_watermarks(id,instance_id,generation,request_id,wal_lsn,evidence,evidence_sha256) VALUES($1,$2,$3,$4,pg_current_wal_lsn(),$5::jsonb,encode(sha256(convert_to($5::jsonb::text,'UTF8')),'hex')) RETURNING evidence_sha256`, s.WatermarkID, s.Proof.InstanceID, s.Proof.Generation, s.Proof.RequestID, string(raw)).Scan(&hash)
			if err != nil {
				return ErrCutoverConflict
			}
			if _, err = tx.ExecContext(ctx, `UPDATE cutover_instances SET phase='sealed' WHERE id=$1`, s.Proof.InstanceID); err != nil {
				return ErrCutoverLease
			}
		} else {
			return ErrCutoverUnavailable
		}
		r.Phase = "sealed"
		r.WatermarkID = s.WatermarkID
		r.EvidenceSHA256 = hash
		result = r
		return nil
	})
	return result, err
}
func (c *CutoverController) checkSealed(ctx context.Context, tx *sql.Tx, h CutoverHandoff) (CutoverReceipt, error) {
	var empty CutoverReceipt
	if !validCutoverIdentity(h.Target) || !valueUUID(h.WatermarkID) || !valueUUID(h.HandoffID) {
		return empty, ErrCutoverInvalid
	}
	if err := c.verify(ctx, tx, false); err != nil {
		return empty, err
	}
	// A terminal immutable handoff cannot be renewed or retargeted. Expiry
	// still denies every new authority mutation, but must not strand the sole
	// already-bound target after a slow restore or uncertain response.
	var terminalReplay bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cutover_handoffs WHERE id=$1 AND instance_id=$2 AND generation=$3 AND request_id=$4 AND watermark_id=$5 AND target_instance_id=$6 AND target_cluster_system_identifier=$7 AND target_database_oid=$8 AND target_database_name=$9)`, h.HandoffID, h.Proof.InstanceID, h.Proof.Generation, h.Proof.RequestID, h.WatermarkID, h.Target.InstanceID, h.Target.ClusterSystemIdentifier, h.Target.DatabaseOID, h.Target.DatabaseName).Scan(&terminalReplay); err != nil {
		return empty, ErrCutoverUnavailable
	}
	r, pins, err := c.authenticate(ctx, tx, h.Proof, terminalReplay)
	if err != nil {
		return empty, err
	}
	if r.Phase != "sealed" || h.Target.ClusterSystemIdentifier == r.Identity.ClusterSystemIdentifier || h.Target.InstanceID == r.Identity.InstanceID {
		return empty, ErrCutoverConflict
	}
	if err = c.fenceSessions(ctx, tx, false); err != nil {
		return empty, err
	}
	raw, err := cutoverEvidenceAt(ctx, tx, pins)
	if err != nil {
		return empty, err
	}
	if err = c.fenceSessions(ctx, tx, false); err != nil {
		return empty, err
	}
	var same bool
	if err = tx.QueryRowContext(ctx, `SELECT evidence=$2::jsonb,evidence_sha256 FROM cutover_watermarks WHERE id=$1 AND request_id=$3`, h.WatermarkID, string(raw), h.Proof.RequestID).Scan(&same, &r.EvidenceSHA256); err != nil || !same {
		return empty, ErrCutoverParity
	}
	r.WatermarkID = h.WatermarkID
	return r, nil
}
func (c *CutoverController) Handoff(ctx context.Context, h CutoverHandoff) (CutoverReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var result CutoverReceipt
	if err := c.physicalPreflight(ctx, nil); err != nil {
		return result, err
	}
	err := c.transaction(ctx, func(tx *sql.Tx) error {
		r, err := c.checkSealed(ctx, tx, h)
		if err != nil {
			return err
		}
		var same bool
		err = tx.QueryRowContext(ctx, `SELECT id=$2 AND watermark_id=$3 AND target_instance_id=$4 AND target_cluster_system_identifier=$5 AND target_database_oid=$6 AND target_database_name=$7 FROM cutover_handoffs WHERE instance_id=$1`, h.Proof.InstanceID, h.HandoffID, h.WatermarkID, h.Target.InstanceID, h.Target.ClusterSystemIdentifier, h.Target.DatabaseOID, h.Target.DatabaseName).Scan(&same)
		if err == nil {
			if !same {
				return ErrCutoverConflict
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.ExecContext(ctx, `INSERT INTO cutover_handoffs(id,instance_id,generation,request_id,watermark_id,target_instance_id,target_cluster_system_identifier,target_database_oid,target_database_name) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, h.HandoffID, h.Proof.InstanceID, h.Proof.Generation, h.Proof.RequestID, h.WatermarkID, h.Target.InstanceID, h.Target.ClusterSystemIdentifier, h.Target.DatabaseOID, h.Target.DatabaseName)
			if err != nil {
				return ErrCutoverConflict
			}
		} else {
			return ErrCutoverUnavailable
		}
		r.HandoffID = h.HandoffID
		result = r
		return nil
	})
	return result, err
}

// Bootstrap holds both dedicated controller lanes throughout fresh source
// authentication, full restored parity and target activation. Different cluster
// IDs are mandatory because LOGIN flags are cluster-wide. It never enables the
// source roles. Caller-provided evidence/parity booleans are not accepted.
func (c *CutoverController) Bootstrap(ctx context.Context, source *CutoverController, b CutoverBootstrap) (CutoverReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var result CutoverReceipt
	if source == nil || c == source {
		return result, ErrCutoverInvalid
	}
	if err := source.physicalPreflight(ctx, nil); err != nil {
		return result, err
	}
	err := source.transaction(ctx, func(stx *sql.Tx) error {
		r, err := source.checkSealed(ctx, stx, b)
		if err != nil {
			return err
		}
		var binding bool
		if err = stx.QueryRowContext(ctx, `SELECT id=$2 AND watermark_id=$3 AND target_instance_id=$4 AND target_cluster_system_identifier=$5 AND target_database_oid=$6 AND target_database_name=$7 FROM cutover_handoffs WHERE instance_id=$1`, b.Proof.InstanceID, b.HandoffID, b.WatermarkID, b.Target.InstanceID, b.Target.ClusterSystemIdentifier, b.Target.DatabaseOID, b.Target.DatabaseName).Scan(&binding); err != nil || !binding {
			return ErrCutoverConflict
		}
		if err = c.physicalPreflight(ctx, &b); err != nil {
			return err
		}
		return c.transaction(ctx, func(ttx *sql.Tx) error {
			physical, err := cutoverPhysical(ctx, ttx)
			if err != nil {
				return err
			}
			if !samePhysical(physical, b.Target) {
				return ErrCutoverConflict
			}
			exists, err := cutoverTargetReady(ctx, ttx, b)
			if err != nil {
				return err
			}

			// Immutable ancestry is compared on both initial activation and response
			// replay. Only this target's own ready child is omitted; no source receipt
			// or earlier ancestor may disappear or change when target writes begin.
			for _, table := range cutoverAuthorityTables {
				sf, err := cutoverTableFingerprint(ctx, stx, table, false)
				if err != nil {
					return err
				}
				var tf cutoverFingerprint
				if table == "cutover_instances" {
					tf, err = cutoverQueryFingerprint(ctx, ttx, `SELECT hash FROM (SELECT encode(sha256(convert_to(to_jsonb(t)::text,'UTF8')),'hex') hash FROM public.cutover_instances t WHERE id<>$1::uuid) rows ORDER BY hash COLLATE "C"`, b.Target.InstanceID)
				} else {
					tf, err = cutoverTableFingerprint(ctx, ttx, table, false)
				}
				if err != nil {
					return err
				}
				if sf != tf {
					return ErrCutoverParity
				}
			}
			if exists {
				if err = c.verify(ctx, ttx, true); err != nil {
					return err
				}
				// An acknowledged ready target may already have valid new writes. A replay
				// only returns its bound receipt; it never overwrites data or repeats LOGIN.
				result = CutoverReceipt{Identity: b.Target, Phase: "ready", HandoffID: b.HandoffID, WatermarkID: b.WatermarkID, EvidenceSHA256: r.EvidenceSHA256}
				return nil
			}
			if err = c.verify(ctx, ttx, false); err != nil {
				return err
			}
			if err = c.fenceSessions(ctx, ttx, false); err != nil {
				return err
			}
			var pins CutoverPins
			if err = stx.QueryRowContext(ctx, `SELECT schema_sha256,image_sha256,config_sha256,content_sha256 FROM cutover_requests WHERE id=$1`, b.Proof.RequestID).Scan(&pins.SchemaSHA256, &pins.ImageSHA256, &pins.ConfigSHA256, &pins.ContentSHA256); err != nil {
				return ErrCutoverUnavailable
			}
			targetEvidence, err := cutoverEvidenceAt(ctx, ttx, pins)
			if err != nil {
				return err
			}
			var matches bool
			if err = stx.QueryRowContext(ctx, `SELECT evidence=$2::jsonb FROM cutover_watermarks WHERE id=$1`, b.WatermarkID, string(targetEvidence)).Scan(&matches); err != nil || !matches {
				return ErrCutoverParity
			}

			if _, err = ttx.ExecContext(ctx, `INSERT INTO cutover_instances(id,cluster_system_identifier,database_oid,database_name,parent_instance_id,parent_watermark_id) VALUES($1,$2,$3,$4,$5,$6)`, b.Target.InstanceID, b.Target.ClusterSystemIdentifier, b.Target.DatabaseOID, b.Target.DatabaseName, b.Proof.InstanceID, b.WatermarkID); err != nil {
				return ErrCutoverConflict
			}
			for _, role := range c.spec.Roles.writers() {
				if _, err = ttx.ExecContext(ctx, `ALTER ROLE `+pq.QuoteIdentifier(role)+` LOGIN`); err != nil {
					return ErrCutoverUnavailable
				}
			}
			result = CutoverReceipt{Identity: b.Target, Phase: "ready", HandoffID: b.HandoffID, WatermarkID: b.WatermarkID, EvidenceSHA256: r.EvidenceSHA256}
			return nil
		})
	})
	return result, err
}

// Stable PostgreSQL16 deparsed definitions exclude cluster-local OIDs. Roles and
// ACLs are checked independently against the exact provisioned contract. This
// digest includes empty tables, all constraints/indexes/triggers/functions and
// sequence parameters, so unchanged data cannot hide structural restore drift.
const cutoverSchemaFingerprintSQL = `
WITH objects AS (
 SELECT jsonb_build_array('relation',c.relname,c.relkind,c.relpersistence,c.relrowsecurity,c.relforcerowsecurity) entry
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN('r','S')
 UNION ALL
 SELECT jsonb_build_array('column',c.relname,a.attnum,a.attname,format_type(a.atttypid,a.atttypmod),a.attnotnull,a.attidentity,a.attgenerated,a.attstorage,co.collname,pg_get_expr(d.adbin,d.adrelid))
 FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace LEFT JOIN pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum LEFT JOIN pg_collation co ON co.oid=a.attcollation
 WHERE n.nspname='public' AND c.relkind='r' AND a.attnum>0 AND NOT a.attisdropped
 UNION ALL
 SELECT jsonb_build_array('constraint',c.relname,k.conname,pg_get_constraintdef(k.oid,false),k.convalidated)
 FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'
 UNION ALL
 SELECT jsonb_build_array('index',c.relname,pg_get_indexdef(i.indexrelid),i.indisvalid,i.indisready)
 FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'
 UNION ALL
 SELECT jsonb_build_array('trigger',c.relname,t.tgname,pg_get_triggerdef(t.oid,false),t.tgenabled)
 FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND NOT t.tgisinternal
 UNION ALL
 SELECT jsonb_build_array('function',p.proname,pg_get_functiondef(p.oid)) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public'
 UNION ALL
 SELECT jsonb_build_array('rewrite',c.relname,r.rulename,r.ev_enabled,pg_get_ruledef(r.oid,false))
 FROM pg_rewrite r JOIN pg_class c ON c.oid=r.ev_class JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'
 UNION ALL
 SELECT jsonb_build_array('inheritance',c.relname,p.relname,i.inhseqno,i.inhdetachpending)
 FROM pg_inherits i JOIN pg_class c ON c.oid=i.inhrelid JOIN pg_class p ON p.oid=i.inhparent JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'
 UNION ALL
 SELECT jsonb_build_array('sequence_owner',s.relname,t.relname,a.attname,d.deptype)
 FROM pg_depend d JOIN pg_class s ON s.oid=d.objid JOIN pg_namespace n ON n.oid=s.relnamespace JOIN pg_class t ON t.oid=d.refobjid JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=d.refobjsubid
 WHERE n.nspname='public' AND s.relkind='S' AND d.classid='pg_class'::regclass AND d.refclassid='pg_class'::regclass AND d.deptype IN('a','i')
 UNION ALL
 SELECT jsonb_build_array('sequence',c.relname,format_type(s.seqtypid,NULL),s.seqstart,s.seqincrement,s.seqmax,s.seqmin,s.seqcache,s.seqcycle)
 FROM pg_sequence s JOIN pg_class c ON c.oid=s.seqrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'
), hashes AS (SELECT encode(sha256(convert_to(entry::text,'UTF8')),'hex') hash FROM objects)
SELECT hash FROM hashes ORDER BY hash COLLATE "C"`
