package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Provider endpoints come only from server-owned configuration. Redirects,
// unbounded response bodies and upstream error text cannot cross this boundary.
func (m *Manager) oauthJSON(ctx context.Context, method, endpoint, bearer string, form url.Values, target any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return ErrOAuthUnavailable
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	req.Header.Set("Accept", "application/json")
	client := *m.oauthHTTP
	client.Timeout = 10 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return ErrOAuthUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrOAuthUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return ErrOAuthUnavailable
	}
	if err = json.Unmarshal(data, target); err != nil {
		return ErrOAuthInvalid
	}
	return nil
}

func (m *Manager) verifyOAuthCode(ctx context.Context, provider, code, verifier, nonceHash string) (string, string, error) {
	cfg := m.oauthCfgs[provider]
	if cfg == nil {
		return "", "", ErrOAuthUnavailable
	}
	form := url.Values{"client_id": {cfg.ClientID}, "client_secret": {cfg.ClientSecret}, "redirect_uri": {cfg.RedirectURL}, "code": {code}, "grant_type": {"authorization_code"}}
	if provider == "google" {
		form.Set("code_verifier", verifier)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
	}
	if err := m.oauthJSON(ctx, http.MethodPost, cfg.Endpoint.TokenURL, "", form, &token); err != nil {
		return "", "", err
	}
	if token.AccessToken == "" || len(token.AccessToken) > 16384 || !strings.EqualFold(token.TokenType, "bearer") {
		return "", "", ErrOAuthInvalid
	}
	switch provider {
	case "google":
		return m.verifyGoogleIdentity(ctx, token.IDToken, token.AccessToken, nonceHash)
	case "facebook":
		return m.verifyFacebookIdentity(ctx, token.AccessToken)
	}
	return "", "", ErrOAuthInvalid
}

type googleIdentityClaims struct {
	jwt.RegisteredClaims
	Nonce           string `json:"nonce"`
	AuthorizedParty string `json:"azp"`
	AccessHash      string `json:"at_hash"`
	Email           string `json:"email"`
	EmailVerified   bool   `json:"email_verified"`
}

func (m *Manager) verifyGoogleIdentity(ctx context.Context, idToken, accessToken, nonceHash string) (string, string, error) {
	if len(idToken) < 1 || len(idToken) > 32768 {
		return "", "", ErrOAuthInvalid
	}
	var keys struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := m.oauthJSON(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/certs", "", nil, &keys); err != nil {
		return "", "", err
	}
	claims := &googleIdentityClaims{}
	parsed, err := jwt.ParseWithClaims(idToken, claims, func(token *jwt.Token) (any, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" || len(kid) > 256 {
			return nil, ErrOAuthInvalid
		}
		var found *rsa.PublicKey
		for _, key := range keys.Keys {
			if key.Kid != kid {
				continue
			}
			if found != nil || key.Kty != "RSA" || (key.Alg != "" && key.Alg != "RS256") || (key.Use != "" && key.Use != "sig") {
				return nil, ErrOAuthInvalid
			}
			n, e1 := base64.RawURLEncoding.DecodeString(key.N)
			e, e2 := base64.RawURLEncoding.DecodeString(key.E)
			if e1 != nil || e2 != nil || len(n) < 256 || len(n) > 1024 || len(e) < 1 || len(e) > 4 {
				return nil, ErrOAuthInvalid
			}
			exponent := new(big.Int).SetBytes(e).Int64()
			if exponent < 3 || exponent > 2147483647 || exponent%2 == 0 {
				return nil, ErrOAuthInvalid
			}
			found = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent)}
		}
		if found == nil {
			return nil, ErrOAuthInvalid
		}
		return found, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithAudience(m.oauth.Google.ClientID), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !parsed.Valid || claims.IssuedAt == nil || (claims.Issuer != "https://accounts.google.com" && claims.Issuer != "accounts.google.com") || claims.Nonce == "" || subtle.ConstantTimeCompare([]byte(codeChallenge(claims.Nonce)), []byte(nonceHash)) != 1 || (claims.AuthorizedParty != "" && claims.AuthorizedParty != m.oauth.Google.ClientID) || (len(claims.Audience) > 1 && claims.AuthorizedParty == "") {
		return "", "", ErrOAuthInvalid
	}
	if claims.AccessHash != "" {
		digest := sha256.Sum256([]byte(accessToken))
		expected := base64.RawURLEncoding.EncodeToString(digest[:len(digest)/2])
		if subtle.ConstantTimeCompare([]byte(expected), []byte(claims.AccessHash)) != 1 {
			return "", "", ErrOAuthInvalid
		}
	}
	email := ""
	if claims.EmailVerified {
		email = claims.Email
	}
	if !validOAuthIdentity("google", claims.Subject, email) {
		return "", "", ErrOAuthInvalid
	}
	return claims.Subject, email, nil
}

// Facebook's confidential server flow validates the returned token against the
// configured application before /me. Email is descriptive, never an identity key.
func (m *Manager) verifyFacebookIdentity(ctx context.Context, access string) (string, string, error) {
	cfg := m.oauth.Facebook
	base := "https://graph.facebook.com/" + cfg.GraphVersion
	var debug struct {
		Data struct {
			AppID       string `json:"app_id"`
			UserID      string `json:"user_id"`
			Valid       bool   `json:"is_valid"`
			Expires     int64  `json:"expires_at"`
			DataExpires int64  `json:"data_access_expires_at"`
		} `json:"data"`
	}
	endpoint := base + "/debug_token?" + url.Values{"input_token": {access}}.Encode()
	if err := m.oauthJSON(ctx, http.MethodGet, endpoint, cfg.ClientID+"|"+cfg.ClientSecret, nil, &debug); err != nil {
		return "", "", err
	}
	d := debug.Data
	now := time.Now().Unix()
	if !d.Valid || d.AppID != cfg.ClientID || d.UserID == "" || d.Expires <= now || (d.DataExpires != 0 && d.DataExpires <= now) {
		return "", "", ErrOAuthInvalid
	}
	proof := hmac.New(sha256.New, []byte(cfg.ClientSecret))
	proof.Write([]byte(access))
	endpoint = base + "/me?" + url.Values{"fields": {"id,email"}, "appsecret_proof": {hex.EncodeToString(proof.Sum(nil))}}.Encode()
	var user struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := m.oauthJSON(ctx, http.MethodGet, endpoint, access, nil, &user); err != nil {
		return "", "", err
	}
	if user.ID != d.UserID || !validOAuthIdentity("facebook", user.ID, user.Email) {
		return "", "", ErrOAuthInvalid
	}
	return user.ID, user.Email, nil
}
