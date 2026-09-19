package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/ratelimit"
)

type deletionService interface {
	BeginEnrollment(context.Context, string) (*auth.DeletionIntent, error)
	EnrollCapability(context.Context, string, string, string, string) error
	BeginCapabilityIntent(context.Context, string, string) (*auth.DeletionIntent, error)
	BeginProviderIntent(context.Context, string, string) (*auth.DeletionIntent, error)
	CompleteProviderIntent(context.Context, string, string, string) error
	IntentStatus(context.Context, string, string) (json.RawMessage, error)
	Confirm(context.Context, auth.DeletionConfirmCommand) (*auth.DeletionConfirmation, error)
	Status(context.Context, string) (json.RawMessage, error)
}

// DeletionHTTPConfig uses engineering ceilings: 30s deadlines, 32 concurrent
// requests, 10,000 peers and at most 10,000 requests/minute or burst tokens.
// DeletionHTTPConfig is explicit rehearsal configuration. The normal router does
// not register this handler; public activation requires the complete D6 journey.
type DeletionHTTPConfig struct {
	PublicOrigin             string
	TrustedProxyCIDRs        []string
	Timeout                  time.Duration
	MaxConcurrent, MaxPeers  int
	RequestsPerMinute, Burst float64
}
type deletionPeer struct {
	limiter *ratelimit.Limiter
	seen    time.Time
}

func NewDeletionHandler(cfg DeletionHTTPConfig, service deletionService) (http.Handler, error) {
	origin, err := url.Parse(cfg.PublicOrigin)
	validPort := true
	if err == nil && origin.Port() != "" {
		n, e := strconv.Atoi(origin.Port())
		validPort = e == nil && n > 0 && n <= 65535
	}
	if err != nil || !validPort || origin.ForceQuery || origin.Opaque != "" || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || cfg.Timeout <= 0 || cfg.Timeout > 30*time.Second || cfg.MaxConcurrent < 1 || cfg.MaxConcurrent > 32 || cfg.MaxPeers < 1 || cfg.MaxPeers > 10000 || math.IsNaN(cfg.RequestsPerMinute) || math.IsInf(cfg.RequestsPerMinute, 0) || cfg.RequestsPerMinute < 1 || cfg.RequestsPerMinute > 10000 || math.IsNaN(cfg.Burst) || math.IsInf(cfg.Burst, 0) || cfg.Burst < 1 || cfg.Burst > 10000 || service == nil {
		return nil, auth.ErrDeletionAuthority
	}
	cfg.TrustedProxyCIDRs = append([]string(nil), cfg.TrustedProxyCIDRs...)
	var mu sync.Mutex
	peers := map[string]deletionPeer{}
	slots := make(chan struct{}, cfg.MaxConcurrent)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
		defer cancel()
		controller := http.NewResponseController(w)
		deadline, _ := ctx.Deadline()
		if controller.SetReadDeadline(deadline) != nil {
			w.Header().Set("Connection", "close")
			deletionError(w, auth.ErrDeletionAuthority)
			return
		}
		defer func() { _ = controller.Flush(); _ = controller.SetReadDeadline(time.Time{}) }()
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			deletionError(w, auth.ErrDeletionRateLimited)
			return
		}
		principal, ok := oauthClientPrincipal(r, cfg.TrustedProxyCIDRs)
		if !ok {
			deletionError(w, auth.ErrDeletionAuthority)
			return
		}
		mu.Lock()
		now := time.Now()
		peer, found := peers[principal]
		if !found {
			for key, old := range peers {
				if now.Sub(old.seen) > max(time.Minute, time.Duration(cfg.Burst/cfg.RequestsPerMinute*float64(time.Minute))) {
					delete(peers, key)
				}
			}
			if len(peers) >= cfg.MaxPeers {
				mu.Unlock()
				deletionError(w, auth.ErrDeletionRateLimited)
				return
			}
			peer = deletionPeer{limiter: ratelimit.New(cfg.RequestsPerMinute/60, cfg.Burst)}
		}
		peer.seen = now
		peers[principal] = peer
		allowed := peer.limiter.Allow()
		mu.Unlock()
		if !allowed {
			deletionError(w, auth.ErrDeletionRateLimited)
			return
		}
		r = r.WithContext(ctx)
		if r.URL.Path == "/api/auth/oauth/callback" {
			deletionCallback(w, r, service)
			return
		}
		if r.URL.RawQuery != "" {
			deletionBadRequest(w)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if got := r.Header.Get("Origin"); got != "" && got != cfg.PublicOrigin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if e != nil || media != "application/json" {
			deletionBadRequest(w)
			return
		}
		var keys []string
		switch r.URL.Path {
		case "/api/account/deletion/enrollment-intents":
			keys = []string{}
		case "/api/account/deletion/capabilities":
			keys = []string{"intent_id", "intent_secret", "capability_id", "capability_secret"}
		case "/api/account/deletion/intents":
			keys = []string{"capability_id", "capability_secret"}
		case "/api/account/deletion/oauth/start":
			keys = []string{"provider", "account_id"}
		case "/api/account/deletion/intent-status":
			keys = []string{"intent_id", "intent_secret"}
		case "/api/account/deletion/confirm":
			keys = []string{"account_id", "intent_id", "intent_secret", "capability_id", "capability_secret", "request_id", "status_secret", "policy_version"}
		case "/api/account/deletion/status":
			keys = []string{"status_secret"}
		default:
			http.NotFound(w, r)
			return
		}
		values, e := deletionBody(w, r, keys)
		if e != nil {
			deletionBadRequest(w)
			return
		}
		var result any
		switch r.URL.Path {
		case "/api/account/deletion/enrollment-intents":
			bearer := r.Header.Get("Authorization")
			if !strings.HasPrefix(bearer, "Bearer ") || len(bearer) <= 7 {
				deletionError(w, auth.ErrDeletionAuthority)
				return
			}
			result, e = service.BeginEnrollment(ctx, bearer[7:])
		case "/api/account/deletion/capabilities":
			e = service.EnrollCapability(ctx, values["intent_id"], values["intent_secret"], values["capability_id"], values["capability_secret"])
			result = map[string]string{"state": "enrolled"}
		case "/api/account/deletion/intents":
			result, e = service.BeginCapabilityIntent(ctx, values["capability_id"], values["capability_secret"])
		case "/api/account/deletion/oauth/start":
			result, e = service.BeginProviderIntent(ctx, values["provider"], values["account_id"])
		case "/api/account/deletion/intent-status":
			result, e = service.IntentStatus(ctx, values["intent_id"], values["intent_secret"])
		case "/api/account/deletion/confirm":
			if values["policy_version"] != auth.DeletionPolicyVersion {
				deletionBadRequest(w)
				return
			}
			result, e = service.Confirm(ctx, auth.DeletionConfirmCommand{AccountID: values["account_id"], IntentID: values["intent_id"], IntentSecret: values["intent_secret"], CapabilityID: values["capability_id"], CapabilitySecret: values["capability_secret"], RequestID: values["request_id"], StatusSecret: values["status_secret"]})
		case "/api/account/deletion/status":
			result, e = service.Status(ctx, values["status_secret"])
		}
		if e != nil {
			deletionError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/account/deletion/confirm" {
			w.WriteHeader(http.StatusAccepted)
		}
		_ = json.NewEncoder(w).Encode(result)
	}), nil
}
func deletionBadRequest(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": "deletion.invalid_request"})
}
func deletionError(w http.ResponseWriter, err error) {
	code, status := "deletion.authority_unavailable", http.StatusServiceUnavailable
	if errors.Is(err, auth.ErrDeletionRecoveryRequired) {
		code, status = "deletion.recovery_required", http.StatusUnauthorized
	}
	if errors.Is(err, auth.ErrDeletionRateLimited) {
		code, status = "deletion.rate_limited", http.StatusTooManyRequests
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}
func deletionBody(w http.ResponseWriter, r *http.Request, keys []string) (map[string]string, error) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil || !utf8.Valid(data) {
		return nil, auth.ErrDeletionAuthority
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, auth.ErrDeletionAuthority
	}
	values := map[string]string{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		allowed := false
		for _, name := range keys {
			if name == key {
				allowed = true
			}
		}
		if _, duplicate := values[key]; err != nil || !ok || !allowed || duplicate {
			return nil, auth.ErrDeletionAuthority
		}
		token, err = d.Token()
		value, ok := token.(string)
		if err != nil || !ok {
			return nil, auth.ErrDeletionAuthority
		}
		values[key] = value
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || len(values) != len(keys) {
		return nil, auth.ErrDeletionAuthority
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, auth.ErrDeletionAuthority
	}
	return values, nil
}
func deletionCallback(w http.ResponseWriter, r *http.Request, service deletionService) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if len(r.URL.RawQuery) > 8192 {
		deletionBadRequest(w)
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(q) != 3 || len(q["provider"]) != 1 || len(q["state"]) != 1 || len(q["code"]) != 1 {
		deletionBadRequest(w)
		return
	}
	if err = service.CompleteProviderIntent(r.Context(), q.Get("provider"), q.Get("state"), q.Get("code")); err != nil {
		deletionError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><html lang=\"en\"><meta charset=\"utf-8\"><title>Knowoff account deletion</title><body><p>Identity verified. Return to Knowoff to confirm deletion.</p></body></html>")
}
