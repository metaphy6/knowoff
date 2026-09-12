package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrAdminOperation = errors.New("operator.invalid_or_conflicting")

type AdminOperationCommand struct {
	ID              string     `json:"id"`
	Kind            string     `json:"kind"`
	TargetAccountID string     `json:"target_account_id,omitempty"`
	RoomID          string     `json:"room_id,omitempty"`
	OwnerID         string     `json:"owner_id,omitempty"`
	OwnerGeneration int64      `json:"owner_generation,omitempty"`
	SourceLedgerID  int64      `json:"source_ledger_id,omitempty"`
	Amount          int64      `json:"amount,omitempty"`
	PriorSanctionID string     `json:"prior_sanction_id,omitempty"`
	Until           *time.Time `json:"until,omitempty"`
	Reason          string     `json:"reason"`
}

type AdminOperationReceipt struct {
	Command          AdminOperationCommand `json:"command"`
	ActorID          string                `json:"actor_id"`
	AffectedAccounts []string              `json:"affected_accounts"`
	Status           string                `json:"status"`
	CreatedAt        time.Time             `json:"created_at"`
	Result           json.RawMessage       `json:"result,omitempty"`
	CompletedAt      *time.Time            `json:"completed_at,omitempty"`
	hash             []byte
}

type AdminOperationStore struct{ db *sql.DB }

// Result carries only delivery facts, never credentials or private match state.
type AdminOperationResult struct {
	Outcome  string `json:"outcome"`
	Detail   string `json:"detail"`
	LedgerID int64  `json:"ledger_id,omitempty"`
}

func NewAdminOperationStore(db *sql.DB) *AdminOperationStore { return &AdminOperationStore{db} }

func validAdminOperation(c AdminOperationCommand, accounts []string) bool {
	if !valueUUID(c.ID) || !utf8.ValidString(c.Reason) || len(c.Reason) > 2000 || utf8.RuneCountInString(c.Reason) > 500 || strings.TrimSpace(c.Reason) == "" || len(accounts) < 1 || len(accounts) > 6 {
		return false
	}
	for _, account := range accounts {
		if !valueUUID(account) {
			return false
		}
	}
	room := c.Kind == "room_kick" || c.Kind == "room_close"
	if room {
		if !valueUUID(c.RoomID) || !valueUUID(c.OwnerID) || c.OwnerGeneration < 1 || c.SourceLedgerID != 0 || c.Amount != 0 || c.PriorSanctionID != "" || c.Until != nil {
			return false
		}
		return c.Kind == "room_close" && c.TargetAccountID == "" || c.Kind == "room_kick" && valueUUID(c.TargetAccountID) && slices.Contains(accounts, c.TargetAccountID)
	}
	if !valueUUID(c.TargetAccountID) || len(accounts) != 1 || accounts[0] != c.TargetAccountID || c.RoomID != "" || c.OwnerID != "" || c.OwnerGeneration != 0 {
		return false
	}
	switch c.Kind {
	case "noin_grant", "noin_refund":
		return c.Amount > 0 && c.Amount <= math.MaxInt32 && c.PriorSanctionID == "" && c.Until == nil && (c.Kind == "noin_grant" && c.SourceLedgerID == 0 || c.Kind == "noin_refund" && c.SourceLedgerID > 0)
	case "account_sanction":
		return c.Amount == 0 && c.SourceLedgerID == 0 && c.PriorSanctionID == "" && (c.Until == nil || !c.Until.IsZero())
	case "sanction_lift":
		return c.Amount == 0 && c.SourceLedgerID == 0 && valueUUID(c.PriorSanctionID) && c.Until == nil
	}
	return false
}

// Decide only records a reviewed command and audit, never its domain effects.
// affected is captured by product code while its target is serialized, not read
// from the browser. Every browser replay rechecks the initiating exact session.
func (s *AdminOperationStore) Decide(ctx context.Context, actor string, c AdminOperationCommand, affected []string) (AdminOperationReceipt, error) {
	var receipt AdminOperationReceipt
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if s == nil || s.db == nil || !HasAdminAuthorization(ctx) || !valueUUID(actor) {
		return receipt, ErrAdminRequired
	}
	affected = slices.Clone(affected)
	slices.Sort(affected)
	affected = slices.Compact(affected)
	if c.Until != nil {
		at := valueTime(*c.Until)
		c.Until = &at
	}
	if !validAdminOperation(c, affected) {
		return receipt, ErrAdminOperation
	}
	raw, _ := json.Marshal(struct {
		Actor    string
		Command  AdminOperationCommand
		Accounts []string
	}{actor, c, affected})
	digest := sha256.Sum256(raw)
	err := WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if c.OwnerID != "" {
			if _, err := tx.ExecContext(ctx, `SET LOCAL row_security=off; LOCK TABLE text_process_current IN SHARE MODE`); err != nil {
				return err
			}
			var owner string
			var generation int64
			if err := tx.QueryRowContext(ctx, `SELECT incarnation_id,generation FROM text_process_current WHERE singleton=1 FOR SHARE`).Scan(&owner, &generation); err != nil || owner != c.OwnerID || generation != c.OwnerGeneration {
				return ErrValueFence
			}
			var active bool
			if err := tx.QueryRowContext(ctx, textOwnerAuthoritySQL, owner, generation, textOwnerLockClass, textOwnerLockObject).Scan(&active); err != nil || !active {
				return ErrValueFence
			}
		}
		if err := LockAdminTx(ctx, tx, actor, []string{"admin"}, affected...); err != nil {
			return err
		}
		if err := lockAdminOperation(ctx, tx, c.ID); err != nil {
			return err
		}
		prior, err := readAdminOperation(ctx, tx, c.ID)
		if err == nil {
			if prior.ActorID != actor || !bytes.Equal(prior.hash, digest[:]) {
				return ErrAdminOperation
			}
			if err = LockAdminTx(ctx, tx, actor, []string{"admin"}, affected...); err != nil {
				return err
			}
			receipt = prior
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if c.SourceLedgerID > 0 {
			var account, kind string
			var amount int64
			if err = tx.QueryRowContext(ctx, `SELECT account_id,event_type,amount FROM noin_ledger WHERE id=$1 FOR SHARE`, c.SourceLedgerID).Scan(&account, &kind, &amount); err != nil || account != c.TargetAccountID || kind != "spend" || amount >= 0 || -amount != c.Amount {
				return ErrAdminOperation
			}
		}
		if c.PriorSanctionID != "" {
			var target, kind string
			if err = tx.QueryRowContext(ctx, `SELECT target_account_id,kind FROM admin_operation_decisions WHERE id=$1`, c.PriorSanctionID).Scan(&target, &kind); err != nil || target != c.TargetAccountID || kind != "account_sanction" {
				return ErrAdminOperation
			}
		}
		accounts, _ := json.Marshal(affected)
		_, err = tx.ExecContext(ctx, `INSERT INTO admin_operation_decisions(id,actor_admin_id,kind,target_account_id,room_id,owner_id,owner_generation,source_ledger_id,amount,prior_sanction_id,sanction_until,reason,affected_accounts,request_hash) VALUES($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,NULLIF($7,0),NULLIF($8,0),NULLIF($9,0),NULLIF($10,'')::uuid,$11,$12,$13,$14)`, c.ID, actor, c.Kind, c.TargetAccountID, c.RoomID, c.OwnerID, c.OwnerGeneration, c.SourceLedgerID, c.Amount, c.PriorSanctionID, c.Until, c.Reason, string(accounts), digest[:])
		if err != nil {
			return err
		}
		audit, _ := json.Marshal(map[string]any{"kind": c.Kind, "request_hash": hex.EncodeToString(digest[:]), "status": "pending"})
		if _, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log(admin_id,action,target_type,target_id,after_state) VALUES($1,'operator_decision','admin_operation',$2,$3)`, actor, c.ID, string(audit)); err != nil {
			return err
		}
		if err = LockAdminTx(ctx, tx, actor, []string{"admin"}, affected...); err != nil {
			return err
		}
		receipt, err = readAdminOperation(ctx, tx, c.ID)
		return err
	})
	return receipt, err
}

type adminOperationQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readAdminOperation(ctx context.Context, q adminOperationQuerier, id string) (AdminOperationReceipt, error) {
	var r AdminOperationReceipt
	var target, room, owner, prior sql.NullString
	var generation, source, amount sql.NullInt64
	var until, completed sql.NullTime
	var affected, result []byte
	query := `SELECT d.id,d.actor_admin_id,d.kind,d.target_account_id,d.room_id,d.owner_id,d.owner_generation,d.source_ledger_id,d.amount,d.prior_sanction_id,d.sanction_until,d.reason,d.affected_accounts,d.request_hash,d.created_at,COALESCE(r.outcome,'pending'),r.result,r.completed_at FROM admin_operation_decisions d LEFT JOIN admin_operation_results r ON r.operation_id=d.id WHERE d.id=$1`
	err := q.QueryRowContext(ctx, query, id).Scan(&r.Command.ID, &r.ActorID, &r.Command.Kind, &target, &room, &owner, &generation, &source, &amount, &prior, &until, &r.Command.Reason, &affected, &r.hash, &r.CreatedAt, &r.Status, &result, &completed)
	if err != nil {
		return r, err
	}
	r.Result = result
	r.Command.TargetAccountID = target.String
	r.Command.RoomID = room.String
	r.Command.OwnerID = owner.String
	r.Command.OwnerGeneration = generation.Int64
	r.Command.SourceLedgerID = source.Int64
	r.Command.Amount = amount.Int64
	r.Command.PriorSanctionID = prior.String
	if until.Valid {
		r.Command.Until = &until.Time
	}
	if completed.Valid {
		r.CompletedAt = &completed.Time
	}
	err = json.Unmarshal(affected, &r.AffectedAccounts)
	return r, err
}

func (s *AdminOperationStore) Get(ctx context.Context, id string) (AdminOperationReceipt, error) {
	if s == nil || s.db == nil || !valueUUID(id) {
		return AdminOperationReceipt{}, ErrAdminOperation
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return readAdminOperation(ctx, s.db, id)
}

func lockAdminOperation(ctx context.Context, tx *sql.Tx, id string) error {
	// Row locking requires UPDATE privilege. Receipts are insert-only, so use a
	// stable transaction identity after account locks and before receipt reads.
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,27))`, "admin_operation:"+id)
	return err
}

// Trusted receipt delivery must retain serialization after a participant is
// soft-deleted. This is not permission to admit, spend or create new decisions.
func lockAdminOperationAccounts(ctx context.Context, tx *sql.Tx, accounts []string) error {
	if _, err := tx.ExecContext(ctx, `SET LOCAL row_security=off`); err != nil {
		return err
	}
	for _, account := range accounts {
		var id string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&id); err != nil {
			if err == sql.ErrNoRows {
				return ErrValueConflict
			}
			return err
		}
	}
	return nil
}

// LockPendingTx is the delivery idempotency boundary. Callers acquire their
// owner/match/week prefix first and inspect Status before performing any effect.
// Immutable receipt reads require no UPDATE privilege.
func (s *AdminOperationStore) LockPendingTx(ctx context.Context, tx *sql.Tx, id string) (AdminOperationReceipt, error) {
	if tx == nil || !valueUUID(id) {
		return AdminOperationReceipt{}, ErrAdminOperation
	}
	decision, err := readAdminOperation(ctx, tx, id)
	if err != nil {
		return decision, err
	}
	if HasAdminAuthorization(ctx) {
		err = LockAdminTx(ctx, tx, decision.ActorID, []string{"admin"}, decision.AffectedAccounts...)
	} else {
		err = lockAdminOperationAccounts(ctx, tx, decision.AffectedAccounts)
	}
	if err != nil {
		return decision, err
	}
	if err = lockAdminOperation(ctx, tx, id); err != nil {
		return decision, err
	}
	decision, err = readAdminOperation(ctx, tx, id)
	if err == nil && HasAdminAuthorization(ctx) {
		err = LockAdminTx(ctx, tx, decision.ActorID, []string{"admin"}, decision.AffectedAccounts...)
	}
	return decision, err
}

// CompleteTx joins receipt and delivery audit to the domain's transaction.
// The caller must acquire any owner/match/week prefix before calling this method
// and must commit its effects in this same transaction. Already committed work
// is replayed without requiring the original administrator to remain authorized.
// Browser-bound calls still recheck that exact session before returning success.
func (s *AdminOperationStore) CompleteTx(ctx context.Context, tx *sql.Tx, id string, result AdminOperationResult) error {
	if tx == nil || !valueUUID(id) || (result.Outcome != "applied" && result.Outcome != "obsolete") || result.Detail == "" || len(result.Detail) > 80 || result.LedgerID < 0 || !utf8.ValidString(result.Detail) {
		return ErrAdminOperation
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	decision, err := readAdminOperation(ctx, tx, id)
	if err != nil {
		return err
	}
	if HasAdminAuthorization(ctx) {
		if err = LockAdminTx(ctx, tx, decision.ActorID, []string{"admin"}, decision.AffectedAccounts...); err != nil {
			return err
		}
	} else {
		if err = lockAdminOperationAccounts(ctx, tx, decision.AffectedAccounts); err != nil {
			return err
		}
	}
	if err = lockAdminOperation(ctx, tx, id); err != nil {
		return err
	}
	decision, err = readAdminOperation(ctx, tx, id)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(result)
	if decision.Status != "pending" {
		var previous AdminOperationResult
		if json.Unmarshal(decision.Result, &previous) != nil || previous != result {
			return ErrAdminOperation
		}
		if HasAdminAuthorization(ctx) {
			return LockAdminTx(ctx, tx, decision.ActorID, []string{"admin"}, decision.AffectedAccounts...)
		}
		return nil
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO admin_operation_results(operation_id,outcome,result) VALUES($1,$2,$3)`, id, result.Outcome, string(raw)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log(action,target_type,target_id,before_state,after_state) VALUES($1,'admin_operation',$2,'{"status":"pending"}',$3)`, "operator_"+result.Outcome, id, string(raw)); err != nil {
		return err
	}
	if HasAdminAuthorization(ctx) {
		return LockAdminTx(ctx, tx, decision.ActorID, []string{"admin"}, decision.AffectedAccounts...)
	}
	return nil
}

func (s *AdminOperationStore) List(ctx context.Context, limit int) ([]AdminOperationReceipt, error) {
	if s == nil || s.db == nil || limit < 1 || limit > 100 {
		return nil, ErrAdminOperation
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM admin_operation_decisions ORDER BY created_at DESC,id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := make([]AdminOperationReceipt, 0, len(ids))
	for _, id := range ids {
		receipt, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, receipt)
	}
	return result, nil
}
