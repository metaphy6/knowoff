package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type AccountSanctionStore struct{ db *sql.DB }

func NewAccountSanctionStore(db *sql.DB) *AccountSanctionStore { return &AccountSanctionStore{db} }

// Apply commits authorization, immutable provenance and revocation together.
// revoke must perform only SQL on this transaction and preserve identity/value.
func (s *AccountSanctionStore) Apply(ctx context.Context, actor string, c AdminOperationCommand, revoke func(context.Context, *sql.Tx, string) error) (AdminOperationReceipt, error) {
	var receipt AdminOperationReceipt
	if s == nil || s.db == nil || revoke == nil || (c.Kind != "account_sanction" && c.Kind != "sanction_lift") {
		return receipt, ErrAdminOperation
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ops := NewAdminOperationStore(s.db)
	err := WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		var err error
		receipt, err = ops.DecideTx(ctx, tx, actor, c, []string{c.TargetAccountID})
		if err != nil {
			return err
		}
		if receipt.Status != "pending" {
			return nil
		}
		if c.Kind == "account_sanction" {
			var applied bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_sanctions WHERE operation_id=$1)`, c.ID).Scan(&applied); err != nil {
				return err
			}
			if applied {
				return LockAdminTx(ctx, tx, actor, []string{"admin"}, c.TargetAccountID)
			}
			var future bool
			if c.Until != nil {
				if err = tx.QueryRowContext(ctx, `SELECT $1::timestamptz>clock_timestamp()`, c.Until).Scan(&future); err != nil {
					return err
				}
				if !future {
					return ErrAdminOperation
				}
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO account_sanctions(operation_id,account_id,until_at) VALUES($1,$2,$3)`, c.ID, c.TargetAccountID, c.Until); err != nil {
				return err
			}
			// Keyset batches bound memory; the transaction either captures every link
			// or rolls back on timeout. No arbitrary installation count is discarded.
			after := ""
			for {
				rows, err := tx.QueryContext(ctx, `SELECT device_hash FROM device_tokens WHERE account_id=$1 AND device_hash>$2 ORDER BY device_hash LIMIT 100`, c.TargetAccountID, after)
				if err != nil {
					return err
				}
				var hashes []string
				for rows.Next() {
					var hash string
					if err = rows.Scan(&hash); err != nil {
						rows.Close()
						return err
					}
					hashes = append(hashes, hash)
				}
				err = rows.Err()
				rows.Close()
				if err != nil {
					return err
				}
				if len(hashes) == 0 {
					break
				}
				for _, hash := range hashes {
					if _, err = tx.ExecContext(ctx, `INSERT INTO auth_installations(device_hash) VALUES($1) ON CONFLICT DO NOTHING`, hash); err != nil {
						return err
					}
					var locked string
					if err = tx.QueryRowContext(ctx, `SELECT device_hash FROM auth_installations WHERE device_hash=$1 FOR UPDATE`, hash).Scan(&locked); err != nil {
						return err
					}
					if _, err = tx.ExecContext(ctx, `INSERT INTO account_sanction_installations(operation_id,device_hash) VALUES($1,$2)`, c.ID, hash); err != nil {
						return err
					}
				}
				after = hashes[len(hashes)-1]
			}
			if err = revoke(ctx, tx, c.TargetAccountID); err != nil {
				return err
			}
		} else {
			result, err := tx.ExecContext(ctx, `INSERT INTO account_sanction_lifts(operation_id,sanction_id) SELECT $1,operation_id FROM account_sanctions WHERE operation_id=$2 AND account_id=$3 ON CONFLICT(sanction_id) DO NOTHING`, c.ID, c.PriorSanctionID, c.TargetAccountID)
			if err != nil {
				return err
			}
			if n, err := result.RowsAffected(); err != nil || n != 1 {
				return ErrAdminOperation
			}
		}
		if err = LockAdminTx(ctx, tx, actor, []string{"admin"}, c.TargetAccountID); err != nil {
			return err
		}
		if c.Kind == "sanction_lift" {
			if err = ops.CompleteTx(ctx, tx, c.ID, AdminOperationResult{Outcome: "applied", Detail: c.Kind}); err != nil {
				return err
			}
		}
		receipt, err = readAdminOperation(ctx, tx, c.ID)
		return err
	})
	if err != nil {
		return AdminOperationReceipt{}, err
	}
	return receipt, nil
}

// ActiveForBinding is a live-delivery check for one exact receipt and the peer's
// current verified binding. It never revokes epochs, including on retries.
func (s *AccountSanctionStore) ActiveForBinding(ctx context.Context, id, account, hash string) (bool, error) {
	var active bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_sanctions s WHERE s.operation_id=$1 AND (s.account_id=$2 OR EXISTS(SELECT 1 FROM account_sanction_installations i WHERE i.operation_id=s.operation_id AND i.device_hash=$3)) AND (s.until_at IS NULL OR s.until_at>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM account_sanction_lifts l WHERE l.sanction_id=s.operation_id))`, id, account, hash).Scan(&active)
	return active, err
}

// ResumePending holds no SQL locks while invoking the lobby. Delivery checks
// every current peer against the receipt under lobby serialization.
func (s *AccountSanctionStore) ResumePending(ctx context.Context, limit int, deliver func(context.Context, string) error) error {
	if limit < 1 || limit > 100 || deliver == nil {
		return ErrAdminOperation
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT s.operation_id FROM account_sanctions s WHERE NOT EXISTS(SELECT 1 FROM account_sanction_deliveries d WHERE d.operation_id=s.operation_id) ORDER BY s.created_at,s.operation_id LIMIT $1`, limit)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.DeliverPending(ctx, id, deliver); err != nil {
			return err
		}
	}
	return nil
}

func (s *AccountSanctionStore) DeliverPending(parent context.Context, id string, deliver func(context.Context, string) error) error {
	if !valueUUID(id) || deliver == nil {
		return ErrAdminOperation
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(parent, cancel)
	defer stop()
	if err := parent.Err(); err != nil {
		return err
	}
	var err error
	if err = deliver(ctx, id); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	err = func() error {
		defer tx.Rollback()
		decision, err := readAdminOperation(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = lockAdminOperationAccounts(ctx, tx, decision.AffectedAccounts); err != nil {
			return err
		}
		if err = lockAdminOperation(ctx, tx, id); err != nil {
			return err
		}
		var delivered bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_sanction_deliveries WHERE operation_id=$1)`, id).Scan(&delivered); err != nil {
			return err
		}
		if delivered {
			return tx.Commit()
		}
		// The delivery outcome is historical. A concurrent expiry/lift only makes
		// subsequent admission possible; it cannot reinstate the delivered peer.
		var outcome string
		if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN (until_at IS NULL OR until_at>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM account_sanction_lifts WHERE sanction_id=$1) THEN 'applied' ELSE 'obsolete' END FROM account_sanctions WHERE operation_id=$1`, id).Scan(&outcome); err != nil {
			return err
		}
		if err := NewAdminOperationStore(s.db).CompleteTx(ctx, tx, id, AdminOperationResult{Outcome: outcome, Detail: "account_sanction_live_delivery"}); err != nil {
			return err
		}
		r, err := tx.ExecContext(ctx, `INSERT INTO account_sanction_deliveries(operation_id,outcome) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, outcome)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if n == 1 {
			if _, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log(action,target_type,target_id,after_state) VALUES('sanction_live_delivery','admin_operation',$1,jsonb_build_object('outcome',$2::text))`, id, outcome); err != nil {
				return err
			}
		}
		return tx.Commit()
	}()
	if err != nil {
		return fmt.Errorf("sanction delivery: %w", err)
	}
	return nil
}
