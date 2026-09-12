package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTextOwnerBusy         = errors.New("text.owner_busy")
	ErrTextOwnerLost         = errors.New("text.owner_lost")
	ErrTextOwnerLegacyActive = errors.New("text.ownerless_active_work_requires_classification")
)

// These two integers identify a database-local, session-level advisory lock.
// Reacquisition is not a health check: PostgreSQL session locks are reentrant.
const textOwnerLockClass = 1263423311
const textOwnerLockObject = 13

type TextOwnerToken struct {
	IncarnationID string
	Generation    int64
}

// TextOwner is one process incarnation. A lost object is permanently unusable.
// Its pinned physical connection is never returned to the shared pool alive.
type TextOwner struct {
	db        *sql.DB
	conn      *sql.Conn
	token     TextOwnerToken
	mu        sync.Mutex
	done      chan struct{}
	watchDone chan struct{}
	ready     atomic.Bool
}

func AcquireTextOwner(ctx context.Context, db *sql.DB) (*TextOwner, error) {
	if db == nil || db.Stats().MaxOpenConnections == 1 {
		return nil, ErrValueConflict
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	acquired := false
	defer func() {
		if !acquired {
			discardTextOwnerConnection(conn)
		}
	}()
	var locked bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1,$2)`, textOwnerLockClass, textOwnerLockObject).Scan(&locked); err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrTextOwnerBusy
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `LOCK TABLE text_process_current IN EXCLUSIVE MODE`); err != nil {
		return nil, err
	}
	var oldGeneration int64
	err = tx.QueryRowContext(ctx, `SELECT generation FROM text_process_current WHERE singleton=1 FOR UPDATE`).Scan(&oldGeneration)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if oldGeneration > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE text_process_owners SET lost_at=COALESCE(lost_at,clock_timestamp()) WHERE generation=$1`, oldGeneration); err != nil {
			return nil, err
		}
	}
	owner := &TextOwner{db: db, conn: conn, token: TextOwnerToken{IncarnationID: uuid.NewString(), Generation: oldGeneration + 1}, done: make(chan struct{}), watchDone: make(chan struct{})}
	if _, err = tx.ExecContext(ctx, `INSERT INTO text_process_owners(incarnation_id,generation,backend_pid,backend_started_at) SELECT $1,$2,pid,backend_start FROM pg_stat_activity WHERE pid=pg_backend_pid()`, owner.token.IncarnationID, owner.token.Generation); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO text_process_current(singleton,incarnation_id,generation) VALUES(1,$1,$2) ON CONFLICT(singleton) DO UPDATE SET incarnation_id=EXCLUDED.incarnation_id,generation=EXCLUDED.generation`, owner.token.IncarnationID, owner.token.Generation); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	acquired = true
	go owner.watch()
	return owner, nil
}

func discardTextOwnerConnection(conn *sql.Conn) {
	// Raw's ErrBadConn instructs database/sql to discard the physical connection;
	// Close alone would return a live session-held advisory lock to the pool.
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}
func (o *TextOwner) Token() TextOwnerToken { return o.token }
func (o *TextOwner) Done() <-chan struct{} { return o.done }
func (o *TextOwner) loseLocked() {
	select {
	case <-o.done:
		return
	default:
	}
	close(o.done)
	o.ready.Store(false)
	discardTextOwnerConnection(o.conn)
}
func (o *TextOwner) watch() {
	defer close(o.watchDone)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-o.done:
			return
		case <-ticker.C:
			// Check owns the probe timeout. A deadline while waiting for another
			// serialized probe must not stop this watch while authority is healthy.
			err := o.Check(context.Background())
			if err != nil {
				return
			}
		}
	}
}

const textOwnerAuthoritySQL = `SELECT EXISTS(
 SELECT 1 FROM text_process_current c JOIN text_process_owners o USING(incarnation_id,generation)
 JOIN pg_stat_activity a ON a.pid=o.backend_pid AND a.backend_start=o.backend_started_at
 JOIN pg_locks l ON l.pid=a.pid AND l.locktype='advisory' AND l.granted
 AND l.classid=$3::oid AND l.objid=$4::oid AND l.objsubid=2
 AND l.database=(SELECT oid FROM pg_database WHERE datname=current_database())
 WHERE c.singleton=1 AND c.incarnation_id=$1 AND c.generation=$2 AND o.lost_at IS NULL)`

// Check observes actual backend/advisory authority with a bounded server probe.
// A request cancellation must never cancel this process-wide physical session.
// Probe failure still fences locally; only lock acquisition confirms durable loss.
func (o *TextOwner) Check(ctx context.Context) error {
	if err := o.lockProbe(ctx); err != nil {
		return err
	}
	defer o.mu.Unlock()
	select {
	case <-o.done:
		return ErrTextOwnerLost
	default:
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	probe, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var valid bool
	err := o.conn.QueryRowContext(probe, textOwnerAuthoritySQL, o.token.IncarnationID, o.token.Generation, textOwnerLockClass, textOwnerLockObject).Scan(&valid)
	if err != nil || !valid {
		o.loseLocked()
		return ErrTextOwnerLost
	}
	return ctx.Err()
}

// Cancellation only stops the caller's queue wait. Once admitted, Check uses
// its independent bounded probe so a player cannot cancel the physical owner.
func (o *TextOwner) lockProbe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.mu.TryLock() {
		return nil
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-o.done:
			return ErrTextOwnerLost
		case <-ticker.C:
			if err := ctx.Err(); err != nil {
				return err
			}
			if o.mu.TryLock() {
				return nil
			}
		}
	}
}

// Release waits for the owner mutex within ctx. A canceled queue wait leaves
// authority intact and reports failure; callers must not claim a stopped owner.
// Once admitted, it unlocks and physically discards the session even if the SQL
// request fails. The next acquired owner records the durable loss.
func (o *TextOwner) Release(ctx context.Context) error {
	if err := o.lockProbe(ctx); err != nil {
		if errors.Is(err, ErrTextOwnerLost) {
			return nil
		}
		return err
	}
	defer o.mu.Unlock()
	select {
	case <-o.done:
		return nil
	default:
	}
	var unlocked bool
	err := o.conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1,$2)`, textOwnerLockClass, textOwnerLockObject).Scan(&unlocked)
	o.loseLocked()
	if err != nil || !unlocked {
		return ErrTextOwnerLost
	}
	return nil
}

// Wait joins the actual heartbeat goroutine after release or physical loss.
// Done signals lost authority; it alone is not a goroutine-completion receipt.
func (o *TextOwner) Wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-o.watchDone:
		return ctx.Err()
	}
}

func (s *TextValueStore) WithOwner(owner *TextOwner) (*TextValueStore, error) {
	if owner == nil || owner.db != s.db {
		return nil, ErrValueConflict
	}
	copy := *s
	copy.owner = owner
	return &copy, nil
}

// ownerFence is always first in an owner transaction's lock order. Its SHARE
// lock remains through commit, so a new singleton generation cannot pass an
// already accepted writer. A lock loss after this check can only be recovered
// after that transaction commits or rolls back.
func (s *TextValueStore) ownerFence(ctx context.Context, tx *sql.Tx, recovery bool) error {
	// RLS-hidden ownership is an error, never evidence that no owner exists.
	if _, err := tx.ExecContext(ctx, `SET LOCAL row_security=off`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `LOCK TABLE text_process_current IN SHARE MODE`); err != nil {
		return err
	}
	var token TextOwnerToken
	err := tx.QueryRowContext(ctx, `SELECT incarnation_id,generation FROM text_process_current WHERE singleton=1 FOR SHARE`).Scan(&token.IncarnationID, &token.Generation)
	if err == sql.ErrNoRows {
		if s.owner == nil {
			return nil
		}
		return ErrValueFence
	}
	if err != nil {
		return err
	}
	if s.owner == nil || s.owner.token != token {
		return ErrValueFence
	}
	select {
	case <-s.owner.done:
		return ErrValueFence
	default:
	}
	if !recovery && !s.owner.ready.Load() {
		return ErrValueFence
	}
	var valid bool
	if err = tx.QueryRowContext(ctx, textOwnerAuthoritySQL, token.IncarnationID, token.Generation, textOwnerLockClass, textOwnerLockObject).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrValueFence
	}
	return nil
}
func (s *TextValueStore) ownerTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	return s.transaction(ctx, func(tx *sql.Tx) error {
		if err := s.ownerFence(ctx, tx, false); err != nil {
			return err
		}
		return fn(tx)
	})
}
func (s *TextValueStore) processOwner() (any, any) {
	if s.owner == nil {
		return nil, nil
	}
	return s.owner.token.IncarnationID, s.owner.token.Generation
}
func (s *TextValueStore) validateProcessOwner(owner string) error {
	if s.owner != nil && s.owner.token.IncarnationID != owner {
		return ErrValueFence
	}
	return nil
}
