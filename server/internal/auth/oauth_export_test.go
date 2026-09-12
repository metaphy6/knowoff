package auth

import "testing"

// Test-only bridge: production constructors expose no provider transport bypass.
func GoogleOAuthHTTPFixtureForTest(t *testing.T) (*Manager, func(string, string, string)) {
	f := newOAuthFixture(t)
	return f.m, func(code, subject, nonce string) { f.token(code, subject, nonce, nil) }
}
