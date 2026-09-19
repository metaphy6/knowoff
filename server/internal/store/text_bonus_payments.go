package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"
)

var ErrTextBonusIneligible = errors.New("reward.bonus_ineligible")

// TextBonusPayment is retained financial metadata. It is not a public delivery
// payload; the separate delivery boundary must authorize its recipient.
type TextBonusPayment struct {
	MatchID, AccountID, Source, ProviderTransactionID, SourceHash string
	Requested, Credited                                           int
	OccurredAt, AppliedAt                                         time.Time
	Items                                                         []TextBonusPaymentItem
}

type TextBonusPaymentItem struct {
	Kind                   string
	Ordinal                int
	BodyHash               string
	ServerDay              time.Time
	BaseCredited, Credited int
	LedgerID               *int64
}

type bonusAward struct {
	TextBonusPaymentItem
	Requested  int
	OccurredAt time.Time
	BaseLedger sql.NullInt64
}

// ApplyBonus is a closed, server-owned payment operation. It derives every
// amount and authority from immutable match-start and settlement evidence.
func (s *TextValueStore) ApplyBonus(ctx context.Context, match, account string) (TextBonusPayment, error) {
	if !valueUUID(match) || !valueUUID(account) {
		return TextBonusPayment{}, ErrValueConflict
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var result TextBonusPayment
	at := valueTime(time.Now())
	err := s.transaction(ctx, func(tx *sql.Tx) error {
		result = TextBonusPayment{}
		m, err := lockTextMatch(ctx, tx, match)
		if err != nil {
			return err
		}
		// Account locking also serializes historical replay with deletion. Reading
		// a retained receipt is allowed; it cannot create any new value.
		var deleted sql.NullTime
		var purpose string
		if err = tx.QueryRowContext(ctx, `SELECT deleted_at,auth_purpose FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&deleted, &purpose); err != nil {
			return err
		}
		prior, err := readTextBonusPayment(ctx, tx, match, account)
		if err == nil {
			var same bool
			if err = tx.QueryRowContext(ctx, `SELECT source_sha256=encode(sha256(convert_to(text_bonus_source(match_id,account_id)::text,'UTF8')),'hex') FROM text_bonus_payments WHERE match_id=$1 AND account_id=$2`, match, account).Scan(&same); err != nil {
				return err
			}
			if !same {
				return ErrValueConflict
			}
			result = prior
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var fenced bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_deletion_fences WHERE account_id=$1)`, account).Scan(&fenced); err != nil {
			return err
		}
		if deleted.Valid || fenced || purpose != "player" {
			return ErrTextBonusIneligible
		}
		if m.state != "completed" && m.state != "scored_low_population" || m.record.Prototype || !m.record.Contract.Eligibility.Rewards {
			return ErrTextBonusIneligible
		}
		_, contractHash, err := valueHash(m.record)
		policyHash, policyErr := m.record.Policy.SHA256()
		var storedContractHash string
		if err := tx.QueryRowContext(ctx, `SELECT contract_hash FROM text_matches WHERE id=$1`, match).Scan(&storedContractHash); err != nil {
			return err
		}
		if err != nil || policyErr != nil || contractHash != storedContractHash || policyHash != m.record.Contract.Tuning.SHA256 {
			return ErrValueConflict
		}
		var premium bool
		var started time.Time
		err = tx.QueryRowContext(ctx, `SELECT premium_bonus_eligible,started_at FROM text_bonus_eligibility WHERE match_id=$1 AND account_id=$2 AND policy_version='match_start_v1'`, match, account).Scan(&premium, &started)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTextBonusIneligible
		}
		if err != nil {
			return err
		}
		if !m.started.Valid || !started.Equal(m.started.Time) {
			return ErrValueConflict
		}
		var state, outcomeHash string
		var effects []byte
		if err = tx.QueryRowContext(ctx, `SELECT state,outcome_hash,effects FROM text_settlements WHERE match_id=$1 AND account_id=$2`, match, account).Scan(&state, &outcomeHash, &effects); errors.Is(err, sql.ErrNoRows) {
			return ErrTextBonusIneligible
		} else if err != nil {
			return err
		}
		if state != "applied" {
			return ErrTextBonusIneligible
		}
		if !m.hash.Valid || m.hash.String != outcomeHash {
			return ErrValueConflict
		}
		var outcome TextOutcome
		if json.Unmarshal(m.outcome, &outcome) != nil {
			return ErrValueConflict
		}
		_, storedOutcomeHash, err := valueHash(outcome)
		if err != nil || storedOutcomeHash != outcomeHash {
			return ErrValueConflict
		}
		result = TextBonusPayment{MatchID: match, AccountID: account, Source: "premium", OccurredAt: outcome.At, AppliedAt: at, Items: []TextBonusPaymentItem{}}
		if !premium {
			result.Source = "ssv"
			err = tx.QueryRowContext(ctx, `SELECT r.provider_transaction_id,r.occurred_at FROM text_reward_ssv_receipts r JOIN text_reward_claims c USING(claim_hash) WHERE c.match_id=$1 AND c.account_id=$2 AND r.occurred_at>=c.issued_at AND r.occurred_at<c.expires_at ORDER BY r.provider_transaction_id LIMIT 1`, match, account).Scan(&result.ProviderTransactionID, &result.OccurredAt)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrTextBonusIneligible
			}
			if err != nil {
				return err
			}
		}
		awards, err := loadBonusAwards(ctx, tx, m, account, effects)
		if err != nil {
			return err
		}
		cap := m.record.Policy.Noin.DailyEarnCap
		if cap < 0 || int64(cap) > math.MaxInt32 {
			return ErrValueConflict
		}
		// All original UTC buckets are locked in sorted order before wallet/ledger.
		sort.Slice(awards, func(i, j int) bool {
			if !awards[i].ServerDay.Equal(awards[j].ServerDay) {
				return awards[i].ServerDay.Before(awards[j].ServerDay)
			}
			if awards[i].Kind != awards[j].Kind {
				return awards[i].Kind < awards[j].Kind
			}
			return awards[i].Ordinal < awards[j].Ordinal
		})
		remaining := map[time.Time]int{}
		for _, a := range awards {
			if _, ok := remaining[a.ServerDay]; ok {
				continue
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, account, a.ServerDay); err != nil {
				return err
			}
			var earned int
			if err = tx.QueryRowContext(ctx, `SELECT earned FROM daily_noin_earned WHERE account_id=$1 AND server_day=$2 FOR UPDATE`, account, a.ServerDay).Scan(&earned); err != nil {
				return err
			}
			if earned < 0 {
				return ErrValueConflict
			}
			remaining[a.ServerDay] = max(0, cap-earned)
		}
		for _, a := range awards {
			i := a.TextBonusPaymentItem
			i.Credited = min(i.BaseCredited, remaining[i.ServerDay])
			remaining[i.ServerDay] -= i.Credited
			if int64(result.Requested)+int64(i.BaseCredited) > math.MaxInt32 {
				return ErrValueConflict
			}
			result.Requested += i.BaseCredited
			result.Credited += i.Credited
			result.Items = append(result.Items, i)
		}
		var proof any
		if result.ProviderTransactionID != "" {
			proof = result.ProviderTransactionID
		}
		err = tx.QueryRowContext(ctx, `WITH b AS (SELECT text_bonus_source($1,$2) j) INSERT INTO text_bonus_payments(match_id,account_id,source,provider_transaction_id,contract_sha256,outcome_sha256,policy_sha256,eligibility_sha256,settlement_sha256,awards_sha256,source_sha256,requested,credited,occurred_at,applied_at)
 SELECT $1,$2,$3,$4,$5,$6,$7,encode(sha256(convert_to((j->'start')::text,'UTF8')),'hex'),encode(sha256(convert_to((j->'settlement')::text,'UTF8')),'hex'),encode(sha256(convert_to((j->'awards')::text,'UTF8')),'hex'),encode(sha256(convert_to(j::text,'UTF8')),'hex'),$8,$9,$10,$11 FROM b RETURNING source_sha256`, match, account, result.Source, proof, contractHash, outcomeHash, policyHash, result.Requested, result.Credited, result.OccurredAt, at).Scan(&result.SourceHash)
		if err != nil {
			return err
		}
		for j := range result.Items {
			i := &result.Items[j]
			if i.Credited > 0 {
				var id int64
				if err = tx.QueryRowContext(ctx, `INSERT INTO noin_ledger(account_id,event_type,amount,reason,payload,server_day) VALUES($1,'match_bonus',$2,'text match bonus',jsonb_build_object('writer','text-bonus-v1','match_id',$3::text,'award_kind',$4::text,'ordinal',$5::int),$6) RETURNING id`, account, i.Credited, match, i.Kind, i.Ordinal, i.ServerDay).Scan(&id); err != nil {
					return err
				}
				i.LedgerID = &id
				if _, err = tx.ExecContext(ctx, `UPDATE daily_noin_earned SET earned=earned+$3,updated_at=now() WHERE account_id=$1 AND server_day=$2`, account, i.ServerDay, i.Credited); err != nil {
					return err
				}
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO text_bonus_payment_items(match_id,account_id,kind,ordinal,body_sha256,server_day,base_credited,credited,ledger_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, match, account, i.Kind, i.Ordinal, i.BodyHash, i.ServerDay, i.BaseCredited, i.Credited, i.LedgerID); err != nil {
				return err
			}
		}
		if result.Credited > 0 {
			if _, err = tx.ExecContext(ctx, `INSERT INTO noin_wallets(account_id,balance) VALUES($1,$2) ON CONFLICT(account_id) DO UPDATE SET balance=noin_wallets.balance+EXCLUDED.balance,updated_at=now()`, account, result.Credited); err != nil {
				return err
			}
		}
		if err = insertBonusDelivery(ctx, tx, result); err != nil {
			return err
		}
		if s.beforeCommit != nil {
			return s.beforeCommit()
		}
		return nil
	})
	if err != nil {
		return TextBonusPayment{}, err
	}
	return result, nil
}

func loadBonusAwards(ctx context.Context, tx *sql.Tx, m lockedTextMatch, account string, effects []byte) ([]bonusAward, error) {
	rows, err := tx.QueryContext(ctx, `SELECT kind,ordinal,body_hash,server_day,requested,credited,ledger_id,occurred_at FROM text_award_receipts WHERE match_id=$1 AND account_id=$2 ORDER BY kind,ordinal LIMIT 10`, m.record.Contract.MatchID, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	awards := []bonusAward{}
	projection := []TextPrivateAward{}
	for rows.Next() {
		var a bonusAward
		if err = rows.Scan(&a.Kind, &a.Ordinal, &a.BodyHash, &a.ServerDay, &a.Requested, &a.BaseCredited, &a.BaseLedger, &a.OccurredAt); err != nil {
			return nil, err
		}
		if (a.Kind == "correct_vote" || a.Kind == "donower_vote_survived") && (a.Ordinal < 1 || a.Ordinal > 3) || a.Kind != "correct_vote" && a.Kind != "donower_vote_survived" && a.Ordinal != 0 {
			return nil, ErrValueConflict
		}
		award := TextAward{MatchID: m.record.Contract.MatchID, Owner: m.record.Owner, Epoch: m.record.Epoch, AccountID: account, Kind: a.Kind, Ordinal: a.Ordinal, Amount: a.Requested, At: a.OccurredAt.UTC()}
		pinned := TextValueStore{tuning: m.record.Policy}
		if err = pinned.validateAwardAmount(award, false); err != nil {
			return nil, err
		}
		_, hash, err := valueHash(award)
		if err != nil {
			return nil, err
		}
		if hash != a.BodyHash || !valueDay(a.OccurredAt).Equal(a.ServerDay) || a.BaseCredited < 0 || a.BaseCredited > a.Requested || (a.BaseCredited > 0) != a.BaseLedger.Valid {
			return nil, ErrValueConflict
		}
		awards = append(awards, a)
		projection = append(projection, TextPrivateAward{Kind: a.Kind, Ordinal: a.Ordinal, Requested: a.Requested, Credited: a.BaseCredited})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(awards) > 9 {
		return nil, ErrValueConflict
	}
	var settled TextPrivateSettlement
	if err = json.Unmarshal(effects, &settled); err != nil {
		return nil, err
	}
	left, _ := json.Marshal(projection)
	right, _ := json.Marshal(settled.Awards)
	if string(left) != string(right) || settled.Interrupted || settled.MatchID != m.record.Contract.MatchID {
		return nil, ErrValueConflict
	}
	return awards, nil
}

func readTextBonusPayment(ctx context.Context, tx *sql.Tx, match, account string) (TextBonusPayment, error) {
	p := TextBonusPayment{MatchID: match, AccountID: account, Items: []TextBonusPaymentItem{}}
	err := tx.QueryRowContext(ctx, `SELECT source,COALESCE(provider_transaction_id,''),source_sha256,requested,credited,occurred_at,applied_at FROM text_bonus_payments WHERE match_id=$1 AND account_id=$2`, match, account).Scan(&p.Source, &p.ProviderTransactionID, &p.SourceHash, &p.Requested, &p.Credited, &p.OccurredAt, &p.AppliedAt)
	if err != nil {
		return p, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT kind,ordinal,body_sha256,server_day,base_credited,credited,ledger_id FROM text_bonus_payment_items WHERE match_id=$1 AND account_id=$2 ORDER BY server_day,kind,ordinal`, match, account)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var i TextBonusPaymentItem
		if err = rows.Scan(&i.Kind, &i.Ordinal, &i.BodyHash, &i.ServerDay, &i.BaseCredited, &i.Credited, &i.LedgerID); err != nil {
			return p, err
		}
		p.Items = append(p.Items, i)
	}
	return p, rows.Err()
}

// RecoverBonuses selects a bounded immutable work set before processing, so a
// concurrent deletion cannot strand the following eligible accounts in the pass.
func (s *TextValueStore) RecoverBonuses(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrValueConflict
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.match_id,e.account_id FROM text_bonus_eligibility e JOIN text_matches m ON m.id=e.match_id JOIN text_settlements t USING(match_id,account_id) JOIN accounts a ON a.id=e.account_id WHERE m.state IN ('completed','scored_low_population') AND m.contract->>'Prototype'='false' AND m.contract#>>'{Contract,eligibility,rewards}'='true' AND e.policy_version='match_start_v1' AND t.state='applied' AND a.deleted_at IS NULL AND a.auth_purpose='player' AND NOT EXISTS(SELECT 1 FROM account_deletion_fences f WHERE f.account_id=e.account_id) AND NOT EXISTS(SELECT 1 FROM text_bonus_payments b WHERE b.match_id=e.match_id AND b.account_id=e.account_id) AND (e.premium_bonus_eligible OR EXISTS(SELECT 1 FROM text_reward_claims c JOIN text_reward_ssv_receipts r USING(claim_hash) WHERE c.match_id=e.match_id AND c.account_id=e.account_id AND r.occurred_at>=c.issued_at AND r.occurred_at<c.expires_at)) ORDER BY e.match_id,e.account_id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	var selected [][2]string
	for rows.Next() {
		var pair [2]string
		if err = rows.Scan(&pair[0], &pair[1]); err != nil {
			rows.Close()
			return 0, err
		}
		selected = append(selected, pair)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, p := range selected {
		if _, err = s.ApplyBonus(ctx, p[0], p[1]); errors.Is(err, ErrTextBonusIneligible) {
			continue
		} else if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
