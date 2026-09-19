package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"unicode/utf8"

	"github.com/knowoff/knowoff/server/internal/auth"
)

func oauthRequest(w http.ResponseWriter, r *http.Request, target any, keys ...string) bool {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	data, err := io.ReadAll(r.Body)
	if err != nil || !utf8.Valid(data) {
		oauthError(w, auth.ErrOAuthInvalid)
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		oauthError(w, auth.ErrOAuthInvalid)
		return false
	}
	values := map[string]string{}
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		allowed := false
		for _, name := range keys {
			if name == key {
				allowed = true
			}
		}
		if _, duplicate := values[key]; err != nil || !ok || !allowed || duplicate {
			oauthError(w, auth.ErrOAuthInvalid)
			return false
		}
		token, err = decoder.Token()
		value, ok := token.(string)
		if err != nil || !ok {
			oauthError(w, auth.ErrOAuthInvalid)
			return false
		}
		values[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || len(values) != len(keys) {
		oauthError(w, auth.ErrOAuthInvalid)
		return false
	}
	if _, err = decoder.Token(); err != io.EOF {
		oauthError(w, auth.ErrOAuthInvalid)
		return false
	}
	if err = json.Unmarshal(data, target); err != nil {
		oauthError(w, auth.ErrOAuthInvalid)
		return false
	}
	return true
}

func oauthError(w http.ResponseWriter, err error) {
	code := auth.OAuthErrorCode(err)
	status := http.StatusBadRequest
	switch code {
	case "oauth.pending":
		status = http.StatusAccepted
	case "oauth.conflict":
		status = http.StatusConflict
	case "oauth.provider_unavailable":
		status = http.StatusServiceUnavailable
	case "oauth.rate_limited":
		status = http.StatusTooManyRequests
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}

type oauthService interface {
	BeginOAuth(context.Context, auth.OAuthStartRequest) (*auth.OAuthStart, error)
	CompleteOAuth(context.Context, string, string, string) error
	OAuthResult(context.Context, string, string) (*auth.TokenPair, error)
}

func registerOAuthRoutes(mux *http.ServeMux, deps AuthDeps) {
	registerOAuthServiceRoutes(mux, deps.Auth, deps.OAuthTrustedProxyCIDRs...)
}
func registerOAuthServiceRoutes(mux *http.ServeMux, service oauthService, trustedProxies ...string) {
	mux.HandleFunc("/api/auth/oauth/start", func(w http.ResponseWriter, r *http.Request) {
		var req auth.OAuthStartRequest
		if !oauthRequest(w, r, &req, "provider", "intent", "device_hash") {
			return
		}
		authorization := r.Header.Get("Authorization")
		if authorization != "" {
			if !strings.HasPrefix(authorization, "Bearer ") {
				oauthError(w, auth.ErrOAuthInvalid)
				return
			}
			req.AccessToken = strings.TrimPrefix(authorization, "Bearer ")
			if req.AccessToken == "" {
				oauthError(w, auth.ErrOAuthInvalid)
				return
			}
		}
		principal, ok := oauthClientPrincipal(r, trustedProxies)
		if !ok {
			oauthError(w, auth.ErrOAuthInvalid)
			return
		}
		req.Principal = principal
		result, err := service.BeginOAuth(r.Context(), req)
		if err != nil {
			oauthError(w, err)
			return
		}
		writeJSON(w, result)
	})
	mux.HandleFunc("/api/auth/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if len(r.URL.RawQuery) > 8192 {
			oauthError(w, auth.ErrOAuthInvalid)
			return
		}
		q := r.URL.Query()
		if len(q["provider"]) != 1 || len(q["state"]) != 1 || len(q["code"]) != 1 {
			oauthError(w, auth.ErrOAuthInvalid)
			return
		}
		if err := service.CompleteOAuth(r.Context(), q.Get("provider"), q.Get("state"), q.Get("code")); err != nil {
			oauthError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html lang=\"en\"><meta charset=\"utf-8\"><title>Knowoff account</title><body><p>Account verified. Return to Knowoff to continue.</p></body></html>")
	})
	mux.HandleFunc("/api/auth/oauth/result", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			FlowID string `json:"flow_id"`
			Secret string `json:"completion_secret"`
		}
		if !oauthRequest(w, r, &req, "flow_id", "completion_secret") {
			return
		}
		result, err := service.OAuthResult(r.Context(), req.FlowID, req.Secret)
		if err != nil {
			oauthError(w, err)
			return
		}
		writeJSON(w, result)
	})
}

// Forwarding is accepted only from explicitly trusted immediate hops. Walking
// from the server back toward the client stops at the first untrusted address,
// so a client-supplied leftmost address never substitutes for the actual client.
func oauthClientPrincipal(r *http.Request, cidrs []string) (string, bool) {
	if len(cidrs) > 16 {
		return "", false
	}
	networks := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil || prefix.Bits() == 0 || prefix.Addr().Is4In6() || prefix != prefix.Masked() {
			return "", false
		}
		networks = append(networks, prefix)
	}
	trusted := func(address netip.Addr) bool {
		for _, network := range networks {
			if network.Contains(address) {
				return true
			}
		}
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", false
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || peer.Zone() != "" {
		return "", false
	}
	peer = peer.Unmap()
	if !trusted(peer) {
		return peer.String(), true
	}
	forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if forwarded == "" {
		return peer.String(), true
	}
	if len(forwarded) > 2048 {
		return "", false
	}
	raw := strings.Split(forwarded, ",")
	if len(raw) > 16 {
		return "", false
	}
	hops := make([]netip.Addr, len(raw))
	for i, value := range raw {
		address, err := netip.ParseAddr(strings.TrimSpace(value))
		if err != nil || address.Zone() != "" {
			return "", false
		}
		hops[i] = address.Unmap()
	}
	for i := len(hops) - 1; i >= 0 && trusted(peer); i-- {
		peer = hops[i]
	}
	return peer.String(), true
}
