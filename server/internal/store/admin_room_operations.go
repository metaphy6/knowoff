package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"slices"
	"time"
)

// ResolveRoomDecision is a trusted reconciliation probe, never a new decision.
// The same account/operation locks as Decide wait for an uncertain original
// transaction to commit or roll back before absence can release a room fence.
func (s *AdminOperationStore) ResolveRoomDecision(ctx context.Context, actor string, c AdminOperationCommand, affected []string) (AdminOperationReceipt, error) {
	var receipt AdminOperationReceipt
	if s == nil || s.db == nil || HasAdminAuthorization(ctx) || !valueUUID(actor) {
		return receipt, ErrAdminRequired
	}
	affected = slices.Clone(affected)
	slices.Sort(affected)
	affected = slices.Compact(affected)
	if (c.Kind != "room_close" && c.Kind != "room_kick") || !validAdminOperation(c, affected) {
		return receipt, ErrAdminOperation
	}
	raw, _ := json.Marshal(struct {
		Actor    string
		Command  AdminOperationCommand
		Accounts []string
	}{actor, c, affected})
	digest := sha256.Sum256(raw)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	err := WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := lockAdminOperationAccounts(ctx, tx, affected); err != nil {
			return err
		}
		if err := lockAdminOperation(ctx, tx, c.ID); err != nil {
			return err
		}
		prior, err := readAdminOperation(ctx, tx, c.ID)
		if err != nil {
			return err
		}
		if prior.ActorID != actor || !bytes.Equal(prior.hash, digest[:]) {
			return ErrAdminOperation
		}
		receipt = prior
		return nil
	})
	return receipt, err
}

// AdminRoomApplication is captured by the live room while membership is locked.
// It is never decoded from HTTP. Reservations map original account to admission.
type AdminRoomApplication struct {
	OperationID, RoomID, MatchID string
	Reservations                 map[string]string
	Effect                       string // release waiting seats, close a match, or acknowledge a live kick
	At                           time.Time
}

// ApplyRoomOperation joins value effects and completion audit in one owner-fenced
// transaction. A committed result is checked before touching value. Active kicks
// have already completed their engine/transport effect; this method records only
// their acknowledgement, never a guessed award or outcome.
func (s *TextValueStore) ApplyRoomOperation(ctx context.Context, request AdminRoomApplication) (AdminOperationReceipt, error) {
	var result AdminOperationReceipt
	if s == nil || s.owner == nil || !valueUUID(request.OperationID) || !valueUUID(request.RoomID) || request.At.IsZero() {
		return result, ErrAdminOperation
	}
	if request.MatchID != "" && !valueUUID(request.MatchID) {
		return result, ErrAdminOperation
	}
	request.At = valueTime(request.At)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	operations := NewAdminOperationStore(s.db)
	err := s.ownerTransaction(ctx, func(tx *sql.Tx) error {
		decision, err := readAdminOperation(ctx, tx, request.OperationID)
		if err != nil {
			return err
		}
		c := decision.Command
		if c.RoomID != request.RoomID || c.OwnerID != s.owner.token.IncarnationID || c.OwnerGeneration != s.owner.token.Generation || (c.Kind != "room_close" && c.Kind != "room_kick") {
			return ErrValueFence
		}
		var match lockedTextMatch
		accounts := []string{}
		if request.MatchID != "" {
			if len(request.Reservations) != 0 || (request.Effect != "close" && request.Effect != "kick") || (request.Effect == "close") != (c.Kind == "room_close") {
				return ErrAdminOperation
			}
			match, err = lockTextMatch(ctx, tx, request.MatchID)
			if err != nil {
				return err
			}
			if match.record.Owner != c.OwnerID || match.record.Contract.RoomID != c.RoomID {
				return ErrValueFence
			}
			admissions, err := matchAdmissions(ctx, tx, request.MatchID)
			if err != nil {
				return err
			}
			for _, a := range admissions {
				accounts = append(accounts, a.account)
			}
		} else {
			if request.Effect != "release" {
				return ErrAdminOperation
			}
			for account, id := range request.Reservations {
				if !valueUUID(account) || id != "" && !valueUUID(id) {
					return ErrAdminOperation
				}
				accounts = append(accounts, account)
			}
			slices.Sort(accounts)
		}
		if !slices.Equal(accounts, decision.AffectedAccounts) {
			return ErrValueConflict
		}
		decision, err = operations.LockPendingTx(ctx, tx, request.OperationID)
		if err != nil {
			return err
		}
		if decision.Status != "pending" {
			result = decision
			return nil
		}
		completion := AdminOperationResult{Outcome: "applied", Detail: "room_closed"}
		if c.Kind == "room_kick" {
			completion.Detail = "room_kicked"
		}
		switch request.Effect {
		case "release":
			for _, account := range accounts {
				if c.Kind == "room_kick" && account != c.TargetAccountID {
					continue
				}
				if id := request.Reservations[account]; id != "" {
					if err = s.cancelReservationTx(ctx, tx, id, account); err != nil {
						return err
					}
				}
			}
		case "close":
			switch match.state {
			case "prepared":
				err = s.cancelPreparedTx(ctx, tx, request.MatchID, c.OwnerID, match.epoch, request.At)
			case "started":
				err = s.interruptTx(ctx, tx, request.MatchID, c.OwnerID, match.epoch, request.At)
			case "completed", "scored_low_population", "interrupted", "cancelled":
				completion = AdminOperationResult{Outcome: "obsolete", Detail: "room_already_terminal"}
			default:
				err = ErrValueConflict
			}
			if err != nil {
				return err
			}
		case "kick":
			// No value mutation. Normal game persistence handles every occurrence.
		default:
			return ErrAdminOperation
		}
		if err = operations.CompleteTx(ctx, tx, request.OperationID, completion); err != nil {
			return err
		}
		result, err = readAdminOperation(ctx, tx, request.OperationID)
		return err
	})
	return result, err
}

// RecoverRoomOperations runs only after RecoverLostOwners has made this owner
// ready. It records obsolete delivery for a lost predecessor whose compensation
// is confirmed complete; it never reconstructs a room or guesses a game result.
// The returned bounded candidate count is less than limit only when no eligible
// pending decision remained. This remains safe with concurrent receipt replays.
func (s *TextValueStore) RecoverRoomOperations(ctx context.Context, limit int) (int, error) {
	if s == nil || s.owner == nil || limit < 1 || limit > 1000 || HasAdminAuthorization(ctx) {
		return 0, ErrAdminOperation
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	operations := NewAdminOperationStore(s.db)
	processed := 0
	for ; processed < limit; processed++ {
		found := false
		err := s.ownerTransaction(ctx, func(tx *sql.Tx) error {
			var id string
			err := tx.QueryRowContext(ctx, `SELECT d.id FROM admin_operation_decisions d JOIN text_process_owners o ON o.incarnation_id=d.owner_id AND o.generation=d.owner_generation
    WHERE d.kind IN ('room_close','room_kick') AND o.lost_at IS NOT NULL AND o.recovered_at IS NOT NULL
    AND NOT EXISTS(SELECT 1 FROM admin_operation_results r WHERE r.operation_id=d.id)
    AND NOT EXISTS(SELECT 1 FROM text_matches m WHERE m.process_generation=o.generation AND m.state IN ('prepared','started'))
    AND NOT EXISTS(SELECT 1 FROM text_admissions a WHERE a.process_generation=o.generation AND a.state='reserved' AND a.match_id IS NULL)
    ORDER BY o.generation,d.created_at,d.id LIMIT 1`).Scan(&id)
			if err == sql.ErrNoRows {
				return nil
			}
			if err != nil {
				return err
			}
			found = true
			receipt, err := operations.LockPendingTx(ctx, tx, id)
			if err != nil {
				return err
			}
			if receipt.Status != "pending" {
				return nil
			}
			return operations.CompleteTx(ctx, tx, id, AdminOperationResult{Outcome: "obsolete", Detail: "owner_lost_after_compensation"})
		})
		if err != nil {
			return processed, err
		}
		if !found {
			break
		}
	}
	return processed, nil
}
