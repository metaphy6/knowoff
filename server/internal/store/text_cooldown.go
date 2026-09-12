package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrTextCooldown = errors.New("admission.cooldown")

type TextAbandon struct {
	MatchID, Owner, AccountID string
	Epoch                     int64
	Seat                      int
	At                        time.Time
}

// Abandon records the first genuine server-observed expired grace for one seat.
// The match/account locks also serialize Start and all other value writers.
func (s *TextValueStore) Abandon(ctx context.Context, a TextAbandon) error {
	if err := s.validateProcessOwner(a.Owner); err != nil {
		return err
	}
	a.At = valueTime(a.At)
	if !valueUUID(a.MatchID) || !valueUUID(a.AccountID) || !valueUUID(a.Owner) || a.At.IsZero() || a.Epoch < 1 || a.Seat < 0 || a.Seat >= 6 {
		return ErrValueConflict
	}
	return s.ownerTransaction(ctx, func(tx *sql.Tx) error {
		m, err := lockTextMatch(ctx, tx, a.MatchID)
		if err != nil {
			return err
		}
		if m.record.Owner != a.Owner || m.epoch != a.Epoch {
			return ErrValueFence
		}
		var seat int
		if err = tx.QueryRowContext(ctx, `SELECT seat FROM text_admissions WHERE match_id=$1 AND account_id=$2`, a.MatchID, a.AccountID).Scan(&seat); err != nil {
			if err == sql.ErrNoRows {
				return ErrValueConflict
			}
			return err
		}
		if seat != a.Seat {
			return ErrValueConflict
		}
		if err = valueAccountLock(ctx, tx, a.AccountID); err != nil {
			return err
		}
		_, hash, err := valueHash(a)
		if err != nil {
			return err
		}
		var old string
		err = tx.QueryRowContext(ctx, `SELECT body_hash FROM text_abandons WHERE match_id=$1 AND account_id=$2`, a.MatchID, a.AccountID).Scan(&old)
		if err == nil {
			if old != hash {
				return ErrValueConflict
			}
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
		if err = checkValueFence(m, a.Owner, a.Epoch, "started"); err != nil {
			return err
		}
		if !m.started.Valid || a.At.Before(m.started.Time) {
			return ErrValueConflict
		}
		if m.record.Prototype || m.record.Contract.Eligibility.EntryPath != "quick_play" {
			return nil
		}
		var previous int
		var retained sql.NullTime
		err = tx.QueryRowContext(ctx, `SELECT abandon_count,cooldown_until FROM queue_cooldowns WHERE account_id=$1`, a.AccountID).Scan(&previous, &retained)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if previous < 0 {
			return ErrValueConflict
		}
		schedule := m.record.Policy.Game.AbandonCooldownsS
		if len(schedule) == 0 {
			return ErrValueConflict
		}
		seconds := schedule[min(previous, len(schedule)-1)]
		if seconds <= 0 {
			return ErrValueConflict
		}
		until := a.At.Add(time.Duration(seconds) * time.Second)
		if retained.Valid && retained.Time.After(until) {
			until = retained.Time
		}
		applied := min(previous+1, 2147483647)
		if _, err = tx.ExecContext(ctx, `INSERT INTO queue_cooldowns(account_id,abandon_count,cooldown_until,updated_at) VALUES($1,$2,$3,$4) ON CONFLICT(account_id) DO UPDATE SET abandon_count=EXCLUDED.abandon_count,cooldown_until=EXCLUDED.cooldown_until,updated_at=GREATEST(queue_cooldowns.updated_at,EXCLUDED.updated_at)`, a.AccountID, applied, until, a.At); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO text_abandons(match_id,account_id,seat,occurred_at,body_hash,prior_count,applied_count,duration_seconds,cooldown_until) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, a.MatchID, a.AccountID, a.Seat, a.At, hash, previous, applied, seconds, until)
		return err
	})
}

// The caller holds the canonical account lock. Local and server-only prototype
// admissions never consume or enforce Quick Play abandonment penalties.
func checkTextCooldown(ctx context.Context, tx *sql.Tx, account, path string, prototype bool, at time.Time) error {
	if path != "quick_play" || prototype {
		return nil
	}
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM queue_cooldowns WHERE account_id=$1 AND cooldown_until>$2)`, account, at).Scan(&active); err != nil {
		return err
	}
	if active {
		return ErrTextCooldown
	}
	return nil
}
