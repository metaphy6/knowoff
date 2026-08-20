package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SignedURLIssuer creates short-lived, round-bound URLs for Nown assets.
// The issuer signs the asset reference, the round it belongs to, and an expiry
// timestamp so a leaked URL cannot be reused in a later round or after it
// expires. Issuing and verification happen in memory using a shared secret.
type SignedURLIssuer struct {
	key []byte
	ttl time.Duration
}

// NewSignedURLIssuer returns an issuer using key and ttl. key must be
// non-empty in production; the empty-key issuer still functions deterministically
// but must only be used in tests.
func NewSignedURLIssuer(key []byte, ttl time.Duration) *SignedURLIssuer {
	return &SignedURLIssuer{key: key, ttl: ttl}
}

// Issue returns a signed token for the requested assetRef valid for one round.
// The returned string is opaque and must be verified with Verify before use.
func (s *SignedURLIssuer) Issue(roundID, assetRef string, now time.Time) (string, time.Time, error) {
	if roundID == "" {
		return "", time.Time{}, fmt.Errorf("roundID is required")
	}
	if assetRef == "" {
		return "", time.Time{}, fmt.Errorf("assetRef is required")
	}
	expires := now.Add(s.ttl)
	token := s.sign(roundID, assetRef, expires)
	return token, expires, nil
}

// Verify checks a token and returns the asset reference it authorizes if the
// token is well-formed, unexpired, and bound to roundID.
func (s *SignedURLIssuer) Verify(roundID, token string, now time.Time) (string, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed token")
	}
	assetRef := parts[0]
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("malformed expiry")
	}
	expires := time.Unix(expiresUnix, 0)
	if now.After(expires) {
		return "", fmt.Errorf("token expired")
	}
	expected := s.sign(roundID, assetRef, expires)
	if !hmac.Equal([]byte(token), []byte(expected)) {
		return "", fmt.Errorf("invalid signature or wrong round")
	}
	return assetRef, nil
}

func (s *SignedURLIssuer) sign(roundID, assetRef string, expires time.Time) string {
	payload := roundID + "\n" + assetRef + "\n" + strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return assetRef + ":" + strconv.FormatInt(expires.Unix(), 10) + ":" + sig
}
