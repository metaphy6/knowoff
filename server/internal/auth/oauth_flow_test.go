package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type oauthTransport func(*http.Request) (*http.Response, error)

func (f oauthTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type oauthFixture struct {
	t         *testing.T
	m         *Manager
	key       *rsa.PrivateKey
	mu        sync.Mutex
	tokens    map[string]string
	exchanges int
}

func newOAuthFixture(t *testing.T) *oauthFixture {
	t.Helper()
	db := setupTestDB(t)
	t.Cleanup(func() { db.Close() })
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(db, []byte("disposable-oauth-proof-signing-key"), "test", "test", time.Hour, 24*time.Hour, OAuthProviders{Google: OAuthProviderConfig{ClientID: "fixture-client", ClientSecret: "fixture-secret", RedirectURL: "https://knowoff.invalid/api/auth/oauth/callback?provider=google"}})
	f := &oauthFixture{t: t, m: m, key: key, tokens: map[string]string{}}
	m.oauthHTTP = &http.Client{Transport: oauthTransport(f.roundTrip)}
	return f
}
func (f *oauthFixture) roundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body any
	switch r.URL.String() {
	case "https://oauth2.googleapis.com/token":
		if r.Method != "POST" {
			f.t.Error("token exchange method")
		}
		if err := r.ParseForm(); err != nil {
			f.t.Error(err)
		}
		if r.Form.Get("client_id") != "fixture-client" || r.Form.Get("client_secret") != "fixture-secret" || len(r.Form.Get("code_verifier")) != 43 {
			f.t.Error("missing confidential PKCE exchange")
		}
		f.exchanges++
		body = map[string]any{"access_token": "fixture-provider-access", "token_type": "Bearer", "id_token": f.tokens[r.Form.Get("code")]}
	case "https://www.googleapis.com/oauth2/v3/certs":
		body = map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "fixture-key", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(f.key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(f.key.E)).Bytes())}}}
	default:
		f.t.Errorf("unexpected provider request %s", r.URL.Host)
		return nil, errors.New("unexpected request")
	}
	encoded, _ := json.Marshal(body)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encoded))), Request: r}, nil
}
func (f *oauthFixture) start(intent, access string) (*OAuthStart, string, string) {
	f.t.Helper()
	hash := HashDevice(uuid.NewString())
	if access != "" {
		claims, err := f.m.parseToken(access, TokenAccess)
		if err != nil {
			f.t.Fatal(err)
		}
		hash = claims.DeviceHash
	}
	flow, err := f.m.BeginOAuth(context.Background(), OAuthStartRequest{DeviceHash: hash, Provider: "google", Intent: intent, AccessToken: access, Principal: uuid.NewString()})
	if err != nil {
		f.t.Fatal(err)
	}
	u, err := url.Parse(flow.URL)
	if err != nil {
		f.t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 || len(q.Get("nonce")) != 43 || len(q.Get("state")) != 43 || len(flow.CompletionSecret) != 43 || strings.Contains(flow.URL, flow.CompletionSecret) {
		f.t.Fatal("missing independent state/nonce/PKCE/completion secret")
	}
	return flow, q.Get("state"), q.Get("nonce")
}
func (f *oauthFixture) token(code, subject, nonce string, mutate func(jwt.MapClaims)) {
	claims := jwt.MapClaims{"iss": "https://accounts.google.com", "aud": "fixture-client", "sub": subject, "nonce": nonce, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(), "email": "player@example.com", "email_verified": true}
	if mutate != nil {
		mutate(claims)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "fixture-key"
	signed, err := token.SignedString(f.key)
	if err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	f.tokens[code] = signed
	f.mu.Unlock()
}
func TestOAuthDurableLinkRestoreAndPrivateResult(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	player, err := f.m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.m.db.Exec(`INSERT INTO noin_wallets(account_id,balance) VALUES($1,93)`, player.AccountID); err != nil {
		t.Fatal(err)
	}
	flow, state, nonce := f.start("link", player.AccessToken)
	if _, err = f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret); !errors.Is(err, ErrOAuthPending) {
		t.Fatal("pending result", err)
	}
	f.token("link-code", "durable-"+player.AccountID, nonce, nil)
	// A replacement service instance consumes the durable state after restart.
	restarted := NewManager(f.m.db, f.m.signingKey, "test", "test", time.Hour, 24*time.Hour, f.m.oauth)
	restarted.oauthHTTP = f.m.oauthHTTP
	if err = restarted.CompleteOAuth(ctx, "google", state, "link-code"); err != nil {
		t.Fatal(err)
	}
	if err = restarted.CompleteOAuth(ctx, "google", state, "link-code"); !errors.Is(err, ErrOAuthInvalid) {
		t.Fatal("callback replay", err)
	}
	if _, err = f.m.OAuthResult(ctx, flow.FlowID, state); !errors.Is(err, ErrOAuthInvalid) {
		t.Fatal("browser state revealed app credentials", err)
	}
	linked, err := f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret)
	if err != nil || linked.AccountID != player.AccountID {
		t.Fatal("link result", err)
	}
	replay, err := restarted.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret)
	if err != nil || *replay != *linked {
		t.Fatal("result retry issued different credential pair", err)
	}
	restored, state, nonce := f.start("restore", "")
	f.token("restore-code", "durable-"+player.AccountID, nonce, nil)
	if err = f.m.CompleteOAuth(ctx, "google", state, "restore-code"); err != nil {
		t.Fatal(err)
	}
	pair, err := f.m.OAuthResult(ctx, restored.FlowID, restored.CompletionSecret)
	if err != nil || pair.AccountID != player.AccountID {
		t.Fatal("second device did not restore account", err)
	}
	var balance, devices int
	if err = f.m.db.QueryRow(`SELECT (SELECT balance FROM noin_wallets WHERE account_id=$1),(SELECT count(*) FROM device_tokens WHERE account_id=$1)`, player.AccountID).Scan(&balance, &devices); err != nil || balance != 93 || devices != 2 {
		t.Fatal("restoration changed wallet or failed to retain both installation links", balance, devices, err)
	}
	if _, err = f.m.Refresh(ctx, linked.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err = f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret); !errors.Is(err, ErrOAuthInvalid) {
		t.Fatal("result revived used refresh", err)
	}
}
func TestOAuthVerifiedIdentityAndSessionFences(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(jwt.MapClaims)
	}{
		{"issuer", func(c jwt.MapClaims) { c["iss"] = "https://attacker.invalid" }},
		{"audience", func(c jwt.MapClaims) { c["aud"] = "other-client" }},
		{"nonce", func(c jwt.MapClaims) { c["nonce"] = "other-flow" }},
		{"expired", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{"missing_expiry", func(c jwt.MapClaims) { delete(c, "exp") }},
		{"missing_subject", func(c jwt.MapClaims) { delete(c, "sub") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newOAuthFixture(t)
			p, err := f.m.AuthenticateDevice(context.Background(), HashDevice(uuid.NewString()))
			if err != nil {
				t.Fatal(err)
			}
			flow, state, nonce := f.start("link", p.AccessToken)
			f.token("bad-code", uuid.NewString(), nonce, test.mutate)
			if err = f.m.CompleteOAuth(context.Background(), "google", state, "bad-code"); !errors.Is(err, ErrOAuthInvalid) {
				t.Fatal("unverified identity accepted", err)
			}
			if _, err = f.m.OAuthResult(context.Background(), flow.FlowID, flow.CompletionSecret); !errors.Is(err, ErrOAuthInvalid) {
				t.Fatal("invalid identity completion", err)
			}
			var n int
			if err = f.m.db.QueryRow(`SELECT count(*) FROM oauth_links WHERE account_id=$1`, p.AccountID).Scan(&n); err != nil || n != 0 {
				t.Fatal("invalid provider linked", n, err)
			}
		})
	}
	f := newOAuthFixture(t)
	ctx := context.Background()
	p, err := f.m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	flow, state, nonce := f.start("link", p.AccessToken)
	f.token("revoked-code", uuid.NewString(), nonce, nil)
	if err = f.m.RevokeSessions(ctx, p.AccountID); err != nil {
		t.Fatal(err)
	}
	if err = f.m.CompleteOAuth(ctx, "google", state, "revoked-code"); !errors.Is(err, ErrOAuthInvalid) {
		t.Fatal("callback linked after epoch revocation", err)
	}
	if _, err = f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret); !errors.Is(err, ErrOAuthInvalid) {
		t.Fatal("revoked callback delivered", err)
	}
	if _, err = f.m.BeginOAuth(ctx, OAuthStartRequest{Provider: "google", Intent: "link", AccessToken: p.AccessToken, Principal: "revoked"}); !errors.Is(err, ErrOAuthInvalid) {
		t.Fatal("revoked session started link", err)
	}
	if _, err = f.m.BeginOAuth(ctx, OAuthStartRequest{Provider: "google", Intent: "restore", AccessToken: p.AccessToken, Principal: "restore"}); !errors.Is(err, ErrOAuthInvalid) {
		t.Fatal("bad Bearer silently became restore", err)
	}
}

func TestOAuthConcurrentCallbackAndResultReceipts(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	p, err := f.m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	flow, state, nonce := f.start("link", p.AccessToken)
	f.token("parallel", uuid.NewString(), nonce, nil)
	results := make(chan error, 8)
	for range 8 {
		go func() { results <- f.m.CompleteOAuth(ctx, "google", state, "parallel") }()
	}
	succeeded := 0
	for range 8 {
		err := <-results
		if err == nil {
			succeeded++
		} else if !errors.Is(err, ErrOAuthInvalid) {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || f.exchanges != 1 {
		t.Fatal("callback performed duplicate exchange", succeeded, f.exchanges)
	}
	pairs := make(chan *TokenPair, 8)
	for range 8 {
		go func() {
			pair, err := f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret)
			results <- err
			pairs <- pair
		}()
	}
	var first *TokenPair
	for range 8 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
		pair := <-pairs
		if first == nil {
			first = pair
		} else if *pair != *first {
			t.Fatal("parallel result minted duplicate tokens")
		}
	}
	f.m.accessTTL += time.Second
	if _, err = f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret); !errors.Is(err, ErrOAuthInvalid) {
		t.Fatal("changed issuer config reissued receipt", err)
	}
}

func TestOAuthUnlinkedConflictExpiryAndBoundedInitiation(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	flow, state, nonce := f.start("restore", "")
	f.token("unlinked", uuid.NewString(), nonce, nil)
	var before, after int
	if err := f.m.db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := f.m.CompleteOAuth(ctx, "google", state, "unlinked"); !errors.Is(err, ErrOAuthUnlinked) {
		t.Fatal("unknown subject created an account", err)
	}
	if _, err := f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret); !errors.Is(err, ErrOAuthUnlinked) {
		t.Fatal("unlinked result", err)
	}
	if err := f.m.db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&after); err != nil || before != after {
		t.Fatal("restoration changed account inventory", err)
	}
	owner, err := f.m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	subject := uuid.NewString()
	if err = f.m.LinkOAuth(ctx, owner.AccountID, "google", subject, "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	flow, state, nonce = f.start("link", other.AccessToken)
	f.token("collision", subject, nonce, nil)
	if err = f.m.CompleteOAuth(ctx, "google", state, "collision"); !errors.Is(err, ErrOAuthConflict) {
		t.Fatal("silent collision", err)
	}
	if _, err = f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret); !errors.Is(err, ErrOAuthConflict) {
		t.Fatal("collision not durable", err)
	}
	flow, state, nonce = f.start("link", owner.AccessToken)
	f.token("expired", subject, nonce, nil)
	if _, err = f.m.db.Exec(`UPDATE oauth_flows SET created_at=created_at-interval '1 hour',expires_at=expires_at-interval '1 hour' WHERE id=$1`, flow.FlowID); err != nil {
		t.Fatal(err)
	}
	if err = f.m.CompleteOAuth(ctx, "google", state, "expired"); !errors.Is(err, ErrOAuthInvalid) {
		t.Fatal("expired state accepted", err)
	}
	principal := uuid.NewString()
	for range 10 {
		if _, err = f.m.BeginOAuth(ctx, OAuthStartRequest{Provider: "google", Intent: "restore", DeviceHash: HashDevice(principal), Principal: principal}); err != nil {
			t.Fatal(err)
		}
	}
	restarted := NewManager(f.m.db, f.m.signingKey, "test", "test", time.Hour, 24*time.Hour, f.m.oauth)
	if _, err = restarted.BeginOAuth(ctx, OAuthStartRequest{Provider: "google", Intent: "restore", DeviceHash: HashDevice(principal), Principal: principal}); !errors.Is(err, ErrOAuthRateLimited) {
		t.Fatal("restart bypassed budget", err)
	}
}

func TestOAuthLinkRejectsRevokedInitiatingAccessToken(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	p, err := f.m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	_, state, nonce := f.start("link", p.AccessToken)
	f.token("revoked-access", uuid.NewString(), nonce, nil)
	claims, err := f.m.parseToken(p.AccessToken, TokenAccess)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.m.revokeToken(ctx, claims.ID, claims.ExpiresAt.Time); err != nil {
		t.Fatal(err)
	}
	if err = f.m.CompleteOAuth(ctx, "google", state, "revoked-access"); !errors.Is(err, ErrOAuthInvalid) {
		t.Fatal("revoked initiating credential linked provider", err)
	}
}

func TestOAuthCallbackAndResultSerializeWithAccountRevocation(t *testing.T) {
	for _, stage := range []string{"callback", "result"} {
		t.Run(stage, func(t *testing.T) {
			f := newOAuthFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			p, err := f.m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
			if err != nil {
				t.Fatal(err)
			}
			flow, state, nonce := f.start("link", p.AccessToken)
			f.token("blocked-account", uuid.NewString(), nonce, nil)
			if stage == "result" {
				if err = f.m.CompleteOAuth(ctx, "google", state, "blocked-account"); err != nil {
					t.Fatal(err)
				}
			}
			barrier, err := f.m.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer barrier.Rollback()
			if _, err = barrier.ExecContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, p.AccountID); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				if stage == "callback" {
					result <- f.m.CompleteOAuth(ctx, "google", state, "blocked-account")
				} else {
					_, err := f.m.OAuthResult(ctx, flow.FlowID, flow.CompletionSecret)
					result <- err
				}
			}()
			for {
				var n int
				if err = f.m.db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock'`).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n == 1 {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("no real account serialization barrier", ctx.Err())
				case <-time.After(time.Millisecond):
				}
			}
			if err = f.m.RevokeSessionsTx(ctx, barrier, p.AccountID); err != nil {
				t.Fatal(err)
			}
			if err = barrier.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-result; !errors.Is(err, ErrOAuthInvalid) {
				t.Fatal("OAuth crossed committed revocation", stage, err)
			}
			if stage == "callback" {
				var n int
				if err = f.m.db.QueryRowContext(ctx, `SELECT count(*) FROM oauth_links WHERE account_id=$1`, p.AccountID).Scan(&n); err != nil || n != 0 {
					t.Fatal("revoked callback altered identity", n, err)
				}
			}
		})
	}
}

func TestOAuthLinkBudgetSeparatesAuthenticatedAccountsBehindProxy(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := context.Background()
	proxy := uuid.NewString()
	var first *TokenPair
	for range 12 {
		p, err := f.m.AuthenticateDevice(ctx, HashDevice(uuid.NewString()))
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = p
		}
		claims, err := f.m.parseToken(p.AccessToken, TokenAccess)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.m.BeginOAuth(ctx, OAuthStartRequest{DeviceHash: claims.DeviceHash, Provider: "google", Intent: "link", AccessToken: p.AccessToken, Principal: proxy}); err != nil {
			t.Fatal("different linked account inherited proxy budget", err)
		}
	}
	firstClaims, err := f.m.parseToken(first.AccessToken, TokenAccess)
	if err != nil {
		t.Fatal(err)
	}
	for range 9 {
		if _, err := f.m.BeginOAuth(ctx, OAuthStartRequest{DeviceHash: firstClaims.DeviceHash, Provider: "google", Intent: "link", AccessToken: first.AccessToken, Principal: uuid.NewString()}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.m.BeginOAuth(ctx, OAuthStartRequest{DeviceHash: firstClaims.DeviceHash, Provider: "google", Intent: "link", AccessToken: first.AccessToken, Principal: uuid.NewString()}); !errors.Is(err, ErrOAuthRateLimited) {
		t.Fatal("account bypassed link budget by changing address", err)
	}
}
