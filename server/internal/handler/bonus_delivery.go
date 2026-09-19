package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/store"
)

type BonusDeliveries interface {
	ClaimBonusDeliveries(context.Context, string, int, []string, func(context.Context, *sql.Tx) error) (store.TextBonusDeliveryPage, error)
	AcknowledgeBonusDelivery(context.Context, string, string, string, func(context.Context, *sql.Tx) error) error
}
type BonusDeliveryHTTPConfig struct {
	Timeout       time.Duration
	MaxConcurrent int
}
type BonusDeliveryHandlers struct{ Claim, ACK http.Handler }

// NewBonusDeliveryHandlers is closed until the joined B6 activation gate.
func NewBonusDeliveryHandlers(cfg BonusDeliveryHTTPConfig, authMgr *auth.Manager, deliveries BonusDeliveries) (BonusDeliveryHandlers, error) {
	if cfg.Timeout <= 0 || cfg.Timeout > 5*time.Second || cfg.MaxConcurrent < 1 || cfg.MaxConcurrent > 32 || authMgr == nil || deliveries == nil {
		return BonusDeliveryHandlers{}, store.ErrRewardClaimUnavailable
	}
	slots := make(chan struct{}, cfg.MaxConcurrent)
	bounded := func(ack bool) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
			defer cancel()
			r = r.WithContext(ctx)
			controller := http.NewResponseController(w)
			deadline, _ := ctx.Deadline()
			if controller.SetReadDeadline(deadline) != nil {
				w.Header().Set("Connection", "close")
				bonusDeliveryError(w, store.ErrRewardClaimUnavailable)
				return
			}
			defer func() { _ = controller.Flush(); _ = controller.SetReadDeadline(time.Time{}) }()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				bonusDeliveryError(w, store.ErrRewardClaimBusy)
				return
			}
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			raw := r.Header.Get("Authorization")
			token := strings.TrimPrefix(raw, "Bearer ")
			if token == raw || token == "" {
				bonusDeliveryError(w, store.ErrBonusDeliveryUnauthorized)
				return
			}
			account, err := authMgr.ValidateAccessToken(ctx, token)
			if err != nil {
				bonusDeliveryError(w, store.ErrBonusDeliveryUnauthorized)
				return
			}
			authorize := func(ctx context.Context, tx *sql.Tx) error {
				got, e := authMgr.ValidateAccessTokenTx(ctx, tx, token)
				if e != nil || got != account {
					return store.ErrBonusDeliveryUnauthorized
				}
				return nil
			}
			keys := []string{"limit", "pending_delivery_ids"}
			if ack {
				keys = []string{"delivery_id", "lease"}
			}
			body, err := bonusDeliveryBody(w, r, keys)
			if err != nil {
				bonusDeliveryError(w, store.ErrBonusDeliveryInvalid)
				return
			}
			if ack {
				var id, lease string
				if json.Unmarshal(body["delivery_id"], &id) != nil || json.Unmarshal(body["lease"], &lease) != nil || !bonusUUID(id) {
					bonusDeliveryError(w, store.ErrBonusDeliveryInvalid)
					return
				}
				b, e := base64.RawURLEncoding.Strict().DecodeString(lease)
				if e != nil || len(b) != 32 {
					bonusDeliveryError(w, store.ErrBonusDeliveryInvalid)
					return
				}
				if err = deliveries.AcknowledgeBonusDelivery(ctx, account, id, lease, authorize); err != nil {
					bonusDeliveryError(w, err)
					return
				}
				writeJSON(w, struct {
					Version      int  `json:"version"`
					Acknowledged bool `json:"acknowledged"`
				}{1, true})
				return
			}
			var limit int
			var pending []string
			if json.Unmarshal(body["limit"], &limit) != nil || json.Unmarshal(body["pending_delivery_ids"], &pending) != nil || limit < 1 || limit > 20 || pending == nil || len(pending) > 20 {
				bonusDeliveryError(w, store.ErrBonusDeliveryInvalid)
				return
			}
			seen := map[string]bool{}
			for _, id := range pending {
				if !bonusUUID(id) || seen[id] {
					bonusDeliveryError(w, store.ErrBonusDeliveryInvalid)
					return
				}
				seen[id] = true
			}
			page, err := deliveries.ClaimBonusDeliveries(ctx, account, limit, pending, authorize)
			if err != nil {
				bonusDeliveryError(w, err)
				return
			}
			writeJSON(w, page)
		})
	}
	return BonusDeliveryHandlers{Claim: bounded(false), ACK: bounded(true)}, nil
}

func bonusUUID(raw string) bool {
	id, err := uuid.Parse(raw)
	return err == nil && id != uuid.Nil && id.String() == raw
}

func bonusDeliveryBody(w http.ResponseWriter, r *http.Request, keys []string) (map[string]json.RawMessage, error) {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) > 1 || len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8") || r.URL.RawQuery != "" {
		return nil, store.ErrBonusDeliveryInvalid
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil || !utf8.Valid(raw) {
		return nil, store.ErrBonusDeliveryInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, store.ErrBonusDeliveryInvalid
	}
	values := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		allowed := false
		for _, k := range keys {
			if k == key {
				allowed = true
			}
		}
		if _, duplicate := values[key]; err != nil || !ok || !allowed || duplicate {
			return nil, store.ErrBonusDeliveryInvalid
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return nil, store.ErrBonusDeliveryInvalid
		}
		values[key] = value
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || len(values) != len(keys) {
		return nil, store.ErrBonusDeliveryInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, store.ErrBonusDeliveryInvalid
	}
	return values, nil
}

func bonusDeliveryError(w http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, "reward.unavailable"
	switch {
	case errors.Is(err, store.ErrBonusDeliveryInvalid):
		status, code = 400, "reward.bonus_delivery_invalid"
	case errors.Is(err, store.ErrBonusDeliveryStale):
		status, code = 409, "reward.bonus_delivery_stale"
	case errors.Is(err, store.ErrBonusDeliveryUnauthorized):
		status, code = 401, "auth.invalid_token"
	case errors.Is(err, store.ErrRewardClaimBusy):
		status, code = 429, "reward.busy"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}
