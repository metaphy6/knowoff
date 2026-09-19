package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/store"
)

type RewardedReceipts interface {
	CheckClaim(context.Context, string, string, string, string, func(context.Context, *sql.Tx) error) error
	IssueClaim(context.Context, string, string, string, func(context.Context, *sql.Tx) error) (store.TextRewardClaim, error)
	RecordVerifiedSSV(context.Context, store.VerifiedRewardInteraction) error
}

// RewardedHandlers are deliberately unmounted until policy, consent, retention
// and private payout delivery gates are complete. These handlers grant no value.
type RewardedHandlers struct{ Claim, Check, SSV http.Handler }

func NewRewardedHandlers(cfg config.RewardedConfig, authMgr *auth.Manager, receipts RewardedReceipts, verifier *economy.AdMobVerifier) (RewardedHandlers, error) {
	if !cfg.Enabled || cfg.Validate() != nil || receipts == nil || verifier == nil {
		return RewardedHandlers{}, store.ErrRewardClaimUnavailable
	}
	slots := make(chan struct{}, cfg.MaxConcurrentRequests)
	bounded := func(next http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.HTTPTimeoutS)*time.Second)
			defer cancel()
			controller := http.NewResponseController(w)
			deadline, _ := ctx.Deadline()
			if err := controller.SetReadDeadline(deadline); err != nil {
				// Fail closed if a wrapper hides deadline support; close the
				// connection so an unfinished body is not drained for reuse.
				w.Header().Set("Connection", "close")
				rewardedError(w, store.ErrRewardClaimUnavailable)
				return
			}
			defer func() {
				// Flush before clearing the deadline: net/http may first drain
				// a body even after authentication, method or budget refusal.
				_ = controller.Flush()
				_ = controller.SetReadDeadline(time.Time{})
			}()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				rewardedError(w, store.ErrRewardClaimBusy)
				return
			}
			next(w, r.WithContext(ctx))
		})
	}
	claimHandler := func(check bool) http.Handler {
		return bounded(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if authMgr == nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			raw := r.Header.Get("Authorization")
			token := strings.TrimPrefix(raw, "Bearer ")
			if token == raw || token == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			account, err := authMgr.ValidateAccessToken(r.Context(), token)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var req struct {
				MatchID string `json:"match_id"`
				AdUnit  string `json:"ad_unit"`
				Claim   string `json:"claim,omitempty"`
			}
			fields := []string{"match_id", "ad_unit"}
			if check {
				fields = append(fields, "claim")
			}
			if !oauthRequest(w, r, &req, fields...) {
				return
			}
			authorize := func(ctx context.Context, tx *sql.Tx) error {
				got, e := authMgr.ValidateAccessTokenTx(ctx, tx, token)
				if e != nil || got != account {
					return store.ErrRewardClaimInvalid
				}
				return nil
			}
			if check {
				if err := receipts.CheckClaim(r.Context(), req.MatchID, account, req.AdUnit, req.Claim, authorize); err != nil {
					rewardedError(w, err)
					return
				}
				writeJSON(w, map[string]any{"version": 1, "eligible": true})
				return
			}
			result, err := receipts.IssueClaim(r.Context(), req.MatchID, account, req.AdUnit, authorize)
			if err != nil {
				rewardedError(w, err)
				return
			}
			writeJSON(w, result)
		})
	}
	ssv := bounded(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		proof, err := verifier.Verify(r.Context(), r.URL.RawQuery)
		if err != nil {
			rewardedError(w, err)
			return
		}
		err = receipts.RecordVerifiedSSV(r.Context(), store.VerifiedRewardInteraction{TransactionID: proof.TransactionID, Fingerprint: proof.Fingerprint, Claim: proof.Claim, AdUnit: proof.AdUnit, OccurredAt: proof.OccurredAt})
		if err != nil {
			rewardedError(w, err)
			return
		}
		// AdMob requires exactly 200 OK to acknowledge its callback.
		w.WriteHeader(http.StatusOK)
	})
	return RewardedHandlers{Claim: claimHandler(false), Check: claimHandler(true), SSV: ssv}, nil
}

func rewardedError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	code := "reward.unavailable"
	switch {
	case errors.Is(err, economy.ErrRewardProof), errors.Is(err, store.ErrRewardClaimInvalid):
		status = http.StatusBadRequest
		code = "reward.invalid_proof"
	case errors.Is(err, store.ErrRewardClaimConflict):
		status = http.StatusConflict
		code = "reward.receipt_conflict"
	case errors.Is(err, economy.ErrRewardBusy), errors.Is(err, store.ErrRewardClaimBusy):
		status = http.StatusTooManyRequests
		code = "reward.busy"
	}
	http.Error(w, code, status)
}
