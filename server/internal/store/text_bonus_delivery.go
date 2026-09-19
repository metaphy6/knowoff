package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/lib/pq"
)

var (
	ErrBonusDeliveryInvalid      = errors.New("reward.bonus_delivery_invalid")
	ErrBonusDeliveryStale        = errors.New("reward.bonus_delivery_stale")
	ErrBonusDeliveryUnauthorized = errors.New("auth.invalid_token")
)

type TextBonusDay struct {
	ServerDay string `json:"server_day"`
	Requested int    `json:"requested"`
	Credited  int    `json:"credited"`
}
type TextBonusPayload struct {
	Version   int            `json:"version"`
	MatchID   string         `json:"match_id"`
	Source    string         `json:"source"`
	Requested int            `json:"requested"`
	Credited  int            `json:"credited"`
	Days      []TextBonusDay `json:"days"`
}
type TextBonusDelivery struct {
	DeliveryID     string           `json:"delivery_id"`
	Lease          string           `json:"lease"`
	LeaseExpiresAt time.Time        `json:"lease_expires_at"`
	PayloadSHA256  string           `json:"payload_sha256"`
	Payload        TextBonusPayload `json:"payload"`
}
type TextBonusDeliveryPage struct {
	Version         int                 `json:"version"`
	Deliveries      []TextBonusDelivery `json:"deliveries"`
	AcknowledgedIDs []string            `json:"acknowledged_ids"`
}

// DeliveryPayload excludes financial source receipts and original award kinds.
// Nine source items imply at most nine original earning days; no item is dropped.
func (p TextBonusPayment) DeliveryPayload() (TextBonusPayload, string, error) {
	b := TextBonusPayload{Version: 1, MatchID: p.MatchID, Source: p.Source, Requested: p.Requested, Credited: p.Credited, Days: []TextBonusDay{}}
	if p.Source == "ssv" {
		b.Source = "rewarded_ad"
	} else if p.Source != "premium" {
		return b, "", ErrValueConflict
	}
	if len(p.Items) > 9 {
		return b, "", ErrValueConflict
	}
	days := map[string]TextBonusDay{}
	for _, i := range p.Items {
		if i.BaseCredited < 0 || i.Credited < 0 || i.Credited > i.BaseCredited || !valueDay(i.ServerDay).Equal(i.ServerDay) {
			return b, "", ErrValueConflict
		}
		key := i.ServerDay.UTC().Format("2006-01-02")
		d := days[key]
		d.ServerDay = key
		if int64(d.Requested)+int64(i.BaseCredited) > math.MaxInt32 {
			return b, "", ErrValueConflict
		}
		d.Requested += i.BaseCredited
		d.Credited += i.Credited
		days[key] = d
	}
	for _, d := range days {
		b.Days = append(b.Days, d)
	}
	sort.Slice(b.Days, func(i, j int) bool { return b.Days[i].ServerDay < b.Days[j].ServerDay })
	hash, err := b.hash()
	return b, hash, err
}

// The versioned compact vector is shared with the client. It contains only
// validated ASCII strings and bounded integers, so JSON encoding is unambiguous.
func (b TextBonusPayload) hash() (string, error) {
	if b.Version != 1 || !valueUUID(b.MatchID) || (b.Source != "premium" && b.Source != "rewarded_ad") || b.Requested < 0 || int64(b.Requested) > math.MaxInt32 || b.Credited < 0 || b.Credited > b.Requested || b.Days == nil || len(b.Days) > 9 {
		return "", ErrValueConflict
	}
	days := make([]any, 0, len(b.Days))
	var requested, credited int64
	prior := ""
	for _, d := range b.Days {
		day, err := time.Parse("2006-01-02", d.ServerDay)
		if err != nil || day.Format("2006-01-02") != d.ServerDay || d.ServerDay <= prior || d.Requested < 0 || int64(d.Requested) > math.MaxInt32 || d.Credited < 0 || d.Credited > d.Requested {
			return "", ErrValueConflict
		}
		prior = d.ServerDay
		requested += int64(d.Requested)
		credited += int64(d.Credited)
		days = append(days, []any{d.ServerDay, d.Requested, d.Credited})
	}
	if requested != int64(b.Requested) || credited != int64(b.Credited) {
		return "", ErrValueConflict
	}
	raw, err := json.Marshal([]any{1, b.MatchID, b.Source, b.Requested, b.Credited, days})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func bonusDeliverySecret(raw string) ([]byte, error) {
	b, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(b) != 32 {
		return nil, ErrBonusDeliveryInvalid
	}
	sum := sha256.Sum256(b)
	return sum[:], nil
}

func insertBonusDelivery(ctx context.Context, tx *sql.Tx, p TextBonusPayment) error {
	payload, hash, err := p.DeliveryPayload()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO text_bonus_outbox(match_id,account_id,payload,payload_sha256) VALUES($1,$2,$3,$4)`, p.MatchID, p.AccountID, raw, hash)
	return err
}

func bonusDeliveryAuthority(ctx context.Context, tx *sql.Tx, account string, authorize func(context.Context, *sql.Tx) error) error {
	if authorize == nil {
		return ErrBonusDeliveryUnauthorized
	}
	// Account first, then the same bearer's installation locks inside authorize.
	if err := lockActiveTextRecipient(ctx, tx, account); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrValueFence) {
			return ErrBonusDeliveryUnauthorized
		}
		return err
	}
	var player bool
	if err := tx.QueryRowContext(ctx, `SELECT auth_purpose='player' FROM accounts WHERE id=$1`, account).Scan(&player); err != nil {
		return err
	}
	if !player {
		return ErrBonusDeliveryUnauthorized
	}
	return authorize(ctx, tx)
}

// ClaimBonusDeliveries returns committed private delivery state only. Neither
// claiming nor acknowledgment changes wallet value. Public routes remain closed.
func (s *TextValueStore) ClaimBonusDeliveries(ctx context.Context, account string, limit int, pending []string, authorize func(context.Context, *sql.Tx) error) (TextBonusDeliveryPage, error) {
	if !valueUUID(account) || limit < 1 || limit > 20 || pending == nil || len(pending) > 20 {
		return TextBonusDeliveryPage{}, ErrBonusDeliveryInvalid
	}
	seen := map[string]bool{}
	for _, id := range pending {
		if !valueUUID(id) || seen[id] {
			return TextBonusDeliveryPage{}, ErrBonusDeliveryInvalid
		}
		seen[id] = true
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var page TextBonusDeliveryPage
	err := s.transaction(ctx, func(tx *sql.Tx) error {
		page = TextBonusDeliveryPage{Version: 1, Deliveries: []TextBonusDelivery{}, AcknowledgedIDs: []string{}}
		if err := bonusDeliveryAuthority(ctx, tx, account, authorize); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT id,payload,payload_sha256 FROM text_bonus_outbox WHERE account_id=$1 AND acknowledged_at IS NULL AND (lease_until IS NULL OR lease_until<=clock_timestamp()) ORDER BY id LIMIT $2 FOR UPDATE`, account, limit)
		if err != nil {
			return err
		}
		for rows.Next() {
			var d TextBonusDelivery
			var raw []byte
			if err = rows.Scan(&d.DeliveryID, &raw, &d.PayloadSHA256); err != nil {
				rows.Close()
				return err
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err = decoder.Decode(&d.Payload); err != nil {
				rows.Close()
				return err
			}
			hash, err := d.Payload.hash()
			if err != nil || hash != d.PayloadSHA256 {
				rows.Close()
				return ErrValueConflict
			}
			page.Deliveries = append(page.Deliveries, d)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for i := range page.Deliveries {
			d := &page.Deliveries[i]
			raw := make([]byte, 32)
			if _, err = rand.Read(raw); err != nil {
				return err
			}
			d.Lease = base64.RawURLEncoding.EncodeToString(raw)
			hash, _ := bonusDeliverySecret(d.Lease)
			if err = tx.QueryRowContext(ctx, `UPDATE text_bonus_outbox SET attempts=attempts+1,lease_sha256=$2,lease_until=clock_timestamp()+interval '30 seconds' WHERE id=$1 RETURNING lease_until`, d.DeliveryID, hash).Scan(&d.LeaseExpiresAt); err != nil {
				return err
			}
		}
		// Read-only recovery for lost ACK responses and ACKs from another device.
		rows, err = tx.QueryContext(ctx, `SELECT id FROM text_bonus_outbox WHERE account_id=$1 AND id=ANY($2::uuid[]) AND acknowledged_at IS NOT NULL ORDER BY id`, account, pq.Array(pending))
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			page.AcknowledgedIDs = append(page.AcknowledgedIDs, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if s.beforeCommit != nil {
			if err = s.beforeCommit(); err != nil {
				return err
			}
		}
		return bonusDeliveryAuthority(ctx, tx, account, authorize)
	})
	if err != nil {
		return TextBonusDeliveryPage{}, err
	}
	return page, nil
}

func (s *TextValueStore) AcknowledgeBonusDelivery(ctx context.Context, account, id, lease string, authorize func(context.Context, *sql.Tx) error) error {
	if !valueUUID(account) || !valueUUID(id) {
		return ErrBonusDeliveryInvalid
	}
	hash, err := bonusDeliverySecret(lease)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.transaction(ctx, func(tx *sql.Tx) error {
		if err := bonusDeliveryAuthority(ctx, tx, account, authorize); err != nil {
			return err
		}
		var retained []byte
		var until, ack sql.NullTime
		err := tx.QueryRowContext(ctx, `SELECT lease_sha256,lease_until,acknowledged_at FROM text_bonus_outbox WHERE id=$1 AND account_id=$2 FOR UPDATE`, id, account).Scan(&retained, &until, &ack)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBonusDeliveryStale
		}
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(hash, retained) != 1 || !until.Valid {
			return ErrBonusDeliveryStale
		}
		if !ack.Valid {
			// PostgreSQL owns lease time, including after an outbox lock wait.
			var active bool
			if err = tx.QueryRowContext(ctx, `SELECT $1::timestamptz>clock_timestamp()`, until.Time).Scan(&active); err != nil {
				return err
			}
			if !active {
				return ErrBonusDeliveryStale
			}
			if _, err = tx.ExecContext(ctx, `UPDATE text_bonus_outbox SET acknowledged_at=clock_timestamp() WHERE id=$1`, id); err != nil {
				return err
			}
		}
		if s.beforeCommit != nil {
			if err = s.beforeCommit(); err != nil {
				return err
			}
		}
		return bonusDeliveryAuthority(ctx, tx, account, authorize)
	})
}
