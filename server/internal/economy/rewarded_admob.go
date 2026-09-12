package economy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/knowoff/knowoff/server/internal/config"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrRewardUnavailable = errors.New("reward.unavailable")
var ErrRewardProof = errors.New("reward.invalid_proof")
var ErrRewardBusy = errors.New("reward.busy")

// AdMobProof authenticates an ad interaction only. The opaque Claim must still
// resolve to an eligible settled match; provider reward_amount grants no Noin.
type AdMobProof struct {
	TransactionID, Claim, AdUnit, Fingerprint string
	OccurredAt                                time.Time
}
type AdMobVerifier struct {
	cfg                  config.RewardedConfig
	client               *http.Client
	now                  func() time.Time
	slots, refresh       chan struct{}
	mu                   sync.Mutex
	keys                 map[int64]*ecdsa.PublicKey
	fetched, lastAttempt time.Time
}

func NewAdMobVerifier(c config.RewardedConfig, transport http.RoundTripper) (*AdMobVerifier, error) {
	if !c.Enabled || c.Validate() != nil {
		return nil, ErrRewardUnavailable
	}
	c.AdUnits = maps.Clone(c.AdUnits)
	return &AdMobVerifier{cfg: c, client: &http.Client{Transport: transport, Timeout: time.Duration(c.HTTPTimeoutS) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrRewardUnavailable }}, now: time.Now, slots: make(chan struct{}, c.MaxConcurrentRequests), refresh: make(chan struct{}, 1)}, nil
}
func (v *AdMobVerifier) Verify(ctx context.Context, raw string) (AdMobProof, error) {
	var proof AdMobProof
	if ctx.Err() != nil {
		return proof, ctx.Err()
	}
	if len(raw) == 0 || int64(len(raw)) > v.cfg.MaxQueryBytes {
		return proof, ErrRewardProof
	}
	parts := strings.Split(raw, "&")
	if len(parts) < 9 || len(parts) > 10 || !strings.HasPrefix(parts[len(parts)-2], "signature=") || !strings.HasPrefix(parts[len(parts)-1], "key_id=") {
		return proof, ErrRewardProof
	}
	fields := map[string]string{}
	allowed := map[string]bool{"ad_network": true, "ad_unit": true, "custom_data": true, "reward_amount": true, "reward_item": true, "timestamp": true, "transaction_id": true, "user_id": true}
	for _, part := range parts[:len(parts)-2] {
		name, value, ok := strings.Cut(part, "=")
		if !ok || !allowed[name] || fields[name] != "" || !rewardASCII(value) {
			return proof, ErrRewardProof
		}
		fields[name] = value
	}
	for name := range allowed {
		if name != "user_id" && fields[name] == "" {
			return proof, ErrRewardProof
		}
	}
	u, ok := v.cfg.AdUnits[fields["ad_unit"]]
	if !ok || fields["reward_item"] != u.RewardItem || fields["reward_amount"] != strconv.Itoa(u.RewardAmount) || len(fields["ad_network"]) > 32 || strings.Trim(fields["ad_network"], "0123456789") != "" {
		return proof, ErrRewardProof
	}
	claim, e := base64.RawURLEncoding.Strict().DecodeString(fields["custom_data"])
	if e != nil || len(claim) != 32 {
		return proof, ErrRewardProof
	}
	txn := fields["transaction_id"]
	if len(txn) > 128 || len(txn)%2 != 0 {
		return proof, ErrRewardProof
	}
	if _, e = hex.DecodeString(txn); e != nil {
		return proof, ErrRewardProof
	}
	ms, e := strconv.ParseInt(fields["timestamp"], 10, 64)
	if e != nil || ms <= 0 || strconv.FormatInt(ms, 10) != fields["timestamp"] || time.UnixMilli(ms).After(v.now().Add(time.Minute)) {
		return proof, ErrRewardProof
	}
	keyText := strings.TrimPrefix(parts[len(parts)-1], "key_id=")
	id, e := strconv.ParseInt(keyText, 10, 64)
	if e != nil || id <= 0 || strconv.FormatInt(id, 10) != keyText {
		return proof, ErrRewardProof
	}
	sigText := strings.TrimPrefix(parts[len(parts)-2], "signature=")
	sig, e := base64.RawURLEncoding.Strict().DecodeString(sigText)
	if e != nil {
		sig, e = base64.URLEncoding.Strict().DecodeString(sigText)
	}
	if e != nil || len(sig) < 8 || len(sig) > 80 {
		return proof, ErrRewardProof
	}
	select {
	case v.slots <- struct{}{}:
		defer func() { <-v.slots }()
	default:
		return proof, ErrRewardBusy
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(v.cfg.HTTPTimeoutS)*time.Second)
	defer cancel()
	key, e := v.key(ctx, id)
	if e != nil {
		return proof, e
	}
	// All signed values are unreserved ASCII. Consequently raw bytes and Java
	// URI.getQuery() bytes in Google's Tink reference are identical. No ambiguous
	// percent/plus normalization, parameter sorting or alternate signing recipe.
	digest := sha256.Sum256([]byte(strings.Join(parts[:len(parts)-2], "&")))
	if !ecdsa.VerifyASN1(key, digest[:], sig) {
		return proof, ErrRewardProof
	}
	return AdMobProof{TransactionID: strings.ToLower(txn), Claim: fields["custom_data"], AdUnit: fields["ad_unit"], OccurredAt: time.UnixMilli(ms).UTC(), Fingerprint: hex.EncodeToString(digest[:])}, nil
}
func rewardASCII(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.ContainsRune("._~-", c)) {
			return false
		}
	}
	return true
}
func (v *AdMobVerifier) cached(id int64, now time.Time) (*ecdsa.PublicKey, error, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	fresh := !v.fetched.IsZero() && now.Before(v.fetched.Add(time.Duration(v.cfg.KeyCacheS)*time.Second))
	if fresh && v.keys[id] != nil {
		return v.keys[id], nil, true
	}
	if !v.lastAttempt.IsZero() && now.Before(v.lastAttempt.Add(time.Duration(v.cfg.KeyRefreshMinS)*time.Second)) {
		if fresh {
			return nil, ErrRewardProof, true
		}
		return nil, ErrRewardUnavailable, true
	}
	return nil, nil, false
}
func (v *AdMobVerifier) key(ctx context.Context, id int64) (*ecdsa.PublicKey, error) {
	if k, e, ok := v.cached(id, v.now()); ok {
		return k, e
	}
	select {
	case v.refresh <- struct{}{}:
		defer func() { <-v.refresh }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if k, e, ok := v.cached(id, v.now()); ok {
		return k, e
	}
	v.mu.Lock()
	v.lastAttempt = v.now()
	v.mu.Unlock()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.gstatic.com/admob/reward/verifier-keys.json", nil)
	if e != nil {
		return nil, ErrRewardUnavailable
	}
	resp, e := v.client.Do(req)
	if e != nil {
		return nil, ErrRewardUnavailable
	}
	b, e := billingReadResponse(resp, v.cfg.MaxResponseBytes)
	if e != nil {
		return nil, ErrRewardUnavailable
	}
	var body struct {
		Keys []struct {
			ID     int64  `json:"keyId"`
			Base64 string `json:"base64"`
		} `json:"keys"`
	}
	if billingJSON(b, &body) != nil || len(body.Keys) == 0 || len(body.Keys) > 32 {
		return nil, ErrRewardUnavailable
	}
	keys := map[int64]*ecdsa.PublicKey{}
	for _, row := range body.Keys {
		if row.ID <= 0 || keys[row.ID] != nil || len(row.Base64) > 1024 {
			return nil, ErrRewardUnavailable
		}
		der, e := base64.StdEncoding.Strict().DecodeString(row.Base64)
		if e != nil {
			return nil, ErrRewardUnavailable
		}
		parsed, e := x509.ParsePKIXPublicKey(der)
		if e != nil {
			return nil, ErrRewardUnavailable
		}
		key, ok := parsed.(*ecdsa.PublicKey)
		if !ok || key.Curve != elliptic.P256() {
			return nil, ErrRewardUnavailable
		}
		keys[row.ID] = key
	}
	v.mu.Lock()
	v.keys = keys
	v.fetched = v.now()
	v.mu.Unlock()
	if keys[id] == nil {
		return nil, ErrRewardProof
	}
	return keys[id], nil
}
