package economy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/knowoff/knowoff/server/internal/config"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func rewardedFixture() config.RewardedConfig {
	return config.RewardedConfig{Enabled: true, MaxQueryBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 1, KeyCacheS: 60, KeyRefreshMinS: 1, MaxConcurrentRequests: 4, ClaimTTLS: 1800, AdUnits: map[string]config.RewardedUnit{"123": {RewardItem: "match_bonus", RewardAmount: 1}}}
}

type rewardedTransport func(*http.Request) (*http.Response, error)

func (f rewardedTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func rewardedKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	b, e := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(map[string]any{"keys": []map[string]any{{"keyId": 7, "base64": base64.StdEncoding.EncodeToString(b)}}})
	if e != nil {
		t.Fatal(e)
	}
	return k, string(raw)
}
func rewardedQuery(t *testing.T, k *ecdsa.PrivateKey, at time.Time) string {
	t.Helper()
	body := fmt.Sprintf("ad_network=5450213213286189855&ad_unit=123&custom_data=%s&reward_amount=1&reward_item=match_bonus&timestamp=%d&transaction_id=0123456789abcdef", base64.RawURLEncoding.EncodeToString(make([]byte, 32)), at.UnixMilli())
	h := sha256.Sum256([]byte(body))
	sig, e := ecdsa.SignASN1(rand.Reader, k, h[:])
	if e != nil {
		t.Fatal(e)
	}
	return body + "&signature=" + base64.RawURLEncoding.EncodeToString(sig) + "&key_id=7"
}
func TestAdMobVerifiesExactSignedBoundedClaim(t *testing.T) {
	k, keys := rewardedKey(t)
	calls := 0
	v, e := NewAdMobVerifier(rewardedFixture(), rewardedTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://www.gstatic.com/admob/reward/verifier-keys.json" {
			t.Fatal("untrusted key origin")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(keys)), Header: make(http.Header)}, nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	q := rewardedQuery(t, k, now)
	proof, e := v.Verify(t.Context(), q)
	if e != nil || proof.AdUnit != "123" || proof.TransactionID != "0123456789abcdef" || !proof.OccurredAt.Equal(now) || proof.Claim == "" {
		t.Fatal(proof, e)
	}
	for _, bad := range []string{
		strings.Replace(q, "reward_amount=1", "reward_amount=2", 1), strings.Replace(q, "ad_unit=123", "ad_unit=999", 1), strings.Replace(q, "key_id=7", "key_id=8", 1), q + "&extra=x", strings.Replace(q, "ad_unit=123", "ad_unit=123&ad_unit=123", 1), strings.Replace(q, "match_bonus", "match%5fbonus", 1), strings.Replace(q, "&reward_amount=1&reward_item=match_bonus", "&reward_item=match_bonus&reward_amount=1", 1), strings.Repeat("x", 16385),
	} {
		if _, e := v.Verify(t.Context(), bad); e == nil {
			t.Fatal("altered or ambiguous claim accepted")
		}
	}
	if calls != 1 {
		t.Fatal("valid cache hammered by malformed/unknown-key callbacks", calls)
	}
}
func TestAdMobKeyExpiryFailureAndRequestCancellation(t *testing.T) {
	k, keys := rewardedKey(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	calls := 0
	v, e := NewAdMobVerifier(rewardedFixture(), rewardedTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > 1 {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(keys))}, nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	v.now = func() time.Time { return now }
	q := rewardedQuery(t, k, now)
	if _, e = v.Verify(t.Context(), q); e != nil {
		t.Fatal(e)
	}
	now = now.Add(61 * time.Second)
	if _, e = v.Verify(t.Context(), q); e == nil {
		t.Fatal("expired keys accepted after failed refresh")
	}
	if _, e = v.Verify(t.Context(), q); e == nil || calls != 2 {
		t.Fatal("failed key refresh was unbounded", calls, e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e = v.Verify(ctx, q); e == nil {
		t.Fatal("cancelled verifier accepted")
	}
	if _, e = NewAdMobVerifier(config.RewardedConfig{}, nil); e == nil {
		t.Fatal("disabled verifier available")
	}
}

func TestAdMobRefusesUntrustedKeyResponses(t *testing.T) {
	k, keys := rewardedKey(t)
	var body struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.Unmarshal([]byte(keys), &body); err != nil {
		t.Fatal(err)
	}
	duplicate, err := json.Marshal(map[string]any{"keys": []map[string]any{body.Keys[0], body.Keys[0]}})
	if err != nil {
		t.Fatal(err)
	}
	_, other := rewardedKey(t)
	for name, raw := range map[string]string{"wrong key": other, "empty": "{\"keys\":[]}", "duplicate": string(duplicate), "ambiguous JSON": "{\"keys\":[],\"Keys\":[]}", "trailing": keys + " {}", "oversized": strings.Repeat("x", 262145)} {
		t.Run(name, func(t *testing.T) {
			v, e := NewAdMobVerifier(rewardedFixture(), rewardedTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, nil
			}))
			if e != nil {
				t.Fatal(e)
			}
			if _, e = v.Verify(t.Context(), rewardedQuery(t, k, time.Now())); e == nil {
				t.Fatal("untrusted key response accepted")
			}
		})
	}
	t.Run("redirect", func(t *testing.T) {
		calls := 0
		v, e := NewAdMobVerifier(rewardedFixture(), rewardedTransport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://example.invalid/keys"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = v.Verify(t.Context(), rewardedQuery(t, k, time.Now())); e == nil || calls != 1 {
			t.Fatal("key redirect followed", calls, e)
		}
	})
}

func TestAdMobBoundsBlockedFetchAndConcurrentRequests(t *testing.T) {
	k, _ := rewardedKey(t)
	cfg := rewardedFixture()
	cfg.MaxConcurrentRequests = 1
	entered := make(chan struct{})
	var once sync.Once
	v, e := NewAdMobVerifier(cfg, rewardedTransport(func(r *http.Request) (*http.Response, error) {
		once.Do(func() { close(entered) })
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	if e != nil {
		t.Fatal(e)
	}
	q := rewardedQuery(t, k, time.Now())
	done := make(chan error, 1)
	go func() { _, e := v.Verify(t.Context(), q); done <- e }()
	<-entered
	if _, e := v.Verify(t.Context(), q); !errors.Is(e, ErrRewardBusy) {
		t.Fatal("unbounded concurrent verification", e)
	}
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("blocked key fetch accepted")
		}
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("key fetch ignored configured timeout")
	}
}
