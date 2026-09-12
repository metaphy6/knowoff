package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/lib/pq"
)

var (
	ErrValueConflict  = errors.New("value.identity_conflict")
	ErrValueFence     = errors.New("value.stale_owner_or_state")
	ErrQuotaExhausted = errors.New("admission.daily_limit")
)

// TextValueStore stores contract/value facts only, never hidden live game state.
// Callers supply authenticated accounts and server-owned clocks/identities.
type TextValueStore struct {
	owner        *TextOwner
	db           *sql.DB
	tuning       config.TuningConfig
	beforeCommit func() error
	startGuard   func(context.Context, *sql.Tx, TextMatchRecord, []string, time.Time) error
}

func NewTextValueStore(db *sql.DB, tuning config.TuningConfig) *TextValueStore {
	return &TextValueStore{db: db, tuning: tuning.Clone()}
}

// WithStartGuard returns an independently configured store; production binds the
// certified pack/access check before admitting any live text match.
func (s *TextValueStore) WithStartGuard(guard func(context.Context, *sql.Tx, TextMatchRecord, []string, time.Time) error) *TextValueStore {
	copy := *s
	copy.startGuard = guard
	return &copy
}

type TextReservation struct {
	ID, AccountID, EntryPath string
	Prototype                bool
	At                       time.Time
}
type TextMatchRecord struct {
	SponsorAccountID string `json:",omitempty"`
	Contract         v2.MatchContract
	Policy           config.TuningConfig
	Owner            string
	Epoch            int64
	AdmissionIDs     []string
	Prototype        bool
}
type TextAward struct {
	MatchID, Owner, AccountID, Kind string
	Epoch                           int64
	Ordinal, Amount                 int
	At                              time.Time
}
type TextPlayerResult struct {
	AccountID                                 string
	Seat                                      int
	Role                                      string
	Points                                    int64
	CorrectVotes, VotesCast, Survivals, Pokes int
	Absent                                    bool
}
type TextOutcome struct {
	MatchID, Owner, Kind, Winner string
	Epoch                        int64
	At                           time.Time
	Players                      []TextPlayerResult
}

func valueHash(v any) ([]byte, string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, "", e
	}
	h := sha256.Sum256(b)
	return b, hex.EncodeToString(h[:]), nil
}
func valueTime(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }
func valueDay(t time.Time) time.Time  { return t.UTC().Truncate(24 * time.Hour) }
func valueUUID(id string) bool {
	parsed, e := uuid.Parse(id)
	return e == nil && parsed != uuid.Nil && parsed.String() == id
}
func (s *TextValueStore) transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	return WithValueTransaction(ctx, s.db, fn)
}

// WithValueTransaction retries the entire unit only for PostgreSQL serialization
// or deadlock failures. Callers preserve logical identity and occurrence time.
func WithValueTransaction(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	for attempt := 0; attempt < 4; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		err = fn(tx)
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if err == nil {
			return nil
		}
		_ = tx.Rollback()
		var pg *pq.Error
		if !errors.As(err, &pg) || (pg.Code != "40001" && pg.Code != "40P01") || attempt == 3 {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return errors.New("value.retry_exhausted")
}
func valueAccountLock(ctx context.Context, tx *sql.Tx, id string) error {
	return LockValueAccount(ctx, tx, id)
}

// LockValueAccount precedes dependent day, profile and wallet locks. Match/week
// or community receipt identity locks, if needed, precede this stable row.
func LockValueAccount(ctx context.Context, tx *sql.Tx, id string) error {
	var got string
	err := tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&got)
	return err
}
func accessKind(ctx context.Context, tx *sql.Tx, account, path string, prototype bool, at time.Time) (string, error) {
	if prototype {
		return "prototype", nil
	}
	if path == "local" {
		return "local", nil
	}
	var premium, pass bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entitlements WHERE account_id=$1 AND entitlement_type IN ('premium_monthly','premium_yearly') AND active_until>$2),EXISTS(SELECT 1 FROM entitlements WHERE account_id=$1 AND entitlement_type IN ('play_pass_1d','play_pass_3d','play_pass_7d') AND active_until>$2)`, account, at).Scan(&premium, &pass)
	if err != nil {
		return "", err
	}
	if premium {
		return "premium", nil
	}
	if pass {
		return "pass", nil
	}
	return "free", nil
}
func (s *TextValueStore) checkQuota(ctx context.Context, tx *sql.Tx, account, exclude string, day time.Time) error {
	var used, reserved int
	err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT count FROM daily_quickplay_counts WHERE account_id=$1 AND server_day=$2),0),(SELECT COUNT(*) FROM text_admissions WHERE account_id=$1 AND quota_day=$2 AND state='reserved' AND access_kind='free' AND id<>$3)`, account, day, exclude).Scan(&used, &reserved)
	if err != nil {
		return err
	}
	if used+reserved >= s.tuning.Economy.FreeDailyQuickplayMatches {
		return ErrQuotaExhausted
	}
	return nil
}
func (s *TextValueStore) Reserve(ctx context.Context, a TextReservation) error {
	a.At = valueTime(a.At)
	if !valueUUID(a.ID) || !valueUUID(a.AccountID) || a.At.IsZero() || (a.EntryPath != "quick_play" && a.EntryPath != "local") {
		return ErrValueConflict
	}
	return s.ownerTransaction(ctx, func(tx *sql.Tx) error {
		if err := valueAccountLock(ctx, tx, a.AccountID); err != nil {
			return err
		}
		if err := checkTextTrust(ctx, tx, []string{a.AccountID}, a.At); err != nil {
			return err
		}
		var id, account, path, state string
		var processOwner sql.NullString
		var prototype bool
		var at time.Time
		err := tx.QueryRowContext(ctx, `SELECT id,account_id,entry_path,prototype,reserved_at,state,process_owner_id FROM text_admissions WHERE id=$1`, a.ID).Scan(&id, &account, &path, &prototype, &at, &state, &processOwner)
		if err == nil {
			if (s.owner != nil && processOwner.String != s.owner.token.IncarnationID) || account != a.AccountID || path != a.EntryPath || prototype != a.Prototype || !at.Equal(a.At) {
				return ErrValueConflict
			}
			if state == "reserved" || state == "started" {
				return nil
			}
			return ErrValueConflict
		}
		if err != sql.ErrNoRows {
			return err
		}
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM text_admissions WHERE account_id=$1 AND state IN ('reserved','started'))`, a.AccountID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrValueConflict
		}
		kind, err := accessKind(ctx, tx, a.AccountID, a.EntryPath, a.Prototype, a.At)
		if err != nil {
			return err
		}
		if kind == "free" {
			if err = s.checkQuota(ctx, tx, a.AccountID, a.ID, valueDay(a.At)); err != nil {
				return err
			}
		}
		processOwnerID, processGeneration := s.processOwner()
		_, err = tx.ExecContext(ctx, `INSERT INTO text_admissions(id,account_id,entry_path,prototype,access_kind,quota_day,reserved_at,state,process_owner_id,process_generation) VALUES($1,$2,$3,$4,$5,$6,$7,'reserved',$8,$9)`, a.ID, a.AccountID, a.EntryPath, a.Prototype, kind, valueDay(a.At), a.At, processOwnerID, processGeneration)
		return err
	})
}
func (s *TextValueStore) CancelReservation(ctx context.Context, id, account string) error {
	if !valueUUID(id) || !valueUUID(account) {
		return ErrValueConflict
	}
	return s.ownerTransaction(ctx, func(tx *sql.Tx) error {
		if err := valueAccountLock(ctx, tx, account); err != nil {
			return err
		}
		var state string
		var match, processOwner sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT state,match_id,process_owner_id FROM text_admissions WHERE id=$1 AND account_id=$2 FOR UPDATE`, id, account).Scan(&state, &match, &processOwner); err != nil {
			return err
		}
		if s.owner != nil && processOwner.String != s.owner.token.IncarnationID {
			return ErrValueFence
		}
		if state == "released" {
			return nil
		}
		if state != "reserved" || match.Valid {
			return ErrValueConflict
		}
		_, err := tx.ExecContext(ctx, `UPDATE text_admissions SET state='released' WHERE id=$1`, id)
		return err
	})
}
func (s *TextValueStore) Prepare(ctx context.Context, m TextMatchRecord, at time.Time) error {
	if err := s.validateProcessOwner(m.Owner); err != nil {
		return err
	}
	at = valueTime(at)
	if !valueUUID(m.Contract.MatchID) || !valueUUID(m.Owner) || m.Epoch != 1 || at.IsZero() || len(m.AdmissionIDs) != m.Contract.OriginalSize || m.Prototype && m.Contract.Eligibility.Rewards {
		return ErrValueConflict
	}
	return s.ownerTransaction(ctx, func(tx *sql.Tx) error {
		// Resolve replay before consulting a replacement process's configuration.
		var previous []byte
		err := tx.QueryRowContext(ctx, `SELECT contract FROM text_matches WHERE id=$1 FOR UPDATE`, m.Contract.MatchID).Scan(&previous)
		if err == nil {
			return compareTextPreparation(m, previous)
		}
		if err != sql.ErrNoRows {
			return err
		}
		m.Policy = s.tuning.Clone()
		policyHash, err := m.Policy.SHA256()
		if err != nil {
			return err
		}
		if m.Contract.Tuning.Version != config.TuningSnapshotVersion || m.Contract.Tuning.SHA256 != policyHash {
			return ErrValueConflict
		}
		limits := m.Policy.Contract
		if err := m.Contract.Validate(v2.Limits{MaxFrameBytes: limits.MaxTextBytes, MaxTextBytes: limits.MaxTextBytes, MaxHistoryEvents: limits.MaxHistoryEvents, MaxHistoryPageEvents: limits.MaxHistoryPageEvents, MaxRequestsPerSeat: limits.MaxRequestsPerSeat}); err != nil {
			return err
		}
		body, hash, err := valueHash(m)
		if err != nil {
			return err
		}
		_, processGeneration := s.processOwner()
		res, err := tx.ExecContext(ctx, `INSERT INTO text_matches(id,room_id,contract,contract_hash,owner_id,fence,state,prototype,created_at,process_generation) VALUES($1,$2,$3,$4,$5,$6,'prepared',$7,$8,$9) ON CONFLICT(id) DO NOTHING`, m.Contract.MatchID, m.Contract.RoomID, body, hash, m.Owner, m.Epoch, m.Prototype, at, processGeneration)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			if err = tx.QueryRowContext(ctx, `SELECT contract FROM text_matches WHERE id=$1 FOR UPDATE`, m.Contract.MatchID).Scan(&previous); err != nil {
				return err
			}
			return compareTextPreparation(m, previous)
		}
		accounts := make([]string, len(m.AdmissionIDs))
		seen := map[string]bool{}
		for i, id := range m.AdmissionIDs {
			if !valueUUID(id) || seen[id] {
				return ErrValueConflict
			}
			seen[id] = true
			if err = tx.QueryRowContext(ctx, `SELECT account_id FROM text_admissions WHERE id=$1`, id).Scan(&accounts[i]); err != nil {
				return err
			}
		}
		if m.SponsorAccountID != "" {
			found := false
			for _, id := range accounts {
				if id == m.SponsorAccountID {
					found = true
				}
			}
			if !found || m.Contract.Eligibility.EntryPath != "local" {
				return ErrValueConflict
			}
		}
		sorted := append([]string(nil), accounts...)
		sort.Strings(sorted)
		for _, id := range sorted {
			if err = valueAccountLock(ctx, tx, id); err != nil {
				return err
			}
		}
		for seat, id := range m.AdmissionIDs {
			res, err = tx.ExecContext(ctx, `UPDATE text_admissions SET match_id=$2,seat=$3 WHERE id=$1 AND state='reserved' AND match_id IS NULL AND entry_path=$4 AND prototype=$5 AND process_generation IS NOT DISTINCT FROM $6`, id, m.Contract.MatchID, seat, m.Contract.Eligibility.EntryPath, m.Prototype, processGeneration)
			if err != nil {
				return err
			}
			if n, _ = res.RowsAffected(); n != 1 {
				return ErrValueConflict
			}
		}
		return nil
	})
}

func compareTextPreparation(request TextMatchRecord, previous []byte) error {
	var stored TextMatchRecord
	if err := json.Unmarshal(previous, &stored); err != nil {
		return err
	}
	request.Policy = stored.Policy
	_, want, err := valueHash(stored)
	if err != nil {
		return err
	}
	_, got, err := valueHash(request)
	if err != nil {
		return err
	}
	if got != want {
		return ErrValueConflict
	}
	return nil
}

type lockedTextMatch struct {
	record  TextMatchRecord
	state   string
	epoch   int64
	outcome []byte
	hash    sql.NullString
	started sql.NullTime
}

func lockTextMatch(ctx context.Context, tx *sql.Tx, id string) (lockedTextMatch, error) {
	var m lockedTextMatch
	var b []byte
	err := tx.QueryRowContext(ctx, `SELECT contract,state,fence,outcome,outcome_hash,started_at FROM text_matches WHERE id=$1 FOR UPDATE`, id).Scan(&b, &m.state, &m.epoch, &m.outcome, &m.hash, &m.started)
	if err == nil {
		err = json.Unmarshal(b, &m.record)
	}
	return m, err
}
func checkValueFence(m lockedTextMatch, owner string, epoch int64, state string) error {
	if m.record.Owner != owner || m.epoch != epoch || m.state != state {
		return ErrValueFence
	}
	return nil
}

type admissionRow struct {
	id, account, path, kind, state string
	day                            time.Time
	prototype                      bool
	seat                           int
}

func matchAdmissions(ctx context.Context, tx *sql.Tx, id string) ([]admissionRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,account_id,entry_path,access_kind,state,quota_day,prototype,seat FROM text_admissions WHERE match_id=$1 ORDER BY account_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []admissionRow
	for rows.Next() {
		var a admissionRow
		if err = rows.Scan(&a.id, &a.account, &a.path, &a.kind, &a.state, &a.day, &a.prototype, &a.seat); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *TextValueStore) Start(ctx context.Context, id, owner string, epoch int64, at time.Time) error {
	if err := s.validateProcessOwner(owner); err != nil {
		return err
	}
	at = valueTime(at)
	return s.ownerTransaction(ctx, func(tx *sql.Tx) error {
		m, err := lockTextMatch(ctx, tx, id)
		if err != nil {
			return err
		}
		if m.state == "started" {
			return checkValueFence(m, owner, epoch, "started")
		}
		if err = checkValueFence(m, owner, epoch, "prepared"); err != nil {
			return err
		}
		if !m.record.Prototype && m.record.Contract.Eligibility.Leaderboard {
			week, err := ensureValueWeek(ctx, tx, at)
			if err != nil {
				return err
			}
			var closed bool
			var closing sql.NullTime
			if err = tx.QueryRowContext(ctx, `SELECT closed,closing_at FROM leaderboard_weeks WHERE week_id=$1 FOR UPDATE`, week).Scan(&closed, &closing); err != nil {
				return err
			}
			if closed || closing.Valid {
				return ErrWeekClosed
			}
		}
		admissions, err := matchAdmissions(ctx, tx, id)
		if err != nil {
			return err
		}
		if len(admissions) != m.record.Contract.OriginalSize {
			return ErrValueConflict
		}
		for _, a := range admissions {
			if err = valueAccountLock(ctx, tx, a.account); err != nil {
				return err
			}
		}
		ids := make([]string, len(admissions))
		for i, a := range admissions {
			ids[i] = a.account
		}
		if err = checkTextTrust(ctx, tx, ids, at); err != nil {
			return err
		}
		if s.startGuard != nil {
			if err = s.startGuard(ctx, tx, m.record, ids, at); err != nil {
				return err
			}
		}
		for _, a := range admissions {
			if a.state != "reserved" {
				return ErrValueConflict
			}
			kind, err := accessKind(ctx, tx, a.account, a.path, a.prototype, at)
			if err != nil {
				return err
			}
			day := valueDay(at)
			if kind == "free" {
				if err = s.checkQuota(ctx, tx, a.account, a.id, day); err != nil {
					return err
				}
				if _, err = tx.ExecContext(ctx, `INSERT INTO daily_quickplay_counts(account_id,server_day,count) VALUES($1,$2,1) ON CONFLICT(account_id,server_day) DO UPDATE SET count=daily_quickplay_counts.count+1`, a.account, day); err != nil {
					return err
				}
			}
			if _, err = tx.ExecContext(ctx, `UPDATE text_admissions SET state='started',access_kind=$2,quota_day=$3 WHERE id=$1`, a.id, kind, day); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE text_matches SET state='started',started_at=$2 WHERE id=$1`, id, at)
		return err
	})
}
func (s *TextValueStore) Interrupt(ctx context.Context, id, owner string, epoch int64, at time.Time) error {
	if err := s.validateProcessOwner(owner); err != nil {
		return err
	}
	at = valueTime(at)
	return s.ownerTransaction(ctx, func(tx *sql.Tx) error { return s.interruptTx(ctx, tx, id, owner, epoch, at) })
}

func (s *TextValueStore) interruptTx(ctx context.Context, tx *sql.Tx, id, owner string, epoch int64, at time.Time) error {
	m, err := lockTextMatch(ctx, tx, id)
	if err != nil {
		return err
	}
	if m.record.Owner != owner {
		return ErrValueFence
	}
	if m.state == "interrupted" && m.epoch == epoch+1 {
		return nil
	}
	if m.state == "completed" || m.state == "scored_low_population" {
		return nil
	}
	if err = checkValueFence(m, owner, epoch, "started"); err != nil {
		return err
	}
	admissions, err := matchAdmissions(ctx, tx, id)
	if err != nil {
		return err
	}
	for _, a := range admissions {
		if err = valueAccountLock(ctx, tx, a.account); err != nil {
			return err
		}
	}
	for _, a := range admissions {
		if a.state != "started" {
			return ErrValueConflict
		}
		if a.kind == "free" {
			res, err := tx.ExecContext(ctx, `UPDATE daily_quickplay_counts SET count=count-1 WHERE account_id=$1 AND server_day=$2 AND count>0`, a.account, a.day)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n != 1 {
				return fmt.Errorf("admission compensation missing consumed count")
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE text_admissions SET state='compensated' WHERE id=$1`, a.id); err != nil {
			return err
		}
	}
	if err = persistTextInterruption(ctx, tx, m, admissions, at); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE text_matches SET state='interrupted',fence=fence+1,ended_at=$2 WHERE id=$1`, id, at)
	return err
}

// CancelPrepared releases all reservations when readiness/settings/dealing or
// access revalidation fails. It never compensates a started match.
func (s *TextValueStore) CancelPrepared(ctx context.Context, id, owner string, epoch int64, at time.Time) error {
	if err := s.validateProcessOwner(owner); err != nil {
		return err
	}
	return s.ownerTransaction(ctx, func(tx *sql.Tx) error { return s.cancelPreparedTx(ctx, tx, id, owner, epoch, at) })
}

func (s *TextValueStore) cancelPreparedTx(ctx context.Context, tx *sql.Tx, id, owner string, epoch int64, at time.Time) error {
	m, err := lockTextMatch(ctx, tx, id)
	if err != nil {
		return err
	}
	if m.state == "cancelled" && m.record.Owner == owner && m.epoch == epoch+1 {
		return nil
	}
	if err = checkValueFence(m, owner, epoch, "prepared"); err != nil {
		return err
	}
	admissions, err := matchAdmissions(ctx, tx, id)
	if err != nil {
		return err
	}
	for _, a := range admissions {
		if err = valueAccountLock(ctx, tx, a.account); err != nil {
			return err
		}
	}
	for _, a := range admissions {
		if a.state != "reserved" {
			return ErrValueConflict
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE text_admissions SET state='released' WHERE match_id=$1`, id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE text_matches SET state='cancelled',fence=fence+1,ended_at=$2 WHERE id=$1`, id, valueTime(at))
	return err
}
