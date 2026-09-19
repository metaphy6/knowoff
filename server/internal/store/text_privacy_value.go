package store

import (
	"bytes"
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// A delivery/read uses active authority, never the accepted-work exception.
func lockActiveTextRecipient(ctx context.Context, tx *sql.Tx, account string) error {
	var deleted sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&deleted); err != nil {
		if err == sql.ErrNoRows {
			return ErrValueFence
		}
		return err
	}
	var fenced bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_deletion_fences WHERE account_id=$1)`, account).Scan(&fenced); err != nil {
		return err
	}
	if deleted.Valid || fenced {
		return ErrValueFence
	}
	return nil
}

// WithTextDeliveryEnqueue linearizes a bounded queue append against deletion.
// The caller already holds its lobby lock; enqueue must neither reenter the
// lobby nor perform SQL/socket I/O. Previously queued frames remain a separate
// transport-disconnect concern. This transaction MUST NOT retry the callback.
func (s *TextValueStore) WithTextDeliveryEnqueue(ctx context.Context, worker string, delivery TextDelivery, enqueue func() error) error {
	if !valueUUID(worker) || !valueUUID(delivery.AccountID) || !valueUUID(delivery.MatchID) || delivery.ID < 1 || enqueue == nil {
		return ErrValueConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockActiveTextRecipient(ctx, tx, delivery.AccountID); err != nil {
		return err
	}
	var payload []byte
	var until time.Time
	err = tx.QueryRowContext(ctx, `SELECT payload,claim_until FROM text_outbox WHERE id=$1 AND account_id=$2 AND match_id=$3 AND claimed_by=$4 AND acknowledged_at IS NULL AND claim_until>clock_timestamp() FOR UPDATE`, delivery.ID, delivery.AccountID, delivery.MatchID, worker).Scan(&payload, &until)
	if err == sql.ErrNoRows {
		return ErrValueFence
	}
	if err != nil {
		return err
	}
	var current bool
	if err = tx.QueryRowContext(ctx, `SELECT $1::timestamptz>clock_timestamp()`, until).Scan(&current); err != nil {
		return err
	}
	if !bytes.Equal(payload, delivery.Payload) || !current {
		return ErrValueFence
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = enqueue(); err != nil {
		return err
	}
	return tx.Commit()
}

// acceptedTextAccount is only constructed for an existing accepted admission.
// Its tombstone authority can suppress effects, never authorize ordinary value.
type acceptedTextAccount struct {
	admission, account, request, contractHash, policyHash string
}

func lockAcceptedTextAccount(ctx context.Context, tx *sql.Tx, m lockedTextMatch, account string) (acceptedTextAccount, error) {
	var a acceptedTextAccount
	a.account = account
	if m.state != "prepared" && m.state != "started" && m.state != "completed" && m.state != "scored_low_population" {
		return a, ErrValueFence
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM text_admissions WHERE match_id=$1 AND account_id=$2`, m.record.Contract.MatchID, account).Scan(&a.admission); err != nil {
		return a, err
	}
	_, hash, err := valueHash(m.record)
	if err != nil {
		return a, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT contract_hash FROM text_matches WHERE id=$1`, m.record.Contract.MatchID).Scan(&a.contractHash); err != nil {
		return a, err
	}
	if a.contractHash != hash {
		return a, ErrValueConflict
	}
	a.policyHash, err = m.record.Policy.SHA256()
	if err != nil {
		return a, err
	}
	a.request, err = lockAcceptedTextTombstone(ctx, tx, account)
	return a, err
}

// Only accepted-work callers may use this primitive. Each proves its immutable
// admission and owner/state first; standalone cancellation additionally locks
// and revalidates its no-match admission after this canonical account row.
func lockAcceptedTextTombstone(ctx context.Context, tx *sql.Tx, account string) (string, error) {
	var deleted sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&deleted); err != nil {
		return "", err
	}
	var request string
	err := tx.QueryRowContext(ctx, `SELECT request_id FROM account_deletion_fences WHERE account_id=$1`, account).Scan(&request)
	if err == sql.ErrNoRows && !deleted.Valid {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return request, nil
}

func recordTextErasure(ctx context.Context, tx *sql.Tx, a acceptedTextAccount, operation string, ordinal int, hash string, at time.Time, outcomeHash string) (string, error) {
	if a.request == "" {
		return "", ErrValueConflict
	}
	var id, previous string
	err := tx.QueryRowContext(ctx, `SELECT id,body_sha256 FROM text_value_erasure_dispositions WHERE admission_id=$1 AND operation=$2 AND ordinal=$3`, a.admission, operation, ordinal).Scan(&id, &previous)
	if err == nil {
		if previous != hash {
			return "", ErrValueConflict
		}
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	id = uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO text_value_erasure_dispositions(id,request_id,admission_id,operation,ordinal,body_sha256,contract_sha256,policy_sha256,outcome_sha256,occurred_at,account_id) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,$11)`, id, a.request, a.admission, operation, ordinal, hash, a.contractHash, a.policyHash, outcomeHash, valueTime(at), a.account)
	return id, err
}

func erasedTextEventReplay(ctx context.Context, tx *sql.Tx, match, account, operation string, ordinal int, hash string) (bool, error) {
	var previous string
	err := tx.QueryRowContext(ctx, `SELECT d.body_sha256 FROM text_value_erasure_dispositions d JOIN text_admissions a ON a.id=d.admission_id WHERE a.match_id=$1 AND a.account_id=$2 AND d.operation=$3 AND d.ordinal=$4`, match, account, operation, ordinal).Scan(&previous)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if previous != hash {
		return false, ErrValueConflict
	}
	return true, nil
}

func markTextSettlementErased(ctx context.Context, tx *sql.Tx, m lockedTextMatch, account string, a acceptedTextAccount, operation string, at time.Time) error {
	if !m.hash.Valid {
		return ErrValueConflict
	}
	id, err := recordTextErasure(ctx, tx, a, operation, 0, m.hash.String, at, m.hash.String)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE text_settlements SET state='erased',erased_at=clock_timestamp(),erasure_disposition_id=$3 WHERE match_id=$1 AND account_id=$2 AND state='pending'`, m.record.Contract.MatchID, account, id)
	return err
}

func recordTextAdmissionErasure(ctx context.Context, tx *sql.Tx, a acceptedTextAccount, operation string, at time.Time) error {
	_, hash, err := valueHash(struct{ Admission, Operation, Contract string }{a.admission, operation, a.contractHash})
	if err != nil {
		return err
	}
	_, err = recordTextErasure(ctx, tx, a, operation, 0, hash, at, "")
	return err
}
