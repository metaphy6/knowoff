package store

import (
	"context"
	"database/sql"
	"time"
)

type TextOwnerRecovery struct {
	Released, Cancelled, Interrupted, Pending int
	Done                                      bool
}

// RecoverLostOwners closes only work whose old authority is durably lost. Each
// item and its compensation commit together; a successor can resume after any
// batch without a cursor, hidden state, or fabricated terminal result.
func (o *TextOwner) RecoverLostOwners(ctx context.Context, values *TextValueStore, limit int) (TextOwnerRecovery, error) {
	result := TextOwnerRecovery{}
	if values == nil || values.owner != o || limit < 1 || limit > 1000 {
		return result, ErrValueConflict
	}
	for i := 0; i < limit; i++ {
		kind := ""
		at := time.Now().UTC()
		err := values.transaction(ctx, func(tx *sql.Tx) error {
			if err := values.ownerFence(ctx, tx, true); err != nil {
				return err
			}
			var legacy bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM text_matches WHERE process_generation IS NULL AND state IN ('prepared','started')) OR EXISTS(SELECT 1 FROM text_admissions WHERE process_generation IS NULL AND state IN ('reserved','started'))`).Scan(&legacy); err != nil {
				return err
			}
			if legacy {
				return ErrTextOwnerLegacyActive
			}
			var id string
			err := tx.QueryRowContext(ctx, `SELECT kind,id FROM (
    SELECT 'match' AS kind,m.id,o.generation FROM text_matches m JOIN text_process_owners o ON o.generation=m.process_generation WHERE o.lost_at IS NOT NULL AND m.state IN ('prepared','started')
    UNION ALL SELECT 'reservation',a.id,o.generation FROM text_admissions a JOIN text_process_owners o ON o.generation=a.process_generation WHERE o.lost_at IS NOT NULL AND a.state='reserved' AND a.match_id IS NULL
   ) work ORDER BY generation,kind,id LIMIT 1`).Scan(&kind, &id)
			if err == sql.ErrNoRows {
				return nil
			}
			if err != nil {
				return err
			}
			if kind == "match" {
				m, err := lockTextMatch(ctx, tx, id)
				if err != nil {
					return err
				}
				// Another recovery transaction may have completed this candidate already.
				switch m.state {
				case "started":
					kind = "interrupted"
					return values.interruptTx(ctx, tx, id, m.record.Owner, m.epoch, at)
				case "prepared":
					kind = "cancelled"
					return values.cancelPreparedTx(ctx, tx, id, m.record.Owner, m.epoch, at)
				default:
					kind = ""
					return nil
				}
			}
			var account string
			if err = tx.QueryRowContext(ctx, `SELECT account_id FROM text_admissions WHERE id=$1`, id).Scan(&account); err != nil {
				return err
			}
			request, err := lockAcceptedTextTombstone(ctx, tx, account)
			if err != nil {
				return err
			}
			var admissionOwner string
			if err = tx.QueryRowContext(ctx, `SELECT process_owner_id FROM text_admissions WHERE id=$1 AND account_id=$2 AND state='reserved' AND match_id IS NULL FOR UPDATE`, id, account).Scan(&admissionOwner); err == sql.ErrNoRows {
				kind = ""
				return nil
			} else if err != nil {
				return err
			}
			var lost bool
			if err = tx.QueryRowContext(ctx, `SELECT lost_at IS NOT NULL FROM text_process_owners WHERE incarnation_id=$1`, admissionOwner).Scan(&lost); err != nil {
				return err
			}
			if !lost {
				return ErrValueFence
			}
			if request != "" {
				if err = recordTextAdmissionErasure(ctx, tx, acceptedTextAccount{admission: id, account: account, request: request}, "cancel_reservation", at); err != nil {
					return err
				}
			}
			update, err := tx.ExecContext(ctx, `UPDATE text_admissions SET state='released' WHERE id=$1 AND state='reserved' AND match_id IS NULL`, id)
			if err != nil {
				return err
			}
			n, err := update.RowsAffected()
			if err != nil {
				return err
			}
			if n == 0 {
				kind = ""
			}
			return nil
		})
		if err != nil {
			return result, err
		}
		switch kind {
		case "reservation":
			result.Released++
		case "cancelled":
			result.Cancelled++
		case "interrupted":
			result.Interrupted++
		}
		if kind == "" {
			break
		}
	}
	err := values.transaction(ctx, func(tx *sql.Tx) error {
		if err := values.ownerFence(ctx, tx, true); err != nil {
			return err
		}
		var remaining bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM text_matches m JOIN text_process_owners o ON o.generation=m.process_generation WHERE o.lost_at IS NOT NULL AND m.state IN ('prepared','started')) OR EXISTS(SELECT 1 FROM text_admissions a JOIN text_process_owners o ON o.generation=a.process_generation WHERE o.lost_at IS NOT NULL AND a.state='reserved' AND a.match_id IS NULL)`).Scan(&remaining); err != nil {
			return err
		}
		result.Done = !remaining
		if _, err := tx.ExecContext(ctx, `UPDATE text_process_owners SET recovered_at=clock_timestamp() WHERE lost_at IS NOT NULL AND recovered_at IS NULL AND generation IN (SELECT o.generation FROM text_process_owners o WHERE o.lost_at IS NOT NULL AND o.recovered_at IS NULL AND NOT EXISTS(SELECT 1 FROM text_matches m WHERE m.process_generation=o.generation AND m.state IN ('prepared','started')) AND NOT EXISTS(SELECT 1 FROM text_admissions a WHERE a.process_generation=o.generation AND a.state='reserved' AND a.match_id IS NULL) ORDER BY o.generation LIMIT $1)`, limit); err != nil {
			return err
		}
		var unrecovered bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM text_process_owners WHERE lost_at IS NOT NULL AND recovered_at IS NULL)`).Scan(&unrecovered); err != nil {
			return err
		}
		result.Done = result.Done && !unrecovered
		return tx.QueryRowContext(ctx, `SELECT count(*) FROM text_settlements WHERE state='pending'`).Scan(&result.Pending)
	})
	if err == nil && result.Done {
		o.ready.Store(true)
	}
	return result, err
}
