package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"maps"
	"strings"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
)

var (
	ErrRewardClaimInvalid     = errors.New("reward.invalid_claim")
	ErrRewardClaimConflict    = errors.New("reward.receipt_conflict")
	ErrRewardClaimBusy        = errors.New("reward.claim_budget")
	ErrRewardClaimUnavailable = errors.New("reward.unavailable")
	ErrRewardPolicyPending    = errors.New("reward.policy_pending")
)

// TextRewardStore is a closed verification foundation. It never writes wallets,
// the ledger, settlements or delivery effects. No production route mounts it yet.
type TextRewardStore struct {
	db           *sql.DB
	cfg          config.RewardedConfig
	now          func() time.Time
	beforeCommit func() error
}

func NewTextRewardStore(db *sql.DB, cfg config.RewardedConfig) (*TextRewardStore, error) {
	if db == nil || !cfg.Enabled || cfg.Validate() != nil {
		return nil, ErrRewardClaimUnavailable
	}
	cfg.AdUnits = maps.Clone(cfg.AdUnits)
	return &TextRewardStore{db: db, cfg: cfg, now: time.Now}, nil
}

type TextRewardClaim struct {
	Claim     string    `json:"claim"`
	ExpiresAt time.Time `json:"expires_at"`
}

// VerifiedRewardInteraction is an internal adapter input, not a client payload.
// The HTTP boundary must first verify the unchanged query with AdMobVerifier.
// Provider reward_amount is deliberately absent: proof supplies no value authority.
type VerifiedRewardInteraction struct {
	TransactionID, Fingerprint, Claim, AdUnit string
	OccurredAt                                time.Time
}

func rewardClaimHash(raw string) (string, error) {
	b, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(b) != 32 {
		return "", ErrRewardClaimInvalid
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func (s *TextRewardStore) eligible(ctx context.Context, tx *sql.Tx, match, account string) error {
	m, err := lockTextMatch(ctx, tx, match)
	if err != nil {
		return err
	}
	if m.state == "interrupted" {
		return ErrRewardPolicyPending
	}
	if m.state != "completed" && m.state != "scored_low_population" {
		return ErrRewardClaimInvalid
	}
	if m.record.Prototype || !m.record.Contract.Eligibility.Rewards {
		return ErrRewardClaimInvalid
	}
	if err = LockValueAccount(ctx, tx, account); err != nil {
		return ErrRewardClaimInvalid
	}
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts a
 WHERE a.id=$1 AND a.auth_purpose='player' AND a.deleted_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM account_deletion_fences f WHERE f.account_id=a.id))
 AND EXISTS(SELECT 1 FROM text_bonus_eligibility e JOIN text_settlements s USING(match_id,account_id)
 WHERE e.match_id=$2 AND e.account_id=$1 AND e.policy_version='match_start_v1' AND NOT e.premium_bonus_eligible AND s.state='applied')`, account, match).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return ErrRewardClaimInvalid
	}
	return nil
}

// IssueClaim authenticates inside the transaction, including after all writes.
// A fresh replacement never invalidates an earlier signed occurrence window.
func (s *TextRewardStore) IssueClaim(ctx context.Context, match, account, unit string, authorize func(context.Context, *sql.Tx) error) (TextRewardClaim, error) {
	var result TextRewardClaim
	if !valueUUID(match) || !valueUUID(account) || authorize == nil {
		return result, ErrRewardClaimInvalid
	}
	if _, ok := s.cfg.AdUnits[unit]; !ok {
		return result, ErrRewardClaimInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.HTTPTimeoutS)*time.Second)
	defer cancel()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return result, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash, _ := rewardClaimHash(token)
	err := WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		result = TextRewardClaim{}
		if e := s.eligible(ctx, tx, match, account); e != nil {
			return e
		}
		if e := authorize(ctx, tx); e != nil {
			return e
		}
		at := s.now().UTC().Truncate(time.Millisecond)
		expires := at.Add(time.Duration(s.cfg.ClaimTTLS) * time.Second)
		var count int
		if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM (SELECT 1 FROM text_reward_claims WHERE match_id=$1 AND account_id=$2 AND issued_at>$3 ORDER BY issued_at DESC LIMIT $4) c`, match, account, at.Add(-time.Duration(s.cfg.ClaimTTLS)*time.Second), s.cfg.MaxClaimsPerMatchWindow).Scan(&count); e != nil {
			return e
		}
		if count >= s.cfg.MaxClaimsPerMatchWindow {
			return ErrRewardClaimBusy
		}
		if _, e := tx.ExecContext(ctx, `INSERT INTO text_reward_claims(claim_hash,match_id,account_id,ad_unit,issued_at,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, hash, match, account, unit, at, expires); e != nil {
			return e
		}
		if s.beforeCommit != nil {
			if e := s.beforeCommit(); e != nil {
				return e
			}
		}
		if e := authorize(ctx, tx); e != nil {
			return e
		}
		if !s.now().Before(expires) {
			return ErrRewardClaimInvalid
		}
		result = TextRewardClaim{Claim: token, ExpiresAt: expires}
		return nil
	})
	if err != nil {
		return TextRewardClaim{}, err
	}
	return result, nil
}

// RecordVerifiedSSV retains proof only. A late delivery is valid when its signed
// occurrence was within the immutable claim interval [issued_at, expires_at).
func (s *TextRewardStore) RecordVerifiedSSV(ctx context.Context, p VerifiedRewardInteraction) error {
	hash, err := rewardClaimHash(p.Claim)
	if err != nil {
		return err
	}
	if p.TransactionID == "" || len(p.TransactionID) > 128 || len(p.TransactionID)%2 != 0 || strings.ToLower(p.TransactionID) != p.TransactionID {
		return ErrRewardClaimInvalid
	}
	if _, err = hex.DecodeString(p.TransactionID); err != nil {
		return ErrRewardClaimInvalid
	}
	if len(p.Fingerprint) != 64 || strings.ToLower(p.Fingerprint) != p.Fingerprint {
		return ErrRewardClaimInvalid
	}
	if _, err = hex.DecodeString(p.Fingerprint); err != nil {
		return ErrRewardClaimInvalid
	}
	if _, ok := s.cfg.AdUnits[p.AdUnit]; !ok {
		return ErrRewardClaimInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.HTTPTimeoutS)*time.Second)
	defer cancel()
	return WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		var match, account, unit string
		var issued, expires time.Time
		if e := tx.QueryRowContext(ctx, `SELECT match_id,account_id,ad_unit,issued_at,expires_at FROM text_reward_claims WHERE claim_hash=$1`, hash).Scan(&match, &account, &unit, &issued, &expires); e != nil {
			if errors.Is(e, sql.ErrNoRows) {
				return ErrRewardClaimInvalid
			}
			return e
		}
		if unit != p.AdUnit || p.OccurredAt.Before(issued) || !p.OccurredAt.Before(expires) {
			return ErrRewardClaimInvalid
		}
		if e := s.eligible(ctx, tx, match, account); e != nil {
			return e
		}
		var priorHash, priorFingerprint string
		var priorAt time.Time
		e := tx.QueryRowContext(ctx, `SELECT claim_hash,fingerprint,occurred_at FROM text_reward_ssv_receipts WHERE provider_transaction_id=$1`, p.TransactionID).Scan(&priorHash, &priorFingerprint, &priorAt)
		if e == nil {
			if priorHash != hash || priorFingerprint != p.Fingerprint || !priorAt.Equal(p.OccurredAt) {
				return ErrRewardClaimConflict
			}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		res, e := tx.ExecContext(ctx, `INSERT INTO text_reward_ssv_receipts(provider_transaction_id,fingerprint,claim_hash,occurred_at,received_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(provider_transaction_id) DO NOTHING`, p.TransactionID, p.Fingerprint, hash, p.OccurredAt, valueTime(s.now()))
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return ErrRewardClaimConflict
		}
		if s.beforeCommit != nil {
			return s.beforeCommit()
		}
		return nil
	})
}
