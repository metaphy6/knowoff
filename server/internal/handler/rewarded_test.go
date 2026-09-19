package handler

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/store"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type rewardHTTPTransport func(*http.Request) (*http.Response, error)

func (f rewardHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type rewardHTTPStore struct {
	calls int
	proof store.VerifiedRewardInteraction
	db    *sql.DB
}

func (s *rewardHTTPStore) RecordVerifiedSSV(_ context.Context, p store.VerifiedRewardInteraction) error {
	s.calls++
	s.proof = p
	return nil
}
func (s *rewardHTTPStore) IssueClaim(ctx context.Context, _ string, _ string, _ string, guard func(context.Context, *sql.Tx) error) (store.TextRewardClaim, error) {
	if s.db != nil {
		tx, e := s.db.BeginTx(ctx, nil)
		if e != nil {
			return store.TextRewardClaim{}, e
		}
		defer tx.Rollback()
		if e = guard(ctx, tx); e != nil {
			return store.TextRewardClaim{}, e
		}
	}
	s.calls++
	return store.TextRewardClaim{}, nil
}

func TestRewardedHTTPClaimFreshAuthorityAndStrictBody(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	cfg := config.RewardedConfig{Enabled: true, MaxQueryBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 2, KeyCacheS: 60, KeyRefreshMinS: 1, MaxConcurrentRequests: 4, ClaimTTLS: 60, MaxClaimsPerMatchWindow: 2, AdUnits: map[string]config.RewardedUnit{"123": {RewardItem: "match_bonus", RewardAmount: 1}}}
	verifier, e := economy.NewAdMobVerifier(cfg, nil)
	if e != nil {
		t.Fatal(e)
	}
	receipts := &rewardHTTPStore{db: econ.DB()}
	h, e := NewRewardedHandlers(cfg, am, receipts, verifier)
	if e != nil {
		t.Fatal(e)
	}
	unsupported := httptest.NewRecorder()
	unsupportedRequest := httptest.NewRequest("POST", "/claim", strings.NewReader(`{"match_id":"a","ad_unit":"123"}`))
	unsupportedRequest.Header.Set("Authorization", "Bearer "+token)
	h.Claim.ServeHTTP(unsupported, unsupportedRequest)
	if unsupported.Code != 503 || receipts.calls != 0 {
		t.Fatal("unbounded response wrapper accepted", unsupported.Code)
	}
	for _, body := range []string{`{"match_id":"a","ad_unit":"123","amount":100}`, `{"match_id":"a","match_id":"b","ad_unit":"123"}`, `{"match_id":"a","ad_unit":"123"}{}`, `{"match_id":"` + strings.Repeat("a", 5000) + `"}`} {
		r := httptest.NewRequest("POST", "/claim", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := &rewardDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
		h.Claim.ServeHTTP(w, r)
		if w.Code != 400 || receipts.calls != 0 {
			t.Fatal("invalid body reached store", w.Code, receipts.calls)
		}
	}
	body := &revokeAvatarBody{Reader: bytes.NewReader([]byte(`{"match_id":"a","ad_unit":"123"}`)), before: func() {
		if e := am.RevokeSessions(t.Context(), account); e != nil {
			t.Fatal(e)
		}
	}}
	r := httptest.NewRequest("POST", "/claim", body)
	r.Header.Set("Authorization", "Bearer "+token)
	w := &rewardDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.Claim.ServeHTTP(w, r)
	if w.Code != 400 || receipts.calls != 0 {
		t.Fatal("revoked credential acquired claim", w.Code, receipts.calls)
	}
}

type rewardDeadlineRecorder struct{ *httptest.ResponseRecorder }

func (*rewardDeadlineRecorder) SetReadDeadline(time.Time) error { return nil }

func TestRewardedHTTPBoundsRealSlowBody(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	_, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	cfg := config.RewardedConfig{Enabled: true, MaxQueryBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 1, KeyCacheS: 60, KeyRefreshMinS: 1, MaxConcurrentRequests: 4, ClaimTTLS: 60, MaxClaimsPerMatchWindow: 2, AdUnits: map[string]config.RewardedUnit{"123": {RewardItem: "match_bonus", RewardAmount: 1}}}
	verifier, e := economy.NewAdMobVerifier(cfg, nil)
	if e != nil {
		t.Fatal(e)
	}
	receipts := &rewardHTTPStore{db: econ.DB()}
	h, e := NewRewardedHandlers(cfg, am, receipts, verifier)
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(h.Claim)
	defer server.Close()
	for _, tc := range []struct {
		method, authorization string
		status                int
	}{{"POST", "Bearer " + token, 400}, {"POST", "", 401}, {"PUT", "Bearer " + token, 405}} {
		for _, connection := range []string{"", "Connection: close\r\n"} {
			conn, e := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
			if e != nil {
				t.Fatal(e)
			}
			defer conn.Close()
			if e = conn.SetDeadline(time.Now().Add(3 * time.Second)); e != nil {
				t.Fatal(e)
			}
			started := time.Now()
			if _, e = fmt.Fprintf(conn, "%s /claim HTTP/1.1\r\nHost: localhost\r\nAuthorization: %s\r\nContent-Length: 100\r\n%s\r\n{", tc.method, tc.authorization, connection); e != nil {
				t.Fatal(e)
			}
			response, e := http.ReadResponse(bufio.NewReader(conn), nil)
			if e != nil {
				t.Fatal("slow body did not receive bounded refusal", e)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.status || time.Since(started) > 2500*time.Millisecond || receipts.calls != 0 {
				t.Fatal("slow body reached store or exceeded request deadline", response.StatusCode)
			}

		}

	}

}

func TestRewardedHTTPProofBeforeStoreAndNoClientValue(t *testing.T) {
	cfg := config.RewardedConfig{Enabled: true, MaxQueryBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 2, KeyCacheS: 60, KeyRefreshMinS: 1, MaxConcurrentRequests: 4, ClaimTTLS: 60, MaxClaimsPerMatchWindow: 2, AdUnits: map[string]config.RewardedUnit{"123": {RewardItem: "match_bonus", RewardAmount: 1}}}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	pub, e := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if e != nil {
		t.Fatal(e)
	}
	keys, e := json.Marshal(map[string]any{"keys": []map[string]any{{"keyId": 7, "base64": base64.StdEncoding.EncodeToString(pub)}}})
	if e != nil {
		t.Fatal(e)
	}
	verifier, e := economy.NewAdMobVerifier(cfg, rewardHTTPTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(keys)))}, nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	receipts := &rewardHTTPStore{}
	h, e := NewRewardedHandlers(cfg, nil, receipts, verifier)
	if e != nil {
		t.Fatal(e)
	}
	claim := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	body := fmt.Sprintf("ad_network=5450213213286189855&ad_unit=123&custom_data=%s&reward_amount=1&reward_item=match_bonus&timestamp=%d&transaction_id=abcdef", claim, time.Now().UnixMilli())
	digest := sha256.Sum256([]byte(body))
	sig, e := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if e != nil {
		t.Fatal(e)
	}
	query := body + "&signature=" + base64.RawURLEncoding.EncodeToString(sig) + "&key_id=7"
	for _, bad := range []string{strings.Replace(query, "reward_amount=1", "reward_amount=99", 1), query + "&extra=1", strings.Replace(query, "transaction_id=abcdef", "transaction_id=abcdff", 1)} {
		w := &rewardDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
		h.SSV.ServeHTTP(w, httptest.NewRequest("GET", "/ssv?"+bad, nil))
		if w.Code != 400 || receipts.calls != 0 {
			t.Fatalf("proof reached store: status=%d calls=%d", w.Code, receipts.calls)
		}
	}
	w := &rewardDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.SSV.ServeHTTP(w, httptest.NewRequest("GET", "/ssv?"+query, nil))
	if w.Code != 200 || receipts.calls != 1 || receipts.proof.Claim != claim || receipts.proof.TransactionID != "abcdef" {
		t.Fatalf("valid proof: %d %+v", w.Code, receipts)
	}
	if w.Body.Len() != 0 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("callback leaked receipt or lacked privacy header")
	}
	w = &rewardDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.Claim.ServeHTTP(w, httptest.NewRequest("POST", "/claim", strings.NewReader(`{"match_id":"x","ad_unit":"123","amount":500}`)))
	if receipts.calls != 1 || w.Code == 200 {
		t.Fatal("client value or unauthenticated claim accepted")
	}
}
